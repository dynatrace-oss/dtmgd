# AI Sandbox Container Assets (Dynatrace / Corporate Example)

This directory is the repo-ready asset bundle for the Dynatrace-flavored AI sandbox container described in [Use dev containers for development with AI agents](https://dynatrace.stackenterprise.co/articles/23444).

It packages a CLI-only Docker-based workspace for running AI coding agents (GitHub Copilot CLI, Kiro CLI, and others) inside an isolated container with deny-by-default outbound network controls and a non-root agent shell.

## What is included

- `Dockerfile` builds the image with Git, GitHub CLI, GitHub Copilot CLI, Kiro CLI (optional), Java, Node.js, Angular CLI, AWS CLI, Azure CLI, `kubectl`, packet capture tools, and a non-root sandbox user created at runtime.
- `entrypoint.sh` applies either a restricted firewall or a discovery mode at container startup. In both modes it creates the sandbox user and drops to it via `capsh`. Restricted mode drops `NET_ADMIN` and `NET_RAW`; discovery mode drops only `NET_ADMIN` (keeping `NET_RAW` for tcpdump).
- `refresh-ipset-allowlist.sh` resolves the concrete allowlist domains into IPv4 and IPv6 `ipset` sets.
- `capture-blocked-traffic.sh` runs as a background root daemon in restricted mode, logging every blocked outbound destination to `/workspace/.agent-blocked/`.
- `capture-agent-destinations.sh` helps you discover additional AI-agent-related DNS and TLS destinations in discovery mode.
- `allowlist-domains.txt` contains the concrete FQDN allowlist for this corporate example, including Copilot, Kiro, Dynatrace, and internal endpoints.
- `allowlist-cidrs.txt` contains explicit IP and CIDR entries, typically loopback plus narrow proxy ranges.
- `allowlist-proxy-domains.txt` contains the wildcard domain patterns (Copilot, Kiro) used by the self-healing daemon for reactive auto-allowing, and optionally by an upstream proxy or FQDN-aware firewall.
- `runme.sh` is the convenience wrapper for building the image and running the container.

## Usage

Build the image:

```bash
./runme.sh build
```

During build, `runme.sh` auto-detects whether Kiro CLI should be installed. If `~/.kiro` exists on the host or `kiro.dev` is reachable, Kiro CLI is included in the image. Otherwise the build proceeds without it — no failure either way.

Run in restricted mode with the firewall enabled:

```bash
./runme.sh restricted /path/to/your/repo
```

Run in discovery mode to capture outbound destinations before tightening the allowlist:

```bash
./runme.sh discovery /path/to/your/repo
```

Inside the container, the repository is mounted at `/workspace`.

## Extracting discovery results

After running in discovery mode, reproduce the AI agent interaction you want to observe, then exit the container (`Ctrl+D`). The pcap capture file persists on the host in the `.agent-discovery` directory inside your workspace.

Extract the DNS and TLS hostname lists:

```bash
docker run --rm --entrypoint capture-agent-destinations.sh \
  -v "/path/to/your/repo:/workspace" "${IMAGE_NAME:-ai-sandbox}" extract /workspace/.agent-discovery
```

The container prints this command with the correct path when discovery mode starts. The output lists:

- DNS queries — hostnames the container attempted to resolve.
- TLS SNI hostnames — HTTPS endpoints presented during TLS handshakes.

Add the discovered hostnames to `allowlist-domains.txt`, rebuild the image, and switch to restricted mode.

## Mounting additional repositories

Set `EXTRA_MOUNTS` to a space-separated list of host paths. Append `:ro` or `:rw` to control per-directory access. The default is read-write.

```bash
# backend is the primary workspace; ui is read-write, reference-docs is read-only
SSH_SCOPE_DIR="$HOME/.ssh/myproject" \
EXTRA_MOUNTS="/path/to/myproject-ui /path/to/reference-docs:ro" \
bash ./runme.sh restricted /path/to/myproject-backend
```

Each path is mounted at `/repos/<basename>` inside the container.

## Host configuration mounts

The container automatically mounts the following directories from the host (if they exist) into the sandbox user's home:

| Host directory | Container path | Mode |
|---|---|---|
| `~/.ssh` (or `SSH_SCOPE_DIR`) | `~/.ssh` | read-only |
| `~/.config/gh` | `~/.config/gh` | read-write |
| `~/.copilot` | `~/.copilot` | read-write |
| `~/.kiro` | `~/.kiro` | read-write |
| `~/.local/share/kiro-cli` | `~/.local/share/kiro-cli` | read-write |
| `~/.aws` | `~/.aws` | read-write |
| `~/.azure` | `~/.azure` | read-write |
| `~/.kube` | `~/.kube` | read-write |

Missing directories are silently skipped.

## Reviewing blocked traffic

When running in restricted mode, blocked outbound destinations are logged automatically to `/workspace/.agent-blocked/`. These files persist on the host via the workspace mount.

| File | Purpose |
|------|---------|
| `blocked.log` | Timestamped log of every blocked connection attempt |
| `blocked-domains.txt` | Deduplicated domain list — copy-paste directly into `allowlist-domains.txt` |
| `blocked-ips.txt` | Deduplicated IPs with no known domain — copy-paste into `allowlist-cidrs.txt` |

To update the allowlist after a session:

```bash
cat /workspace/.agent-blocked/blocked-domains.txt
# copy the domain lines (below the comment header) → paste into allowlist-domains.txt

cat /workspace/.agent-blocked/blocked-ips.txt
# copy the IP lines → paste into allowlist-cidrs.txt
```

Then rebuild the image and restart the container.

## Security model (restricted mode)

1. **iptables** sets a deny-by-default OUTPUT policy and allows only the allowlisted destinations.
2. **Capability drop**: after iptables is configured, the agent shell is started via `capsh --drop=cap_net_admin,cap_net_raw`, so it cannot modify firewall rules or create raw sockets regardless of file permissions.
3. **Non-root user**: the agent runs as a sandbox user whose username, UID, and GID match the host user that started the container (detected automatically by `runme.sh` via `id -u`, `id -g`, `id -un`, `id -gn`). Override by setting `SANDBOX_UID`, `SANDBOX_GID`, `SANDBOX_USER`, `SANDBOX_GROUP` before running.
4. **Background daemons**: the ipset refresh loop and the blocked-traffic capture daemon are forked before the capability drop and retain their root capabilities to do their jobs.
5. **Self-healing allowlist**: when a blocked IP maps to a domain that is already in `allowlist-domains.txt` or matches a wildcard pattern from `allowlist-proxy-domains.txt`, the daemon adds the IP to the active ipset on the fly. This cannot be exploited by the sandbox user: the internal lookup tables (DNS map, domain caches) are stored in a root-only directory (`/run/agent-blocked-internal`, mode 700) inaccessible to the sandbox shell, and `CAP_NET_RAW` is dropped so DNS responses cannot be spoofed. Set `SELF_HEALING_ENABLED=0` to disable self-healing entirely and use logging-only mode.

Discovery mode runs as the sandbox user with unrestricted egress and `NET_RAW` retained (for tcpdump). It is intended for supervised traffic observation only.

## Corporate customization points

- Update `allowlist-domains.txt` with your actual Dynatrace environments, MCP endpoints, artifact repositories, and Git hosts.
- If agent traffic must go through a corporate proxy, keep wildcard domains in `allowlist-proxy-domains.txt` and allow only the proxy IPs in `allowlist-cidrs.txt`.
- The sandbox user identity (`SANDBOX_UID`, `SANDBOX_GID`, `SANDBOX_USER`, `SANDBOX_GROUP`) is detected automatically from the host user at runtime. No build-time args needed.
- Review the default values in `runme.sh`, especially `IMAGE_NAME` and `SSH_SCOPE_DIR`, before publishing this into a separate repository.
- Mount only the host credentials you actually need.

## Important notes

- Plain `iptables` cannot pre-resolve wildcard domains such as `*.githubcopilot.com` or `*.kiro.dev` into IP addresses. The self-healing daemon handles this reactively by auto-allowing IPs whose resolved domains match wildcard patterns in `allowlist-proxy-domains.txt`. An upstream proxy provides proactive enforcement if available.
- The concrete domains in `allowlist-domains.txt` are a practical baseline, not a guarantee that every future agent endpoint is covered.
- The asset set is intentionally CLI-only and does not depend on VS Code dev containers.
- Kiro CLI installation is conditional: it is only included in the image when `~/.kiro` exists on the host or `kiro.dev` is reachable at build time. Other users who cannot access `kiro.dev` get a working image without Kiro.
