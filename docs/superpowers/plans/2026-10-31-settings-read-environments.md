# Settings 2.0, phase 1: reading in environments — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** dtmgd reads Settings 2.0 in environment contexts (schemas,
stored objects and effective values), and shows readable tables for the
update-window, OneAgent update and ActiveGate update schemas.

**Architecture:** Four new commands, each a `fetchSpec` run through
`a.runFetch`, so `--limit`, `--env` fan-out, agent envelopes and every
output format behave as they do for other commands. One `Rows` function
picks a table by the `schemaId` of the items it gets: a typed view for the
three update schemas, a generic view for any other schema. Window names for
the OneAgent and ActiveGate tables come from a second, table-only request.

**Tech Stack:** Go, Cobra, resty (`pkg/client`), the `table:` struct-tag
printer (`pkg/output`), the httptest harness in `cmd/harness_test.go`.

**Spec:** `docs/superpowers/specs/2026-10-31-settings-2-design.md`
(phase 1 of 3; phases 2 and 3 get their own plans).

## Global Constraints

- Branch `feat/settings-read`, created from `docs/settings-2-design` so the
  spec and this plan travel in the same PR.
- **Read only.** Phase 1 sends GETs only; nothing in this plan writes to a
  cluster.
- **Agent envelope.** Every new command returns the agent envelope in agent
  mode. stdout is data only. json, yaml and the envelope's `result` are the
  API's own response.
- **Scope.** All four commands declare `settings.read` with `withScopes`.
  The README scope list between `<!-- scopes:start -->` and
  `<!-- scopes:end -->` gains the line for it (`TestREADMEListsScopes`).
- **Generated reference.** Run `make docs` in every task that adds or
  changes a command or flag (`TestReferenceDocsAreCurrent`).
- **Tests.**
  - Run the CLI through `runCLI` / `runCLIWithEnv` against `newFakeAPI`,
    and compare tables with `assertGolden`. Regenerate goldens with
    `go test ./cmd/ -run <Test> -update`, then read the diff.
  - Error cases use 400 or 403, never 5xx or 429.
  - Never touch the default config path.
- **Fixtures** use synthetic object IDs and `example.com` hosts, never real
  ones.
- **Code limits:** golangci-lint `gocognit` ≤ 15 in non-test code. Test
  functions stay under Sonar's limit of 15 through helpers.
- **Lint** (`make lint`) and `go test ./...` pass at every commit.
- **Endpoints**, relative to `{host}/e/{env}/api/v2`:
  - `GET /settings/schemas`;
  - `GET /settings/schemas/{schemaId}`;
  - `GET /settings/objects`;
  - `GET /settings/objects/{objectId}`;
  - `GET /settings/effectiveValues`.
- **Paging:** a later page sends `nextPageKey` alone (`GetV2Paged` already
  does). Page size is at most 500.
- **`fields` on objects:**
  `objectId,schemaId,scope,externalId,summary,updateToken,schemaVersion,value`.
  **`fields` on effective values:** `schemaId,origin,summary,value`.
- **Schema IDs:**
  - `builtin:deployment.management.update-windows`
  - `builtin:deployment.oneagent.updates` (mode `AUTOMATIC_DURING_MW`,
    windows in `maintenanceWindows[].maintenanceWindow`)
  - `builtin:deployment.activegate.updates` (mode `AUTOMATIC_DURING_UW`,
    windows in `updateWindows[].updateWindow`)

  A window reference holds the update window's `objectId`.
- **Live checks** need the maintainer's go-ahead each time. Report counts
  only, never host names, IDs or values.

## Review Focus

1. **A dangling window reference.** OneAgent or ActiveGate updates can name
   a window that was deleted, because the server doesn't block such
   deletes. The WINDOWS cell shows the raw object ID, not a blank or an
   error. Pinned by `TestUpdatesTableShowsUnknownWindowID` (Task 3).
2. **An effective value with no `origin`** (a schema default). The ORIGIN
   cell reads `default`. Pinned by `TestEffectiveEmptyOriginShowsDefault`
   (Task 4).
3. **Values in older or partial shapes.** An ActiveGate updates value from
   before schema 1.0.4 (`autoUpdate` boolean, no `updateMode`), or a window
   whose recurrence block is missing. The row renders with `—` cells and the
   schedule falls back to the recurrence name, exit 0. Pinned by
   `TestOldValueShapesStillRender` (Task 3).
