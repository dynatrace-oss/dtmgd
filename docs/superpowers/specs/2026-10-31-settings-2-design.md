# Settings 2.0: reading and writing — design

Date: 2026-10-31. Status: agreed in conversation with the maintainer;
awaiting their review of this document.

## Why

dtmgd serves thousands of Dynatrace Managed customers, mostly through their
AI agents. Their needs span the whole of Settings 2.0, so dtmgd must
eventually reach every schema. It cannot read settings today: the catalog
has no settings command and no code calls `/settings`.

The maintainer decided that dtmgd grows past its read-only model, and that
Settings 2.0 is the first thing it writes. Customers decide what dtmgd may
change through two controls: the scopes of the tokens they create, and a
per-context write level that starts at `readonly`.

The work starts with the update settings customers ask about most, in this
order (the maintainer's):

1. environment update windows
2. OneAgent auto-update
3. Environment ActiveGate auto-update
4. cluster-level update windows
5. cluster ActiveGate auto-update

## Goals

- Read any Settings 2.0 schema, object and effective value in an
  environment, and later in the cluster.
- Write any Settings 2.0 object (create, update, delete) in an environment,
  and later in the cluster, where the context's write level allows it.
- Show readable tables, not raw JSON, for the update-window, OneAgent
  update and ActiveGate update schemas.
- Read and write cluster ActiveGate auto-update through the Cluster API's
  public endpoints.
- Match dtctl: the same verbs, the same apply file and the same per-context
  write levels, with the gaps listed under "Deliberate differences from
  dtctl" fixed.
- Keep the skill, README, AGENTS.md, `docs/` and the generated reference
  accurate in every phase, including what each token scope allows.

## Non-goals

- **Anything without a public API.** The maintainer: keep only what the
  public APIs already offer, and plan the rest when it is published. That
  excludes, for now:
  - the cluster's own automatic-update maintenance window (CMC → Settings →
    Automatic update). Only the CMC's internal UI endpoint changes it, and
    it needs a logged-in user, not a token;
  - the Settings 2.0 schema for cluster ActiveGate updates
    (`builtin:deployment.cluster.activegate.updates`). It is still in
    development and hidden from the public Cluster settings API.
- Writes outside Settings 2.0 and cluster ActiveGate auto-update: closing
  problems, tagging entities and the like.
- The older endpoints that Settings 2.0 replaces: Configuration API v1
  OneAgent auto-update (`/api/config/v1/hosts/autoupdate`, …) and the
  environment API's `/api/v2/activeGates/autoUpdate`.
- Settings history (Cluster API only), object permissions and ownership
  transfer, and the `adminAccess` parameter.
