# Settings 2.0 for any schema: closing the gaps — design

Date: 2026-11-01. Status: agreed in conversation with the maintainer;
awaiting their review of this document.

## Why

dtmgd v0.5.0 reads and writes any Settings 2.0 schema, in environments and
on clusters (design: `2026-10-31-settings-2-design.md`). Only the update
settings got readable tables, a delete guard and a cheat sheet in the skill,
and the documentation reads as if they were all dtmgd supports: the
maintainer believed exactly that until asked.

The maintainer's goal: an AI agent can take any request a Dynatrace Managed
customer makes about configuration ("alert this team's webhook", "put this
log rule first", "turn this off on that host"), find the schema through the
schema API, learn it, and read or change the setting safely.

On 2026-11-01 the general path was tested live on the maintainer's `stage`
environment and `stage-cluster` with schemas other than the update ones. It
works, and it found three gaps that matter for "configure anything" plus a
few smaller faults. This design closes them, rewrites the documentation
around any schema, and keeps the live tests so they can be repeated after a
cluster upgrade.

## Goals

1. **Secrets are never printed, and copying one is never silent.** Write
   reports show `<secret>` instead of a secret's value. A create whose
   secret properties hold a masked export value (`***<hash>***`) is refused
   before anything is sent, naming the property.
2. **Position in ordered lists.** Creates and updates of objects in
   ordered schemas can say where the object goes: first, last, or before or
   after another object. That object can be named by `objectId` or
   `externalId`, so one file works in several environments.
3. **Settings that other settings use are not deleted.** `delete settings`
   refuses to delete an object that another settings object still
   references, for every schema, not only update windows.
4. **The smaller faults are fixed:**
   - On a cluster context, `get settings --scope <scope>` without
     `--schema` works.
   - A cluster 404 that carries the API's own message no longer adds the
     wrong-host hint.
   - A 403 "Setting not applicable" no longer blames token scopes.
   - Human tables show summaries without the cluster's markdown escaping.
   - `describe settings-schema` shows each schema's maturity and key
     property, and the schema each reference points at.
5. **The documentation is about any schema.** README, SKILL.md,
   `references/settings.md` and `references/cluster-api.md` explain the
   workflow for any request, every scope kind, ordered lists, references,
   secrets and maturity. The update settings become one example among
   several.
6. **The live tests stay.** The checks made by hand on 2026-11-01 go into
   `test/integration`, with the new behaviour.

## Non-goals

- **Settings change history** (`/settings/history`). It answers on Managed
  1.352, but it is not in the Managed API documentation (`env2temp.json`),
  and the maintainer's rule is public APIs only.
- **Per-object settings permissions** (`/settings/objects/{id}/permissions`).
  These endpoints accept OAuth only and reject the `Api-Token` scheme dtmgd
  uses.
- **The Configuration API (v1).** Configuration that exists only there stays
  out of reach. The documentation says so.
- **References from outside Settings 2.0.** Dashboards, user-group
  permissions and similar can use a management zone too. The delete check
  sees settings references only, and the documentation says so.
- **Value templates.** Nothing generates a value skeleton from a schema.
  Agents read the defaults and allowed values from
  `describe settings-schema`.
- **A position column in tables.** The list order already is the
  evaluation order, and the documentation says so.
- **Writing `IN_DEVELOPMENT` schemas differently.** dtmgd shows the maturity
  and the skill tells agents to ask first. Nothing is blocked.

## Anti-goals

- **A guard that blocks legitimate work without a way forward.** Every
  refusal names what blocks it and the next step.
- **Different behaviour on the two APIs.** Every change works on
  environment and cluster contexts alike, unless the API itself differs.
- **Changing what json, yaml or the agent envelope return for reads.** They
  stay the API's own response. The fixes to escaping and hints touch human
  output and hints only.
- **Touching existing settings in live tests.** Live writes create, change
  and delete only `dtmgd-it-…` objects the test made. Existing objects get
  `--dry-run` and `--validate-only` only.

## Constraints

- **Public APIs only.**
- **Write levels and token scopes stay the only permission controls.** No
  new write path bypasses `runWrites`, its level checks or its never-resend
  rule.
- **Agent output is the contract.** New refusals are errors with hints,
  which agent mode turns into `suggestions`. New result fields appear in
  json and yaml as well.
- **Documentation travels with the change:** SKILL.md and its references,
  README, AGENTS.md, the knowledge base in `.claude/CLAUDE.md`, and
  `make docs`. After the release, the installed skill copies are refreshed.
- **Live calls:** reads may go to all four contexts (`prod`, `stage`,
  `prod-cluster`, `stage-cluster`). Writes go only to `stage` and
  `stage-cluster`, through a temporary config copy, and only to `dtmgd-it-…`
  objects (the maintainer's permission of 2026-11-01).
- **Repository rules stay:**
  - no real host names or environment IDs in repository files;
  - tokens are never printed;
  - tests never use the default config path;
  - `gocognit` stays at 15;
  - the coverage gate stays at 60%.

## Facts the design rests on

All observed live on 2026-11-01: `stage` runs Managed 1.352, and
`stage-cluster` is its cluster. The API semantics come from `env2temp.json`.

**Coverage.**
- `stage` has 235 schemas and 1,103 objects at environment scope, in 49
  schemas.
- A dry run applying back up to 5 exported objects of each of those 49
  schemas (116 objects) planned `unchanged` for every one.
- Creating, re-applying (`unchanged`), updating and deleting worked for:
  - alerting profiles;
  - teams;
  - a disabled webhook problem notification with a secret header;
  - disabled process-availability rules, at environment and host-group
    scope;
  - an unused network zone on `stage-cluster`.
- A failed `--validate-only` reports the server's own text, for example
  `maxProcesses: Value must be between 1 and 100`.

**Secrets.**
- 8 schemas on `stage` have secret properties (type `secret`).
- A GET returns them as `***` + 16 lowercase hex characters + `***`, for
  example `***3f9ff26c722fbb93***`.
- The hash is salted: storing the same secret again gives a new hash.
- **Update:** applying an export back with the mask kept the stored secret.
  The hash stayed the same, where a newly stored value would have changed
  it.
- **Create:** creating a new object from an export holding the mask
  succeeded, both validate-only and real. The cluster cannot know the real
  value, so the copy holds the mask, not the secret.
- A file holding the real secret always plans `update`, because the stored
  value cannot be compared.

**Ordered schemas.**
- 46 schemas on `stage` are `ordered: true`, among them log storage,
  service detection, process grouping and failure detection rules.
- Without a position, a create goes last and an update keeps its place.
- The API's position fields:

  | Request | Field | Value | Effect |
  |---|---|---|---|
  | POST | `insertAfter` | `null` | last |
  | POST | `insertAfter` | `""` | first |
  | POST | `insertAfter` | object ID | after that object |
  | PUT | `insertAfter` | `""` | first |
  | PUT | `insertBefore` | `""` | last |
  | PUT | `insertAfter` / `insertBefore` | object ID | after / before that object |

  A PUT takes at most one of `insertAfter` and `insertBefore`; with neither
  set, the object keeps its place.
- These fields apply only to ordered schemas.

**References.**
- A schema declares a reference as a property of type `setting` with
  `referencedType: <schemaId>`, also nested inside its types.
- 13 schemas on `stage` declare references:
  - most of them to `builtin:management-zones`;
  - `builtin:problem.notifications` → `builtin:alerting.profile`;
  - OneAgent and ActiveGate updates → update windows.
- The cluster deleted an alerting profile that two notifications still
  used, and left them pointing at nothing.
- A management-zone reference is stored as the zone's settings `objectId`.
  An alerting profile created with the zone's numeric ID (the one entity
  details show) and one created with its `objectId` both stored the
  `objectId`. Neither `stage` nor `prod` has any setting that references a
  zone today; the check was made with two `dtmgd-it-…` profiles, deleted
  afterwards.
- Reading every schema definition takes about 5–10 s on `stage`: the list,
  then 235 definitions fetched 8 at a time. A sample of 30 took 1.7 s.
  Every listed schema's definition could be read (235 of 235 on `stage`, 19
  of 19 on `stage-cluster`).

