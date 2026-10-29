---
name: dtmgd
description: Query and investigate Dynatrace Managed (self-hosted) environments from the CLI. Use this skill when the user asks about problems, incidents, entities, events, logs, metrics, SLOs, or security vulnerabilities in a Dynatrace Managed cluster — even if they don't mention dtmgd by name. Covers connectivity checks, entity topology, time-series metric queries, log search, and every tool of the Dynatrace Managed MCP Server.
---

# Dynatrace Managed CLI — dtmgd

`dtmgd` is a `kubectl`-inspired read-only CLI for **Dynatrace Managed (self-hosted)** clusters.
It covers every tool of the Dynatrace Managed MCP Server as shell commands; the differences are listed under "Feature Parity" below.

## Contexts — Multiple Environments Are User-Defined

`dtmgd` supports any number of named environments ("contexts"). The names —
`prod`, `staging`, `dev`, `eu-prod`, `customer-acme`, etc. — are **user-defined
aliases**, not hard-coded keywords. Throughout this document, `prod` and
`staging` appear only as illustrative examples; substitute whatever names exist
in the user's config.

Run `dtmgd ctx` (or `dtmgd config get-contexts`) to see the actual context
names configured on this system.

## Recommended Initialization

At the start of any investigation, run these to learn the commands and confirm connectivity:

```bash
# Discover what dtmgd can do: the resources of get, describe and query
dtmgd commands --brief

# Show active context (and all configured environments)
dtmgd ctx

# Verify connectivity, token and cluster version (checks every context; -c <name> for one)
dtmgd doctor

# Check version of the CLI itself
dtmgd version
```

Start with `dtmgd commands --brief` rather than parsing `--help`. For one
command's flags and the token scopes it needs, read the full catalog,
`dtmgd commands` (JSON, never wrapped in an agent envelope). Look up an entry by
its `path`, for example `"get problems"`.

Use `dtmgd doctor` for connectivity checks, not `dtmgd get environments`: that
role has moved to `doctor`, and `get environments` will list environments from
the Cluster API in a coming release. `doctor` gives one row per context with
`status` `ok`, `warn` (the cluster version is below 1.328.0, or the token cannot
read it) or `fail` (with the reason in `detail`), and exits 1 if any context
fails.

Agent mode is auto-detected — output will be wrapped in `{ok, result, context}` envelopes automatically.

## Output Formats

Use `-o json` for AI/script consumption. Default is human-friendly `table`.

```bash
-o table    # default; human-readable
-o wide     # table with extra columns (timestamps, root cause, etc.)
-o json     # indented JSON — best for parsing and piping to jq
-o yaml     # YAML output (-o yml also works)
```

Any other `-o` value is a usage error (exit 2) before any request is made.

