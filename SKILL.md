---
name: dtmgd
description: Query and investigate Dynatrace Managed (self-hosted) environments from the CLI. Use this skill when the user asks about problems, incidents, entities, events, logs, metrics, SLOs, or security vulnerabilities in a Dynatrace Managed cluster — even if they don't mention dtmgd by name. Covers connectivity checks, entity topology, time-series metric queries, log search, and full feature parity with the Dynatrace Managed MCP Server.
---

# Dynatrace Managed CLI — dtmgd

`dtmgd` is a `kubectl`-inspired read-only CLI for **Dynatrace Managed (self-hosted)** clusters.
It mirrors the full tool set of the Dynatrace Managed MCP Server as shell commands.

## Recommended Initialization

At the start of any investigation, run these to confirm connectivity and current context:

```bash
# Show active context (and all configured environments)
dtmgd ctx

# Verify connectivity and get cluster version (checks all environments)
dtmgd get environments

# Check version of the CLI itself
dtmgd version
```

Agent mode is auto-detected — output will be wrapped in `{ok, result, context}` envelopes automatically.

## Output Formats

Use `-o json` for AI/script consumption. Default is human-friendly `table`.

```bash
-o table    # default; human-readable
-o wide     # table with extra columns (timestamps, root cause, etc.)
-o json     # indented JSON — best for parsing and piping to jq
-o yaml     # YAML output
```

### AI Agent Envelope Mode

dtmgd auto-detects AI agent environments (Claude Code, Cursor, GitHub Copilot,
Amazon Q, Kiro, Junie, OpenCode) and wraps all output in a structured envelope:

```json
{
  "ok": true,
  "result": { "problems": [...], "totalCount": 5 },
  "context": { "resource": "problems" }
}
```

Errors are also wrapped: `{"ok": false, "error": {"code": "error", "message": "..."}}`.

Force with `-A`/`--agent`, disable with `--no-agent`.

**Tip for AI agents**: prefer `-o json` or `-A` when parsing results:

```bash
dtmgd get problems --status OPEN -o json
dtmgd describe problem <uuid> -A
```

## Global Flags

```
-c, --context <name>    override the current context (environment)
-e, --env <spec>        target environment(s): name, "prod;staging", or ALL_ENVIRONMENTS
-o, --output <format>   table | wide | json | yaml
-A, --agent             force agent envelope output
--no-agent              disable auto-detected agent mode
--max-pages <n>         max pages to fetch (0 = all). Pagination is automatic.
--columns <cols>        comma-separated columns to show in table output
-w, --watch             re-run the command periodically
--watch-interval <d>    interval between watch refreshes (default: 5s)
-v                      verbose: show HTTP request/response summary
-vv                     extra verbose: full headers + body (auth redacted)
--config <file>         use a specific config file instead of the default
```

## Multi-Environment Queries

Query multiple environments in parallel with `--env`:

```bash
# All configured environments
dtmgd get problems --env ALL_ENVIRONMENTS -o json

# Specific environments (semicolon-separated)
dtmgd get problems --env "prod;staging" -o json

# Single env → unwrapped data
# Multi env → { "prod": {...}, "staging": {...} }
```

Works with all `get`, `describe`, and `query` commands.

## Pagination

All list commands automatically follow `nextPageKey` to fetch complete results.
Use `--limit` to cap to a single page, or `--max-pages` to limit page count.

## Watch Mode

Monitor resources in real-time:

```bash
dtmgd get problems --status OPEN --watch
dtmgd get events --from now-1h --watch --watch-interval 10s
```

## Column Filtering

Show only specific columns in table output:

```bash
dtmgd get problems --columns "PROBLEM-ID,TITLE,STATUS"
dtmgd get entities --selector 'type(HOST)' --columns "ENTITY-ID,DISPLAY-NAME"
```

## Context Management

```bash
# List all configured environments
dtmgd ctx
dtmgd config get-contexts

# Show or switch context
dtmgd ctx current
dtmgd ctx <context-name>

# Configure a new environment
dtmgd config set-context prod \
  --host https://managed.company.com \
  --env-id abc12345 \
  --token-ref prod-token

dtmgd config set-credentials prod-token --token <api-token>

# Delete context
dtmgd ctx delete <name>
```

Proxy support: add `http-proxy` and/or `https-proxy` to the context in the config file for environments behind corporate proxies.

## Problems

```bash
# List open problems (last 24 h default)
dtmgd get problems --status OPEN

# Across all environments
dtmgd get problems --status OPEN --env ALL_ENVIRONMENTS

# Narrow by time, impact level, or entity
dtmgd get problems --from now-2h --to now
dtmgd get problems --impact SERVICE --limit 20
dtmgd get problems --entity 'type(SERVICE),entityName.contains("checkout")'

# Sort: open first, or newest first
dtmgd get problems --sort "+status"
dtmgd get problems --sort "-startTime"

# Full problem details (use UUID from PROBLEM-ID column, NOT P-XXXXX display ID)
dtmgd describe problem <uuid> -o json
```

> **Important**: `describe problem` requires the internal UUID (e.g. `8a3f1b2c-...`),
> **not** the short display ID like `P-12345`.

## Entities

```bash
# List all entity types
dtmgd get entity-types

# Discover entities (--selector required, one entity type per query)
dtmgd get entities --selector 'type(SERVICE)'
dtmgd get entities --selector 'type(HOST),healthState("HEALTHY")'
dtmgd get entities --selector 'type(SERVICE),entityName.contains("payment")'
dtmgd get entities --selector 'type(SERVICE),tag("env:production")'

# Filter by management zone
dtmgd get entities --selector 'type(HOST)' --mz 'mzName("Production")'

# Get entity details (all properties, tags, management zones, relations)
dtmgd describe entity <entity-id> -o json

# Entity type schema
dtmgd describe entity-type SERVICE -o json

# Relationships to/from an entity
dtmgd describe entity-relations <entity-id> -o json
```

