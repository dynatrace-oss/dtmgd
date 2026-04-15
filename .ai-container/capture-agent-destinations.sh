#!/usr/bin/env bash
set -euo pipefail

command="${1:-start}"
capture_dir="${2:-/workspace/.agent-discovery}"
pcap_file="$capture_dir/agent-traffic.pcap"
dns_out="$capture_dir/agent-dns.txt"
sni_out="$capture_dir/agent-sni.txt"
pid_file="$capture_dir/tcpdump.pid"
log_file="$capture_dir/tcpdump.log"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
}

ensure_capture_dir() {
  if [[ -e "$capture_dir" && ! -d "$capture_dir" ]]; then
    printf 'Capture path exists but is not a directory: %s\n' "$capture_dir" >&2
    exit 1
  fi
  mkdir -p "$capture_dir"
}

tcpdump_running() {
  [[ -f "$pid_file" ]] && kill -0 "$(cat "$pid_file")" >/dev/null 2>&1
}

extract_capture() {
  ensure_capture_dir
  require_command tshark

  if [[ ! -f "$pcap_file" ]]; then
    printf 'No capture file found at %s\n' "$pcap_file" >&2
    exit 1
  fi

  tshark -r "$pcap_file" -Y "dns.qry.name" -T fields -e dns.qry.name 2>/dev/null | sort -u > "$dns_out" || true
  tshark -r "$pcap_file" -Y "tls.handshake.extensions_server_name" -T fields -e tls.handshake.extensions_server_name 2>/dev/null | sort -u > "$sni_out" || true

  printf 'DNS queries saved to %s\n' "$dns_out"
  printf 'TLS SNI hostnames saved to %s\n' "$sni_out"
}

start_capture() {
  ensure_capture_dir
  require_command tcpdump

  if tcpdump_running; then
    printf 'Capture already running with PID %s\n' "$(cat "$pid_file")"
    exit 0
  fi

  : > "$log_file"
  nohup tcpdump -i any -n -s 0 -U -w "$pcap_file" 'port 53 or port 443' </dev/null >>"$log_file" 2>&1 &
  local tcpdump_pid=$!
  disown "$tcpdump_pid" 2>/dev/null || true
  echo "$tcpdump_pid" > "$pid_file"

  printf 'Capture started with PID %s\n' "$tcpdump_pid"
  printf 'Writing pcap to %s\n' "$pcap_file"
  printf 'Capture log: %s\n' "$log_file"
}

stop_capture() {
  if ! [[ -f "$pid_file" ]]; then
    printf 'No capture PID file found at %s\n' "$pid_file" >&2
    exit 1
  fi

  local tcpdump_pid
  tcpdump_pid="$(cat "$pid_file")"

  if kill -0 "$tcpdump_pid" >/dev/null 2>&1; then
    kill "$tcpdump_pid"
    wait "$tcpdump_pid" 2>/dev/null || true
  fi

  rm -f "$pid_file"
  extract_capture
}

status_capture() {
  if tcpdump_running; then
    printf 'Capture is running with PID %s\n' "$(cat "$pid_file")"
  else
    printf 'Capture is not running\n'
  fi

  if [[ -f "$pcap_file" ]]; then
    printf 'Capture file: %s\n' "$pcap_file"
  fi
}

case "$command" in
  start)
    start_capture
    ;;
  stop)
    stop_capture
    ;;
  extract)
    extract_capture
    ;;
  status)
    status_capture
    ;;
  *)
    printf 'Usage: %s {start|stop|extract|status} [capture-dir]\n' "$0" >&2
    exit 1
    ;;
esac