**Cluster contexts.**
- `GET /api/cluster/v2/settings/objects?scopes=cluster`, with no
  `schemaIds`, fails with 404 "No schema with schemaId
  builtin:accounting.ddu.limit", a cluster bug. The same scope with
  `schemaIds` works.
- `stage-cluster` lists 19 schemas. Besides `cluster`, they use the scopes
  `environment-default` (defaults for every environment; 19 objects),
  `realm`, `metric` and `CLUSTER_ACTIVE_GATE`.
- `builtin:networkzones.zones` lists `cluster` among its scopes, but the
  cluster answers 404 "No schema with topic identifier" there. Its objects
  are stored at `environment-default`.
- dtmgd appends its "No Cluster API answered at this host" hint to these
  404s, although the host is right.
- A wrong host answers differently. Asked for four Cluster API paths, the
  node addresses of `stage` and `prod` (port 9999) answer 404 with a JSON
  body that has no `error` envelope, so dtmgd reads no API message. A real
  Cluster API answers a missing environment with the generic message "HTTP
  404 Not Found", and a missing schema or object with a specific one ("No
  schema with …", "Settings objectId '…' not found").

**Errors and maturity.**
- For a setting that does not apply to an entity, the cluster answers 403
  "Setting not applicable: '…'". dtmgd adds "Permission denied. Your API
  token may be missing required scopes." and exits 5.