4. **Name lookups across several environments at once** (`--env "a;b"`).
   The names are shared between parallel fetches without a data race (the
   suite runs with `-race`). Pinned by `TestWindowNamesAcrossEnvironments`
   (Task 3).
5. **A listing that spans several schemas** (`--scope HOST-…` returns every
   schema stored there). It gets the generic table with a SCHEMA column,
   and every item is a row. Pinned by `TestGetSettingsMixedSchemas`
   (Task 2).

---

### Task 1: Settings schemas — `get settings-schemas`, `describe settings-schema`

**Files:**
- Create: `cmd/get_settings_schemas.go`, `cmd/describe_settings_schema.go`,
  `cmd/settings_schemas_test.go`
- Create fixtures:
  - `cmd/testdata/api/settings_schemas.json`: 5 items, among them the three
    update schemas, plus `builtin:alerting.profile` and
    `builtin:anomaly-detection.services`, with `displayName` and
    `latestSchemaVersion`; `totalCount` 5.
  - `cmd/testdata/api/settings_schema_oneagent_updates.json`: version
    `1.0.4`, `multiObject` false, `allowedScopes`
    `["HOST","HOST_GROUP","environment"]`.
    - Properties: `targetVersion` (text, default `latest`), `revision`
      (text), `updateMode` (`{"$ref":"#/enums/UpdateMode"}`, default
      `AUTOMATIC`), `maintenanceWindows` (`set` of
      `{"$ref":"#/types/MaintenanceWindow"}`).
    - `enums.UpdateMode.items`: `AUTOMATIC`, `AUTOMATIC_DURING_MW`,
      `MANUAL`.
    - `types.MaintenanceWindow.properties.maintenanceWindow`: type
      `setting`.
- Create goldens: `get_settings_schemas_table`,
  `describe_settings_schema_human`
- Modify:
  - `cmd/scopes.go`: `scopeSettingsRead`;
  - `cmd/get.go` and `cmd/describe.go`: `AddCommand` and the resource
    lists in `Long`;
  - `README.md`: scope line;
  - `docs/reference/`: `make docs`.

**Interfaces:**
- Produces:
  - `const scopeSettingsRead = "settings.read"` (`cmd/scopes.go`);
  - `const settingsSchemasPath = "/settings/schemas"`;
  - `newGetSettingsSchemasCmd(a *App) *cobra.Command` (Use
    `settings-schemas [schema-id]`, aliases `schema`, `schemas`);
  - `newDescribeSettingsSchemaCmd(a *App) *cobra.Command` (Use
    `settings-schema <schema-id>`, alias `schema`);
  - `type settingsSchemaRow struct { SchemaID string table:"SCHEMA-ID"; Name string table:"NAME"; Version string table:"VERSION" }`.

- [ ] **Step 1: Write the failing tests** in `cmd/settings_schemas_test.go`:

```go
const schemasPath = "GET /e/env1/api/v2/settings/schemas"

func TestGetSettingsSchemasTable(t *testing.T) // golden get_settings_schemas_table; columns SCHEMA-ID NAME VERSION, API order
func TestGetSettingsSchemasSearch(t *testing.T)
// `get settings-schemas --search ONEAGENT -o json`: items == [the oneagent.updates entry] (matches schemaId or
// displayName, case-insensitive); body["totalCount"] == 1; table form prints 1 data row.
func TestGetSettingsSchemasLimit(t *testing.T)
// `--limit 2`: 2 rows; stderr contains "Showing 2 of 5 settings-schemas".
func TestGetSettingsSchemaByID(t *testing.T)
// `get settings-schemas builtin:deployment.oneagent.updates`: one hit on
// "GET /e/env1/api/v2/settings/schemas/builtin:deployment.oneagent.updates", one row, VERSION 1.0.4.
func TestDescribeSettingsSchemaHuman(t *testing.T)
// golden describe_settings_schema_human; stdout contains "AUTOMATIC, AUTOMATIC_DURING_MW, MANUAL",
// "HOST, HOST_GROUP, environment", "one per scope", "set of MaintenanceWindow".
func TestDescribeSettingsSchemaNotFound(t *testing.T)
// route 404 {"error":{"code":404,"message":"Configuration schema builtin:nope does not exist."}}:
// exit 4; stderr contains "dtmgd get settings-schemas --search".
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'SettingsSchema' -count=1`
Expected: FAIL, unknown command "settings-schemas" / "settings-schema".

