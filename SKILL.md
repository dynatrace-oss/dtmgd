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

# Filter by management zone: use --selector with managementZones("Name") DSL
# Default timeframe is 24 h — use --from for a wider window
dtmgd get problems --selector 'managementZones("bookstore")' --from now-7d --limit 5 --sort "-startTime"
dtmgd get problems --status OPEN --selector 'managementZones("bookstore")' --from now-7d

# Combine status + impact + management zone (all merged into problemSelector DSL)
dtmgd get problems --status OPEN --impact SERVICE --selector 'managementZones("bookstore")' --from now-7d

# Sort: open first, or newest first
dtmgd get problems --sort "+status"
dtmgd get problems --sort "-startTime"

# Full problem details (use UUID from PROBLEM-ID column, NOT P-XXXXX display ID)
dtmgd describe problem <uuid> -o json
```

> **Important**: `describe problem` requires the internal UUID (e.g. `8a3f1b2c-...`),
> **not** the short display ID like `P-12345`. Some UUIDs are negative integers — pass
> them after `--` to avoid flag-parsing errors:
> ```bash
> dtmgd describe problem -- -6546711275898328738_1776193140000V2
> ```

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

# Per-entity summary table (use --resolution Inf + :splitBy + :names for compact output)
# Shows one row per entity sorted by value desc, with resolved entity names.
dtmgd query metrics \
  --metric 'builtin:service.requestCount.server:splitBy("dt.entity.service"):sum:names' \
  --entity 'type(SERVICE),tag("[Environment]BookStore")' \
  --from now-1h --resolution Inf

# Service performance summary for BookStore (requests/min + failures/min)
# Step 1: total requests over last 1h per service
dtmgd query metrics \
  --metric 'builtin:service.requestCount.server:splitBy("dt.entity.service"):sum:names' \
  --entity 'type(SERVICE),tag("[Environment]BookStore")' \
  --from now-1h --resolution Inf

# Step 2: total errors over last 1h per service
dtmgd query metrics \
  --metric 'builtin:service.errors.server.count:splitBy("dt.entity.service"):sum:names' \
  --entity 'type(SERVICE),tag("[Environment]BookStore")' \
  --from now-1h --resolution Inf

# Step 3: average response time per service
dtmgd query metrics \
  --metric 'builtin:service.response.time:splitBy("dt.entity.service"):avg:names' \
  --entity 'type(SERVICE),tag("[Environment]BookStore")' \
  --from now-1h --resolution Inf

# Divide by 60 to convert 1h totals to per-minute rates.
# Key BookStore service metrics (builtin:service.*):
#   builtin:service.requestCount.server          — request count (server-side)
#   builtin:service.errors.server.count          — server error count
#   builtin:service.errors.server.rate           — server error rate (%)
#   builtin:service.response.time                — average response time (µs)
#   builtin:service.errors.fivexx.count          — HTTP 5xx error count
#   builtin:service.errors.fourxx.count          — HTTP 4xx error count
```

## Logs

```bash
# Search logs (--query and --from are required)
# Use plain text — do NOT use "content:error" or "status:ERROR" structured syntax
# Use --entity to scope to specific services
dtmgd query logs --query "error" --from now-1h --to now
dtmgd query logs --query "timeout" --from now-30m --limit 50
dtmgd query logs --query "OutOfMemoryError" --from now-6h --sort -timestamp
dtmgd query logs --query "error" --from now-1h --entity 'type(SERVICE),tag("[Environment]BookStore")'

# Aggregate log counts by service and log level (INFO/WARN/ERROR)
# This uses /api/v2/logs/aggregate with full-text level matching (Spring Boot log format)
# --entity is required; --from defaults to now-1h
dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]BookStore")' --from now-1h
dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]BookStore")' --from now-30m --to now

# IMPORTANT: On DT Managed Classic, structured field queries (e.g. loglevel:ERROR) are NOT
# supported in LQL. Log level counts use full-text matching ("INFO", "WARN", "ERROR") which
# is accurate for standard Spring Boot/Java logs where the level appears in each line.
# "WARN" counts may under-count if some frameworks use "WARNING" instead.
```

## SLOs

```bash
# List SLOs
dtmgd get slos
dtmgd get slos --enabled false      # disabled SLOs
dtmgd get slos --evaluate           # include current evaluated % (auto-pages in batches of 25)

# Filter SLOs by management zone, name, or text (sloSelector DSL)
# Supported predicates: name("..."), text("..."), managementZone("..."), managementZoneID("..."),
#   entityIDs("..."), healthState("HEALTHY"|"UNHEALTHY")
dtmgd get slos --selector 'managementZone("bookstore")' --evaluate
dtmgd get slos --selector 'text("OrderController")' --evaluate
dtmgd get slos --selector 'healthState("UNHEALTHY")' --evaluate

# SLO status: SUCCESS (green) = at/above warning threshold
#             WARNING (yellow) = at/above target but below warning threshold
#             FAILURE (red)   = below target
#             NONE            = not evaluated

# SLO detail with error budget
dtmgd describe slo <slo-id>
dtmgd describe slo <slo-id> --timeframe CURRENT
dtmgd describe slo <slo-id> --from now-2w --to now --timeframe GTF
```

## Security Problems (CVE Vulnerabilities)

```bash
# List vulnerabilities (always sorted by risk score descending, includes RISK/SCORE columns)
dtmgd get security-problems
dtmgd get security-problems --risk CRITICAL
dtmgd get security-problems --status OPEN --limit 50

# Filter by management zone: use --selector with managementZones("Name") DSL
# --status and --risk are automatically merged into the selector
dtmgd get security-problems --status OPEN --selector 'managementZones("bookstore")'
dtmgd get security-problems --status OPEN --selector 'managementZones("bookstore")' --limit 5

# --status, --risk, and --selector are composable — combined with AND logic:
# e.g. CRITICAL open vulnerabilities in the bookstore management zone:
dtmgd get security-problems --status OPEN --risk CRITICAL --selector 'managementZones("bookstore")'

# Full CVE detail (use the display ID, e.g. S-42)
dtmgd describe security-problem S-42 --env prod -o json
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
- Problem UUIDs can be **negative integers** (e.g. `-6546711275898328738_1776193140000V2`). Always pass them after `--` to prevent the leading `-` being parsed as a flag: `dtmgd describe problem -- -6546711275898328738_V2`
- `get events` requires `--from`; it won't default like `get problems` does.
- `query logs` uses plain text search only — no `content:`, `status:`, or `loglevel:` structured syntax (LQL structured queries are unsupported on DT Managed Classic).
- `query log-counts` counts log levels using full-text matching; accurate for Spring Boot/Java logs, may under-count WARN if framework uses "WARNING".
- `query log-counts` internally converts `type(SERVICE)` entity selectors to `type(PROCESS_GROUP)` because DT Managed Classic attributes logs to process groups, not services. Services that log at ERROR-only level (e.g., BookStore prod profile) will show 0 INFO and 0 WARN — this is correct behavior, not a bug.
- The logs aggregate endpoint's `entitySelector` param is `hidden=true` on DT Managed Classic and does not actually filter results. `query log-counts` works around this by fetching entities first, then filtering aggregate results client-side.
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
| `query_logs` | `dtmgd query logs --query <text> --from <t> --to <t> [--entity <sel>]` |
| `aggregate_logs` | `dtmgd query log-counts --entity <sel> --from <t> --to <t>` |
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