- Templating (dtctl's `apply --set`) and a `diff` verb. Both can come later
  (my call, see "Calls I made").
- Any change to the Dynatrace Managed MCP server (another repository).

## Anti-goals

Each of these is a failure even if everything else works:

- An upgrade that makes an existing context writable.
- A dry run or validate-only run that writes anything.
- A write that is sent twice because the client retried it.
- A batch where some objects fail and the result does not say so.
- A token, or a secret setting value, in output, logs or `-vv` dumps.
- A live test that writes anywhere but the `stage` environment, or touches
  anything but objects it created.

## Constraints

- **Public APIs only** (see Non-goals).
- **Agent output is the contract.** Every new command returns the agent
  envelope in agent mode, like every command except `commands` and
  `completion`. stdout is data only.
- **Documentation travels with each phase:** SKILL.md and its references,
  README, AGENTS.md, `docs/`, and `make docs` for `docs/reference`. Check the
  skill against `dtmgd commands --brief -o json` (commands) and
  `docs/reference` (flags).
- **Live calls need the maintainer's go-ahead each time.** Reads go to any
  of their contexts; writes go only to `stage`, their development
  environment.
- **Repository rules stay:** no real host names or environment IDs in
  repository files, tokens never printed, tests never use the default config
  path, golangci-lint `gocognit` limit 15, coverage gate 60%.

## Facts the design rests on

From the OpenAPI specs in `docs/managed/_data/openapi/` and the cluster
server source at Managed 1.352.

**Endpoints.** The environment API (`{host}/e/{env}/api/v2/settings/…`) and
the Cluster API (`{host}/api/cluster/v2/settings/…`) offer the same
operations: list and read schemas; list, read, create (`POST`, an array),
update (`PUT`) and delete objects; and read effective values.

**Listing objects.** The first page needs `schemaIds` or `scopes`. Later
pages send only `nextPageKey`; any other parameter, `pageSize` included,
gets 400. Page size defaults to 100, at most 500. `fields` defaults to
`objectId,value`; the server also offers `schemaId`, `scope`, `summary`,
`externalId`, `updateToken`, `schemaVersion`, `created`, `modified`,
`author` and others.

**Effective values** return what applies at a scope after inheritance,
falling back to the schema default. The `scope` parameter is required, and
it takes `environment` or an entity ID (`HOST-…`, `HOST_GROUP-…`,
`ENVIRONMENT_ACTIVE_GATE-…`). `origin` is the scope the winning value is
stored at. The response has no `objectId` and no `updateToken`.

**Writes.**
- `validateOnly=true` checks a create or update on the server without
  saving it. A pass returns `{"code":200}`; a failure returns 400 with
  `constraintViolations` (`path`, `message`).
- `PUT` has accepted `validateOnly` only since **Managed 1.288**. An older
  cluster ignores the unknown parameter and **saves the change**. `DELETE`
  has no `validateOnly`.
- Conflict protection is the `updateToken`: a field in the `PUT` body, a
  query parameter on `DELETE`. A stale token gets **409**. Without one, the
  last write wins. Managed ignores `If-Match`.
- `POST` answers per object (`code`, `objectId`, `error`, `invalidValue`).
  The overall status is 207 when codes differ. The batch is **not atomic**:
  valid objects are saved even when others fail.
- An `externalId` (up to 500 characters, unique per schema and scope) makes
  `POST` update the object that already has it, instead of creating one.
- Ordered schemas take `insertAfter` (`POST`, `PUT`) or `insertBefore`
  (`PUT`). Unordered schemas reject both with 400.

**Errors.**
- An unknown schema gets 404 "Configuration schema X does not exist."
- A scope the schema doesn't allow also gets 404.
- A missing token scope gets 403.

**The update schemas** (environment API; `tenant` is what the API calls the
`environment` scope):

| Schema | Objects | Scopes | Key fields |
|---|---|---|---|
| `builtin:deployment.management.update-windows` | many (≤100), unordered; `name` unique | environment | `enabled`, `name`, `recurrence` (`ONCE`/`DAILY`/`WEEKLY`/`MONTHLY`) and the matching block: `updateTime` {`startTime` "HH:mm", `timeZone` `GMT-12:00`…`GMT+12:00` in whole hours, `duration` 1–1440 min}, `recurrenceRange` {`start`, `end`} |
| `builtin:deployment.oneagent.updates` | one per scope | environment, `HOST_GROUP`, `HOST` | `updateMode` `AUTOMATIC`/`AUTOMATIC_DURING_MW`/`MANUAL`; `targetVersion` (`latest`/`previous`/`older`/`major.minor`); `revision`; `maintenanceWindows` = references to update windows |
| `builtin:deployment.activegate.updates` | one per scope | environment, `ENVIRONMENT_ACTIVE_GATE` | `updateMode` `AUTOMATIC`/`AUTOMATIC_DURING_UW`/`MANUAL`; `targetVersion`; `updateWindows` = references to update windows |

Some details of these schemas matter to the commands:
- OneAgent's mode is `…_MW` and ActiveGate's is `…_UW`.
- A window reference holds the update window's `objectId`.
- An update window must send both `recurrenceRange.start` and `end`: the
  defaults (1970) fail validation.
- Inheritance runs host → host group → environment → schema default. There
  is no "inherit" value; deleting the object at a scope makes that scope
  inherit again.
- **Deleting an update window that update settings still reference
  succeeds by default.** The server's check is behind a feature flag that
  is off, so the references are left dangling.

**The cluster side.**
- `/api/cluster/v2/settings` serves the GA cluster-level schemas. Among
  them is `builtin:deployment.management.update-windows`; the cluster's
  scope reads `cluster` in responses, and `cluster`, `environment` and
  `tenant` are accepted in requests.
- The OneAgent and environment ActiveGate update schemas are
  environment-only.
- Cluster ActiveGate auto-update has its own public endpoints:
  - `GET`/`PUT /api/cluster/v2/activeGates/autoUpdate`, global:
    `globalSetting` `ENABLED`/`DISABLED`, `targetVersion`, `updateWindows`
    `{windows:[{id,name}]}`;
  - `GET`/`PUT /activeGates/{agId}/autoUpdate`, per ActiveGate: `setting`
    `ENABLED`/`DISABLED`/`INHERITED`, a read-only `effectiveSetting`,
    `targetVersion`, `updateWindows`;
  - a `POST …/validator` next to each, which answers 204 when the body is
    valid.
- `/activeGates/autoUpdate/{envId}` is deprecated (1.340). It writes the
  environment's `builtin:deployment.activegate.updates`, which dtmgd
  reaches through the environment API instead.

**Token scopes.**

| Token | Read | Write |
|---|---|---|
| Environment, Settings | `settings.read` | `settings.write`. The ActiveGate updates schema also accepts `ActiveGatesRead`/`ActiveGatesWrite` |
| Cluster, Settings | `settings.read`; `ServiceProviderAPI` alone is not enough | `settings.write`; creating (`POST`) also needs `ServiceProviderAPI` |
| Cluster, ActiveGate auto-update | `ServiceProviderAPI` | `ServiceProviderAPI` |

**dtmgd today.**
- The HTTP client retries on 429, on 5xx, and on connection resets and
  unexpected EOF. For a write, those last cases can mean the server already
  received the request.
- The live suite's `run` accepts only read verbs (`readVerbs` in
  `test/integration/harness_test.go`).
