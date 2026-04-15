#!/usr/bin/env bash
set -euo pipefail

# Background daemon: captures outbound traffic that is blocked in restricted mode.
# Must be started as a root process before exec capsh so it retains CAP_NET_RAW.
# Reads blocked packets via tshark on the NFLOG netlink group that entrypoint.sh
# configures (default group 100).
#
# Usage: capture-blocked-traffic.sh [capture_dir]
#
# Writes to <capture_dir>:
#   blocked.log          timestamped log of every blocked destination
#   blocked-domains.txt  deduplicated domains  → copy-paste into allowlist-domains.txt
#   blocked-ips.txt      deduplicated IPs      → copy-paste into allowlist-cidrs.txt
#
# Internal state (dns-map, caches) is stored under a root-only directory
# (/run/agent-blocked-internal by default) so the sandbox user cannot
# tamper with the self-healing lookup tables.

capture_dir="${1:-/workspace/.agent-blocked}"
nflog_group="${NFLOG_GROUP:-100}"
domains_file="${ALLOWLIST_DOMAINS_FILE:-/tmp/allowlist-domains.txt}"
proxy_domains_file="${ALLOWLIST_PROXY_DOMAINS_FILE:-/tmp/allowlist-proxy-domains.txt}"
ipv4_set_name="${ALLOWLIST_IPV4_SET:-allowed_ipv4}"
ipv6_set_name="${ALLOWLIST_IPV6_SET:-allowed_ipv6}"
self_healing="${SELF_HEALING_ENABLED:-1}"

# Internal state directory — root-only, not on the bind-mounted workspace.
internal_dir="/run/agent-blocked-internal"
mkdir -p "$internal_dir"
chmod 700 "$internal_dir"

dns_map="$internal_dir/dns-map.txt"
blocked_log="$capture_dir/blocked.log"
blocked_domains="$capture_dir/blocked-domains.txt"
blocked_ips="$capture_dir/blocked-ips.txt"

mkdir -p "$capture_dir"
: > "$dns_map"

# Build a flat list of allowlisted domains (comments and blanks stripped) for
# fast grep lookups in the self-healing path.
allowed_domains_cache="$internal_dir/allowed-domains-cache"
if [[ -f "$domains_file" ]]; then
  grep -v '^\s*#' "$domains_file" | grep -v '^\s*$' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' \
    > "$allowed_domains_cache"
else
  : > "$allowed_domains_cache"
fi

# Build a list of wildcard domain patterns from the proxy-domains file.
# Each line like "*.example.com" becomes a suffix match so that
# "anything.example.com" or "deep.sub.example.com" is auto-allowed.
wildcard_patterns_cache="$internal_dir/wildcard-patterns-cache"
if [[ -f "$proxy_domains_file" ]]; then
  grep -v '^\s*#' "$proxy_domains_file" | grep -v '^\s*$' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' \
    > "$wildcard_patterns_cache"
else
  : > "$wildcard_patterns_cache"
fi

init_output_files() {
  if [[ ! -f "$blocked_domains" ]]; then
    cat > "$blocked_domains" <<'HEADER'
# Blocked domains — copy-paste these lines into allowlist-domains.txt
# Generated automatically while the container runs in restricted mode.
# Each domain below was attempted but rejected by the outbound firewall.

HEADER
  fi
  if [[ ! -f "$blocked_ips" ]]; then
    cat > "$blocked_ips" <<'HEADER'
# Blocked IPs with no known domain — copy-paste into allowlist-cidrs.txt
# If you can identify the owning service, prefer adding the domain to allowlist-domains.txt instead.
# Generated automatically while the container runs in restricted mode.

HEADER
  fi
  if [[ ! -f "$blocked_log" ]]; then
    printf '%-25s  %-10s  %-42s  %s\n' "TIMESTAMP" "PROTO:PORT" "IP" "DOMAIN" >> "$blocked_log"
    printf '%s\n' "$(printf '─%.0s' {1..100})" >> "$blocked_log"
  fi
}

lookup_domain() {
  grep -m1 "^${1} " "$dns_map" 2>/dev/null | awk '{print $2}' || true
}

is_allowlisted_domain() {
  local domain="$1"
  [[ -z "$domain" ]] && return 1
  grep -qxF "$domain" "$allowed_domains_cache" 2>/dev/null
}

matches_wildcard_domain() {
  local domain="$1"
  [[ -z "$domain" ]] && return 1
  while IFS= read -r pattern; do
    # pattern is e.g. "*.example.com" — strip the "*" prefix to get ".example.com"
    local suffix="${pattern#\*}"
    # match if domain ends with the suffix (e.g. "foo.bar.example.com" ends with ".example.com")
    [[ "$domain" == *"$suffix" ]] && return 0
  done < "$wildcard_patterns_cache"
  return 1
}