- Summaries come escaped as markdown (`dtmgd\-it\-team`), and dtmgd's tables
  show the backslashes.
- Maturity on `stage`: 219 `GENERAL_AVAILABILITY`, 13 `EARLY_ADOPTER`, 3
  `OPT_IN_GENERAL_AVAILABILITY`. On `stage-cluster`: 18
  `GENERAL_AVAILABILITY` and one `IN_DEVELOPMENT`
  (`builtin:deployment.cluster.activegate.updates`, whose effective value
  reads normally).

**Out of reach.**
- `/settings/history` answers 200 on `stage`, but it is not in the Managed
  API documentation.
- The settings permission endpoints reject `Api-Token` with 401.

## The design

### Secrets: never print them, and refuse a masked value in a create

- **Where the secrets are.** `settingsSchemaShape` (read once per schema by
  the planners) also records where the schema's secret properties are. A
  walk over a value, guided by the definition, finds them:
  - a property of type `secret`;
  - inside a property whose type is `{"$ref": "#/types/X"}`, the
    properties of type X;
  - in a `list` or `set` property, each item, by the item type.

  The same walk serves the two rules below.
- **Reports never print a secret.** Every write report replaces the value
  of each secret property with `<secret>`, in `value` and `current`, in
  every output format. That covers a dry run, the agent envelope and a
  failed write, and matters because a file with the real secret is planned
  as an update every time. The cluster's own mask in `current` is replaced
  the same way. Only what dtmgd prints changes; the request sent to the
  cluster carries the file's value.
- **A masked value in a create is refused.** For a schema with secrets, a
  planned **create** refuses before anything is sent when a secret
  property's value matches `^\*\*\*[0-9a-f]{16}\*\*\*$`, the cluster's mask.
  This covers:
  - `create settings`;
  - `apply` creating an object;
  - `apply` with an `objectId` the environment does not have, which becomes
    a create.

  The error is a usage error (exit 2). It names the JSON path of each match
  (for example `webHookNotification.headers[0].secretValue`), with the hint
  "Exports mask secrets. Put the real value in the file; dtmgd never prints
  it." Because the refusal happens while planning, `--dry-run` shows it too.
- **Updates are not checked:** the cluster keeps the stored secret.
  `references/settings.md` explains the one surprise: applying a file to an
  object matched by `externalId` in another environment keeps *that*
  object's secret.

### Ordered lists: `position`

- An apply-file object may carry `position`, and `create settings` gets the
  same flag `--position`. The value is one of:
  - `first`;
  - `last`;
  - `after:<ref>`;
  - `before:<ref>`.

  `<ref>` is an `objectId` or an `externalId` of an object of the same
  schema and scope, looked up in each context's listing (`objectId` first),
  or the `externalId` of an object earlier in the same file. `position` is
  not part of `value` and is never sent as such.