- A local `.dtmgd.yaml`, when found, is used instead of the global config.

## The design

### Commands

Names follow dtctl.

| Command | What it does |
|---|---|
| `get settings-schemas [schema-id]` (aliases `schema`, `schemas`) | lists schemas, or one schema |
| `describe settings-schema <schema-id>` | a schema's version, scopes, multi-object/ordered flags and properties (type, enum values, default), in readable form |
| `get settings --schema <id> [--scope <scope>]`, `get settings --scope <scope>`, `get settings <object-id>` | stored objects. Lists need `--schema` or `--scope`, as the API does |
| `get settings --effective --scope <scope> [--schema <id>]` | effective values at one scope, with their `origin` (my addition: dtctl has none) |
| `describe settings <object-id>` | one object in full |
| `apply -f <file\|->` | creates or updates the objects in a file |
| `create settings -f <value-file> --schema <id> --scope <scope>` | creates one object from a bare value, as in dtctl. An `externalId` is set through `apply` only |
| `edit setting <object-id>` | opens the object's value in an editor, then updates it. For people: refused in agent mode, and when stdin isn't a terminal, with a hint to use `apply` |
| `delete settings <object-id>` | deletes one object |

**Flags on the write commands:**
- `--dry-run` sends only GETs and reports what would happen. It works at
  every write level.
- `--validate-only` asks the server to validate without saving. It applies
  to `apply`, `create` and `edit`, not to `delete`, which the API cannot
  validate.
- `delete` takes `-y/--yes`.

**`apply`'s file** is dtctl's envelope: `{objectId?, externalId?, schemaId,
scope, value, updateToken?}`. It can be YAML or JSON, and it can hold one
object, an array, or a list response with `items`.
- `describe settings <id> -o yaml` and `get settings -o yaml` print the
  API's own objects. dtmgd asks for `objectId`, `schemaId`, `scope`,
  `externalId`, `summary`, `updateToken` and `schemaVersion`, so exported
  objects apply back unchanged.
- Read-only fields (`summary`, `created`, …) are ignored on apply.

**How `apply` decides, per object.** dtmgd first finds out whether the object
exists, and that answer, not the HTTP method, decides the action. The
action then drives everything else: the write-level check, the dry run,
`unchanged` detection and the reported result.