- [ ] **Step 3: Implement `get settings-schemas`**
  - **Fetch:** `c.GetV2(settingsSchemasPath, nil, &body)`. The endpoint has
    no paging.
  - **`--search`:** keeps items whose `schemaId` or `displayName` contains
    the text, case-insensitively. It then sets `body["totalCount"]` to the
    kept count.
  - **Limit:** truncate to the limit the way `licenseUsageFetch` does
    (`HasMore`, `Count`).
  - **With a `schema-id` argument:** GET `settingsSchemasPath + "/" +
    url.PathEscape(id)`, and make one row from `schemaId`, `displayName`
    and `version`. `--search` with an argument is a `UsageError`.
  - **Spec fields:** `List: true`, `Empty: "No settings schemas found."`,
    `Rows` builds `[]settingsSchemaRow`. Declare `withScopes(cmd,
    scopeSettingsRead)`.

- [ ] **Step 4: Implement `describe settings-schema`** with `describeFetch`
  and a `Human` renderer:
  - **Header lines:** Schema, Name, Version, Scopes (comma-joined
    `allowedScopes`), Objects, Description.
    - Objects reads `one per scope` when `multiObject` is false. Otherwise
      it reads `up to N per scope`, with `, ordered` appended when
      `ordered` is true.
  - **A property table:** PROPERTY, TYPE, DEFAULT, VALUES.
  - **One more such table per entry in `types`,** headed `Type <name>:`.
  - **Type text:**
    - a string type prints as is;
    - `{"$ref":"#/enums/X"}` prints `enum`, and VALUES lists `enums.X.items[].value`
      joined by `, `;
    - `{"$ref":"#/types/Y"}` prints `Y`;
    - `set` or `list` with `items.type` prints `set of Y` / `list of Y`.
  - **Missing default:** `—`.
  - **404 hint:** wrap the Fetch so a 404 gets
    `withHint(err, "List schema IDs with: dtmgd get settings-schemas --search <text>")`,
    in the manner of `hintOnBadRequest`.

- [ ] **Step 5: Register both,** add `scopeSettingsRead`, and add the README
  scope line `- Read settings (`settings.read`): settings schemas, settings
  objects and effective values`. Then update the `get` and `describe`
  `Long` lists, and run `make docs`.

- [ ] **Step 6: Run the tests**

Run: `go test ./cmd/ -run 'SettingsSchema|READMEListsScopes|ReferenceDocsAreCurrent|Commands' -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmd README.md docs/reference
git commit -m "feat: get settings-schemas and describe settings-schema"
```

---

### Task 2: Stored settings — `get settings`, `describe settings`, generic table

**Files:**
- Create:
  - `cmd/get_settings.go`, `cmd/describe_settings.go`;
  - `cmd/settings_views.go`: rows and the view dispatch;
  - `cmd/settings_test.go`.
- Create fixtures:
  - `settings_objects_alerting.json`: 2 objects of
    `builtin:alerting.profile`;
  - `settings_objects_mixed.json`: 3 objects of 3 schemas at scope
    `HOST-0000000000000001`;
  - `settings_object.json`: one object with `externalId` `ext-1`.
- Create goldens: `get_settings_generic_table`, `get_settings_mixed_table`,
  `describe_settings_human`
- Modify: `cmd/get.go`, `cmd/describe.go`, `docs/reference/`

**Interfaces:**
- Consumes: `scopeSettingsRead`, and the 404-hint wrapper from Task 1.
- Produces:
  - `const settingsObjectsPath = "/settings/objects"`;
  - `const settingsObjectFields = "objectId,schemaId,scope,externalId,summary,updateToken,schemaVersion,value"`;
  - `type settingsObject struct { ObjectID string json:"objectId"; SchemaID string json:"schemaId"; Scope string json:"scope"; ExternalID string json:"externalId"; Summary string json:"summary"; UpdateToken string json:"updateToken"; SchemaVersion string json:"schemaVersion"; Origin string json:"origin"; Value map[string]interface{} json:"value" }`;
  - `type getSettingsOptions struct { schemas []string; scope string; effective bool; limit int }`;
  - `func settingsParams(o getSettingsOptions) map[string]string`;
  - `type settingsView struct{}` with
    `func (v settingsView) rows(body interface{}) (interface{}, int, error)`,
    used as `Rows: settingsView{}.rows`. Task 3 adds the field
    `names *windowNames`, and Task 4 adds `effective bool`. The signature
    never changes.
  - `rows` accepts a list body (`items`) or a single object.
  - `type genericSettingsRow struct { ObjectID table:"OBJECT-ID"; Scope table:"SCOPE"; Summary table:"SUMMARY,max=80" }`
    and `genericMultiSchemaRow` (SCHEMA, then the same columns).