- `position` on a schema that is not ordered is a usage error. Any other
  value, a `<ref>` that resolves to nothing, or a `<ref>` naming the object
  itself also fail the plan, with a hint to list the objects
  (`get settings --schema <id> --scope <scope>`).
- **How it maps to the API.** dtmgd works out what to send from the
  listing it already reads for lookups. The list order is the evaluation
  order.

  | `position` | Create (POST) sends | Update (PUT) sends |
  |---|---|---|
  | `first` | `insertAfter: ""` | `insertAfter: ""` |
  | `last` | no position field | `insertBefore: ""` |
  | `after:X` | `insertAfter: X` | `insertAfter: X` |
  | `before:X` | `insertAfter: <X's predecessor>`, or `""` if X is first | `insertBefore: X` |
- **Planning:**
  - An update with a value unchanged and the object already in that place
    is `unchanged`.
  - A different place makes it an `update`, even with an unchanged value
    (a move).
  - Without `position`, nothing changes from today.
  - The plan shows `position: {current: N, new: M}` (1-based places in the
    scope's list; `current` is absent for a create) in json, yaml and the
    dry-run report. A move whose value is unchanged is still reported as
    `update`.
- **Anchors earlier in the same file:**
  - Within one context the writes go in file order. An anchor that names an
    earlier object of the file by `externalId` is resolved after that
    object's write, from its returned `objectId`.
  - If that write failed, this one fails with "its position anchor was not
    written", and nothing is sent for it.
  - Under `--validate-only` the anchor does not exist, so that write is
    validated without its position, and its result says so.
- An exported listing (`get settings -o yaml`) applied to an environment
  that has none of its objects is created in file order. Its relative order
  is kept with no `position` at all. The documentation says so.

### References: refuse to delete what other settings use

- **Planning a `delete settings` in one context:**
  1. Read the object, as today.
  2. List the schemas, then read every definition, 8 at a time, once per
     context per run.
  3. Collect each schema that declares a `setting` property whose
     `referencedType` is the object's schema.
  4. If any schema does, list its objects at every scope (`schemaIds=…`,
     paged, without `scopes`). An object references the deleted one when
     any string in its value equals the deleted object's ID.
- **When references exist:**
  - The plan fails with exit 1, listing up to 10 referencing objects
    (schema, scope, object ID, summary) and the total count.
  - The hint: "Change or delete those settings first. Settings only:
    dashboards and other configuration outside Settings 2.0 are not
    checked."
  - `--dry-run` shows the refusal.
  - There is no override flag.
- **Update windows:**
  - In environments, this check replaces `referencesWindow`, the special
    case for OneAgent and ActiveGate update windows: both schemas declare
    the reference.
  - The cluster's check of ActiveGate auto-update (`clusterWindowReferences`,
    not a settings object) stays as it is.
