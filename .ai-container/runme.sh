#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./runme.sh build [image-name]
  ./runme.sh restricted [workspace-dir]
  ./runme.sh discovery [workspace-dir]

Commands:
  build       Build the AI sandbox image from this asset directory
  restricted  Run the container with the firewall enabled (agent runs as non-root, NET_ADMIN/NET_RAW dropped)
  discovery   Run the container with unrestricted egress and background capture (runs as sandbox user)

Environment variables:
  IMAGE_NAME          Image to use or build (default: ai-sandbox)
  SSH_SCOPE_DIR       Host SSH subdirectory to mount as ~/.ssh (default: ~/.ssh)
  SANDBOX_UID         UID for the container user (default: host user's id -u)
  SANDBOX_GID         GID for the container user (default: host user's id -g)
  SANDBOX_USER        Username for the container user (default: host username from id -un)
  SANDBOX_GROUP       Group name for the container user (default: host primary group from id -gn)
  EXTRA_MOUNTS        Space-separated list of extra host directories to mount under /repos.
                      Append :ro or :rw to control access per directory (default: rw).
                      Examples:
                        EXTRA_MOUNTS="/path/to/repo"              # read-write (default)
                        EXTRA_MOUNTS="/path/to/repo:ro"           # read-only
                        EXTRA_MOUNTS="/path/to/a:ro /path/to/b"  # a=read-only, b=read-write
  SELF_HEALING_ENABLED  Set to 0 to disable self-healing allowlist (default: 1).
                        When disabled, blocked traffic is logged but IPs are never auto-allowed.
EOF
}

command="${1:-restricted}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
image_name="${IMAGE_NAME:-ai-sandbox}"

build_image() {
  local build_image_name="${1:-$image_name}"
  local build_args=()

  # Detect whether Kiro CLI should be installed into the image.
  # Installs if ~/.kiro exists on the host or kiro.dev is reachable.
  if [[ -d "$HOME/.kiro" ]] || curl -fsS --max-time 3 -o /dev/null https://kiro.dev 2>/dev/null; then
    build_args+=(--build-arg INSTALL_KIRO=1)
  fi

  docker build "${build_args[@]}" -t "$build_image_name" "$script_dir"
}


# Resolve symlinks to their real path; falls back to the original if not found.
# Passing resolved paths to Docker avoids cross-distro symlink resolution issues
# in Rancher Desktop / WSL2 rootless mode (e.g. ~/.aws → /mnt/c/Users/...).
resolve_path() { readlink -f "$1" 2>/dev/null || printf '%s' "$1"; }

# Append bind-mount flags to an array only if the resolved source directory exists.
# Warns and skips silently if the directory is missing — avoids Docker creating
# an empty directory at the mount target instead of mounting real content.
add_mount_if_exists() {
  local -n _flags=$1
  local original_src="$2" dst="$3" opts="${4:-rw}"
  local src
  src="$(resolve_path "$original_src")"
  if [[ -d "$src" ]]; then
    _flags+=(-v "$src:$dst:$opts")
  else
    printf 'WARNING: skipping mount — directory not found: %s\n' "$original_src" >&2
  fi
}

run_container() {
  local mode="$1"
  local workspace_dir
  workspace_dir="$(resolve_path "${2:-$PWD}")"
  local capture_dir_name="${DISCOVERY_CAPTURE_DIR_NAME:-.agent-discovery}"
  local capture_enabled="0"
  local ssh_scope_dir
  ssh_scope_dir="$(resolve_path "${SSH_SCOPE_DIR:-$HOME/.ssh}")"

  if [[ ! -d "$workspace_dir" ]]; then
    printf 'ERROR: workspace directory does not exist: %s\n' "${2:-$PWD}" >&2
    exit 1
  fi

  # NET_ADMIN: required in both modes for iptables setup.
  # NET_RAW: required in both modes — restricted uses it for the background blocked-traffic capture daemon;
  #          discovery uses it for tcpdump. The agent shell has NET_RAW dropped in restricted mode via capsh.
  local capabilities=(--cap-add=NET_ADMIN --cap-add=NET_RAW)

  # In restricted mode, config dirs are mounted into the sandbox user's home.
  # In discovery mode, configs also mount into the sandbox user's home so that
  # files created during discovery (e.g. AI agent sessions) have correct ownership
  # when the container is later run in restricted mode.
  local sandbox_username="${SANDBOX_USER:-$(id -un)}"
  local dev_home="/home/$sandbox_username"
  if [[ "$mode" == "discovery" ]]; then
    capture_enabled="1"
    mkdir -p "$workspace_dir/$capture_dir_name"
  fi

  # Validate and build EXTRA_MOUNTS flags; abort early if any path is missing.
  local extra_mount_flags=()
  if [[ -n "${EXTRA_MOUNTS:-}" ]]; then
    for entry in $EXTRA_MOUNTS; do
      local dir opt real_dir
      dir="${entry%%:*}"
      opt="${entry##*:}"
      [[ "$opt" == "$dir" ]] && opt="rw"
      real_dir="$(resolve_path "$dir")"
      if [[ ! -d "$real_dir" ]]; then
        printf 'ERROR: EXTRA_MOUNTS path does not exist: %s\n' "$dir" >&2
        exit 1
      fi
      extra_mount_flags+=(-v "$real_dir:/repos/$(basename "$dir"):$opt")
    done
  fi

  # Build optional config mount flags; skip and warn for any directory not found.
  local config_mount_flags=()
  add_mount_if_exists config_mount_flags "$ssh_scope_dir"     "$dev_home/.ssh"       ro
  add_mount_if_exists config_mount_flags "$HOME/.config/gh"   "$dev_home/.config/gh"
  add_mount_if_exists config_mount_flags "$HOME/.copilot"     "$dev_home/.copilot"
  add_mount_if_exists config_mount_flags "$HOME/.kiro"        "$dev_home/.kiro"
  add_mount_if_exists config_mount_flags "$HOME/.local/share/kiro-cli" "$dev_home/.local/share/kiro-cli"
  add_mount_if_exists config_mount_flags "$HOME/.aws"         "$dev_home/.aws"
  add_mount_if_exists config_mount_flags "$HOME/.azure"       "$dev_home/.azure"
  add_mount_if_exists config_mount_flags "$HOME/.kube"        "$dev_home/.kube"

  docker run -it --rm \
    "${capabilities[@]}" \
    --add-host=host.docker.internal:host-gateway \
    --cpus="4.0" \
    --memory="8g" \
    -e DEV_CONTAINER_MODE="$mode" \
    -e DISCOVERY_CAPTURE_ENABLED="$capture_enabled" \
    -e DISCOVERY_CAPTURE_DIR="/workspace/$capture_dir_name" \
    -e HOST_WORKSPACE_DIR="$workspace_dir" \
    -e IMAGE_NAME="$image_name" \
    -e SANDBOX_UID="${SANDBOX_UID:-$(id -u)}" \
    -e SANDBOX_GID="${SANDBOX_GID:-$(id -g)}" \
    -e SANDBOX_USER="${SANDBOX_USER:-$(id -un)}" \
    -e SANDBOX_GROUP="${SANDBOX_GROUP:-$(id -gn)}" \
    ${SELF_HEALING_ENABLED:+-e SELF_HEALING_ENABLED="$SELF_HEALING_ENABLED"} \
    -v "$workspace_dir:/workspace" \
    "${extra_mount_flags[@]}" \
    "${config_mount_flags[@]}" \
    -w /workspace \
    "$image_name"
}

case "$command" in
  build)
    build_image "${2:-$image_name}"
    ;;
  restricted|discovery)
    run_container "$command" "${2:-$PWD}"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac

