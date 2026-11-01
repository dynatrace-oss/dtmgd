# Settings 2.0 for Any Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the gaps the 2026-11-01 live tests found, so an agent can safely read and write any Settings 2.0 schema:
- secrets are never printed, and a masked secret is never copied;
- positions in ordered lists;
- a delete check that sees references from any schema;
- the cluster fixes and the smaller fixes;
- documentation about any schema;
- live tests that can be repeated.

**Architecture:**
- **New helpers in `cmd/`:**
  - `settings_secrets.go`: walk a value by its schema to find secret properties.
  - `settings_position.go`: parse `position` and place an object in a scope's order.
  - `settings_refs.go`: read every schema definition and find the settings objects that reference an object.
- **Where they plug in:**
  - The write runner (`cmd/writes.go`) guards secrets after every plan and carries the position fields to the request bodies.
  - `apply` and `create` plan positions.
  - `delete` calls the reference check.
  - `get settings` gains a fallback for scope-only listings on clusters.
  - Hints change in `cmd/cluster.go`, `cmd/errors.go` and `pkg/client`.

**Tech Stack:** Go 1.26, Cobra, resty. Tests use the `cmd/harness_test.go` fake API and golden files. The live suite in `test/integration` runs with build tag `integration`.

**Spec:** `docs/superpowers/specs/2026-11-01-settings-any-schema-design.md`, approved 2026-11-01. Read its "Facts the design rests on" before starting.

## Global Constraints

**Rules from the spec:**
- Public APIs only. Not built: settings history, per-object permissions, the Configuration API (v1).
- Every write goes through `a.runWrites`. Write levels and token scopes stay the only permission controls. A write is never resent.
- Every change works on environment and cluster contexts alike. Settings calls go through `cmd/settings_api.go` (`settingsGet`, `settingsPaged`, `settingsPost`, `settingsPut`, `settingsDelete`), never `GetV2` directly.
- json, yaml and agent output of reads stay the API's own response. Only human output, hints and write reports change.
- New refusals are errors with a hint (`withHint`). Agent mode turns hints into `suggestions`.

**Repository rules:**
- No real host names or environment IDs in repository files: use `example.com` hosts and synthetic IDs. Tokens are never printed.
- Tests never use the default config path: `runCLI` with a temp config.
- HTTP error cases in tests use 400, 403 or 404, never 5xx or 429.
- `gocognit` stays at 15 (non-test code; keep tests under it with helpers). The coverage gate stays at 60%.

**Exact values the spec fixes:**
- **Masks.** The cluster's mask is `^\*\*\*[0-9a-f]{16}\*\*\*$`. The report placeholder is `<secret>`.
- **`position` values:**
  - `first`;
  - `last`;
  - `after:<ref>`;
  - `before:<ref>`.

  `<ref>` is an `objectId` or an `externalId`.
- **Parallelism.** The delete check reads definitions and the cluster fallback reads schemas 8 at a time.
- **References.** A refusal lists at most 10 referencing objects, plus the total.

**Live calls:**
- Reads may go to `prod`, `stage`, `prod-cluster` and `stage-cluster`.
- Writes go only to `stage` and `stage-cluster`, through a temporary config copy, and only to `dtmgd-it-…` objects.
- Filter all output: no hosts, IPs, emails, license keys or tokens.

**Commits** end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

**Commands:**
- Lint: `PATH=$HOME/.local/bin:$PATH make lint`.
- Unit tests: `go test ./... -count=1`.
- Regenerate a golden file: `go test ./cmd/ -run <Test> -update`, then read the diff.

## Review Focus

1. **The anchor exists in one environment of `--env "a;b"` and not in the other.** The plan must fail before anything is written anywhere; today's runner plans every context first. Test: Task 2, `TestPositionAnchorMissingInOneContextWritesNothing`.
2. **An update changes the value and the position at once.** One PUT must carry both `value` and the position field, not two requests. Test: Task 2, `TestPositionMoveAndValueChangeInOnePut`.
3. **The referencing objects span several pages.** A reference on page 2 must still block the delete. Test: Task 4, `TestDeleteFindsAReferenceOnALaterPage`.
4. **A scope-only cluster listing falls back with `--limit` below the merged total.** The truncation note must say more exist. Test: Task 5, `TestClusterScopeFallbackTruncationIsNotSilent`.
5. **A summary holds a Windows path or a literal backslash.** Only a backslash before ASCII punctuation is dropped, so `C:\Program Files` stays as it is and `\\` becomes `\`. Test: Task 6, `TestUnescapeMarkdown`.

---

### Task 1: Secrets: never print them, refuse a masked value in a create

**Files:**
- Create: `cmd/settings_secrets.go`
- Create: `cmd/settings_secrets_test.go`
- Modify: `cmd/create.go`. Extend `settingsSchemaShape` (lines 174-177).
- Modify: `cmd/writes.go`. Add a `plannedWrite` field, call the guard in the plan closure (around line 300), and change `plannedResults` (line 422).

**Interfaces:**
- Produces:
  - `settingsSchemaShape` gains `Ordered bool \`json:"ordered"\``, `Properties map[string]schemaProperty \`json:"properties"\`` and `Types map[string]struct{ Properties map[string]schemaProperty \`json:"properties"\` } \`json:"types"\``. It still decodes from `GET …/settings/schemas/{id}` through `schemaShape(c, id)`.
  - `func (s settingsSchemaShape) hasSecrets() bool`
  - `func maskSecrets(s settingsSchemaShape, value map[string]interface{}) map[string]interface{}`: a deep copy in which every non-nil secret value is `"<secret>"`. A nil value stays nil.
  - `func maskedSecretPaths(s settingsSchemaShape, value map[string]interface{}) []string`: paths such as `webHookNotification.headers[0].secretValue` whose secret value matches the cluster's mask. Sorted.
  - `func guardSecrets(c *client.Client, ws []plannedWrite, dryRun bool) error`, which works in place:
    - It skips writes with `Exec != nil` or an empty `SchemaID`.
    - It reads a schema only when a write needs it, and each schema once. A write needs it when it is a create, or when this is a dry run and the write has a `Value` or `Current`. A real update or delete reads nothing more.
    - It refuses a create (`Op == config.OpCreate`) that has masked paths.
    - When `dryRun`, it sets `ws[i].Current = maskSecrets(shape, Current)` and `ws[i].ReportValue = maskSecrets(shape, Value)`.
  - `plannedWrite.ReportValue map[string]interface{}`: what a dry-run report shows as `value`.

**How the walk finds secrets** (one walker serves both functions; it is not part of the interface):
- A property whose type is the plain name `secret` is a secret.
- A property whose type is `{"$ref":"#/types/X"}` holding an object: recurse into `Types[X].Properties`.
- A `list` or `set` property: each item, by `Items.Type`, which is `secret` or a `$ref` to a type.
- Anything else, including enum refs and properties missing from the value, is skipped.

- [ ] **Step 1: Write the failing tests** in `cmd/settings_secrets_test.go`. The fixture `notificationShape()` decodes this JSON into `settingsSchemaShape`:

  ```json
  {"properties":{"type":{"type":"text"},"webHookNotification":{"type":{"$ref":"#/types/WebHook"}}},
   "types":{"WebHook":{"properties":{"url":{"type":"text"},"secretUrl":{"type":"secret"},
            "headers":{"type":"set","items":{"type":{"$ref":"#/types/Header"}}}}},
            "Header":{"properties":{"name":{"type":"text"},"secretValue":{"type":"secret"}}}}}
  ```

  ```go
  func TestMaskSecretsReplacesEverySecretAndCopies(t *testing.T) {
      v := map[string]interface{}{"type": "WEBHOOK", "webHookNotification": map[string]interface{}{
          "url": "https://example.invalid/x", "secretUrl": "s3cret",
          "headers": []interface{}{map[string]interface{}{"name": "X", "secretValue": "real"}}}}
      got := maskSecrets(notificationShape(), v)
      // got: secretUrl and headers[0].secretValue == "<secret>"; url and name unchanged
      // v (the input) still holds "s3cret" and "real"
  }
  func TestMaskedSecretPaths(t *testing.T) {
      // secretValue "***3f9ff26c722fbb93***" and secretUrl "s3cret":
      // want []string{"webHookNotification.headers[0].secretValue"}
      // a non-secret property holding a mask-shaped string is not reported
  }
  func TestMaskSecretsWithoutSecretsIsAnEqualCopy(t *testing.T) // a shape with no secret: reflect.DeepEqual(got, v)
  ```

  Harness tests: a fake with the definition at `"GET /e/env1/api/v2/settings/schemas/builtin:problem.notifications"`, the listing, `POST /e/env1/api/v2/settings/objects` and a PUT route.
  - `TestCreateRefusesAMaskedSecret`: `create settings` with the masked header. Want exit 2, stderr names `webHookNotification.headers[0].secretValue`, the hint contains "Exports mask secrets", and `api.Hits(postObjects) == 0`. With `--dry-run`: also exit 2.
  - `TestApplyRefusesAMaskedSecretInACreate`: two cases, both exit 2 and no POST.
    - an apply file without `objectId`;
    - an apply file whose `objectId` answers 404 (it becomes a create).
  - `TestApplyMaskedSecretUpdatePasses`: the object exists and its value differs. Want exit 0, `updated`, and the PUT body carries the mask unchanged.
  - `TestEmptySecretCreatePasses`: `secretValue: ""`. Want exit 0, `created`.
  - `TestDryRunNeverPrintsASecret`: an apply file whose `secretValue` is `real-value`, with `--dry-run -o json`, with `-o yaml`, and in agent mode (`runCLIWithEnv` with `CLAUDECODE=1`). Stdout never contains `real-value`, and `results[0].value…secretValue == "<secret>"`. `current`'s mask shows as `<secret>` too.
  - `TestRealWriteSendsTheRealSecret`: the same file without `--dry-run`. The PUT body (`api.Bodies`) contains `real-value`, and stdout does not.

- [ ] **Step 2: Run them.** `go test ./cmd/ -run 'TestMaskSecrets|TestMaskedSecretPaths|Secret' -count=1`. Expected: FAIL. The functions are undefined (compile error), then the assertions fail.

- [ ] **Step 3: Implement.**
  - Write the walker, `maskSecrets` and `maskedSecretPaths` in `cmd/settings_secrets.go`, and extend `settingsSchemaShape`.
  - Write `guardSecrets` with the refusal:
    - error `withHint(&UsageError{Err: fmt.Errorf("creating a %s object at %s: %s hold(s) a masked secret (***…***), not the secret itself", schemaID, scope, strings.Join(paths, ", "))}, "Exports mask secrets. Put the real value in the file; dtmgd never prints it.")`;
    - the schema read reuses `schemaShape`.
  - Call `guardSecrets(c, p.writes, s.DryRun)` in `planContexts`'s closure right after `s.Plan`. A guard error fails that context's plan, so nothing is written anywhere.
  - In `plannedResults`, set `r.Value = w.ReportValue` (no fallback to `w.Value`).

- [ ] **Step 4: Run.** `go test ./cmd/ -count=1`. Expected: PASS. Existing dry-run tests whose fake has no schema definition now fail at `guardSecrets`. Give them the definition route (`{"multiObject":true}` and so on) rather than skipping the guard on a 404.

- [ ] **Step 5: Lint and commit.**

  ```bash
  PATH=$HOME/.local/bin:$PATH make lint
  git add cmd/settings_secrets.go cmd/settings_secrets_test.go cmd/create.go cmd/writes.go cmd/*_test.go
  git commit -m "feat(settings): never print secrets in write reports; refuse creating from a masked secret"
  ```

### Task 2: Ordered lists: `position` with anchors that exist

**Files:**
- Create: `cmd/settings_position.go`
- Create: `cmd/settings_position_test.go`
- Modify: `cmd/apply.go`. Add `applyObject.Position` and `check()`, the planner's order cache, and the position handling in `plan`, `planByLookup` and `updateOrUnchanged`.
- Modify: `cmd/create.go`. Add the `--position` flag and change `planCreate`'s signature.
- Modify: `cmd/writes.go`. Add the `plannedWrite` and `writeResult` fields, the `executeWrite` bodies, and `result()`.

**Interfaces:**
- Consumes: `settingsSchemaShape.Ordered` (Task 1).
- Produces:
  - `type settingsPosition struct{ Kind, Ref string }`, with Kind `"first"`, `"last"`, `"after"`, `"before"`, or `""` for no position.
  - `func parsePosition(s string) (settingsPosition, error)`:
    - `""` gives the zero value;
    - `after:`/`before:` need a non-empty ref, which is everything after the first colon;
    - anything else is an error naming the four forms.
  - `type orderEntry struct{ ObjectID, ExternalID string; Planned int }`: one object in a scope's order. `Planned` is 0 for an existing object, or the 1-based plan index of a create (Task 3).
  - `type placement struct{ InsertAfter, InsertBefore *string; Current, New int; AnchorPlanned int }`: what to send and the 1-based places. `Current` is 0 for a create. `AnchorPlanned` is 0, or the plan index whose objectId fills the anchor at execution (Task 3).
  - `func placeObject(order []orderEntry, selfID string, pos settingsPosition) (placement, error)`:
    - `selfID == ""` means a create.
    - A ref resolves by `ObjectID` first, then `ExternalID`.
    - A ref resolving to nothing gives `withHint(fmt.Errorf("position %s: no object %q at this scope", …), "List the objects and their order: dtmgd get settings --schema <schema-id> --scope <scope>")`.
    - A ref naming self gives a `UsageError`.
  - `plannedWrite` gains `InsertAfter, InsertBefore *string` and `Position *positionChange`.
  - `type positionChange struct{ Current int \`json:"current,omitempty" yaml:"current,omitempty"\`; New int \`json:"new" yaml:"new"\` }`.
  - `writeResult.Position *positionChange \`json:"position,omitempty" yaml:"position,omitempty"\``, set by `plannedWrite.result()`.
  - `planCreate(c *client.Client, schemaID, scope string, value map[string]interface{}, pos settingsPosition)`.
  - The apply file key `position`; the create flag `--position`.