- [ ] **Step 1: Write the failing tests** in `cmd/settings_test.go`:

```go
const objectsPath = "GET /e/env1/api/v2/settings/objects"

func TestGetSettingsNeedsSchemaOrScope(t *testing.T) // no flags, no arg: exit 2, stderr has "--schema or --scope", api.totalHits()==0
func TestGetSettingsQuery(t *testing.T)
// `--schema a --schema b,c --scope HOST-1`: one query with schemaIds=="a,b,c", scopes=="HOST-1",
// fields==settingsObjectFields, pageSize=="500"; with --limit 3 pageSize=="3".
func TestGetSettingsPagingSendsOnlyNextPageKey(t *testing.T) // page-2 query keys == {"nextPageKey"}
func TestGetSettingsGenericTable(t *testing.T) // golden; header OBJECT-ID SCOPE SUMMARY, no SCHEMA
func TestGetSettingsMixedSchemas(t *testing.T) // golden; header starts SCHEMA; 3 data rows (Review Focus 5)
func TestGetSettingsJSONIsAPIShape(t *testing.T) // -o json: items[0] has objectId, schemaId, scope, value as in the fixture
func TestGetSettingsByObjectID(t *testing.T)
// `get settings vu9/x+y=`: one hit on objects path + "/" + url.PathEscape("vu9/x+y="); one row.
// `get settings ID --schema a`: exit 2.
func TestGetSettingsUnknownSchema(t *testing.T) // 404 "Configuration schema x does not exist.": exit 4, hint names "get settings-schemas --search"
func TestGetSettingsAgentEnvelope(t *testing.T) // CLAUDECODE=1: ok true, result.items length 2, context.resource "settings"
func TestDescribeSettingsHuman(t *testing.T)
// golden; lines "Object ID:", "Schema:", "Scope:", "Summary:", "External ID: ext-1", "Schema version:", then "Value:" and
// the value as YAML. "External ID:" is absent for an object without one.
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'GetSettings|DescribeSettingsHuman' -count=1`
Expected: FAIL, unknown command "settings".

- [ ] **Step 3: Implement `get settings`** (Use `settings [object-id]`,
  alias `setting`, `cobra.MaximumNArgs(1)`).
  - **Flags:** `--schema` (StringSlice, repeatable and comma-separated),
    `--scope` and `--limit`. Task 4 adds `--effective`.
  - **Without an argument:** `pagedFetch(settingsObjectsPath,
    settingsParams(o), "items", 500, a.Opts.MaxPages)`, with
    `List: true`, `Empty: "No settings found."`, `Rows:
    settingsView{}.rows`.
    `settingsParams` sends `schemaIds`, `scopes` and `fields`, each only
    when it has a value; `fields` always has one.
  - **With an argument:** `describeFetch(settingsObjectsPath + "/" +
    url.PathEscape(id), nil)` with the same `Rows`. json and yaml print the
    single object.
    - `--schema` or `--scope` with an ID is a `UsageError`.
    - No ID and no `--schema`/`--scope` is a `UsageError`: "give --schema
      or --scope (the API needs one), or an object ID".
  - **404 on the list:** the Task 1 hint.

- [ ] **Step 4: Implement `settingsView.rows` in `cmd/settings_views.go`.**
  - Decode `items` (or the single object) into `[]settingsObject`.
  - If they span more than one `schemaId`, the rows are
    `genericMultiSchemaRow`; otherwise `genericSettingsRow`.
  - Task 3 adds the typed branches in front of the generic ones. Keep the
    dispatch a `switch` on the single schema ID, so `gocognit` stays under
    15.

- [ ] **Step 5: Implement `describe settings <object-id>`** (alias
  `setting`): `describeFetch` with a `Human` renderer printing the header
  lines in the order the test pins, then the value through the YAML
  printer.