- **Errors:**
  - A 403 while reading definitions or referencing objects fails the
    delete and names `settings.read`.
  - A schema that the list names but whose definition answers 404 is
    skipped, with a warning naming it: it disappeared, or the cluster cannot
    serve it, and then its objects cannot be listed either. On `stage` and
    `stage-cluster` every listed definition was readable.
  - Any other failure (a transport error, a 5xx after the client's retries)
    fails the delete. A delete is never sent with a referencing schema left
    unchecked.

### Cluster contexts

- **Listing by scope alone.** On a cluster context, a scope-only
  `get settings` (no `--schema`) goes out as today. If the cluster answers
  404 with a message starting "No schema with schemaId", dtmgd falls back:
  1. List the cluster's schemas.
  2. Ask each with the scope (`schemaIds=<one>&scopes=<scope>`), 8 at a
     time, skipping those that answer 404.
  3. Merge the items in schema ID order.

  `--limit`, truncation notes and `--effective` behave as for any listing.
  Environment contexts are unchanged.
- **The wrong-host hint** (`hintNoClusterAPI`) is added only to 404s
  whose API message is empty (a wrong host's answer) or the generic "HTTP
  404 Not Found". Any other message, such as "No schema with …" or
  "Settings objectId … not found", means the Cluster API answered, and the
  hint is left out. The fake API's default 404 body (`{"error": {"code":
  404, "message": "not found"}}` in `cmd/harness_test.go`) carries a
  message, so the host-hint tests switch to a body without an `error`
  envelope, as a wrong host sends.

### Smaller fixes

- **403 "Setting not applicable".** When a 403's API message starts with
  `Setting not applicable`, the hint is "The cluster says this setting does
  not apply to this scope (its reason is above). Check the scopes in
  `dtmgd describe settings-schema <id>`, or read the setting at another
  scope." It replaces the token-scope hint. The exit code stays 5 (every 403
  is 5).
- **Summaries.** Human tables (`get settings`, `--effective`, and the write
  report's SUMMARY) unescape the cluster's markdown: a backslash before an
  ASCII punctuation character is dropped. json, yaml and agent output keep
  the raw summary.
- **`describe settings-schema`** (human output) shows:
  - `Maturity: <value>`;
  - `Key property: <name>`, when the schema has one;
  - `setting → <referencedType>` in the TYPE column for references.

  Its json and yaml stay the raw definition.

### Documentation

- **SKILL.md, the Settings 2.0 section,** leads with the workflow for any
  request:
  1. Find the schema: `get settings-schemas --search` (ID and display name).
  2. Read its definition: `describe settings-schema` (scopes, objects,
     ordered, properties, defaults, allowed values, references, secrets,
     maturity).
  3. Read what is stored or effective.
  4. Write a file, from an export or from the defaults.
  5. `--dry-run`, then `--validate-only`, then `apply`.

  Its examples mix schemas: an alerting profile, a process-availability
  rule, the update settings. Maturity: before writing an `IN_DEVELOPMENT`
  or `EARLY_ADOPTER` schema, ask the user.
- **`references/settings.md`** is restructured:
  - the workflow, with two worked examples:
    - an alerting profile plus a webhook notification (secret, reference);
    - a rule put first in an ordered list;
  - scopes: whatever `allowedScopes` lists, meaning environment, entity
    scopes, and on clusters `cluster`, `environment-default`, `realm` and
    `metric`;
  - ordered lists and `position`;
  - references and the delete check;
  - secrets (what round-trips, what is refused, why a file with a real
    secret always plans an update);
  - "when the cluster says no": `Setting not applicable`, validation
    messages;
  - maturity;
  - the update settings, kept as one section;
  - what dtmgd cannot reach: Configuration API v1, history, permissions.
- **`references/cluster-api.md`:**
  - the `environment-default` and `realm` scopes;
  - network zones live at `environment-default`;
  - scope-only listing.
- **README:** the settings paragraph says "any schema" with a non-update
  example, and the command table shows `--position`.
- **The knowledge base in `.claude/CLAUDE.md`** gets one entry with the
  facts above, among them: the mask format and salted hash, create-from-mask
  saves the mask, the insert semantics, `referencedType`, zone references
  stored as the zone's `objectId`, the cluster scope-only bug, what a wrong
  host's 404 looks like, network zones at `environment-default`, `Setting
  not applicable`, history not public, permissions OAuth-only.
- **`make docs`.**

### Testing

- **Unit tests** go through the harness (`runCLI`, the fake API):
  - the masked-create refusal for each entry point (`create settings`,
    `apply` creating, `apply` with an `objectId` the environment does not
    have);
  - masked updates passing;
  - secrets replaced by `<secret>` in reports: a dry run, the agent
    envelope and a failed write, for a top-level secret property, one
    inside a type, and one inside a list item;
  - the request body still carrying the real value;
  - each `position` value on create and update: the request bodies, through
    `fakeAPI.Bodies`;
  - an unchanged place being `unchanged`, and a move with an unchanged value;
  - anchors by `objectId`, by `externalId`, and to an earlier object of the
    file, including that object failing and `--validate-only`;
  - `position` refused on an unordered schema;
  - the reference refusal listing referencing objects, the 403 path, a
    definition that answers 404 (skipped, with a warning), and any other
    failure (the delete fails);
  - update-window deletes still refused, through the generic check;
  - the cluster scope-only fallback and the hint rules;
  - the summary unescaping;
  - the describe lines.

  Golden files change where human output changes; read every diff.
- **Live tests** go in `test/integration`, build tag `integration`, using
  `writeRun` and the temporary config. The write tests run only with
  `DTMGD_IT_WRITE_ENV` / `DTMGD_IT_WRITE_CLUSTER`.
  - **Alerting profile and webhook notification** on the write environment,
    both `dtmgd-it-…`, the notification disabled, its URL on
    `example.invalid`:
    1. Create both; re-apply them (`unchanged`).
    2. Update the profile.
    3. Apply the notification's export changed: its mask is unchanged
       afterwards.
    4. A copy created from the export is refused (exit 2).
    5. Deleting the profile is refused while the notification uses it.
    6. Delete the notification, then the profile.
  - **Ordered rules** on the write environment: three disabled
    `builtin:processavailability` rules (`dtmgd-it-…`):
    1. Create two.
    2. Create a third with `position: first`.
    3. Move one with `before:` and with `after:` by `externalId`.
    4. Check the listing order after each step.
    5. Delete them.
  - **Exports round-trip** on `DTMGD_IT_ENV`: a dry run applying up to 5
    objects of each schema at environment scope plans `unchanged` for every
    one. GETs only.
  - **Cluster scope listing** on `DTMGD_IT_CLUSTER`: `get settings --scope
    cluster` succeeds.
  - **Cluster network zone** on the write cluster: create an unused
    `dtmgd-it-…` zone at `environment-default`, re-apply it (`unchanged`),
    delete it.
  - **Cleanup:** every write test registers a cleanup that deletes the
    `dtmgd-it-…` objects of its schemas, notifications before profiles.
- Before the PR, the live suite runs on `stage` + `stage-cluster` (writes)
  and on `prod` + `prod-cluster` (reads), under the maintainer's standing
  permission. The PR reports the counts only.

## Delivery

One branch (`feat/settings-any-schema`), one plan, one pull request. The
changes are independent of each other but serve one purpose. The release
that follows (version and timing) is the maintainer's call.

## Calls I made

- **Secrets are found by walking the schema's properties**, and reports
  print `<secret>` in their place. The reviewer pointed out that a dry run
  would otherwise print a real secret from the file, and the earlier design
  counts a secret in output as a failure. The masked-create check uses the
  same walk, so it looks only at secret properties.
- **A listed schema whose definition answers 404 is skipped with a
  warning** during the delete check; every other failure stops the delete.
- **One `position` key**, rather than mirroring the API's `insertAfter`
  and `insertBefore`. It is portable across environments (anchors by
  `externalId`), reads the same on create and update, and hides the API's
  asymmetry (`""` means first in one field and last in the other).
- **The delete check reads every schema definition on each delete,** about
  5–10 s. That is the price of being right for every schema and cluster
  version, rather than a built-in table that goes stale. There is no disk
  cache unless measurement says otherwise (see Left to the builder).
- **No override for the reference check,** matching the update-window
  check: the way forward is to change the referencing settings first.
- **A 403 "Setting not applicable" keeps exit 5.** The HTTP-to-exit-code
  contract stays simple; the hint carries the meaning.
- **The cluster scope-only fallback runs only after the cluster's own
  error,** so a fixed cluster version gets the single request again.
- **One pull request** instead of phases.

## Left to the builder

- Names of new helpers and files, and how the reference scan and the
  schema-definition fetch are shared between `delete` and future users.
- If reading the definitions takes more than 15 s on `stage`, add a cache
  for the run. A disk cache, keyed by host, schema ID and
  `latestSchemaVersion`, needs the maintainer's say first.
- How the fallback merges paging for `--limit`, as long as truncation is
  never silent.
- The worked examples' exact values in `references/settings.md`. They must
  pass `--validate-only` on `stage`.
