# Settings 2.0 with dtmgd

Read this when the user asks how something is configured, or wants it
changed. dtmgd reads and writes **any Settings 2.0 schema**: alerting
profiles, problem notifications, process-availability rules, log storage,
management zones, update windows, OneAgent and ActiveGate auto-update, and
the rest of the 200 or more schemas an environment offers. It works on
environment contexts and on cluster contexts (the cluster's own settings,
see [cluster-api.md](cluster-api.md)). Configuration outside Settings 2.0 is
out of reach (see the end of this page).

## The workflow for any request

1. **Find the schema.** `--search` matches the schema ID and display name,
   ignoring case. Try the user's words and the product area ("alerting",
   "notification", "process", "log", "zone"). Agent mode lists 50 at a
   time, so search rather than list.

   ```bash
   dtmgd get settings-schemas --search alerting
   ```

2. **Learn the schema.**

   ```bash
   dtmgd describe settings-schema builtin:alerting.profile
   ```

   It shows:
   - `Scopes`: where the schema can be set (see Scopes below);
   - `Objects`: `one per scope`, or `up to N per scope`, with `ordered`
     when the list order matters (see Ordered lists);
   - `Maturity` (see Maturity) and `Key property` (the property that
     identifies an object at a scope: apply updates the object with the same
     key instead of creating another);
   - each property's type, default and allowed values (`VALUES`), with a
     `Type …` table for each nested type. `setting → <schema-id>` is a
     reference to another settings object (see References); `secret` is a
     secret (see Secrets).

   `-o json` prints the raw definition, with descriptions and constraints.
   An unknown schema ID exits 4.

3. **Read what is there**: the stored objects, or the values that apply at
   one scope (next section).

4. **Write the file.** To change an object, export it (`describe settings
   <object-id> -o yaml`) and edit `value`. For a new object, write
   `{schemaId, scope, externalId, value}` from the properties and defaults
   that step 2 showed. Leave out `objectId` and `updateToken`.

5. **Check, then write.** Show the user the dry run, and write only if they
   want it.

   ```bash
   dtmgd apply -f object.yaml --dry-run         # GETs only: the action and the current and new value of each object
   dtmgd apply -f object.yaml --validate-only   # the cluster checks every change without saving it
   dtmgd apply -f object.yaml                   # writes; each result is created, updated, unchanged or failed
   ```

   `--dry-run` and `--validate-only` work at every write level; only the
   real write needs a context whose `safety-level` allows it.

## Stored objects versus effective values

- **Stored objects** are what someone configured, at the scope they set it:

  ```bash
  dtmgd get settings --schema builtin:processavailability --scope environment
  dtmgd get settings --schema builtin:deployment.oneagent.updates --scope HOST_GROUP-0000000000000001
  dtmgd get settings --schema builtin:alerting.profile    # every scope
  dtmgd get settings --scope HOST-0000000000000001        # every schema stored on one host
  dtmgd get settings <object-id>                          # one object
  dtmgd describe settings <object-id> -o yaml             # one object, value included
  ```

  A list needs `--schema` or `--scope` (the API requires one). `--schema`
  takes several IDs (repeat it or separate them with commas). An empty list
  means nothing is stored there, not that the setting is off.

- **Effective values** are what actually applies at one scope, after
  inheritance and schema defaults:

  ```bash
  dtmgd get settings --effective --scope HOST-0000000000000001 --schema builtin:deployment.oneagent.updates
  ```

  `--effective` needs `--scope`. The `ORIGIN` column (`origin` in json) is
  the scope the value is stored at; `default` means no one set it and the
  schema default applies. Effective values have no object ID.

## Writing

- **Write levels.** Every context has a `safety-level`: `readonly` (the
  default: no writes), `readwrite-mine` (create only), `readwrite-all`,
  `dangerously-unrestricted`. A blocked write exits `6` (`safety_blocked`)
  and sends nothing. Raising a level is `dtmgd config set-context <name>
  --safety-level readwrite-all`; the token's scopes are the customer's real
  control (`settings.write`), so ask the user before raising a level for
  them.