- [ ] **Step 6: Register both, update the `Long` lists, run `make docs`, and
  run the tests**

Run: `go test ./cmd/ -run 'GetSettings|DescribeSettings|ReferenceDocsAreCurrent' -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmd docs/reference
git commit -m "feat: get settings and describe settings"
```

---

### Task 3: Readable tables for update windows, OneAgent and ActiveGate updates

**Files:**
- Modify: `cmd/settings_views.go`, `cmd/get_settings.go`
- Modify `cmd/harness_test.go`: a route key may also name the
  `schemaIds` query value, `"GET /path schemaIds=<value>"`. It is tried
  after the `nextPageKey` key, before the plain key.
- Create: `cmd/settings_views_test.go`
- Create fixtures:
  - `settings_update_windows.json`: 4 windows, one per recurrence:
    - `WEEKLY` (Mon and Fri, every 2, 03:00, `GMT+01:00`, 90 min);
    - `DAILY` (every 1);
    - `MONTHLY` (day 15, every 1);
    - `ONCE` (start `2026-11-01T03:00:00Z`, end `2026-11-01T04:00:00Z`).

    Object IDs `uw-weekly`, `uw-daily`, `uw-monthly`, `uw-once`; the
    weekly window is disabled.
  - `settings_oneagent_updates.json`: three objects.
    - Scope `environment`: `AUTOMATIC_DURING_MW`, windows `uw-weekly` and
      `uw-gone`.
    - `HOST_GROUP-0000000000000001`: `MANUAL`.
    - `HOST-0000000000000001`: `AUTOMATIC`, targetVersion `1.341`,
      revision `latest`.
  - `settings_activegate_updates.json`: scope `environment`,
    `AUTOMATIC_DURING_UW`, window `uw-daily`; and an object with the old
    value `{"autoUpdate": true}`.
- Create goldens: `get_settings_update_windows_table`,
  `get_settings_oneagent_updates_table`,
  `get_settings_activegate_updates_table`

**Interfaces:**
- Consumes: `settingsObject`, `settingsView`, `settingsObjectsPath`.
- Produces:
  - `const schemaUpdateWindows`, `schemaOneAgentUpdates`,
    `schemaActiveGateUpdates`, holding the three IDs in Global
    Constraints;
  - `func updateWindowSchedule(v map[string]interface{}) string`;
  - `type windowNames struct` (mutex-guarded `map[string]string`) with
    `func (n *windowNames) set(id, name string)` and
    `func (n *windowNames) name(id string) string`. `name` returns `id`
    when it is unknown.
  - `func lookupWindowNames(c *client.Client, n *windowNames) error`: one
    paged GET of `settingsObjectsPath` with
    `schemaIds=schemaUpdateWindows`, `scopes=environment`,
    `fields=objectId,value`, `pageSize=500`;
  - row types:
    - `updateWindowRow`: NAME, ENABLED, SCHEDULE, OBJECT-ID;
    - `oneAgentUpdatesRow`: SCOPE, UPDATE-MODE, TARGET-VERSION, REVISION,
      WINDOWS, OBJECT-ID;
    - `activeGateUpdatesRow`: SCOPE, UPDATE-MODE, TARGET-VERSION, WINDOWS,
      OBJECT-ID.

- [ ] **Step 1: Write the failing tests** in `cmd/settings_views_test.go`:

