#!/usr/bin/env bash
set -euo pipefail

domains_file="${1:-/tmp/allowlist-domains.txt}"
cidrs_file="${2:-/tmp/allowlist-cidrs.txt}"
ipv4_set_name="${3:-allowed_ipv4}"
ipv6_set_name="${4:-allowed_ipv6}"

if [[ ! -f "$domains_file" ]]; then
  printf 'Domains file not found: %s\n' "$domains_file" >&2
  exit 1
fi

if [[ ! -f "$cidrs_file" ]]; then
  printf 'CIDR file not found: %s\n' "$cidrs_file" >&2
  exit 1
fi

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

is_ipv4() {
  [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]
}

is_ipv4_cidr() {
  [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/([0-9]|[12][0-9]|3[0-2])$ ]]
}

is_ipv6_or_cidr() {
  [[ "$1" == *:* ]]
}

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
}

require_command ipset
require_command getent

ipset create "$ipv4_set_name" hash:net family inet -exist
ipset create "$ipv6_set_name" hash:net family inet6 -exist
ipset flush "$ipv4_set_name"
ipset flush "$ipv6_set_name"

while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
  line="$(trim "$raw_line")"
  if [[ -z "$line" || "${line:0:1}" == "#" ]]; then
    continue
  fi

  if is_ipv4 "$line" || is_ipv4_cidr "$line"; then
    ipset add "$ipv4_set_name" "$line" -exist
    continue
  fi

  if is_ipv6_or_cidr "$line"; then
    ipset add "$ipv6_set_name" "$line" -exist
    continue
  fi

  mapfile -t resolved_ipv4 < <(getent ahostsv4 "$line" | awk '{print $1}' | sort -u)
  mapfile -t resolved_ipv6 < <(getent ahostsv6 "$line" | awk '{print $1}' | sort -u)

  if [[ ${#resolved_ipv4[@]} -eq 0 && ${#resolved_ipv6[@]} -eq 0 ]]; then
    printf 'Warning: no IPv4 or IPv6 addresses resolved for %s; continuing\n' "$line" >&2
    continue
  fi

  for ip in "${resolved_ipv4[@]}"; do
    ipset add "$ipv4_set_name" "$ip" -exist
  done

  for ip in "${resolved_ipv6[@]}"; do
    ipset add "$ipv6_set_name" "$ip" -exist
  done
done < "$domains_file"

while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
  line="$(trim "$raw_line")"
  if [[ -z "$line" || "${line:0:1}" == "#" ]]; then
    continue
  fi

  if is_ipv4 "$line" || is_ipv4_cidr "$line"; then
    ipset add "$ipv4_set_name" "$line" -exist
    continue
  fi

  if is_ipv6_or_cidr "$line"; then
    ipset add "$ipv6_set_name" "$line" -exist
    continue
  fi

  printf 'Invalid IP address or CIDR in %s: %s\n' "$cidrs_file" "$line" >&2
  exit 1
done < "$cidrs_file"