- **stdout is data only.** Notes for a person ("No problems found", "Showing
  10 of 15 problems", warnings, errors, hints) go to stderr, so piping stdout to
  `jq` is safe.
- **json, yaml and the envelope's `result` are the API's own response**, with
  its camelCase keys (`problemId`, `totalCount`, `nextPageKey`), not the table
  columns. This holds for every `get`, `describe` and `query` command except
  `get environments` and `query log-counts` (see [Logs](#logs)).
- **An empty result prints the object with an empty list** (`"problems": []`),
  not nothing.
- **Tables**: `describe problem`, `describe slo` and `describe security-problem`
  print a summary; the other describe commands print the API object as YAML.
- `--plain` swaps stderr symbols for words (`Warning:`, `Error:`, `Hint:`). It
  is automatic when stdout is not a terminal or `NO_COLOR` is set (unless
  `FORCE_COLOR` is set).

### AI Agent Envelope Mode

dtmgd auto-detects AI agent environments (Claude Code, Cursor, GitHub Copilot,
Codex, Gemini CLI, Amazon Q, Kiro, Junie, OpenCode, and others) and wraps all
output in dtctl's structured envelope:

```json
{
  "ok": true,
  "result": { "problems": [...], "totalCount": 5 },
  "context": { "verb": "get", "resource": "problems", "total": 5 }
}
```

**In agent mode a list returns at most 50 items unless you pass `--limit`.**
`context.total` is how many exist (when the API reports it) and
`context.has_more` is `true` when `result` holds fewer — pass a larger
`--limit` or narrow the filters.

Errors are also wrapped, with a classified code and next steps:
`{"ok": false, "result": null, "error": {"code": "permission_denied", "message": "...", "status_code": 403, "suggestions": ["..."]}}`.
Codes: `auth_required`, `permission_denied`, `not_found`, `bad_request`,
`rate_limited`, `server_error`, `config_error`, `timeout`, `partial_failure`,
`validation_error` and `unknown_command` (usage errors), `error`.

Force with `-A`/`--agent`, disable with `--no-agent`.

When agent mode is auto-detected, dtmgd also prints one line to **stderr**
(`Agent mode is on (detected claude-code). …`). Don't merge stderr into
stdout (`2>&1`) before parsing JSON — or pass `-A` explicitly, which
suppresses the line.

To install this skill from an agent's shell (no terminal), name the agent:
`dtmgd skills install --for claude`.

**Tip for AI agents**: prefer `-o json` or `-A` when parsing results:

```bash
dtmgd get problems --status OPEN -o json
dtmgd describe problem <uuid> -A
```

## Global Flags

```
-c, --context <name>    override the current context (environment)
-e, --env <spec>        target environment(s): name, "prod;staging", or ALL_ENVIRONMENTS
-o, --output <format>   table | wide | json | yaml (yml)
-A, --agent             force agent envelope output
--no-agent              disable auto-detected agent mode
--plain                 words instead of symbols on stderr (automatic when stdout is not a terminal)
--max-pages <n>         max pages to fetch (0 = all). Pagination is automatic.
--columns <cols>        comma-separated columns to show in table output
-v                      verbose: show HTTP request/response summary
-vv                     extra verbose: full headers + body (auth redacted)
--config <file>         use a specific config file instead of the default
```

`--watch` / `--watch-interval` are not global: only `get problems` and
`get events` have them.

## Exit Codes

`0` success, `1` any other error, `2` usage error (unknown command, resource
or flag; missing argument or required flag), `3` authentication failed (401),
`4` not found (404), `5` permission denied (403). A multi-environment
(`--env`) call exits non-zero if any environment fails — the shared code when
all failed the same way, else `1` — and still prints the environments that
answered.

## Multi-Environment Queries

Query multiple environments in parallel with `--env`. Environment names are
**user-defined aliases** (any names the user chose during setup) — `prod` and
`staging` below are illustrative only.

```bash
# All configured environments
dtmgd get problems --env ALL_ENVIRONMENTS -o json

# Specific environments (semicolon-separated; substitute the user's actual context names)
dtmgd get problems --env "prod;staging" -o json

# One context (-c, or --env with one name) → unwrapped data
# ALL_ENVIRONMENTS or a ';' list → { "<context-name-1>": {...}, "<context-name-2>": {...} }
```

With `--env ALL_ENVIRONMENTS` or a `;` list, json/yaml/agent output is one map
keyed by context name, even when only one context is selected; a failed environment holds `{"error": "..."}` instead of its result. A
table prints a `== <name> ==` section per environment. Every environment that
answered is printed, and the exit code is non-zero if any failed.

Works with every `get`, `describe` and `query` command except
`get environments` (always lists every context). `config`, `ctx`, `skills`,
`doctor` and `commands` never fan out over `--env`; a single `--env` name acts
like `-c` where they show the current context. Use `dtmgd ctx` first to
discover which context names are available.

## Pagination

All list commands automatically follow `nextPageKey` to fetch complete results.
`--limit <n>` returns at most `n` items in every format and stops paging once
it has them; `--max-pages` limits page count. A truncated json/yaml result
keeps the API's `nextPageKey`. In agent mode, lists default to 50 items when
`--limit` is not given.

## Watch Mode

Monitor resources in real-time (`get problems` and `get events` only):

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

Contexts are named environments. **Names are arbitrary, user-defined aliases**
— `prod`, `staging`, `dev`, `acme-tenant1`, anything. The examples below use
`prod` purely as a placeholder.

```bash
# List all configured environments (shows the user's actual context names)
dtmgd ctx
dtmgd config get-contexts

# Show or switch context
dtmgd ctx current
dtmgd ctx <context-name>

# Configure a new environment (pick any name you like for <context-name>)
dtmgd config set-context <context-name> \
  --host https://managed.company.com \
  --env-id abc12345 \
  --token-ref <context-name>-token

# Store the token. Prompts for it with echo disabled; use --token-stdin when
# scripting. Avoid --token: it leaves the token in the process list and shell
# history.
dtmgd config set-credentials <context-name>-token
echo "$DT_API_TOKEN" | dtmgd config set-credentials <context-name>-token --token-stdin

# Create contexts from a Dynatrace Managed MCP server config (DT_CONFIG_FILE's
# YAML/JSON file, or DT_ENVIRONMENT_CONFIGS with --from-env). Each alias becomes
# a context; ${VAR} tokens stay references. Preview with --dry-run; existing
# contexts are skipped unless --overwrite.
dtmgd config import dt-config.yaml --dry-run
dtmgd config import dt-config.yaml

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
dtmgd get problems --impact SERVICES --limit 20
dtmgd get problems --entity 'type(SERVICE),entityName.contains("checkout")'

# Filter by management zone: use --selector with managementZones("Name") DSL
# Default timeframe is 24 h — use --from for a wider window
dtmgd get problems --selector 'managementZones("production")' --from now-7d --limit 5 --sort "-startTime"
dtmgd get problems --status OPEN --selector 'managementZones("production")' --from now-7d

# Combine status + impact + management zone (all merged into problemSelector DSL)
dtmgd get problems --status OPEN --impact SERVICES --selector 'managementZones("production")' --from now-7d

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
  --entity 'type(SERVICE),tag("[Environment]Production")' \
  --from now-1h --resolution Inf

# Service performance summary for production services (requests/min + failures/min)
# Step 1: total requests over last 1h per service
dtmgd query metrics \
  --metric 'builtin:service.requestCount.server:splitBy("dt.entity.service"):sum:names' \
  --entity 'type(SERVICE),tag("[Environment]Production")' \
  --from now-1h --resolution Inf

# Step 2: total errors over last 1h per service
dtmgd query metrics \
  --metric 'builtin:service.errors.server.count:splitBy("dt.entity.service"):sum:names' \
  --entity 'type(SERVICE),tag("[Environment]Production")' \
  --from now-1h --resolution Inf

# Step 3: average response time per service
dtmgd query metrics \
  --metric 'builtin:service.response.time:splitBy("dt.entity.service"):avg:names' \
  --entity 'type(SERVICE),tag("[Environment]Production")' \
  --from now-1h --resolution Inf

# Divide by 60 to convert 1h totals to per-minute rates.
# Key service metrics (builtin:service.*):
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
# NOTE: --entity scopes log search but requires PROCESS_GROUP selector (not SERVICE).
# On DT Managed Classic, logs are attributed to process groups, not services.
dtmgd query logs --query "error" --from now-1h --entity 'type(PROCESS_GROUP),tag("[Environment]Production")'

# Aggregate log counts by service and log level (INFO/WARN/ERROR)
# This uses /api/v2/logs/aggregate with full-text level matching (Spring Boot log format)
# --entity is required; --from defaults to now-1h
dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]Production")' --from now-1h
dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]Production")' --from now-30m --to now

# IMPORTANT: On DT Managed Classic, structured field queries (e.g. loglevel:ERROR) are NOT
# supported in LQL. Log level counts use full-text matching ("INFO", "WARN", "ERROR") which
# is accurate for standard Spring Boot/Java logs where the level appears in each line.
# "WARN" counts may under-count if some frameworks use "WARNING" instead.
```

`query log-counts` output is computed by dtmgd, not an API response. json,
yaml and agent output is
`{"logCounts": [{"service": …, "info": …, "warn": …, "error": …, "total": …}]}`,
one entry per process group display name. A count is `null` (`-` in the table)
when that process group fell outside the API's top-N aggregation window for
the level (`--max-services`, at most 100): it is unknown, not 0.

## SLOs

```bash
# List SLOs
dtmgd get slos
dtmgd get slos --enabled false      # disabled SLOs
dtmgd get slos --evaluate           # include current evaluated % (auto-pages in batches of 25)

# Filter SLOs by management zone, name, or text (sloSelector DSL)
# Supported predicates: name("..."), text("..."), managementZone("..."), managementZoneID("..."),
#   entityIDs("..."), healthState("HEALTHY"|"UNHEALTHY")
dtmgd get slos --selector 'managementZone("production")' --evaluate
dtmgd get slos --selector 'text("OrderController")' --evaluate
dtmgd get slos --selector 'healthState("UNHEALTHY")' --evaluate

# SLO status: SUCCESS (green) = at/above warning threshold
#             WARNING (yellow) = at/above target but below warning threshold
#             FAILURE (red)   = below target
#             NONE            = not evaluated

# SLO detail with error budget
dtmgd describe slo <slo-id>
dtmgd describe slo <slo-id> --timeframe CURRENT
dtmgd describe slo <slo-id> --from now-2w --to now   # --from/--to imply --timeframe GTF
```

## Security Problems (CVE Vulnerabilities)

```bash
# List vulnerabilities (always sorted by risk score descending, includes RISK/SCORE columns)
dtmgd get security-problems
dtmgd get security-problems --risk CRITICAL
dtmgd get security-problems --status OPEN --limit 50

# Filter by management zone: use --selector with managementZones("Name") DSL
# --status and --risk are automatically merged into the selector
dtmgd get security-problems --status OPEN --selector 'managementZones("production")'
dtmgd get security-problems --status OPEN --selector 'managementZones("production")' --limit 5

# --status, --risk, and --selector are composable — combined with AND logic:
# e.g. CRITICAL open vulnerabilities in the production management zone:
dtmgd get security-problems --status OPEN --risk CRITICAL --selector 'managementZones("production")'

# Full CVE detail (use the display ID, e.g. S-42)
dtmgd describe security-problem S-42 --env prod -o json
```

## Typical Investigation Workflow

```bash
# 1. Check connectivity (all environments)
dtmgd doctor

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
- `query log-counts` internally converts `type(SERVICE)` entity selectors to `type(PROCESS_GROUP)` because DT Managed Classic attributes logs to process groups, not services. Services that log at ERROR level only (e.g. a production logging profile) will show 0 INFO and 0 WARN — this is correct behavior, not a bug.
- `query logs --entity` also requires `type(PROCESS_GROUP)` on DT Managed Classic (not `type(SERVICE)`). Using a SERVICE selector returns 0 results even though the API accepts the parameter.
- The logs aggregate endpoint's `entitySelector` param is `hidden=true` on DT Managed Classic and does not actually filter results. `query log-counts` works around this by fetching entities first, then filtering aggregate results client-side.
- Entity selectors must specify **exactly one** entity type per query.
- Log search on Managed clusters does not support structured query syntax.
- `--limit` applies to every output format and stops paging once met. Without it, all pages are fetched automatically — except in agent mode, where lists default to 50 items (`context.has_more` tells you there are more).
- Multi-env results are keyed by context name: `{"prod": {...}, "staging": {"error": "..."}}` — a failed environment holds `error`, and the call exits non-zero.
- Agent mode is auto-detected; use `--no-agent` to get human-readable output in AI environments.

## Feature Parity with Dynatrace Managed MCP Server

dtmgd covers every tool of the [Dynatrace Managed MCP Server](https://github.com/dynatrace-oss/dynatrace-managed-mcp)
(compared with its 2.0.0 release, which prefixes every tool name with
`dynatrace_managed_`; the table leaves the prefix out). The two send mostly the
same requests; the table lists where they differ. "Undocumented" means the parameter is not in the
endpoint's published API specification; whether a cluster honours it has not
been verified.

| MCP tool | dtmgd command | Differences |
|---|---|---|
| `get_environments_info` | `dtmgd doctor` | `doctor` checks each context live (token, cluster version ≥ 1.328.0, falling back to a metrics call) and exits 1 on failure. The MCP server reports the check it ran at startup and a fixed scope list. |
| `list_available_metrics` | `dtmgd get metrics [--search] [--entity]` | Both send `--entity` as `entitySelector`, which `/metrics` does not document. |
| `get_metric_details` | `dtmgd describe metric <id>` | Same request. |
| `query_metrics_data` | `dtmgd query metrics --metric <sel> --from <t> [--to] [--resolution] [--entity]` | The MCP server defaults to `resolution=Inf` (one value per series) and requires `to`. dtmgd sends no resolution unless `--resolution` is given (the API default: a time series), and `--to` defaults to now. |
| `query_logs` | `dtmgd query logs --query <text> --from <t> [--to] [--sort] [--limit] [--entity]` | Both sort newest first (`-timestamp`) and return one slice of at most 1000 records (100 by default; 50 for dtmgd in agent mode). dtmgd clamps a larger `--limit` to 1000 where the MCP server rejects it, and `--to` defaults to now. dtmgd's `--entity` sends `entitySelector`, which `/logs/search` does not document. |
| `list_events` | `dtmgd get events --from <t> [--to] [--type] [--entity] [--watch]` | Both send `--type` as `eventType`, which `/events` does not document (it documents `eventSelector`). dtmgd adds `--watch`. |
| `get_event_details` | `dtmgd describe event <id>` | Same request. |
| `list_entity_types` | `dtmgd get entity-types` | dtmgd's json output has the full type schemas; the MCP server lists type names. |
| `get_entity_type_details` | `dtmgd describe entity-type <type>` | Same request. |
| `discover_entities` | `dtmgd get entities --selector <sel> [--mz] [--from] [--to] [--sort]` | The MCP server returns 10 entities unless asked for more (at most 50). Both send `--mz` as `mzSelector`, which `/entities` does not document. |
| `get_entity_details` | `dtmgd describe entity <id>` | dtmgd asks for properties, relationships, tags and management zones explicitly (`fields`); the MCP server relies on the API's default fields. |
| `get_entity_relationships` | `dtmgd describe entity-relations <id>` | dtmgd asks for the relationship fields explicitly. |
| `list_problems` | `dtmgd get problems [--status] [--impact] [--selector] [--entity] [--sort] [--watch]` | The MCP server sends the impact level as a top-level `impactLevel`, which `/problems` does not document; dtmgd puts it in `problemSelector`. dtmgd adds `--selector` and `--watch`, and sends `fields=+affectedEntities,+managementZones`, which `/problems` does not document. Both default to the last 24 hours. |
| `get_problem_details` | `dtmgd describe problem <id>` | Same request. A negative problem ID goes after `--`. |
| `list_security_problems` | `dtmgd get security-problems [--risk] [--status] [--selector] [--from] [--to] [--sort]` | The MCP server sends `riskLevel` and `entitySelector` as top-level parameters, which `/securityProblems` does not document. dtmgd puts the risk level in `securityProblemSelector`, and scopes to entities with selector criteria (`--selector 'relatedHostIds("HOST-…")'`). dtmgd requests the risk assessment and sorts by risk score by default. |
| `get_security_problem_details` | `dtmgd describe security-problem <id \| S-NNN>` | dtmgd also accepts the display ID (`S-1234`), and requests the details fields: risk assessment, CVE description, vulnerable components, code-level details, affected and exposed entities. The MCP server requests none. |
| `list_slos` | `dtmgd get slos [--selector] [--timeframe] [--from] [--to] [--evaluate] [--sort] [--enabled] [--show-global] [--demo]` | The MCP server never sends `showGlobalSlos=false` or `demo=false`; dtmgd sends either flag whenever it is given. dtmgd turns `--from`/`--to` into `timeFrame=GTF`, without which the API ignores them, and asks for 25 SLOs per page with `--evaluate`, the API's limit. |
| `get_slo_details` | `dtmgd describe slo <id> [--timeframe] [--from] [--to]` | Same request, including the GTF rule. |

Differences across all tools:

- **Result counts.** The MCP server returns one page per call, capped per tool
  (for example 50 problems, 100 security problems). dtmgd fetches every page
  unless `--limit` or `--max-pages` stops it; in agent mode lists default to 50,
  and `context.has_more` says when there are more.
- **Several environments.** Both accept `"a;b"` and `ALL_ENVIRONMENTS`. The MCP
  server queries the environments one after another and fails the whole call
  if one fails. dtmgd queries them in parallel, prints every environment that
  answered (keyed by context name), and exits non-zero if any failed.
- **UI links.** The MCP server adds a Dynatrace UI link when one environment
  answered. dtmgd puts the links in agent output's `context.links`, keyed by
  context name for several environments, and `describe` commands print
  `Open in Dynatrace:` on stderr.
- **Only in the MCP server:** HTTP transport with per-request tokens, rate
  limiting, usage telemetry, and output trimmed for model context (at most 11
  tags or dimensions, 50 data points per series).
- **Only in dtmgd:** `query log-counts` (log counts per process group and
  level), the `--selector` DSL on problems and security problems, `--watch`,
  display IDs for security problems, paging past the first page (with
  `--max-pages` to stop it), table, wide, json and yaml output, `commands`,
  `config import` (creates contexts from an MCP server config), and shell
  completion.