```go
func TestUpdateWindowSchedule(t *testing.T) {
	cases := map[string]string{ // fixture value → want
		"weekly every 1, Mon, 03:00 GMT+00:00, 60":   "weekly Mon 03:00 GMT+00:00, 60 min",
		"weekly every 2, Mon+Fri, 03:00 GMT+01:00, 90": "every 2 weeks Mon,Fri 03:00 GMT+01:00, 90 min",
		"daily every 1, 03:00 GMT+00:00, 60":         "daily 03:00 GMT+00:00, 60 min",
		"daily every 3":                              "every 3 days 03:00 GMT+00:00, 60 min",
		"monthly every 1, day 15":                    "monthly on day 15 03:00 GMT+00:00, 60 min",
		"monthly every 2, day 15":                    "every 2 months on day 15 03:00 GMT+00:00, 60 min",
		"once":                                       "once 2026-11-01T03:00:00Z – 2026-11-01T04:00:00Z",
		"WEEKLY without weeklyRecurrence":            "WEEKLY",
	}
	// build each value map in the test; assert updateWindowSchedule(v) == want
}
func TestGetSettingsUpdateWindowsTable(t *testing.T) // golden; header NAME ENABLED SCHEDULE OBJECT-ID; weekly row ENABLED "no"
func TestGetSettingsOneAgentUpdatesTable(t *testing.T)
// golden; environment row WINDOWS "Weekly maintenance, uw-gone"; MANUAL row WINDOWS "—";
// HOST row TARGET-VERSION "1.341" REVISION "latest".
func TestUpdatesTableShowsUnknownWindowID(t *testing.T) // Review Focus 1: "uw-gone" appears verbatim, exit 0
func TestGetSettingsActiveGateUpdatesTable(t *testing.T) // golden; WINDOWS shows the daily window's name
func TestOldValueShapesStillRender(t *testing.T)
// Review Focus 3: the {"autoUpdate":true} object renders UPDATE-MODE "—" TARGET-VERSION "—" WINDOWS "—"; exit 0
func TestWindowLookupOnlyForTables(t *testing.T)
// `get settings --schema builtin:deployment.oneagent.updates` with -o json, -o yaml and CLAUDECODE=1:
// no query on objectsPath has schemaIds==schemaUpdateWindows. In table form: exactly one such query,
// with scopes=environment and fields=objectId,value
func TestWindowLookupFailureWarns(t *testing.T)
// lookup route 403: exit 0, WINDOWS shows "uw-weekly", stderr contains "update window names unavailable"
func TestWindowNamesAcrossEnvironments(t *testing.T)
// Review Focus 4: two fake envs, `--env "a;b"` table: exit 0, both sections show the resolved name
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'UpdateWindow|UpdatesTable|OldValueShapes|WindowLookup|WindowNames' -count=1`
Expected: FAIL, `undefined: updateWindowSchedule` (build failure).

- [ ] **Step 3: Extend the fake API's route lookup** in
  `cmd/harness_test.go` (route key `"GET /path schemaIds=<value>"`).
  Document it in the `fakeAPI` comment.

- [ ] **Step 4: Implement `updateWindowSchedule`.**
  - The phrase is `<cadence> <when> <startTime> <timeZone>, <duration>
    min`.
  - **Cadence:**
    - `every` 1 gives `daily`, `weekly` or `monthly`;
    - otherwise `every N days|weeks|months`.
  - **When:**
    - weekly: the selected days as `Mon`…`Sun`, in week order, joined by
      `,`;
    - monthly: `on day N`.
  - **`ONCE`:** `once <start> – <end>`, from
    `onceRecurrence.recurrenceRange`.
  - **A missing block,** or an unknown recurrence, prints the
    `recurrence` value.

- [ ] **Step 5: Implement the typed rows and the table-only lookup.**
  - **Dispatch:** in `settingsView.rows`, a single schema ID equal to one
    of the three constants selects its row type.
  - **A missing value field** renders `—`.
  - **WINDOWS:**
    - the mode is `AUTOMATIC_DURING_MW` (OneAgent) or
      `AUTOMATIC_DURING_UW` (ActiveGate): window names via `names.name`,
      in value order, joined by `, `;
    - any other mode: `—`.
  - **The lookup:** `get settings` creates one `*windowNames` per
    invocation and sets it on `settingsView.names`. Its Fetch calls
    `lookupWindowNames` only when
    `a.humanOutput()` is true and the items include a OneAgent or
    ActiveGate updates object.
    - A failure appends the warning `"update window names unavailable: " +
      err.Error()` and leaves the IDs.
    - With `--env` fan-out the fetches run in parallel, so `windowNames`
      guards its map with a `sync.Mutex`. Object IDs are unique across
      environments.

- [ ] **Step 6: Run the tests**

Run: `go test -race ./cmd/ -run 'Settings|UpdateWindow|Window' -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmd
git commit -m "feat: readable tables for update windows and auto-update settings"
```

---

### Task 4: Effective values — `get settings --effective`

**Files:**
- Modify: `cmd/get_settings.go`, `cmd/settings_views.go`, `docs/reference/`
- Create: `cmd/settings_effective_test.go`
- Create fixtures:
  - `settings_effective_oneagent.json`: one item, `origin`
    `HOST_GROUP-0000000000000001`, `AUTOMATIC_DURING_MW` with window
    `uw-weekly`;
  - `settings_effective_default.json`: one OneAgent item without `origin`;
  - `settings_effective_mixed.json`: 3 items of 3 schemas with origins.