**What `placeObject` sends** (spec table; `pred(X)` is X's predecessor in `order`, ignoring self):

| Kind | Create | Update |
|---|---|---|
| first | `InsertAfter=""` | `InsertAfter=""` |
| last | nothing | `InsertBefore=""` |
| after:X | `InsertAfter=X` | `InsertAfter=X` |
| before:X | `InsertAfter=pred(X)`, or `""` if X is first | `InsertBefore=X` |

`New` comes from simulating: remove self from `order`, then insert at the position. Without a position, a create is last and an update keeps its place.

**Planning:**
- `position` on a schema whose shape is not `Ordered` gives `UsageError`: `position works only on ordered schemas; %s is not ordered`.
- The apply planner keeps `order map[string][]orderEntry`, keyed `schemaID+"\x00"+scope`, initialised from `p.list` in listing order. It is separate from `p.listed`, which the lookups use.
- An object with `objectId` and a position reads the shape and the listing at `cur.Scope`.
- `updateOrUnchanged` gains the placement:
  - equal values and `New == Current` stay `unchanged`, with no position fields sent;
  - otherwise the write is an update carrying the position fields.

- [ ] **Step 1: Write the failing tests.**

  Unit tests in `cmd/settings_position_test.go`:

  ```go
  func TestParsePosition(t *testing.T) {
      // "" -> {}; "first" -> {first,""}; "after:abc:def" -> {after,"abc:def"}
      // errors: "middle", "after:", "before"
  }
  func TestPlaceObject(t *testing.T) {
      order := []orderEntry{{ObjectID: "o1", ExternalID: "x1"}, {ObjectID: "o2"}, {ObjectID: "o3"}}
      // create first          -> InsertAfter "" , New 1
      // create last           -> no fields, New 4
      // create before:o1      -> InsertAfter "" , New 1
      // create before:o3      -> InsertAfter "o2", New 3
      // create after:x1       -> InsertAfter "o1", New 2  (externalId ref)
      // update o3 first       -> InsertAfter "", Current 3, New 1
      // update o1 last        -> InsertBefore "", Current 1, New 3
      // update o2 before:o1   -> InsertBefore "o1", Current 2, New 1
      // update o1 after:o1    -> UsageError; create after:nope -> error with the listing hint
  }
  ```

  Harness tests: an ordered schema `builtin:test.ordered` with definition `{"multiObject":true,"ordered":true}` and a listing of three objects.
  - `TestApplyPositionBodies`: per position, the POST body (`api.Bodies(postObjects)`) holds exactly the `insertAfter` above. A PUT body holds `insertAfter` or `insertBefore`, and never both.
  - `TestApplyPositionUnchangedPlaceIsUnchanged`: same value, `position: first` for the object already first. Want `unchanged`, no PUT.
  - `TestApplyPositionMoveWithSameValueIsUpdate`: same value, a new place. Want `update` in the dry run, with `results[0].position == {current: 3, new: 1}`, and one PUT.
  - `TestPositionMoveAndValueChangeInOnePut` (Review Focus 2): one PUT, whose body has `value` and `insertAfter`.
  - `TestPositionRefusedOnUnorderedSchema`: exit 2, no POST or PUT.
  - `TestCreateSettingsPositionFlag`: `--position after:<externalId>` sends `insertAfter: <objectId>`.
  - `TestPositionAnchorMissingInOneContextWritesNothing` (Review Focus 1): two writable contexts, the anchor listed in one only. Want exit 1, and zero POST/PUT hits on both fakes.
  - `TestApplyWithoutPositionSendsNoPositionFields`: existing behaviour pinned. No `insertAfter` or `insertBefore` key in any body.

- [ ] **Step 2: Run them.** `go test ./cmd/ -run 'Position|PlaceObject' -count=1`. Expected: FAIL (undefined, then the assertions).

- [ ] **Step 3: Implement** `parsePosition` and `placeObject` in `cmd/settings_position.go`.
  - `applyObject.check()` runs `parsePosition`; an error is a `UsageError` naming the object index.
  - Wire the planner and `planCreate`.
  - In `executeWrite`, a create sets `obj["insertAfter"]` when `w.InsertAfter != nil`. An update sets `body["insertAfter"]` or `body["insertBefore"]` when non-nil.

- [ ] **Step 4: Run.** `go test ./cmd/ -count=1`. Expected: PASS. Run `make docs` (the `create settings` flag changed). The stale-reference test must pass.

- [ ] **Step 5: Lint and commit.**

  ```bash
  git add cmd/settings_position*.go cmd/apply.go cmd/create.go cmd/writes.go cmd/*_test.go docs/reference
  git commit -m "feat(settings): position objects in ordered schemas (first, last, before/after an object)"
  ```

### Task 3: Ordered lists: anchors created earlier in the same file

**Files:**
- Modify: `cmd/apply.go`. The planner's virtual order includes planned creates.
- Modify: `cmd/settings_position.go`. `placeObject` handles planned entries.
- Modify: `cmd/writes.go`. `executeWrites` resolves anchors; add `writeResult.Note`.
- Modify: `cmd/harness_test.go`. `route` gains `Seq []string`: bodies served in order, the last one repeating, so one route can answer two POSTs differently.
- Test: `cmd/settings_position_test.go`

**Interfaces:**
- Consumes: `orderEntry.Planned`, `placement.AnchorPlanned` and `plannedWrite.InsertAfter`/`InsertBefore` (Task 2).
- Produces:
  - `plannedWrite.anchorFrom int` (1-based plan index whose result fills the anchor; 0 = none);
  - `plannedWrite.anchorBefore bool` (true: the result fills `InsertBefore`; false: `InsertAfter`);
  - `writeResult.Note string \`json:"note,omitempty" yaml:"note,omitempty"\``.

**Rules:**
- After planning a create or a move in an ordered schema, the planner updates its virtual order:
  - a create inserts `orderEntry{ExternalID: o.ExternalID, Planned: i+1}` at its place;
  - a move repositions the entry.
- A later `after:X`/`before:X` whose X resolves to an entry with `Planned > 0` sets `anchorFrom` and leaves `InsertAfter`/`InsertBefore` nil. For a create, `before:X` resolves to `pred(X)`: an existing ID, a planned entry, or `""` (first).
- In `executeWrites`, each context keeps the objectId of each executed write by plan index. Before sending a write with `anchorFrom > 0`:
  - if that write has no objectId (it failed), the result is `failed` with `writeError{Message: "its position anchor was not written"}`, and nothing is sent;
  - under `--validate-only`, the write goes without its position, and `Note` is `validated without its position: its anchor is created by this file`;
  - otherwise the field is filled with that objectId.
- The dry-run `position.new` values come from the virtual order, so they are the places after the whole file.

- [ ] **Step 1: Write the failing tests.**
  - `TestPositionAnchorEarlierInFile`: the file creates A (`externalId: a`, `position: first`), then B (`position: after:a`). The POST route uses `Seq` to answer `new-a`, then `new-b`. Want the second POST body `insertAfter == "new-a"`, and dry-run positions 1 and 2.
  - `TestPositionAnchorNotWrittenFailsOnlyItsDependent`: the first POST answers an item with code 400. Want results `failed`, `failed`, with the second's message "its position anchor was not written", and exactly one POST.
  - `TestPositionAnchorUnderValidateOnly`: `--validate-only` (the fake serves a cluster version ≥ 1.288). Want the second POST body without `insertAfter`, and `results[1].note` set.

- [ ] **Step 2: Run them.** `go test ./cmd/ -run 'PositionAnchor' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** as in Rules.
- [ ] **Step 4: Run.** `go test ./cmd/ -count=1`. Expected: PASS.
- [ ] **Step 5: Lint and commit.** `git commit -m "feat(settings): a position may name an object created earlier in the same file"`

### Task 4: References: refuse to delete what other settings use

**Files:**
- Create: `cmd/settings_refs.go`
- Create: `cmd/settings_refs_test.go`
- Modify: `cmd/delete.go`.
  - `planDelete` calls the check for every object.
  - Remove `windowReferences`, `referencesWindow` and `windowInUse`.
  - Keep `clusterWindowReferences` and the ActiveGate check in `checkWindowUnused` for cluster contexts.
  - Update the command's `Long` text.
- Modify: `cmd/writes.go`.
  - `plannedWrite.Warnings []string`.
  - `writeReport.Warnings []string \`json:"warnings,omitempty" yaml:"warnings,omitempty"\``, collected from every context's plan as `"<context>: <warning>"`.
  - `renderWriteReport` prints each with `output.PrintWarning` outside agent mode.
- Modify: `cmd/delete_test.go`, plus the cluster delete tests that now need the schema catalog.

**Interfaces:**
- Produces:
  - `func schemaDefinitions(c *client.Client) (defs map[string]json.RawMessage, warnings []string, err error)`:
    - `GET settingsSchemasPath`, then every `…/schemas/{id}`, 8 at a time.
    - A 404 skips that schema, with the warning `schema <id> could not be read (404); the delete check skipped it`.
    - A 403 is `withHint(err, "Reading settings schemas needs settings.read.")`.
    - Any other error is returned.
  - `func referencingSchemas(defs map[string]json.RawMessage, target string) []string`: sorted IDs of the schemas in which some JSON object has `"type":"setting"` and `"referencedType": target`, at any depth.
  - `func findReferences(c *client.Client, objectID string, schemas []string) ([]settingsObject, error)`:
    - `settingsPaged` with `schemaIds` (the sorted IDs, comma-joined), `fields=objectId,schemaId,scope,summary,value` and `pageSize=500`, without `scopes`;
    - it keeps objects whose value holds a string equal to `objectID` at any depth;
    - a 403 gets the same hint as in `schemaDefinitions`.
  - `func checkUnreferenced(c *client.Client, cur settingsObject) (warnings []string, err error)`. The refusal is a plain error (exit 1):
    - message `%s object %s (%s) is still used by %d settings object(s): %s`, listing at most 10 entries as `<schemaId> at <scope> (<objectId>, <summary>)` with `, …` when more;
    - hint "Change or delete those settings first. Settings only: dashboards and other configuration outside Settings 2.0 are not checked."

- [ ] **Step 1: Write the failing tests.**

  Unit tests:

  ```go
  func TestReferencingSchemas(t *testing.T) {
      defs := map[string]json.RawMessage{
          "builtin:problem.notifications": json.RawMessage(`{"properties":{"alertingProfile":{"type":"setting","referencedType":"builtin:alerting.profile"}}}`),
          "builtin:x.nested": json.RawMessage(`{"types":{"F":{"properties":{"mz":{"type":"list","items":{"type":"setting","referencedType":"builtin:alerting.profile"}}}}}}`),
          "builtin:other": json.RawMessage(`{"properties":{"a":{"type":"setting","referencedType":"builtin:management-zones"}}}`),
      }
      // want []string{"builtin:problem.notifications", "builtin:x.nested"}
  }
  ```

  Harness tests. A helper `schemaCatalog(prefix string, defs map[string]string) map[string]route` builds the list route (`{"items":[{"schemaId":…}…],"totalCount":n}`) and one definition route per schema. Use it in `deleteFake` and in the cluster delete fakes.
  - `TestDeleteRefusesAReferencedObject`: deleting an alerting profile that a notification references. Want exit 1, stderr names the notification's schema and objectId, the hint is as above, and zero DELETE hits. `--dry-run` refuses too.
  - `TestDeleteUnreferencedObject`: exit 0, one DELETE.
  - `TestDeleteNoReferencingSchemaListsNothing`: no schema references the target. Want no objects listing request (`api.Hits` on the listing route is 0).
  - `TestDeleteFindsAReferenceOnALaterPage` (Review Focus 3): the listing page 1 answers with `nextPageKey`, and page 2 holds the reference. Want refused.
  - `TestDeleteRefusalListsAtMostTen`: 12 references. Want 10 listed and the message says 12.
  - `TestDeleteSkipsAnUnreadableDefinition`: one definition answers 404. Want exit 0, and stderr (`-o json`: `warnings`) names that schema.
  - `TestDeleteFailsWhenTheScanFails`: the referencing listing answers 400 or 403. Want a non-zero exit, no DELETE, and the 403 hint names `settings.read`.
  - `TestDeleteRefusesAReferencedWindow` (existing, rewritten): the OneAgent update definition declares the `setting` reference to update windows. Want refused through the generic check.
  - The cluster window lifecycle tests keep passing with the catalog added: the ActiveGate check stays.

- [ ] **Step 2: Run them.** `go test ./cmd/ -run 'Delete|ReferencingSchemas' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** `cmd/settings_refs.go`, then wire `planDelete` and the warnings. Use a bounded worker pool (`sync.WaitGroup` + a channel of 8) for the definitions, and keep results in a map under a mutex.
- [ ] **Step 4: Run.** `go test ./cmd/ -count=1`, then `make docs` (the delete help changed). Expected: PASS.
- [ ] **Step 5: Lint and commit.** `git commit -m "feat(settings): delete refuses an object that other settings reference, for every schema"`

### Task 5: Cluster contexts: scope-only listing fallback, wrong-host hint

**Files:**
- Modify: `pkg/client/errors.go`. Add `func (e *ErrAPI) Message() string`, which returns `apiErrorMessage(e.Body)`.
- Modify: `cmd/settings_api.go`. Add `clusterScopeFallback`.
- Modify: `cmd/get_settings.go`. In `settingsFetch`, wrap the stored and effective listings when no `--schema` is given.
- Modify: `cmd/cluster.go`. The `clusterHint` 404 branch.
- Modify: `cmd/harness_test.go`. Add a `"*"` route that answers any unmatched request.
- Modify: `cmd/get_environments_test.go`. `TestClusterCommand404Hint` and `TestClusterCommand404HintNamesHostEverywhere` use `"*": wrongHost404`.
- Test: `cmd/settings_cluster_test.go`

**Interfaces:**
- Consumes: `schemaCatalog` (Task 4's test helper), for the cluster's schema list in the fallback tests.
- Produces:
  - `(*client.ErrAPI).Message() string` (Task 6 uses it).
  - `func clusterScopeFallback(fetch func(*client.Client, int) (fetched, error), path string, params map[string]string, maxPages int) func(*client.Client, int) (fetched, error)`.
  - The test helpers: `route` key `"*"`, and `var wrongHost404 = route{Status: 404, Body: \`{"code":404}\`}` (a JSON body without an `error` envelope, as a wrong host sends).

**Fallback rules:**
- **When it runs:** only when `c.ClusterScoped()`, the wrapped fetch failed with 404, and `Message()` starts with `No schema with schemaId`.
- **What it does:**
  1. `GET settingsSchemasPath`.
  2. For each schema ID, 8 at a time, call `settingsPaged(c, path, params+{"schemaIds": id}, …)` with the same page cap. A 404 skips that schema; any other error fails.
  3. Concatenate the items in schema ID order, keeping each schema's own order.
- **Paging and limits:** `Total` is the item count when no schema had more; otherwise `-1`. `HasMore` is true when any schema had more, or when the merged list exceeds `limit` (then truncate to `limit`).
- Environment contexts never take the fallback.

**Hint rule:** the 404 branch adds `hintNoClusterAPI` only when `Message()` is `""` or `"HTTP 404 Not Found"`. Otherwise:
- the command's own `NotFound` hint applies alone, when it has one;
- with none, the error passes through, and the generic diagnosis applies.

- [ ] **Step 1: Write the failing tests.**
  - `TestClusterScopeFallbackMergesSchemas`: the scope-only route answers 404 `{"error":{"code":404,"message":"No schema with schemaId builtin:accounting.ddu.limit"}}`. The catalog lists `builtin:b`, `builtin:a`, `builtin:c`; `a` and `b` answer items, `c` answers 404 "No schema with topic identifier". Want exit 0, items in the order a then b, and `-o json` totalCount 3 (or the list length).
  - `TestClusterScopeFallbackForEffectiveValues`: the same through `--effective --scope cluster`.
  - `TestClusterScopeFallbackTruncationIsNotSilent` (Review Focus 4): `--limit 1` with 3 merged items. Want 1 shown and the truncation note on stderr.
  - `TestEnvironmentScopeListingHasNoFallback`: an environment context with the same 404. Want exit 4, and no request to `/settings/schemas`.
  - `TestClusterSettings404WithMessageHasNoHostHint`: `describe settings bad` answering a 404 whose message is "Settings objectId 'bad' not found". Stderr has the object hint and does not contain `port 9999`.
  - Update `TestClusterCommand404Hint` and `TestClusterCommand404HintNamesHostEverywhere` to `map[string]route{"*": wrongHost404}`. They pass unchanged otherwise. `TestClusterSettings404KeepsTheirHint` keeps passing.

- [ ] **Step 2: Run them.** `go test ./cmd/ -run 'ClusterScopeFallback|EnvironmentScopeListing|404' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** as above.
- [ ] **Step 4: Run.** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Lint and commit.** `git commit -m "fix(cluster): list settings by scope alone despite the cluster's 404; wrong-host hint only for a wrong host"`

### Task 6: Smaller fixes: "Setting not applicable", summary escapes, describe settings-schema

**Files:**
- Modify: `pkg/client/diagnostic.go`. Add `IsNotApplicable`, and make `Diagnose` use it.
- Modify: `cmd/errors.go`. `reportError` skips the scopes line for it.
- Modify: `cmd/cluster.go`. The `clusterHint` 403 branch skips `s.Forbidden` for it.
- Modify: `cmd/settings_views.go`. Add `unescapeMarkdown`; use it in the generic rows (lines 143, 151) and the effective rows (line 301).
- Modify: `cmd/writes.go`. Write report rows (lines 652, 658).
- Modify: `cmd/delete.go`. `deletePrompt`.
- Modify: `cmd/describe_settings_schema.go`.
  - `settingsSchemaDetail` gains `Maturity`, `KeyProperty`.
  - `schemaProperty` and its `Items` gain `ReferencedType string \`json:"referencedType"\``.
  - `printSettingsSchemaDetail` prints the new lines; `propertyType` appends the reference.
- Tests: `pkg/client/diagnostic_test.go`, `cmd/settings_views_test.go` (or the existing views test file), `cmd/describe_settings_schema_test.go`, and goldens.

**Interfaces:**
- Consumes: `(*client.ErrAPI).Message()` (Task 5).
- Produces:
  - `func IsNotApplicable(err error) bool`: true for an `*ErrAPI` with status 403 whose `Message()` starts with `Setting not applicable`.
  - `func unescapeMarkdown(s string) string`: drops a backslash that precedes an ASCII punctuation character; anything else is kept.

**Exact copy:**
- The not-applicable hint: `The cluster says this setting does not apply to this scope (its reason is above). Check the scopes with dtmgd describe settings-schema <schema-id>, or read the setting at another scope.` It replaces the 403 hint; the exit code stays 5.
- `describe settings-schema` human lines, after `Objects:`:
  - `Maturity:     <value>`, when non-empty;
  - `Key property: <name>`, when non-empty.
- The TYPE cell for a reference is the existing text plus ` → <referencedType>`: `setting → builtin:alerting.profile`, `list of setting → builtin:management-zones`.

- [ ] **Step 1: Write the failing tests.**

  ```go
  func TestIsNotApplicable(t *testing.T) {
      // APIError(403, `{"error":{"code":403,"message":"Setting not applicable: 'x'"}}`) -> true
      // APIError(403, `{"error":{"code":403,"message":"Token is missing required scope"}}`) -> false
      // APIError(404, same not-applicable body) -> false
  }
  func TestUnescapeMarkdown(t *testing.T) { // Review Focus 5
      // `dtmgd\-it\-team` -> `dtmgd-it-team`; `a\.b\_c\*` -> `a.b_c*`; `\\` -> `\`
      // `C:\Program Files` -> unchanged; `trailing\` -> unchanged
  }
  ```

  Harness tests:
  - `TestNotApplicable403HasItsOwnHint`: `get settings --effective --scope HOST-0000000000000001 --schema builtin:x` answering that 403. Want exit 5, stderr has "does not apply to this scope", and lacks both "Permission denied" and "This command needs the token scope(s)". In agent mode, `suggestions` carry the same hint.
  - `TestClusterNotApplicable403SkipsTheScopeHint`: the same on a cluster context. Stderr lacks the cluster settings-scopes hint.
  - `TestTablesUnescapeSummaries`: a listing whose summary is `dtmgd\\-it\\-team` (JSON-escaped). The table shows `dtmgd-it-team`, and `-o json` keeps `dtmgd\\-it\\-team`.
  - `TestDescribeSchemaShowsMaturityKeyAndReferences`: a definition with `maturity`, `keyProperty` and a `setting` property, checked through `assertGolden`.

- [ ] **Step 2: Run them.** `go test ./pkg/client/ ./cmd/ -run 'NotApplicable|Unescape|DescribeSchemaShows' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** Regenerate the affected goldens with `-update` and read every diff: only summaries, the describe lines and the hints may change.
- [ ] **Step 4: Run.** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Lint and commit.** `git commit -m "fix(settings): not-applicable 403 hint, unescaped summaries, maturity/key/references in describe settings-schema"`

### Task 7: Documentation about any schema

**Files:**
- Modify:
  - `skills/dtmgd/SKILL.md` (the Settings 2.0 section);
  - `skills/dtmgd/references/settings.md` (restructured as the spec's Documentation section lists);
  - `skills/dtmgd/references/cluster-api.md` (the "The cluster's own settings" section);
  - `README.md` (the settings paragraph and the command table);
  - `AGENTS.md` ("Adding a write command": writes go through `guardSecrets`; deletes go through `checkUnreferenced`);
  - `.claude/CLAUDE.md` (one new knowledge-base entry);
  - `docs/reference` (`make docs`).

**What each must say** (content from the spec; the wording is the builder's, in the house style: plain sentences, no marketing):

- **SKILL.md.** Lead with the five-step workflow for any request:
  1. `get settings-schemas --search`;
  2. `describe settings-schema`;
  3. read stored or effective values;
  4. write the file from an export or the defaults;
  5. `--dry-run`, then `--validate-only`, then `apply`.

  Examples use an alerting profile, a process-availability rule and an update window. One line: ask the user before writing an `IN_DEVELOPMENT` or `EARLY_ADOPTER` schema. Keep the section about as long as it is now; details go to `references/settings.md`.
- **`references/settings.md`.**
  - The intro no longer says "environment contexts" only.
  - New sections:
    - the workflow;
    - scopes: anything `allowedScopes` lists, meaning environment, entity scopes, and on clusters `cluster`, `environment-default`, `realm` and `metric`;
    - ordered lists and `position`, with the four forms, anchors by `externalId` across environments, anchors earlier in the file, and the fact that the list order is the evaluation order. An exported listing applied where none of its objects exist is created in file order, so it keeps its order without `position`;
    - references and the delete check, settings only;
    - secrets:
      - `<secret>` in reports;
      - a masked value round-trips on update;
      - a create from a mask is refused;
      - a file with the real secret always plans an update;
      - an update matched by `externalId` in another environment keeps that object's secret;
    - "when the cluster says no": not applicable, validation messages;
    - maturity;
    - the update settings as one section;
    - out of reach: Configuration API v1, history, permissions.
  - Replace the wrong secrets paragraph.
  - Two worked examples:
    - an alerting profile plus a disabled webhook notification, with a secret header and a reference;
    - a disabled process-availability rule with `position: first`.
- **`references/cluster-api.md`.** The scopes `environment-default` (defaults for every environment) and `realm`; network zones are stored at `environment-default` although the schema lists `cluster`; a scope-only listing works.
- **README.** "any schema", with a non-update example; `--position` in the `apply`/`create` rows.
- **Knowledge-base entry** ("Settings 2.0 for any schema: facts observed live", Context, Insight, When it applies, Date 2026-11-01, Ref = this plan). Insight bullets come from the spec's Facts:
  - the mask format and salted hash;
  - update keeps the secret, create stores the mask;
  - the 46 ordered schemas and the insert semantics;
  - `referencedType`, and zone references stored as `objectId`;
  - definition read cost;
  - the cluster scope-only 404 at 1.348 and 1.352 (stored and effective);
  - what a wrong host's 404 looks like;
  - network zones at `environment-default`;
  - `Setting not applicable`;
  - `/settings/history` answers but is not public;
  - the permission endpoints reject `Api-Token`.

- [ ] **Step 1: Check the worked examples live** (read and validate only), with a temporary config in which only `stage` may write, made by the scratchpad `mkcfg` program:

  ```bash
  dtmgd --config <tmp> --context stage apply -f <example>.yaml --validate-only -o json | jq -r '.results[].action'
  ```

  Expected: `validated` for every object. The alerting-profile example anchors the notification by `externalId`, so validate it as one file. Then delete the temporary config.
- [ ] **Step 2: Write the docs**, then run `make docs`.
- [ ] **Step 3: Check the skill against the code.**
  - `make build && ./dtmgd commands --brief`: every `dtmgd <verb> <resource>` that SKILL.md and `references/settings.md` name appears in the output.
  - Every flag they name appears in that command's `docs/reference/dtmgd_<verb>_<resource>.md`.
  - `go test ./skills/... ./cmd/ -count=1` passes, including the README scopes test and the stale-reference test.
- [ ] **Step 4: Commit.**

  ```bash
  git add -f .claude/CLAUDE.md
  git add skills README.md AGENTS.md docs/reference
  git commit -m "docs(settings): document any schema: workflow, scopes, positions, references, secrets, maturity"
  ```

### Task 8: Live tests in the integration suite, and the live run

**Files:**
- Create: `test/integration/settings_any_schema_test.go` (build tag `integration`)
- Modify: `test/integration/harness_test.go`, only if a helper is missing (for example a generic `testObjects`).

**Interfaces:**
- Consumes: `writeConfig`, `writeConfigFor`, `writeRun`, `applied`, `decode`, `items`, `brief`, `run`, and `requireEnv` (existing, `test/integration/harness_test.go` and `settings_write_test.go`).
- Produces:
  - `func testObjects(t *testing.T, ctx, schema, scope string) []map[string]any`: the objects whose `externalId` starts with `dtmgd-it-`;
  - `func cleanupTestObjects(t *testing.T, cfg, ctx string, schemas ...string)`: registered with `t.Cleanup`; deletes in the given order.

**Tests.** Write tests skip without their variables, like the existing ones. All objects are `dtmgd-it-…`, disabled where the schema allows, and URLs are on `example.invalid`.
- `TestAlertingProfileAndNotificationLifecycle` (`DTMGD_IT_WRITE_ENV`):
  1. Create the profile and the notification (secret header) in one file; re-apply: `unchanged`.
  2. Update the profile.
  3. Apply the notification's export with `displayName` changed: `updated`, and the header's mask afterwards equals the mask before.
  4. A copy from the export (no `objectId`, a new `externalId`) gives exit 2, and no new object is listed.
  5. Deleting the profile gives exit 1, and the profile is still listed.
  6. Delete the notification, then the profile.
- `TestOrderedRulesPosition` (`DTMGD_IT_WRITE_ENV`), with three disabled `builtin:processavailability` rules:
  1. Create a and b. The order of the `dtmgd-it-` entries is a, b.
  2. Create c with `position: first`: c, a, b.
  3. Apply b with `position: before:dtmgd-it-order-c`: b, c, a.
  4. Apply a with `position: after:dtmgd-it-order-b`: b, a, c.
  5. Delete all three.

  Compare only the relative order of the test entries, because other rules may exist.
- `TestExportsRoundTrip` (`DTMGD_IT_ENV`, GETs only):
  - Read `get settings --scope environment -o json`, keep up to 5 items per schema, and write the file to `t.TempDir()`.
  - Run `apply --dry-run -o json`. `writeRun` allows a dry run on any context.
  - Every action is `unchanged`. Report only counts.
- `TestClusterScopeListing` (`DTMGD_IT_CLUSTER`): `get settings --scope cluster` and `get settings --effective --scope cluster` exit 0.
- `TestClusterNetworkZoneLifecycle` (`DTMGD_IT_WRITE_CLUSTER`): create `dtmgd-it-zone` at `environment-default` (fallbackMode NONE, no alternative zones), re-apply (`unchanged`), delete, and none is left.

- [ ] **Step 1: Write the tests.** `go vet -tags integration ./test/integration/` and `PATH=$HOME/.local/bin:$PATH make lint`. Expected: clean.
- [ ] **Step 2: Run the live suite on stage** (writes; the maintainer's standing permission). The variables name contexts; the config path is passed, and its contents are never printed:

  ```bash
  DTMGD_IT_CONFIG=$HOME/.config/dtmgd/config DTMGD_IT_ENV=stage DTMGD_IT_CLUSTER=stage-cluster \
  DTMGD_IT_WRITE_ENV=stage DTMGD_IT_WRITE_CLUSTER=stage-cluster \
  go test -tags integration ./test/integration/ -count=1 2>&1 | tail -40
  ```

  Expected: PASS, with skips only where a scope or variable is missing. If a test fails, fix the code through TDD (unit test first), not the live test.
- [ ] **Step 3: Run the read checks on prod.**

  ```bash
  DTMGD_IT_CONFIG=$HOME/.config/dtmgd/config DTMGD_IT_ENV=prod DTMGD_IT_CLUSTER=prod-cluster \
  go test -tags integration ./test/integration/ -count=1 2>&1 | tail -40
  ```

  Expected: PASS; the write tests skip.
- [ ] **Step 4: Check stage is clean.** List the touched schemas on `stage` and `stage-cluster` and count the `dtmgd-it-` objects. Expected: 0.
- [ ] **Step 5: Commit.** `git commit -m "test(integration): live checks for any schema: secrets, positions, references, round-trip, cluster scopes"`
