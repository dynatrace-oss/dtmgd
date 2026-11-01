---
name: dtmgd
description: Query and investigate Dynatrace Managed (self-hosted) environments from the CLI, and read or change their configuration through Settings 2.0, for any schema (alerting profiles, problem notifications, process-availability rules, update windows, OneAgent and ActiveGate auto-update, and the rest). Use this skill when the user asks about problems, incidents, entities, events, logs, metrics, SLOs, or security vulnerabilities in a Dynatrace Managed cluster, or how something is configured there or should be changed — even if they don't mention dtmgd by name. Covers connectivity checks, entity topology, time-series metric queries, log search, every tool of the Dynatrace Managed MCP Server, and the cluster's environments, nodes, license and backup configuration through the Cluster Management API.
---

# Dynatrace Managed CLI — dtmgd

`dtmgd` is a `kubectl`-inspired CLI for **Dynatrace Managed (self-hosted)** clusters. It reads; it writes only Settings 2.0 objects, through contexts whose write level allows it.
It covers every tool of the Dynatrace Managed MCP Server as shell commands; the differences are in [references/mcp-parity.md](references/mcp-parity.md).

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
# Discover what dtmgd can do: the resources of get, describe, query, create, edit, update and delete
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

Use `dtmgd doctor` for connectivity checks. (`dtmgd get environments` lists a
cluster's environments and needs a cluster context; up to v0.2 it was the
connectivity check.) `doctor` gives one row per context with `scope`
(`environment` or `cluster`), `status` `ok`, `warn` (the cluster version is
below 1.328.0, or the token cannot read it) or `fail` (with the reason in
`detail`), and exits 1 if any context fails.

Agent mode is auto-detected — output will be wrapped in `{ok, result, context}` envelopes automatically, for every command except `commands` (its catalog is JSON already) and `completion` (a shell script). Commands that print one line for a person (`version`, `config current-context`, `ctx current`) put an object in `result`, such as `{"current-context": "prod"}`.

## Reading Output

- Parse with `-o json`, or rely on the agent envelope. **stdout is data only**:
  outside agent mode, notes, warnings, errors and hints go to stderr. In agent
  mode everything is in the envelope on stdout (errors as `ok: false`,
  warnings in `context.warnings`), and stderr carries only notices such as the
  agent-mode line. Either way, don't merge stderr into stdout (`2>&1`) before
  parsing.
- json, yaml and the envelope's `result` are **the API's own response**, with
  its camelCase keys (`problemId`, `totalCount`, `nextPageKey`), not the table
  columns. An empty result is the object with an empty list.
- The envelope is `{"ok": true, "result": {...}, "context": {"verb": "get", "resource": "problems", "total": 5}}`.
  **In agent mode a list returns at most 50 items unless you pass `--limit`**;
  `context.has_more` is `true` when there are more.
- Errors are `{"ok": false, "error": {"code": "...", "message": "...", "suggestions": [...]}}`,
  with codes such as `auth_required`, `permission_denied`, `not_found`,
  `validation_error` and `partial_failure`.
- Force the envelope with `-A`, turn it off with `--no-agent`.

Formats, every error code, paging, `--watch`, `--columns` and time formats:
[references/output.md](references/output.md).

## Global Flags

