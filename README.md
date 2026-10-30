# dtmgd — Dynatrace Managed CLI

`dtmgd` is a `kubectl`-inspired read-only CLI for **Dynatrace Managed** (self-hosted) environments, modeled after [`dtctl`](https://github.com/dynatrace-oss/dtctl).

It gives you terminal access to problems, entities, events, logs, metrics, SLOs, and security vulnerabilities via the Dynatrace Managed classic API — covering every tool of the [Dynatrace Managed MCP Server](https://github.com/dynatrace-oss/dynatrace-managed-mcp); the differences are listed in [MCP server parity](#dynatrace-managed-mcp-server-parity).

> [!NOTE]
> This open source product is supported by the community.
> For feature requests, questions, or assistance, please use [GitHub Issues](https://github.com/dynatrace-oss/dtmgd/issues).

## Architecture

![dtmgd architecture — a local CLI fanning out across any number of Dynatrace Managed environments](assets/architecture.png)

`dtmgd` runs entirely on your workstation and talks directly to the Dynatrace
Managed APIs, with no broker and no third-party service in between. Each named
context points at one Dynatrace Managed environment. Environments may live on
the same cluster or on separate clusters, in different locations, running
different Dynatrace versions. Because the same systems are often monitored by
several environments (production, staging, development), a single command can
query one environment or fan out across every environment you have configured.
See [Multi-Environment Queries](docs/configuration.md#multi-environment-queries). A context without
an `env-id` is a cluster context, for the cluster's own Cluster Management API:
its environments, nodes, license and backup configuration. See
[Cluster Contexts](docs/configuration.md#cluster-contexts).

## Installation

**Option 1 — Homebrew (macOS / Linux)**

```bash
brew install dynatrace-oss/tap/dtmgd

# Upgrades work automatically:
brew upgrade dtmgd
```

**Option 2 — install script (Linux, macOS, Windows)**

For hosts without Homebrew or Go, such as jump hosts:

```bash
# Linux / macOS: installs to ~/.local/bin (Linux) or /usr/local/bin (macOS)
curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.sh | sh
```

```powershell
# Windows: installs to %LOCALAPPDATA%\dtmgd and adds it to your user PATH
irm https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.ps1 | iex
```

The scripts check the archive against the release's `checksums.txt` before
installing, and leave an installed dtmgd as it was if anything fails. That
catches a corrupted or incomplete download; it cannot catch a compromised
release, because `checksums.txt` comes from the same place. For that, verify
the signature (see [Verifying a download](#verifying-a-download)). The
scripts read these environment variables:

| Variable | Meaning |
|---|---|
| `DTMGD_VERSION` | Version to install, e.g. `0.3.1` (default: the latest release; required with `DTMGD_DOWNLOAD_BASE`) |
| `DTMGD_INSTALL_DIR` | Install directory |
| `DTMGD_DOWNLOAD_BASE` | A mirror of the release downloads, laid out as `<base>/v<version>/<asset>`, for hosts that cannot reach GitHub |
| `DTMGD_NO_MODIFY_PATH` | `1` keeps `install.ps1` from changing your user PATH |

**Option 3 — go install (requires Go 1.26.10+)**

```bash
go install github.com/dynatrace-oss/dtmgd@latest
```

`dtmgd version` then reports the module version you installed.

**Option 4 — build from source**

```bash
git clone https://github.com/dynatrace-oss/dtmgd.git
cd dtmgd
make install   # installs to $GOPATH/bin (or $HOME/go/bin)
```

Make sure `$(go env GOPATH)/bin` is in your `$PATH`.

### Shell Completion

```bash
# Bash
dtmgd completion bash > /etc/bash_completion.d/dtmgd

# Zsh
dtmgd completion zsh > "${fpath[1]}/_dtmgd"

# Fish
dtmgd completion fish > ~/.config/fish/completions/dtmgd.fish
```

### Verifying a download

Every release has a `checksums.txt` for its archives. From v0.3.1 on,
`checksums.txt` is also signed with [cosign](https://docs.sigstore.dev/)
(keyless, by the release workflow), and each archive has an SPDX SBOM next to
it (`<archive>.sbom.json`). With the archive, `checksums.txt` and
`checksums.txt.sigstore.json` from the release page in one directory:

```bash
# Is checksums.txt the one the dtmgd release workflow signed?
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/dynatrace-oss/dtmgd/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# Does the archive match it? (macOS: shasum -a 256 --ignore-missing -c checksums.txt)
sha256sum --ignore-missing -c checksums.txt
```

## Quick Start

`dtmgd` supports any number of named environments ("contexts"). The name you
choose is only an alias — there are no reserved names. Pick whatever fits
your setup: `prod`, `staging`, `dev`, `eu-prod`, `acme-prod`, anything.

```bash
# 1. Create a context pointing to your Managed cluster.
#    Replace "prod" below with any alias you want for this environment.
dtmgd config set-context prod \
  --host https://managed.company.com \
  --env-id abc12345 \
  --token-ref prod-token

# 2. Store your API token. You are prompted for it, with echo disabled, so it
#    stays out of your shell history. Saved to the OS keyring when available.
dtmgd config set-credentials prod-token

# 3. Verify connectivity, token and cluster version
dtmgd doctor

# 4. List open problems
dtmgd get problems --status OPEN
```

You can repeat step 1 as many times as needed to register additional
environments — there is no limit on the number of contexts, and each can use
any alias you choose. See [Multi-Environment Queries](docs/configuration.md#multi-environment-queries)
for cross-context fan-out.

> **Note on token storage**: when an OS keyring (macOS Keychain, GNOME Keyring,
> Windows Credential Manager) is available, the token is stored there and the
> config file keeps only an empty placeholder. On headless Linux systems without
> a keyring daemon, the token is stored in plaintext in the config file
> (`~/.config/dtmgd/config` on Linux; see [Which config file is used](docs/configuration.md#which-config-file-is-used)), with mode 0600. Run
> `dtmgd config migrate-tokens` any time after a keyring becomes available.

### Supplying the token in scripts and CI

Pipe the token in rather than passing it as a flag:

```bash
echo "$DT_API_TOKEN" | dtmgd config set-credentials prod-token --token-stdin
dtmgd config set-credentials prod-token --token-stdin < /run/secrets/dt-token
```

`--token <value>` still works, but the kernel copies command-line arguments
into the process image at `execve(2)`, so the value is readable from
`/proc/<pid>/cmdline` by any process running as you, and your shell records the
whole line in its history. Nothing dtmgd does after startup can undo that,
which is why it warns when you use the flag.

In CI, prefer skipping stored credentials altogether: reference the variable
from the config with `${DT_API_TOKEN}` (see
[Environment variables in the config](docs/configuration.md#environment-variables-in-the-config)) and let
the secret live only in the environment. On headless runners there is no
keyring, so a stored token is written to the config file in plaintext — a
`${VAR}` reference never is.

## Authentication

`dtmgd` uses API token-based authentication. Create an API token in your Managed cluster with the required scopes.

For more information about creating API tokens in Managed deployments, refer to the [Dynatrace Managed documentation](https://docs.dynatrace.com/managed/discover-dynatrace/references/dynatrace-api/basics/dynatrace-api-authentication).

### Required API Token Scopes

Your API token needs these scopes for every command to work. Each command
declares the scopes it uses, and a permission error names the ones the failed
command needs; a test keeps this list identical to those declarations.

<!-- scopes:start -->
- Access problem and event feed, metrics, and topology (`DataExport`): `doctor`
- Read entities (`entities.read`): entities, entity types and relations; name lookups in `query metrics` and `query log-counts`
- Read events (`events.read`): events
- Read logs (`logs.read`): `query logs`, `query log-counts`
- Read metrics (`metrics.read`): metrics, `query metrics`
- Read problems (`problems.read`): problems
- Read security problems (`securityProblems.read`): security problems
- Read SLO (`slo.read`): SLOs
- Service Provider API (`ServiceProviderAPI`), on a cluster context's token: `get environments`, `describe environment`, `get nodes`, `describe cluster`, `describe license`, `get license-usage`, `describe backup`, `doctor`
<!-- scopes:end -->

> **Note:** API token scopes in Managed deployments differ from SaaS Platform tokens. Ensure you select the correct scopes for your Managed cluster version.

## Commands

The grammar is `dtmgd <verb> <resource> [flags]`, as in `kubectl`. Every
command and flag is in the [command reference](docs/reference/dtmgd.md), and
`dtmgd commands` prints the same as a JSON catalog for scripts and agents.

| Verb | Resources |
|---|---|
| `get` | `problems`, `entities`, `entity-types`, `events`, `metrics`, `slos`, `security-problems`; cluster contexts: `environments`, `nodes`, `license-usage` |
| `describe` | `problem`, `entity`, `entity-type`, `entity-relations`, `event`, `metric`, `slo`, `security-problem`; cluster contexts: `environment`, `cluster`, `license`, `backup` |
| `query` | `metrics`, `logs`, `log-counts` |
| `doctor` | checks every context: connectivity, token, cluster version |
| `config`, `ctx` | contexts and credentials ([configuration](docs/configuration.md)) |
| `skills` | installs the dtmgd skill for an AI agent ([agent mode](docs/agent-mode.md)) |
| `commands`, `version`, `completion` | the command catalog, the CLI's version, shell completion |

```bash
dtmgd get problems --status OPEN --from now-24h
dtmgd describe problem <problem-id> -o json
dtmgd query metrics --metric builtin:service.response.time --from now-1h
dtmgd get problems --env ALL_ENVIRONMENTS     # every environment context, in parallel
dtmgd get nodes --context prod-cluster        # a cluster context
```

Output is a table by default, or `-o wide|json|yaml|csv|tsv`. Exit codes, paging and
time formats: [docs/output.md](docs/output.md).

## Contexts

A context names one target. An **environment context** has an `env-id` and
reads one environment's API (problems, entities, metrics and so on); you can
query several at once with `--env "prod;staging"` or `--env ALL_ENVIRONMENTS`.
A **cluster context** has no `env-id` and reads the cluster's Cluster
Management API (environments, nodes, license, backup) with a
ServiceProviderAPI token. The config file, `${VAR}` references, cluster
contexts and multi-environment output are described in
[docs/configuration.md](docs/configuration.md).

## AI agents

dtmgd detects AI agents (Claude Code, Cursor, GitHub Copilot, Codex and
others) and then wraps its output in a JSON envelope with classified errors
and suggestions. `dtmgd skills install` installs a skill that teaches the
agent how to use dtmgd. Details: [docs/agent-mode.md](docs/agent-mode.md).

## Dynatrace Managed MCP server parity

dtmgd covers every tool of the
[Dynatrace Managed MCP Server](https://github.com/dynatrace-oss/dynatrace-managed-mcp),
and adds the Cluster Management API. Where the two send different requests,
the [parity table](skills/dtmgd/references/mcp-parity.md) says so, tool by
tool.

## Documentation

- [docs/](docs/README.md): configuration and contexts, output and exit codes, AI agent mode
- [Command reference](docs/reference/dtmgd.md), generated from the CLI
- [The dtmgd skill](skills/dtmgd/SKILL.md), for AI agents

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md). Report
security issues as described in [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE)