- Create goldens: `get_settings_effective_oneagent_table`,
  `get_settings_effective_mixed_table`

**Interfaces:**
- Consumes: `settingsView` (this task adds its `effective bool` field),
  `windowNames`, `lookupWindowNames`, the Task 3 row logic.
- Produces:
  - `const settingsEffectivePath = "/settings/effectiveValues"`;
  - `const settingsEffectiveFields = "schemaId,origin,summary,value"`;
  - effective row types, where ORIGIN replaces SCOPE and OBJECT-ID:
    - `oneAgentUpdatesEffectiveRow`: ORIGIN, UPDATE-MODE, TARGET-VERSION,
      REVISION, WINDOWS;
    - `activeGateUpdatesEffectiveRow`: ORIGIN, UPDATE-MODE,
      TARGET-VERSION, WINDOWS;
    - `updateWindowEffectiveRow`: NAME, ENABLED, SCHEDULE, ORIGIN;
    - `genericEffectiveRow`: SCHEMA, ORIGIN, SUMMARY.

- [ ] **Step 1: Write the failing tests** in `cmd/settings_effective_test.go`:

```go
const effectivePath = "GET /e/env1/api/v2/settings/effectiveValues"

func TestGetSettingsEffectiveNeedsScope(t *testing.T) // --effective alone: exit 2 naming --scope; --effective with an object ID: exit 2; no hits
func TestGetSettingsEffectiveQuery(t *testing.T)
// `--effective --scope HOST-1 --schema builtin:deployment.oneagent.updates`: one query on effectivePath with
// scope=="HOST-1", schemaIds set, fields==settingsEffectiveFields, pageSize=="500", and no "scopes" key
func TestGetSettingsEffectiveOneAgentTable(t *testing.T) // golden; header ORIGIN UPDATE-MODE …; no OBJECT-ID; WINDOWS resolved
func TestEffectiveEmptyOriginShowsDefault(t *testing.T)  // Review Focus 2: ORIGIN cell "default"
func TestGetSettingsEffectiveMixed(t *testing.T)         // golden; header SCHEMA ORIGIN SUMMARY; 3 rows
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'Effective' -count=1`
Expected: FAIL, unknown flag `--effective`.

- [ ] **Step 3: Implement `--effective`.**
  - The `--effective` flag requires `--scope` and no object ID; anything
    else is a `UsageError`.
  - Fetch with `pagedFetch(settingsEffectivePath, params, "items", 500,
    a.Opts.MaxPages)`. The params are `scope` (singular), `schemaIds` when
    given, and `fields`.
  - `settingsView{names: …, effective: true}.rows` picks the effective
    row types. An empty `origin` prints `default`.
  - Reuse the Task 3 value-to-cell helpers, so the stored and effective
    rows can't drift.

- [ ] **Step 4: Run `make docs` and the tests**

Run: `go test -race ./cmd/ -run 'Settings|Effective|ReferenceDocsAreCurrent' -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd docs/reference
git commit -m "feat: get settings --effective"
```

---

### Task 5: Docs, skill and knowledge base

**Files:**
- Modify `README.md`:
  - the verbs table: `get` gains `settings-schemas`, `settings`;
    `describe` gains `settings-schema`, `settings`;
  - two examples in the Commands block.

  The read-only wording stays: phase 1 writes nothing.
- Modify `docs/configuration.md`: a "Token scopes" paragraph saying an
  environment token needs `settings.read` for the settings commands.
- Modify `skills/dtmgd/SKILL.md`:
  - a "Settings" section of at most 12 lines, with the four commands, and
    `--effective` for "what applies to this host";
  - a References bullet for `references/settings.md`.
- Create `skills/dtmgd/references/settings.md`:
  - finding a schema (`--search`), reading stored objects versus effective
    values, and scopes (`environment`, `HOST_GROUP-…`, `HOST-…`,
    `ENVIRONMENT_ACTIVE_GATE-…`);
  - the three update schemas, their fields and mode names (`_MW` versus
    `_UW`), and window references by object ID;
  - inheritance (host → host group → environment → default; `origin`);
  - the token scope;
  - a recipe: "which OneAgent update mode applies to host X". Find the
    host's ID with `get entities --selector 'type(HOST),entityName("…")'`,
    then run `get settings --effective --scope HOST-… --schema
    builtin:deployment.oneagent.updates`.
