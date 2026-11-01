# Settings 2.0 with dtmgd

Read this when the user asks how something is configured in an environment,
or wants it changed: update windows, OneAgent or ActiveGate auto-update, or
any other setting. dtmgd reads and writes Settings 2.0 in environment
contexts.

## Find the schema

Every setting belongs to a schema with an ID such as
`builtin:deployment.oneagent.updates`. There are hundreds, and agent mode
lists 50 at a time, so search:

```bash
dtmgd get settings-schemas --search oneagent
dtmgd describe settings-schema builtin:deployment.oneagent.updates   # scopes, properties, defaults, enum values
```

`describe settings-schema` shows where the schema can be set (`Scopes`),
whether a scope holds one object or many (`Objects`), and each property's
type, default and allowed values. An unknown schema ID exits 4.

## Stored objects versus effective values

- **Stored objects** are what someone configured, at the scope they set it:

  ```bash
  dtmgd get settings --schema builtin:deployment.management.update-windows
  dtmgd get settings --schema builtin:deployment.oneagent.updates --scope HOST_GROUP-0000000000000001
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

**Scopes:** `environment` (the whole environment), or an entity ID:
`HOST_GROUP-…`, `HOST-…`, `ENVIRONMENT_ACTIVE_GATE-…`. Which ones a schema
takes is in `describe settings-schema`.

**Inheritance:** host → host group → environment → schema default. A value
at a narrower scope overrides the wider ones; with nothing stored at a
scope, it inherits.

## Output

- json and yaml print the API's own objects: `objectId`, `schemaId`,
  `scope`, `externalId`, `summary`, `updateToken`, `schemaVersion`, `value`.
- Tables are readable for three schemas (below); any other schema shows
  `OBJECT-ID`, `SCOPE` and `SUMMARY`, with a `SCHEMA` column when a listing
  spans several schemas.

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
  longer exists: the cluster lets a window be deleted while settings still
  name it.
- The update-window table's `SCHEDULE` reads like
  `every 2 weeks Mon,Fri 03:00 GMT+01:00, 90 min, from 2026-01-01 until 2027-01-01`.
  A window whose `until` date has passed no longer opens, so settings that
  use only such windows never update.

## Recipe: which OneAgent update mode applies to host X?

```bash
# 1. The host's entity ID
dtmgd get entities --selector 'type(HOST),entityName("web-01")' -o json
# 2. What applies to it, and where that comes from
dtmgd get settings --effective --scope HOST-0000000000000001 --schema builtin:deployment.oneagent.updates
# 3. If the mode uses update windows, their schedules
dtmgd get settings --schema builtin:deployment.management.update-windows
```

## Writing

**Always run the change with `--dry-run` first** and show the user what it
would do. Then run it for real only if they want it.

```bash
dtmgd describe settings <object-id> -o yaml > object.yaml   # export: the file apply takes
# change value: in object.yaml
dtmgd apply -f object.yaml --dry-run       # action update/unchanged/create, current and new value
dtmgd apply -f object.yaml --validate-only # the cluster checks it without saving (Managed 1.288+)
dtmgd apply -f object.yaml                 # writes; result action created/updated/unchanged/failed
```

- **Write levels.** Every context has a `safety-level`: `readonly` (the
  default: no writes), `readwrite-mine` (create only), `readwrite-all`,
  `dangerously-unrestricted`. A blocked write exits `6` (`safety_blocked`)
  and sends nothing. Raising a level is `dtmgd config set-context <name>
  --safety-level readwrite-all`; the token's scopes are the customer's real
  control (`settings.write`), so ask the user before raising a level for
  them.
- **The apply file** is `{objectId?, externalId?, schemaId, scope, value,
  updateToken?}`, one object, an array, or a `get settings -o yaml` list.
  With `objectId` apply updates that object (or creates it in an environment
  where it does not exist); without it, it updates the scope's object of a
  one-per-scope schema (OneAgent and ActiveGate updates) or the object with
  the same `externalId`, and otherwise creates. **Give objects of
  multi-object schemas (update windows) an `externalId`**, or a repeated
  apply creates duplicates.
- **Conflicts.** An exported `updateToken` makes the write fail with `409`
  if someone changed the object meanwhile: re-read it and apply again.
- **Several environments:** `apply` and `create settings` take
  `--env "a;b"` or `--env ALL_ENVIRONMENTS`. Every context is checked before
  any is written; one blocked context means nothing is written anywhere.
- **Creating one object** from a bare value: `dtmgd create settings -f
  value.yaml --schema <id> --scope <scope>`. It refuses a one-per-scope
  schema whose scope already has its object: use apply.
- **Deleting:** `dtmgd delete settings <object-id> --yes` (`--yes` is
  required in agent mode). Deleting the object at a scope makes that scope
  inherit again. An update window that a OneAgent or ActiveGate update
  setting still uses is not deleted: change those first.
- **`edit setting`** opens an editor and is for people; it refuses in agent
  mode.
- **Secret properties** come back masked in exports, and dtmgd does not
  treat them specially: check `describe settings-schema` for `secret`-type
  properties before applying an exported object, and set those values
  explicitly.
- **Update window example** (a disabled window, so it changes nothing until
  enabled):

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
      selectedWeekDays: {saturday: true}
      updateTime: {startTime: "02:00", timeZone: "GMT+00:00", duration: 120}
      recurrenceRange: {start: "2026-01-01T00:00:00Z", end: "2027-01-01T00:00:00Z"}
  ```

## Token scopes

An environment token needs `settings.read` for every settings command, and
`settings.write` to write. `--validate-only` also reads the cluster version,
which needs `DataExport`. A 403 names the missing scope.