The server can turn a `POST` into an overwrite in three cases:
- a single-object schema;
- a matching `externalId`;
- a schema whose key property (declared in the schema) matches an existing
  object.

So dtmgd never lets a `POST` overwrite without having looked first.

1. **With `objectId`:** GET it.
   - If it exists, the action is `update`: PUT the new value with an
     `updateToken`. Use the file's token if it has one (this catches
     changes since the export), otherwise the one from the GET.
   - If the GET returns 404, the action is `create`, as in dtctl. That
     moves a file between environments; the new `objectId` is reported.
2. **Without `objectId`:** list the schema's objects at that scope and
   match:
   - in a single-object schema (OneAgent and ActiveGate updates), the
     object at that scope;
   - otherwise the object with the same `externalId`;
   - otherwise the object with the same key property, if the schema
     declares one.

   A match makes the action `update`, sent as a PUT with the match's
   `objectId` and `updateToken`. No match makes it `create`, a POST.
   - The listing is client-side, not the `externalIds` filter. That filter
     is missing from the published environment spec, so an older cluster
     might ignore it.
   - The skill recommends `externalId` for repeatable applies of
     multi-object schemas.
3. **When the new value equals the current one,** nothing is sent and the
   action is `unchanged` (my addition).

**Readable tables** (`-o table` and `wide`; json and yaml stay the API's
own shape):
- **Update windows:** name, enabled, recurrence and schedule in one phrase
  (e.g. `weekly Mon 03:00 GMT+00:00, 60 min`), object ID.
- **OneAgent updates:** scope, update mode, target version, revision,
  window names (resolved from their object IDs), object ID.
- **ActiveGate updates:** the same without revision.
- **Effective values of these schemas:** the same columns plus `origin`.
- **Any other schema:** object ID, scope, summary, and the schema when
  several are listed.

The builder chooses the exact columns and wording.

**Token scopes per command** (`withScopes`): `settings.read` on the read
commands and on `--dry-run`; `settings.read` and `settings.write` on writes.
The README scope list and its test follow.

### Write levels

- **Field:** each context gains `safety-level` (json and yaml key), with
  dtctl's values: `readonly`, `readwrite-mine`, `readwrite-all`,
  `dangerously-unrestricted`.
- **Default `readonly`:** an empty or missing field means `readonly`. Every
  existing config therefore stays read-only after an upgrade. This is
  deliberately unlike dtctl, whose default is `readwrite-all`; the
  maintainer agreed.
- **An unknown value** is an error naming the four valid ones, not a
  silent `readonly`.
- **Setting a level:** `config set-context <name> --safety-level <level>`,
  and the same flag on `ctx set`. Agents may run it. The maintainer: token
  scopes are the customer's control, and no extra mechanism should stop
  agents.
- **Showing the level:**
  - context listings (`ctx`, `config get-contexts`) and `config view` show
    it, in a new column and key;
  - `doctor` reports each context's level.
- **What each level allows:**

  | Level | Create | Update | Delete |
  |---|---|---|---|
  | `readonly` | — | — | — |
  | `readwrite-mine` | ✓ | — | — |
  | `readwrite-all` | ✓ | ✓ | ✓ |
  | `dangerously-unrestricted` | ✓ | ✓ | ✓ |

  dtmgd cannot tell who owns a settings object, so `readwrite-mine` blocks
  updates and deletes, as dtctl does. `dangerously-unrestricted` allows
  nothing more than `readwrite-all` today; it exists to keep dtctl's
  levels.
- **Where the check runs:** before any request, in one place that every
  write command goes through (dtctl's `checkSafety` pattern, with a test
  that bans calling the checker directly).
  - `apply` checks each object against the action it resolved to.
  - `--dry-run` skips the check, because it sends only GETs.
  - `--validate-only` counts as a read once the version gate below has
    passed.
- **A blocked write:**
  - exits **6** (new) without sending anything;
  - names the context, its level and the level needed;
  - gives the `config set-context … --safety-level …` command as the hint;
  - in agent mode, the error code is `safety_blocked`, as in dtctl.