auto_allow_ip() {
  local ip="$1"
  if [[ "$ip" == *:* ]]; then
    ipset add "$ipv6_set_name" "$ip" -exist 2>/dev/null
  else
    ipset add "$ipv4_set_name" "$ip" -exist 2>/dev/null
  fi
}

log_blocked() {
  local timestamp="$1" proto="$2" dst_ip="$3" dst_port="$4"
  local domain
  domain="$(lookup_domain "$dst_ip")"

  # Self-healing: if the blocked IP maps to an allowlisted domain (exact or
  # wildcard), add it to the ipset immediately so subsequent packets go through
  # without waiting for the next scheduled refresh.
  if [[ "$self_healing" == "1" ]] && { is_allowlisted_domain "$domain" || matches_wildcard_domain "$domain"; }; then
    auto_allow_ip "$dst_ip"
    printf '%-25s  %-10s  %-42s  %s (auto-allowed)\n' \
      "$timestamp" "$proto:$dst_port" "$dst_ip" "$domain" >> "$blocked_log"
    return
  fi

  printf '%-25s  %-10s  %-42s  %s\n' \
    "$timestamp" "$proto:$dst_port" "$dst_ip" "${domain:-(no domain)}" >> "$blocked_log"
  if [[ -n "$domain" ]]; then
    grep -qxF "$domain" "$blocked_domains" 2>/dev/null || printf '%s\n' "$domain" >> "$blocked_domains"
  else
    grep -qxF "$dst_ip" "$blocked_ips" 2>/dev/null || printf '%s\n' "$dst_ip" >> "$blocked_ips"
  fi
}

start_dns_map_builder() {
  # Captures DNS responses and builds a live IP → FQDN map.
  (
    tshark -i any -n -l \
      -f "port 53" \
      -Y "dns.flags.response == 1 and (dns.a or dns.aaaa)" \
      -T fields -e dns.resp.name -e dns.a -e dns.aaaa \
      2>"$capture_dir/tshark-dns-errors.log" | \
    while IFS=$'\t' read -r raw_name a_list aaaa_list; do
      # dns.resp.name can return comma-separated duplicates; take the first.
      local name="${raw_name%%,*}"
      [[ -z "$name" ]] && continue
      for ip in ${a_list//,/ }; do
        [[ -z "$ip" ]] && continue
        grep -qxF "$ip $name" "$dns_map" 2>/dev/null || printf '%s %s\n' "$ip" "$name" >> "$dns_map"
      done
      for ip in ${aaaa_list//,/ }; do
        [[ -z "$ip" ]] && continue
        grep -qxF "$ip $name" "$dns_map" 2>/dev/null || printf '%s %s\n' "$ip" "$name" >> "$dns_map"
      done
    done
  ) &
}

start_blocked_watcher() {
  # Reads blocked packets from the NFLOG netlink group via tshark.
  # entrypoint.sh adds an NFLOG rule at the END of the OUTPUT chain (after all
  # ACCEPT rules) with --nflog-group 100.  Every packet that reaches it is
  # about to be dropped by the default DROP policy.
  #
  # NFLOG delivers packets to userspace via netlink, which works reliably in
  # all environments including WSL2 with the nf_tables backend.  The older LOG
  # target (dmesg) silently fails in many container/WSL2 setups.
  (
    tshark -i "nflog:$nflog_group" -l -n \
      -T fields -e ip.dst -e tcp.dstport -e udp.dstport \
      2>"$capture_dir/tshark-nflog-errors.log" | \
    while IFS=$'\t' read -r dst tcp_port udp_port; do
      local port="${tcp_port:-$udp_port}"
      [[ -z "$dst" || -z "$port" ]] && continue
      local proto="TCP"
      [[ -n "$udp_port" && -z "$tcp_port" ]] && proto="UDP"
      ts="$(date -u '+%Y-%m-%dT%H:%M:%S')"
      log_blocked "$ts" "$proto" "$dst" "$port"
    done
  ) &
}

init_output_files
start_dns_map_builder
start_blocked_watcher

printf 'Blocked traffic capture started → %s\n' "$capture_dir"
if [[ "$self_healing" == "1" ]]; then
  printf '  self-healing: ON (exact + wildcard domain matching)\n'
else
  printf '  self-healing: OFF (logging only)\n'
fi
printf '  blocked.log         — full timestamped log\n'
printf '  blocked-domains.txt — copy-paste to allowlist-domains.txt\n'
printf '  blocked-ips.txt     — copy-paste to allowlist-cidrs.txt\n'