- **The apply file** is `{objectId?, externalId?, schemaId, scope, value,
  updateToken?, position?}`: one object, an array, a `get settings -o yaml`
  list, or several `---` documents. An export made in agent mode (the
  envelope) applies too.
  - With `objectId`, apply updates that object, which must have the file's
    `schemaId` and `scope` (to copy a value to another scope, remove
    `objectId`). In an environment where the object does not exist, apply
    creates it.
  - Without `objectId`, apply updates the scope's object of a one-per-scope
    schema, else the object with the same `externalId`, else the one with
    the same key property, and otherwise creates.
  - **Give objects of multi-object schemas an `externalId`**, so a repeated
    apply updates them. Update windows have unique names, so a repeated
    create of one fails with 400; schemas without a unique field would get a
    duplicate.
  - `position` places the object in an ordered schema (see Ordered lists).
  - Two objects of one file may not write the same object.
- **Conflicts.** An exported `updateToken` makes the write fail with `409`
  if someone changed the object meanwhile: re-read it and apply again.
- **A write lost in transit** (`failed`, "the cluster may have saved this
  change") may have been saved: check with `get settings` before running it
  again. dtmgd never resends a write itself.
- **Several environments:** `apply` and `create settings` take
  `--env "a;b"` or `--env ALL_ENVIRONMENTS`. Every context is checked before
  any is written; one blocked context means nothing is written anywhere.
- **Creating one object** from a bare value: `dtmgd create settings -f
  value.yaml --schema <id> --scope <scope>` (`--position` in an ordered
  schema). It gives the object no `externalId`, and it refuses when the
  create would overwrite an object (the scope's object of a one-per-scope
  schema, or one with the same key property): use apply.
- **Deleting:** `dtmgd delete settings <object-id> --yes` (`--yes` is
  required in agent mode). Deleting the object at a scope makes that scope
  inherit again. An object that other settings still use is not deleted
  (see References).
- **`edit setting`** opens an editor and is for people; it refuses in agent
  mode.

## Worked example: an alerting profile and a webhook notification

The user wants availability problems sent to their on-call webhook, but
nothing sent yet. The notification references the profile and carries a
secret header.

```bash
dtmgd get settings-schemas --search alerting         # builtin:alerting.profile
dtmgd get settings-schemas --search notification     # builtin:problem.notifications
dtmgd describe settings-schema builtin:alerting.profile
dtmgd describe settings-schema builtin:problem.notifications   # alertingProfile: setting → builtin:alerting.profile; secretValue: secret
```

1. The profile, `profile.yaml`:

   ```yaml
   schemaId: builtin:alerting.profile
   scope: environment
   externalId: oncall-availability
   value:
     name: On-call availability
     severityRules:
       - severityLevel: AVAILABILITY
         delayInMinutes: 0
         tagFilterIncludeMode: NONE
     eventFilters: []
   ```

   Run `apply -f profile.yaml` with `--dry-run`, then `--validate-only`,
   then for real. The result holds the new profile's `objectId`
   (`results[0].objectId`; `result.results[0].objectId` in the agent
   envelope).

2. The notification, `notification.yaml`, disabled until the user turns it
   on. `alertingProfile` holds the profile's `objectId` from step 1: a
   reference holds an `objectId`, never an `externalId` or a name.

   ```yaml
   schemaId: builtin:problem.notifications
   scope: environment
   externalId: oncall-webhook
   value:
     enabled: false
     type: WEBHOOK
     displayName: On-call webhook
     alertingProfile: <objectId of the profile>
     webHookNotification:
       url: https://example.invalid/oncall
       acceptAnyCertificate: false
       notifyEventMergesEnabled: false
       notifyClosedProblems: true
       payload: '{"problemId": "{ProblemID}", "title": "{ProblemTitle}", "state": "{State}"}'
       headers:
         - name: Authorization
           secret: true
           secretValue: replace-with-the-real-secret
   ```

   Ask the user for the real secret and put it in the file. The dry run
   shows it as `<secret>`. Then `--validate-only`, then apply.

Afterwards:
- Once the profile exists, both objects may live in one file, as two `---`
  documents. Re-applying it leaves the profile `unchanged` (found by its
  `externalId`), but the notification plans `update` every time, because it
  holds a real secret (see Secrets).
- In another environment the profile has another `objectId`: create the
  profile there first and put that environment's `objectId` in the
  notification.
- Deleting the profile is refused while the notification uses it: delete
  or change the notification first.

## Worked example: a rule first in an ordered list

The user wants to know when no `backup-agent` process runs on Linux hosts,
with this rule evaluated before the others. It stays disabled until they
turn it on.

```bash
dtmgd get settings-schemas --search availability                               # builtin:processavailability
dtmgd describe settings-schema builtin:processavailability                     # Objects: up to … per scope, ordered
dtmgd get settings --schema builtin:processavailability --scope environment    # the current order
```

`rule.yaml`; `position` sits next to `value`, not inside it:

```yaml
schemaId: builtin:processavailability
scope: environment
externalId: backup-agent-running
position: first
value:
  enabled: false
  name: backup-agent running
  operatingSystem: [LINUX]
  minimumProcesses: 1
  metadata: []
  rules:
    - ruleType: RuleTypeProcess
      property: executable
      condition: '$eq(backup-agent)'
```

```bash
dtmgd apply -f rule.yaml --dry-run -o yaml     # position: {new: 1}
dtmgd apply -f rule.yaml --validate-only
dtmgd apply -f rule.yaml
```

- With `--env "a;b"` the rule goes first in each environment.
- A second rule right after it: `position: after:backup-agent-running`.
  The `externalId` names the anchor in every environment.
- For one host group only, use scope `HOST_GROUP-0000000000000001`. Each
  scope has its own order.

## Scopes

A schema can be set at the scopes its `Scopes` line lists (`allowedScopes`
in json), and nowhere else:

- `environment`: the whole environment. The API also calls it `tenant`;
  dtmgd treats the two as one.
- An entity: the line names the type (`HOST`, `HOST_GROUP`,
  `PROCESS_GROUP`, `ENVIRONMENT_ACTIVE_GATE`, …), and the file holds an ID
  of that type, such as `HOST_GROUP-0000000000000001`. Find it with
  `dtmgd get entities --selector 'type(HOST_GROUP),entityName("…")'`.
- On a cluster context: `cluster`, `environment-default` (defaults for every
  environment of the cluster), and in a few schemas `realm`, `metric` or
  `CLUSTER_ACTIVE_GATE`. See [cluster-api.md](cluster-api.md).

To see the scopes a schema's objects use, list them without `--scope`:
`dtmgd get settings --schema <id>` shows each object's `SCOPE`.

**Inheritance:** host → host group → environment → schema default. A value
at a narrower scope overrides the wider ones; with nothing stored at a
scope, it inherits. `get settings --effective` shows the result.

## Ordered lists and `position`

In a schema whose `Objects` line says `ordered` (`"ordered": true` in json),
each scope's objects form a list, and the cluster evaluates them in that
order. `get settings --schema <id> --scope <scope>` lists them in it
(agent mode shows 50; pass `--limit` for more).

- Without `position`, a create goes last and an update keeps its place.
- `position` in an apply-file object, or `create settings --position`, puts
  the object somewhere else:
  - `first`;
  - `last`;
  - `after:<ref>`;
  - `before:<ref>`.
- `<ref>` is the `objectId` or the `externalId` of an object of the same
  schema at the same scope, looked up in each context (`objectId` first).
  Object IDs differ between environments and external IDs don't, so name
  anchors by `externalId` in a file meant for several environments.
- `<ref>` may also be the `externalId` of an object created earlier in the
  same file. The writes go in file order, and the anchor is filled with that
  object's new `objectId`.
  - If that object's write failed, this one fails with "its position anchor
    was not written" and is not sent.
  - Under `--validate-only` nothing is created, so this object is validated
    without its position, and its result carries a `note` saying so.
- An update whose value and place are unchanged is `unchanged`. A different
  place makes it `update`, even with the same value: a move.
- The dry run shows `position: {current: N, new: M}` (1-based places after
  the whole file is applied; no `current` for a create) in json, yaml and
  the agent envelope. Tables don't show it.
- `position` on a schema that is not ordered exits 2. An anchor that names
  no object fails the plan, with a hint to list the objects.
- An exported listing (`get settings --schema <id> --scope <scope> -o yaml`)
  applied where none of its objects exist is created in file order. It keeps
  its order without any `position`.

## References and the delete check

- A property of type `setting → <schema-id>` holds the `objectId` of a
  settings object of that schema. Examples:
  - a problem notification's `alertingProfile` → `builtin:alerting.profile`;
  - management-zone references → `builtin:management-zones`;
  - OneAgent and ActiveGate updates' windows →
    `builtin:deployment.management.update-windows`.
- Write the `objectId`. Get it from `get settings --schema <schema-id>
  -o json` (`items[].objectId`), or from apply's result when you create the
  object. A management-zone reference is stored as the zone's settings
  `objectId`.
- Object IDs differ between environments, so a file holding a reference
  works in one environment only.
- **`delete settings` refuses an object that other settings still use**
  (exit 1). The cluster itself would delete it and leave them pointing at
  nothing. The error lists up to 10 of them (schema, scope, object ID,
  summary) and the total. Change or delete those settings first. There is
  no override, and `--dry-run` shows the refusal too.
- To find them, a delete reads every schema's definition, which takes a few
  seconds (5–10 s on a large environment). A schema whose definition the
  cluster cannot serve is skipped, with a warning (on stderr; in agent mode
  in `context.warnings`). Any other failure stops the delete.
- **Settings only:** dashboards, user-group permissions and other
  configuration outside Settings 2.0 can use an object too, and are not
  checked. Before deleting a management zone, say so to the user.
- On a cluster context, an update window that the cluster ActiveGates'
  auto-update uses is refused too.

## Secrets

- A property of type `secret` (in `describe settings-schema`) holds a
  password, token or header value. A read returns a mask instead: `***` and
  16 hex characters and `***`, such as `***3f9ff26c722fbb93***`. The mask is
  salted: the same secret stored twice gets two different masks.
- **dtmgd never prints a secret.** Write reports show `<secret>` in `value`
  and `current`, in every format and in the agent envelope. The request
  carries the file's value.
- **Applying an export back to the same object keeps its secret.** With the
  mask left as it is, the cluster keeps the stored value.
- **Creating from a mask is refused.** A create would store the mask itself
  as the secret. dtmgd exits 2 before sending anything, naming each path
  (such as `webHookNotification.headers[0].secretValue`), with the hint
  "Exports mask secrets. Put the real value in the file; dtmgd never prints
  it." This covers `create settings`, apply creating an object, and apply
  with an `objectId` the environment does not have. `--dry-run` shows the
  refusal too.
- **A file holding the real secret always plans `update`**: the stored
  value cannot be compared with it.
- **An update matched by `externalId` in another environment keeps that
  object's secret.** An export from environment A applied to B, where an
  object with the same `externalId` exists, updates B's object, and B keeps
  its own secret. To change it, put the real value in the file.
- Get the real value from the user, and don't echo it back.

## When the cluster says no

- **403 "Setting not applicable: '…'"**: the setting does not apply to that
  scope, for the reason the message gives. It exits 5 like every 403, but
  it is not a token problem. Check the scopes in `describe settings-schema`,
  or use another scope.
- **Another 403** names the token scope the command needs (`settings.read`,
  or `settings.write` to write).
- **Validation errors** come back in the cluster's own words, from
  `--validate-only` or from a write: the object's result is `failed`, with
  `error.message` and `error.constraintViolations[]` (`path`, `message`),
  such as `maxProcesses: Value must be between 1 and 100`. Fix the value and
  run it again. The command exits non-zero after printing every result.
- **409**: the object changed since its export (see Writing).

## Maturity

`describe settings-schema` prints `Maturity:` (`maturity` in json):
`GENERAL_AVAILABILITY`, `OPT_IN_GENERAL_AVAILABILITY`, `EARLY_ADOPTER` or
`IN_DEVELOPMENT`. Before writing an `IN_DEVELOPMENT` or `EARLY_ADOPTER`
schema, ask the user: such schemas may change or disappear. dtmgd does not
block them, and reading them is fine.

## Output

- json and yaml print the API's own objects: `objectId`, `schemaId`,
  `scope`, `externalId`, `summary`, `updateToken`, `schemaVersion`, `value`.
- Tables are readable for the three update schemas (below); any other
  schema shows `OBJECT-ID`, `SCOPE` and `SUMMARY`, with a `SCHEMA` column
  when a listing spans several schemas. Tables show summaries without the
  cluster's markdown escapes; json and yaml keep them as the API sends them
  (`on\-call`).

## The update settings

| Schema | Scopes | Objects | Fields |
|---|---|---|---|
| `builtin:deployment.management.update-windows` | environment | many (names unique) | `enabled`, `name`, `recurrence` (`ONCE`, `DAILY`, `WEEKLY`, `MONTHLY`) with its block: `updateTime` (`startTime` HH:mm, `timeZone` GMT±hh:00, `duration` minutes), `recurrenceRange` (`start`, `end`) |
| `builtin:deployment.oneagent.updates` | environment, `HOST_GROUP`, `HOST` | one per scope | `updateMode` (`AUTOMATIC`, `AUTOMATIC_DURING_MW`, `MANUAL`), `targetVersion` (`latest`, `previous`, `older` or `major.minor`), `revision`, `maintenanceWindows[].maintenanceWindow` |
| `builtin:deployment.activegate.updates` | environment, `ENVIRONMENT_ACTIVE_GATE` | one per scope | `updateMode` (`AUTOMATIC`, `AUTOMATIC_DURING_UW`, `MANUAL`), `targetVersion`, `updateWindows[].updateWindow` |

- The mode that uses update windows is spelled `AUTOMATIC_DURING_MW` for
  OneAgent and `AUTOMATIC_DURING_UW` for ActiveGate.
- OneAgent and Environment ActiveGate updates share the same update windows.
  A window reference holds the window's object ID; the tables show the
  window's name instead.
- A window ID in the `WINDOWS` column instead of a name means that window no
  longer exists (deleted outside dtmgd, which refuses to delete a window in
  use).
- The update-window table's `SCHEDULE` reads like
  `every 2 weeks Mon,Fri 03:00 GMT+01:00, 90 min, from 2026-01-01 until 2027-01-01`.
  A window whose `until` date has passed no longer opens, so settings that
  use only such windows never update.

Which OneAgent update mode applies to host X:

```bash
# 1. The host's entity ID
dtmgd get entities --selector 'type(HOST),entityName("web-01")' -o json
# 2. What applies to it, and where that comes from
dtmgd get settings --effective --scope HOST-0000000000000001 --schema builtin:deployment.oneagent.updates
# 3. If the mode uses update windows, their schedules
dtmgd get settings --schema builtin:deployment.management.update-windows
```

An update window, disabled so it changes nothing until enabled.
`selectedWeekDays` must list all seven days, each `true` or `false`:

```yaml
schemaId: builtin:deployment.management.update-windows
scope: environment
externalId: weekend-window
value:
  enabled: false
  name: Weekend window
  recurrence: WEEKLY
  weeklyRecurrence:
    every: 1
    selectedWeekDays:
      {monday: false, tuesday: false, wednesday: false, thursday: false, friday: false, saturday: true, sunday: false}
    updateTime: {startTime: "02:00", timeZone: "GMT+00:00", duration: 120}
    recurrenceRange: {start: "2026-01-01T00:00:00Z", end: "2027-01-01T00:00:00Z"}
```

## Cluster contexts

Every settings command works on a cluster context as well: `--context
<cluster-context>`, with the cluster's scopes (`cluster`,
`environment-default`, …). The cluster token then needs
`settings.read`/`settings.write` besides `ServiceProviderAPI`. See
[cluster-api.md](cluster-api.md), which also covers the cluster ActiveGates'
auto-update (`activegate-updates`).

## Token scopes

An environment token needs `settings.read` for every settings command, and
`settings.write` to write (`--validate-only` included). `--validate-only`
also reads the cluster version, which needs `DataExport`. A 403 names the
missing scope.

## Out of reach

- **The Configuration API (v1).** Configuration that exists only there
  cannot be read or changed with dtmgd. If `get settings-schemas --search`
  finds no schema for what the user asks, tell them so rather than guess.
- **Settings change history.** The cluster's history endpoint is not part of
  the public Managed API.
- **Per-object settings permissions.** Those endpoints accept OAuth only,
  not API tokens.