- **No cap on local configs.** The level comes from whichever config is in
  use: global, a local `.dtmgd.yaml`, or `--config`.
  - dtctl caps a local file at the global level for the same token.
    dtmgd does not, because the cap would protect little: a local
    `.dtmgd.yaml` can already pair a keyring credential with any host. That
    risk is accepted and disclosed by the warning for files found in a
    parent directory.
  - Capping would also mean a project-local config, the setup
    `config init` creates, or a CI machine without a global config, could
    never write.
  - The maintainer wants no mechanism beyond the token's scopes.
- **The docs say plainly** that the write level is a client-side guard, not
  a security boundary. The boundary is the token's scopes: a customer who
  wants dtmgd unable to write creates tokens without `settings.write`.

### Write safety in the client and commands

- **No write is ever sent twice.**
  - `POST`, `PUT` and `DELETE` retry only when the request cannot have
    reached the server: a connect or TLS-handshake failure before sending,
    or 429.
  - They never retry on 5xx, connection reset or unexpected EOF.
  - GETs keep today's retries.
- **The validate-only version gate:** before sending `validateOnly` on a
  `PUT`, dtmgd reads the cluster version.
  - Below 1.288 it refuses (exit 2), explaining that the cluster would save
    the change.
  - If the version cannot be read, it also refuses.
  - Reading the version needs `DataExport` on an environment context
    (`/api/v1/config/clusterversion`) and `ServiceProviderAPI` on a cluster
    context. `--validate-only` declares that scope too.
  - A refusal for a missing scope names the scope and suggests `--dry-run`,
    which needs only `settings.read`.
  - The builder confirms in the cluster source since when `POST` honours
    `validateOnly`, and gates it the same way if needed.
- **Update-window deletes check references first.**
  - Before deleting an update window, dtmgd lists the
    `builtin:deployment.oneagent.updates` and
    `builtin:deployment.activegate.updates` objects at every scope.
  - It refuses (exit 1) if any references the window, naming the
    referencing objects and their scopes.
  - There is no override: change those settings first.
  - On the cluster API, the same check runs against cluster ActiveGate
    auto-update.
- **Deletes confirm.**
  - A person at a terminal confirms with `[y/N]`, or passes `--yes`.
  - In agent mode, or without a terminal, `delete` requires `--yes` and
    otherwise exits 2 with a hint. That fixes dtctl, which skips the prompt
    silently there.
- **A conflict (409)** exits 1 with the hint: re-read the object and apply
  again.
- **Secrets stay out of output.**
  - `-vv` dumps of write requests mask values that the schema marks as
    secret. The builder may simply mask the whole `value` of write
    requests.
  - Error output prints `invalidValue` only as the server returns it,
    already masked.

### What a write returns

Every write command prints its result on stdout, as data, in every format.
In agent mode that is the envelope's `result`:

```json
{
  "dryRun": false,
  "validateOnly": false,
  "results": [
    {"context": "prod", "action": "updated", "objectId": "…",
     "schemaId": "…", "scope": "…", "summary": "…", "error": null}
  ]
}
```

- **`action` values:** `created`, `updated`, `unchanged`, `deleted`,
  `validated`, `failed`.
- **A dry run** adds the planned change per object: the current and the
  new value, or a diff. The builder picks the form.
- **People get a table** (ACTION, OBJECT-ID, SCHEMA, SCOPE, SUMMARY).
- **A batch with any `failed` object:**
  - exits non-zero;
  - lists each failure with the server's message and constraint
    violations;
  - in agent mode, the envelope says `ok: false` and still carries every
    result.

### Several environments in one write

The maintainer decided that writes fan out like reads.

- **Which commands:** `apply` and `create settings` take several
  environment contexts through `--env "a;b"` or `--env ALL_ENVIRONMENTS`.
  An example is setting OneAgent auto-update in every environment with one
  command.
- **Which don't:** `delete settings` and `edit setting` name one object ID,
  which exists in one environment only. Given several contexts, they exit 2.
- **Checks first, everywhere.** Before writing anywhere, dtmgd resolves
  every object's action in every context and runs that context's checks:
  the write level and, for `--validate-only`, the version gate.
  - If any context fails a check, nothing is written anywhere.
  - The command reports each context's outcome and exits with the code of
    the failed check.
  - A write blocked in one environment never leaves the others half
    changed.