## Events

```bash
# List events (--from is required)
dtmgd get events --from now-1h --to now
dtmgd get events --from now-6h --type CUSTOM_DEPLOYMENT
dtmgd get events --from now-24h --entity 'entityId("SERVICE-123ABC")'

# Event detail
dtmgd describe event <event-id> -o json
```

## Metrics

```bash
# Search available metric IDs
dtmgd get metrics --search response.time
dtmgd get metrics --search cpu --entity 'type(HOST)'

# Get metric descriptor/metadata
dtmgd describe metric builtin:service.response.time -o json

# Query time-series data (--metric and --from are required)
dtmgd query metrics --metric builtin:service.response.time --from now-1h --to now
dtmgd query metrics --metric builtin:host.cpu.usage --from now-24h --resolution 1h
dtmgd query metrics \
  --metric builtin:host.mem.usage \
  --from now-6h \
  --entity 'type(HOST),entityName.contains("web")' \
  --resolution 30m
```

## Logs

```bash
# Search logs (--query and --from are required)
# Use plain text — do NOT use "content:error" structured syntax
dtmgd query logs --query "error" --from now-1h --to now
dtmgd query logs --query "timeout" --from now-30m --limit 50
dtmgd query logs --query "OutOfMemoryError" --from now-6h --sort -timestamp
```

## SLOs

```bash
# List SLOs
dtmgd get slos
dtmgd get slos --enabled false      # disabled SLOs
dtmgd get slos --evaluate           # include current compliance %

# SLO detail with error budget
dtmgd describe slo <slo-id>
dtmgd describe slo <slo-id> --timeframe CURRENT
dtmgd describe slo <slo-id> --from now-2w --to now --timeframe GTF
```

## Security Problems (CVE Vulnerabilities)

```bash
# List vulnerabilities
dtmgd get security-problems
dtmgd get security-problems --risk CRITICAL
dtmgd get security-problems --status OPEN --limit 50
dtmgd get security-problems --entity 'type(SERVICE),entityName.equals("frontend")'

# Full CVE detail
dtmgd describe security-problem <security-problem-id> -o json
```

## Typical Investigation Workflow

```bash
# 1. Check connectivity (all environments)
dtmgd get environments

# 2. Find open problems (optionally across all environments)
dtmgd get problems --status OPEN -o json
dtmgd get problems --status OPEN --env ALL_ENVIRONMENTS -o json

# 3. Get details on a specific problem (copy UUID from step 2 PROBLEM-ID column)
dtmgd describe problem <uuid> -o json

# 4. Find affected entities
dtmgd get entities --selector 'type(SERVICE),entityName.contains("affected-service")'

# 5. Check metrics on the entity
dtmgd query metrics \
  --metric builtin:service.response.time \
  --from now-2h \
  --entity 'entityId("SERVICE-XXXX")' \
  --resolution 5m

# 6. Search related logs
dtmgd query logs --query "exception" --from now-2h --to now --limit 100
```

## Time Format Reference

| Format   | Example                    |
|----------|----------------------------|
| Relative | `now-1h`, `now-24h`, `now-7d`, `now-30m` |
| ISO 8601 | `2024-01-01T10:00:00Z`     |
| Unix ms  | `1640995200000`            |

## Gotchas

- `describe problem` requires the **UUID** (`PROBLEM-ID` column), not the display ID (`P-XXXXX`).
- `get events` requires `--from`; it won't default like `get problems` does.
- `query logs` uses plain text search only — no `content:` prefix or structured syntax.
- `query metrics` defaults to table/text summary; add `-o json` to get raw time-series data.
- Entity selectors must specify **exactly one** entity type per query.
- Log search on Managed clusters does not support structured query syntax.
- `--limit` caps results to a single page. Without it, all pages are fetched automatically.
- Multi-env results are keyed by context name: `{"prod": {...}, "staging": {...}}`.
- Agent mode is auto-detected; use `--no-agent` to get plain output in AI environments.

## Feature Parity with Dynatrace Managed MCP Server

| MCP Tool | dtmgd command |
|---|---|
| `get_environments_info` | `dtmgd get environments` |
| `list_available_metrics` | `dtmgd get metrics [--search <text>]` |
| `get_metric_details` | `dtmgd describe metric <id>` |
| `query_metrics_data` | `dtmgd query metrics --metric <id> --from <t> --to <t>` |
| `query_logs` | `dtmgd query logs --query <text> --from <t> --to <t>` |
| `list_events` | `dtmgd get events --from <t>` |
| `get_event_details` | `dtmgd describe event <id>` |
| `list_entity_types` | `dtmgd get entity-types` |
| `get_entity_type_details` | `dtmgd describe entity-type <type>` |
| `discover_entities` | `dtmgd get entities --selector <sel>` |
| `get_entity_details` | `dtmgd describe entity <id>` |
| `get_entity_relationships` | `dtmgd describe entity-relations <id>` |
| `list_problems` | `dtmgd get problems` |
| `get_problem_details` | `dtmgd describe problem <uuid>` |
| `list_security_problems` | `dtmgd get security-problems` |
| `get_security_problem_details` | `dtmgd describe security-problem <id>` |
| `list_slos` | `dtmgd get slos` |
| `get_slo_details` | `dtmgd describe slo <id>` |
