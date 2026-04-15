# dtmgd — Dynatrace Managed CLI

`dtmgd` is a `kubectl`-inspired read-only CLI for **Dynatrace Managed** (self-hosted) clusters, modeled after [`dtctl`](https://github.com/dynatrace-oss/dtctl).

It gives you terminal access to problems, entities, events, logs, metrics, SLOs, and security vulnerabilities via the Dynatrace Managed classic API — with the same feature set as the [Dynatrace Managed MCP Server](https://github.com/dynatrace-oss/dynatrace-managed-mcp).

## API Endpoints

| Target | URL pattern |
|---|---|
| Environment API (observations) | `{host}/e/{env-id}/api/v2/` |
| Cluster management API | `{host}/api/v1.0/onpremise/` |

## Installation

**Option 1 — go install (requires Go 1.22+)**

```bash
go install github.com/dynatrace-oss/dtmgd@latest
```

**Option 2 — build from source**

```bash
git clone https://github.com/dynatrace-oss/dtmgd.git
cd dtmgd
make install   # installs to $GOPATH/bin (or $HOME/go/bin)
```

Make sure `$(go env GOPATH)/bin` is in your `$PATH`.

## Quick Start

```bash
# 1. Create a context pointing to your Managed cluster
dtmgd config set-context prod \
  --host https://managed.company.com \
  --env-id abc12345 \
  --token-ref prod-token

# 2. Store your API token (saved securely in OS keyring when available)
dtmgd config set-credentials prod-token --token <your-api-token>

# 3. Verify connectivity
dtmgd get environments

# 4. List open problems
dtmgd get problems --status OPEN
```

> **Note on token storage**: when an OS keyring (macOS Keychain, GNOME Keyring,
> Windows Credential Manager) is available, the token is stored there and the
> config file keeps only an empty placeholder. On headless Linux systems without
> a keyring daemon, the token is stored in plaintext in the config file
> (`~/.config/dtmgd/config`, mode 0600). Run
> `dtmgd config migrate-tokens` any time after a keyring becomes available.

## Authentication

`dtmgd` uses API token-based authentication. Create an API token in your Managed cluster with the required scopes.

For more information about creating API tokens in Managed deployments, refer to the [Dynatrace Managed documentation](https://docs.dynatrace.com/managed/discover-dynatrace/references/dynatrace-api/basics/dynatrace-api-authentication).

### Required API Token Scopes

Your API token must include the following scopes for full functionality:

- Access problem and event feed, metrics, and topology (`DataExport`) — required for `dtmgd get environments`
- Read cluster configuration (`ReadConfig`)
- Read audit logs (`auditLogs.read`)
- Read entities (`entities.read`)
- Read events (`events.read`)
- Read logs (`logs.read`)
- Read metrics (`metrics.read`)
- Read network zones (`networkZones.read`)
- Read problems (`problems.read`)
- Read security problems (`securityProblems.read`)
- Read SLO (`slo.read`)

> **Note:** API token scopes in Managed deployments differ from SaaS Platform tokens. Ensure you select the correct scopes for your Managed cluster version.

## Commands

### Configuration

```
dtmgd config set-context <name> --host <url> --env-id <id> --token-ref <ref>
dtmgd config set-credentials <name> --token <api-token>
dtmgd config get-contexts          # list all contexts
dtmgd config current-context       # show active context name
dtmgd config use-context <name>    # switch active context
dtmgd config delete-context <name> # remove a context
dtmgd config migrate-tokens        # move plaintext tokens to OS keyring
dtmgd config view                  # dump full config
dtmgd config init                  # create .dtmgd.yaml in current directory
dtmgd ctx                          # shortcut: list or switch contexts
dtmgd ctx [context-name]           # switch context
dtmgd ctx current                  # show current context
dtmgd ctx delete <name>            # delete context
```

### Get (list resources)

```
dtmgd get environments       # verify connectivity and cluster version
dtmgd get problems           # --from, --to, --status, --impact, --selector, --entity, --limit, --sort
dtmgd get entities           # --selector (required), --from, --to, --limit, --sort, --mz
dtmgd get entity-types       # list all entity types
dtmgd get events             # --from (required), --to, --type, --entity, --limit
dtmgd get metrics            # --search, --entity, --limit
dtmgd get slos               # --enabled, --limit, --evaluate, --selector (sloSelector DSL)
dtmgd get security-problems  # --risk, --status, --selector, --limit
```

> `describe problem` accepts the UUID from the `PROBLEM-ID` column, **not** the
> short `P-XXXXX` display ID shown in the `DISPLAY-ID` column.
>
> Some problem UUIDs are negative integers (e.g. `-6546711275898328738_1776193140000V2`).
> Pass them after `--` to prevent the leading `-` from being parsed as a flag:
> ```
> dtmgd describe problem -- -6546711275898328738_1776193140000V2
> ```

### Describe (detail view, outputs JSON by default)

```
dtmgd describe problem <problem-id>
dtmgd describe entity <entity-id>
dtmgd describe entity-type <type>
dtmgd describe entity-relations <entity-id>
dtmgd describe event <event-id>
dtmgd describe metric <metric-id>
dtmgd describe slo <slo-id>  [--timeframe CURRENT|GTF --from ... --to ...]
dtmgd describe security-problem <security-problem-id>
```

### Query (time-range data)

```
dtmgd query metrics --metric builtin:service.response.time --from now-1h --to now
dtmgd query metrics --metric builtin:host.cpu.usage --from now-24h --resolution 1h
dtmgd query logs --query "error" --from now-1h --to now
dtmgd query logs --query "timeout" --from now-30m --limit 50
dtmgd query logs --query "error" --from now-1h --entity 'type(SERVICE),tag("[Environment]BookStore")'
dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]BookStore")' --from now-1h
# Note: type(SERVICE) is auto-converted to type(PROCESS_GROUP) internally (logs are attributed
# to process groups on DT Managed Classic). Services with ERROR-only log level show 0 INFO/WARN.
```

## Global Flags

| Flag | Description |
|---|---|
| `--config <file>` | Config file path (default: `~/.config/dtmgd/config`) |
| `-c, --context <name>` | Override current context |
| `-e, --env <spec>` | Target environment(s): context name, `"prod;staging"`, or `ALL_ENVIRONMENTS` |
| `-o, --output <format>` | Output format: `table` (default), `wide`, `json`, `yaml` |
| `-A, --agent` | Force AI agent envelope output (`{ok, result, context}`) |
| `--no-agent` | Disable auto-detected agent mode |
| `--max-pages <n>` | Maximum pages to fetch (0 = all, default). Pagination is automatic. |
| `--columns <cols>` | Comma-separated columns to show in table output |
| `-w, --watch` | Re-run the command periodically |
| `--watch-interval <d>` | Interval between watch refreshes (default: `5s`) |
| `-v` | Verbose: show HTTP request/response summaries |
| `-vv` | Extra verbose: full headers and body |

## Multi-Environment Queries

Query one, several, or all configured environments in a single command.
Requests fan out in parallel and results are merged.

```bash
# Query all environments
dtmgd get problems --env ALL_ENVIRONMENTS -o json

# Query specific environments (semicolon-separated)
dtmgd get problems --env "prod;staging" -o json

# Single env result → unwrapped data
# Multi env result → { "prod": {...}, "staging": {...} }
```

The `--env` flag works with all `get`, `describe`, and `query` commands.

## AI Agent Mode

When running inside an AI agent (Claude Code, Cursor, GitHub Copilot, Kiro, etc.),
dtmgd auto-detects the environment and wraps all output in a structured JSON envelope:

```json
{
  "ok": true,
  "result": { "problems": [...], "totalCount": 5 },
  "context": { "resource": "problems" }
}
```

Errors are also wrapped:

```json
{
  "ok": false,
  "error": { "code": "error", "message": "API error 401: Unauthorized" }
}
```

Force it on with `-A` / `--agent`, or disable auto-detection with `--no-agent`.

## Pagination

All list commands (`get problems`, `get entities`, etc.) automatically follow
`nextPageKey` to fetch all pages. Use `--limit` to cap results to a single page,
or `--max-pages` to limit the number of pages fetched.

## Watch Mode

Monitor resources in real-time with `--watch`:

```bash
dtmgd get problems --status OPEN --watch
dtmgd get problems --watch --watch-interval 30s
dtmgd get events --from now-1h --watch --watch-interval 10s
```

Press `Ctrl+C` to stop. Default interval is 5 seconds.

## Time Formats

Both relative and absolute times are accepted:

| Format | Example |
|---|---|
| Relative | `now-1h`, `now-24h`, `now-7d`, `now-30m` |
| ISO 8601 | `2024-01-01T10:00:00Z` |
| Unix ms | `1640995200000` |

## Configuration File Format

```yaml
apiVersion: dtmgd.io/v1
kind: Config
current-context: production
contexts:
  - name: production
    context:
      host: https://managed.company.com   # Managed cluster base URL
      env-id: abc12345                     # Environment ID
      token-ref: prod-token
      description: Production environment
      http-proxy: http://proxy.corp:8080   # optional HTTP proxy
      https-proxy: http://proxy.corp:8080  # optional HTTPS proxy
tokens:
  - name: prod-token
    token: ""   # empty when stored in OS keyring
preferences:
  output: table
```

Environment variables are expanded in the config file: `${DT_MANAGED_HOST}`.

A project-local `.dtmgd.yaml` takes precedence over the global `~/.config/dtmgd/config`.

## Building

```bash
# Requires Go 1.22+
make build        # produces ./dtmgd
make install      # installs to $GOPATH/bin (or $HOME/go/bin)
make vet          # run go vet
make fmt          # format source with gofmt
make clean        # remove ./dtmgd binary
```

### Shell Completion

```bash
# Bash
dtmgd completion bash > /etc/bash_completion.d/dtmgd

# Zsh
dtmgd completion zsh > "${fpath[1]}/_dtmgd"

# Fish
dtmgd completion fish > ~/.config/fish/completions/dtmgd.fish
```

## Feature Parity with Dynatrace Managed MCP Server

| MCP Tool | dtmgd command |
|---|---|
| `get_environments_info` | `dtmgd get environments` |
| `list_available_metrics` | `dtmgd get metrics` |
| `get_metric_details` | `dtmgd describe metric <id>` |
| `query_metrics_data` | `dtmgd query metrics` |
| `query_logs` | `dtmgd query logs` |
| `list_events` | `dtmgd get events` |
| `get_event_details` | `dtmgd describe event <id>` |
| `list_entity_types` | `dtmgd get entity-types` |
| `get_entity_type_details` | `dtmgd describe entity-type <type>` |
| `discover_entities` | `dtmgd get entities --selector <sel>` |
| `get_entity_details` | `dtmgd describe entity <id>` |
| `get_entity_relationships` | `dtmgd describe entity-relations <id>` |
| `list_problems` | `dtmgd get problems` |
| `get_problem_details` | `dtmgd describe problem <id>` |
| `list_security_problems` | `dtmgd get security-problems` |
| `get_security_problem_details` | `dtmgd describe security-problem <id>` |
| `list_slos` | `dtmgd get slos` |
| `get_slo_details` | `dtmgd describe slo <id>` |