- Modify `.claude/CLAUDE.md`: one knowledge-base entry, "Settings 2.0
  reads: paging, fields, effective values, update-schema quirks". It holds
  the Global Constraints facts above, plus "an update-window delete is not
  blocked by references, so tables must tolerate unknown window IDs".

**Interfaces:**
- Consumes: the command names and flags from Tasks 1–4.
- Produces: nothing code depends on.

- [ ] **Step 1: Write the docs and the skill reference** as listed above.
  Use synthetic IDs only (`HOST-0000000000000001`).

- [ ] **Step 2: Check the skill against the CLI**

Run:
```bash
catalog=$(mktemp) && go run . commands --brief -o json --no-agent > "$catalog"
grep -ohE 'dtmgd (get|describe|query) [a-z-]+' skills/dtmgd/SKILL.md skills/dtmgd/references/*.md | sort -u \
  | while read -r _ verb res; do jq -e --arg v "$verb" --arg r "$res" '.verbs[$v] | index($r)' "$catalog" >/dev/null || echo "MISSING: $verb $res"; done
grep -ohE -- '--[a-z][a-z-]+' skills/dtmgd/references/settings.md | sort -u \
  | while read -r f; do grep -rq -- "$f" docs/reference || echo "UNKNOWN FLAG: $f"; done
```
Expected: no `MISSING` or `UNKNOWN FLAG` lines.

- [ ] **Step 3: Run the guards**

Run: `go test ./... -count=1 && make lint`
Expected: PASS (`TestEveryTopLevelCommandIsInTheREADME`, the embed test
and the reference test included).

- [ ] **Step 4: Commit**

```bash
git add README.md docs skills
git add -f .claude/CLAUDE.md
git commit -m "docs: reading Settings 2.0 in the README, skill and knowledge base"
```

---

### Task 6: Live read checks

**Files:**
- Create: `test/integration/settings_test.go` (build tag `integration`)
- Modify: `.integrationtests.env.example` only if a new setting is needed
  (none is planned).

**Interfaces:**
- Consumes: `run`, `decode`, `items`, `brief`, `env(t)` from the existing
  suite. Every check uses `--context env(t)` and read verbs only.

- [ ] **Step 1: Write the checks:**

```go
func TestSettingsSchemasIncludeTheUpdateSchemas(t *testing.T)
// `get settings-schemas --search deployment -o json`: items' schemaIds include all three update schemas
func TestOneAgentUpdatesSchemaDescribes(t *testing.T)
// `describe settings-schema builtin:deployment.oneagent.updates -o json`: has properties.updateMode,
// allowedScopes contains "HOST" (report the allowedScopes values on failure: they are not secret)
func TestEffectiveOneAgentUpdatesAtEnvironment(t *testing.T)
// `get settings --effective --scope environment --schema builtin:deployment.oneagent.updates -o json`:
// exactly 1 item, value.updateMode non-empty
func TestUpdateWindowsDecode(t *testing.T)
// `get settings --schema builtin:deployment.management.update-windows --scope environment -o json`:
// every item's value.recurrence is one of ONCE, DAILY, WEEKLY, MONTHLY (an empty list passes)
func TestSettingsTablesRender(t *testing.T)
// the three update schemas in table form (`--scope environment`), and --effective for OneAgent: exit 0 each, failures via brief(r)
```

- [ ] **Step 2: Vet them offline**

Run: `go vet -tags integration ./test/integration/ && go test -tags integration ./test/integration/ -count=1`
Expected: vet clean. The tests all **skip** (no `DTMGD_IT_CONFIG` in the
environment), and the offline self-tests pass.

- [ ] **Step 3: Commit**

```bash
git add test/integration/settings_test.go
git commit -m "test: live read checks for Settings 2.0"
```

- [ ] **Step 4: The live run.** Ask the maintainer for a go-ahead, naming
  the contexts (`prod`, `stage`). Then run `make integration` with
  `DTMGD_IT_ENV` set to each. Report pass, skip and fail counts only. If a
  table check fails on real data, the fix gets its own fixture and failing
  test first.
