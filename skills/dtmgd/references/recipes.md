# dtmgd recipes: problems, entities, events, metrics, logs, SLOs, security

Read the section for the data you are investigating: the commands, their filters and the API quirks that matter.

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