- **Then the writes,** per context; running them in parallel is the
  builder's choice.
- **Server-side failures can still be partial.** They are reported per
  context and object, with today's partial-failure exit-code rules for
  multi-environment reads.
- **Every result carries its `context`,** including single-context runs,
  so the result shape is the same either way.
- **Cluster contexts never fan out.** `ALL_ENVIRONMENTS` already excludes
  them.

### Cluster contexts

Phase 3 adds two things.

**Settings on cluster contexts.** The same commands, routed to
`/api/cluster/v2/settings` when the context is a cluster context, through
`runClusterFetch` / `clusterCommand`.
- The scope is `cluster`.
- The update-window table is reused.
- Scopes: `settings.read` to read; `settings.write` to write; creates also
  need `ServiceProviderAPI`.

**Cluster ActiveGate auto-update** (my call on the command shape: dtctl has
no equivalent):
- `get activegate-updates [ag-id]`: the global configuration, followed by
  one row per cluster ActiveGate with its setting, effective setting,
  target version and window names. With an ID, one ActiveGate.
- `describe activegate-updates [ag-id]`.
- `update activegate-updates [ag-id] -f <file> [--dry-run]
  [--validate-only]`, and `edit activegate-updates [ag-id]`. `update` is
  dtctl's verb for resources that `apply` does not detect. Without an ID,
  these change the global configuration.
- `--validate-only` uses the `…/validator` endpoints.
- The write levels apply: changing the configuration counts as an update.
- Scope: `ServiceProviderAPI`.
- **Target version and update windows may be ignored by the cluster.**
  Clusters honour `targetVersion` and `updateWindows` on these endpoints
  only behind a server feature flag that is off by default. With the flag
  off:
  - `GET` returns only `globalSetting`, or `setting` and
    `effectiveSetting`;
  - `PUT` and the validator answer success but drop those two fields
    silently.

  So dtmgd shows those columns only when the `GET` returned them. It
  refuses (exit 2) a file that sets them when the cluster's `GET` did not
  return them, rather than report a change that never happened. The same
  `GET` decides whether the cluster update-window reference check has
  anything to check.

### Testing

- **Offline:** every command runs through `runCLI` against the fake API, and
  tests pin each request: method, path, query and body, as
  `cmd/filters_test.go` does for filters. The following get tests:
  - the write-level matrix;
  - the default `readonly`;
  - a `POST` that would overwrite an object, resolved as an `update`;
  - a multi-environment apply where one context is blocked, which sends no
    write at all;
  - the 1.288 gate;
  - no-retry on a 5xx `POST`;
  - the update-window reference check;
  - `--yes` in agent mode;
  - 409 handling;
  - batch partial failure;
  - `unchanged` detection;
  - the `-vv` masking.
- **Live, writing:**
  - A new setting, `DTMGD_IT_WRITE_ENV`, names the only context the suite
    may write to (`stage`). Write tests skip without it. Its own `run`
    helper refuses any other context.
  - **Writes go through dtmgd's write guard**, and that context's
    `safety-level` must be `readwrite-all`.
  - **Update windows:** create a disabled window named
    `dtmgd-it-<timestamp>`, read it back, update it, check its effective
    value, delete it.
    - Cleanup runs even when a test fails.
    - Each run first deletes leftover `dtmgd-it-` windows from crashed
      runs.
  - **OneAgent and ActiveGate updates:** only `--validate-only` and
    `--dry-run`, never a real write, because a real change could start
    updates.
- **Live, reading:** the read checks run on any configured context. The
  first cluster-level read (`get settings-schemas` on `prod-cluster`) also
  confirms which cluster schemas this cluster version offers.
- **Rules that don't change:** each live run needs the maintainer's
  go-ahead, and output stays filtered through `brief`.

### Documentation

- **README:**
  - the first line and the "absent on purpose" paragraph become "reads by
    default; writes Settings 2.0 where a context's write level allows it";
  - the verbs table and the scope list grow.