```
-c, --context <name>    override the current context (environment)
-e, --env <spec>        target environment(s): name, "prod;staging", or ALL_ENVIRONMENTS
-o, --output <format>   table | wide | json | yaml (yml) | csv | tsv
-A, --agent             force agent envelope output
--no-agent              disable auto-detected agent mode
--plain                 no colors, words instead of symbols on stderr (automatic when stdout is not a terminal)
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
`4` not found (404), `5` permission denied (403), `6` a write the context's safety-level does not allow (nothing was sent). A multi-environment
(`--env`) call exits non-zero if any environment fails — the shared code when
all failed the same way, else `1` — and still prints the environments that
answered.

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

## Settings 2.0

dtmgd reads and writes **any** Settings 2.0 schema, in environments and on cluster contexts. For any configuration request:

```bash
dtmgd get settings-schemas --search alerting                # 1. find the schema
dtmgd describe settings-schema builtin:alerting.profile     # 2. scopes, ordered, properties, defaults, references, secrets, maturity
dtmgd get settings --schema builtin:processavailability --scope environment                    # 3. what is stored,
dtmgd get settings --effective --scope HOST-<id> --schema builtin:deployment.oneagent.updates   #    or what applies
dtmgd describe settings <object-id> -o yaml > window.yaml   # 4. export and edit it, or write it from the schema
dtmgd apply -f window.yaml --dry-run                        # 5. ALWAYS first: GETs only, what would change
dtmgd apply -f window.yaml --validate-only                  #    the cluster checks it and saves nothing
dtmgd apply -f window.yaml                                  #    the write
```

- `get settings` needs `--schema` or `--scope`; `--effective` adds inherited values and defaults (`ORIGIN`). Token scopes: `settings.read`, and `settings.write` to write.
- Every context starts `readonly`: a blocked write exits `6` (`safety_blocked`) and sends nothing. `--dry-run` and `--validate-only` work at any level.
- Give a new object of a multi-object schema an `externalId`, so a repeated apply updates it.
- Ask the user before writing a schema whose `Maturity` is `IN_DEVELOPMENT` or `EARLY_ADOPTER`.
- Ordered schemas evaluate their list in order. Place an object with `position:` `first`, `last`, `after:<ref>` or `before:<ref>` in the file, or `create settings --position`.
- A reference (`setting → <schema>`) holds an `objectId`. `delete settings <object-id> --yes` (`--yes` is required in agent mode) refuses an object that other settings use.
- Reads show a secret as the cluster's mask (`***…***`), write reports as `<secret>`. The mask keeps the secret on update but cannot create an object (exit 2): put the real value in the file.
- Cluster contexts use the scopes `cluster`, `environment-default` and a few more; the cluster ActiveGates' auto-update is `get`/`update activegate-updates` there.

Details and worked examples (an alerting profile with a webhook notification, a rule put first): [references/settings.md](references/settings.md).

## Gotchas

- `describe problem` requires the **UUID** (`PROBLEM-ID` column), not the display ID (`P-XXXXX`).
- Problem UUIDs can be **negative integers** (e.g. `-6546711275898328738_1776193140000V2`). `describe problem` inserts the `--` that keeps the leading `-` from being read as a flag, so `dtmgd describe problem -6546711275898328738_V2` works; an explicit `dtmgd describe problem -- -6546711275898328738_V2` works too.
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
- `get environments` lists a cluster's environments and needs a cluster context. For "which environments can I query", use `dtmgd ctx` (configured contexts) or `dtmgd doctor` (and whether each answers).

## References

Read the one that matches the task:

- [references/recipes.md](references/recipes.md): problems, entities, events,
  metrics, logs, SLOs and security problems — commands, filters and API quirks.
- [references/output.md](references/output.md): output formats, the agent
  envelope and its error codes, pagination, watch mode, column filtering, time
  formats.
- [references/contexts.md](references/contexts.md): managing contexts,
  importing an MCP server config, multi-environment (`--env`) queries.
- [references/cluster-api.md](references/cluster-api.md): a cluster's
  environments, nodes, license, license usage, backup, its own settings, and
  its ActiveGates' auto-update (cluster contexts).
- [references/settings.md](references/settings.md): Settings 2.0 for any
  schema — the workflow with worked examples, stored versus effective values,
  scopes, ordered lists and `position`, references and the delete check,
  secrets, refusals, maturity, and the update settings.
- [references/mcp-parity.md](references/mcp-parity.md): which dtmgd command
  matches each Dynatrace Managed MCP Server tool, and how they differ.