- **AGENTS.md:**
  - the "read-only" description becomes the new rule;
  - the rule for agents on live calls covers writes: only to `stage`, only
    with the go-ahead;
  - a section on adding a write command: the write guard, no retries, the
    envelope, the dry run.
- **`docs/configuration.md`:**
  - write levels;
  - the token scopes for environment and cluster tokens, from the table
    above.
- **The skill:**
  - SKILL.md gains a settings section;
  - a new `references/settings.md` covers reading, writing, the apply file,
    `externalId`, write levels, and the update schemas with examples;
  - the guidance says the token is the customer's control, and tells agents
    to dry-run before writing.
- **The knowledge base in `.claude/CLAUDE.md`** gets the server facts above
  that are easy to get wrong:
  - the 1.288 gate;
  - `updateToken`, not `If-Match`;
  - non-atomic batches;
  - dangling window references;
  - `_MW` versus `_UW`.

## Phases

Each phase ships on its own, with its docs and skill updates.

1. **Read settings in environments.**
   - `get settings-schemas`, `describe settings-schema`, `get settings`
     (with `--effective`), `describe settings`.
   - Readable tables for the three schemas.
   - Scopes and docs.
   - Live read checks.
2. **Write settings in environments.**
   - Write levels.
   - The client's write methods and no-retry rule.
   - `apply`, `create settings`, `edit setting`, `delete settings`, with
     dry run, validate-only and its version gate, the reference check, and
     the write result.
   - Multi-environment `apply` and `create`, with every context checked
     before any write.
   - The doc and skill rewrite of the read-only promise.
   - Live write tests on `stage`.
3. **The cluster.**
   - Settings on cluster contexts.
   - Cluster ActiveGate auto-update commands.
   - Cluster token-scope docs.
   - Live checks: reads on `prod-cluster`; writes only through
     `--validate-only` and `--dry-run` unless the maintainer names a safe
     target.

## Calls I made

The maintainer agreed the direction: match dtctl, `readonly` default,
reading first, public APIs only, live writes on `stage`, agents may raise
levels, and writes fan out across environments with every context checked
before any write. These finer calls are mine; each can be changed:

- **`delete` and `edit` stay single-context:** they name one object ID.

- **dtctl's gaps fixed:**
  - writes return results on stdout;
  - dry runs are marked as dry runs;
  - delete requires `--yes` in agent mode;
  - live write tests go through the write guard.
- **Additions dtctl lacks:**
  - effective values, as `get settings --effective`;
  - `unchanged` detection in `apply`.
- **Exit code 6** for a write the context's level blocks.
- **`--validate-only` counts as a read** for the write guard, behind the
  version gate.
- **The update-window reference check before delete,** with no override.
- **`apply` always looks before it writes:** by single-object scope,
  `externalId` or key property, listing client-side. The resolved action,
  not the HTTP method, drives the write level, the dry run and the result.
  A `POST` that would overwrite can't pass as a create under
  `readwrite-mine`.
- **No `--external-id` on `create settings`** (dtctl has none either):
  `externalId` goes through `apply`.
- **No cap on a local config's write level,** unlike dtctl (see Write
  levels).
- **`--validate-only` also needs the scope that reads the cluster version**
  (`DataExport` on environment contexts) for its 1.288 gate.
- **Cluster ActiveGate target version and windows:** shown and written only
  when the cluster's `GET` returns them.
- **Deferred:** `apply --set` templating, a `diff` verb, and `--filter` on
  `get settings`. The environment API honours `filter` only since 1.277,
  and an older cluster would silently ignore it.
- **Cluster ActiveGate auto-update** as `get`, `describe`, `update` and
  `edit activegate-updates`.

## Left to the builder

- Package layout, type and function names, and how the commands share code
  with `runFetch` / `runClusterFetch`.
- Exact table columns, schedule wording and how schema properties are
  rendered.
- The dry-run diff format.
- The editor fallback order for `edit` (dtctl: `$EDITOR`, then `vim`).
- Whether masking in `-vv` follows the schema's secret properties or
  covers the whole value.
- Whether `get settings` accepts several `--schema` values.
