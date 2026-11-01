# Settings 2.0, phase 2: writing in environments — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** dtmgd writes Settings 2.0 objects in environment contexts:
`apply`, `create settings`, `edit setting` and `delete settings`. A
per-context write level, `readonly` by default, gates every write.

**Architecture:** Each context gets a `safety-level`. Every write command
goes through one runner, `a.runWrites`, in two passes:
1. **Plan:** a GET-only pass that resolves each object's action in every
   selected context.
2. **Execute:** runs only when every context passed its checks (write level,
   version gate).

`--dry-run` stops after the plan. `--validate-only` executes with
`validateOnly=true`. Writes are never retried once they may have reached the
server.

**Tech Stack:** Go, Cobra, resty v2 (`pkg/client`), gopkg.in/yaml.v3, the
CLI harness in `cmd/harness_test.go`, the live suite in `test/integration`.

**Spec:** `docs/superpowers/specs/2026-10-31-settings-2-design.md` (phase 2
of 3). Phase 1 is merged (#72).

## Global Constraints

- **Branch** `feat/settings-write` from `main`.
- **Write levels** use dtctl's names: `readonly`, `readwrite-mine`,
  `readwrite-all`, `dangerously-unrestricted`.
  - An empty or missing value means `readonly`.
  - An unknown value is an error naming the four valid ones, never a silent
    `readonly`.
  - The config key is `safety-level`, in json and yaml.
- **What each level allows:**
  - `readonly`: nothing;
  - `readwrite-mine`: create only;
  - `readwrite-all` and `dangerously-unrestricted`: create, update and
    delete.
- **A blocked write** exits **6** without sending any request. In agent
  mode its error code is `safety_blocked`. Its hint is
  `dtmgd config set-context <name> --safety-level readwrite-all`.
- **`--dry-run`** sends only GETs and works at every level.
- **`--validate-only`** counts as a read for the write guard, but only once
  the version gate has passed:
  - the cluster version must be **≥ 1.288**, or dtmgd refuses with exit 2;
  - an unreadable version is refused as well;
  - the refusal names `DataExport` (environment contexts read their version
    through it) and suggests `--dry-run`.
- **No write is sent twice.**
  - `POST`, `PUT` and `DELETE` retry only after a connect or TLS-handshake
    failure before sending, or on 429.
  - They never retry on 5xx, connection reset or unexpected EOF.
  - GETs keep their current retries.
- **Conflict protection** is `updateToken` (the PUT body field, the DELETE
  query parameter). Never `If-Match`. A 409 exits 1 with the hint
  "re-read the object and apply again".
- **Deleting an update window** first lists the
  `builtin:deployment.oneagent.updates` and
  `builtin:deployment.activegate.updates` objects at every scope. If any
  references the window, it refuses (exit 1), naming them. There is no
  override.
- **Delete confirmation:**
  - in agent mode, or without a terminal, it needs `--yes`; otherwise it
    exits 2 with a hint;
  - a person at a terminal answers `[y/N]`.
- **Write result:** stdout, in every format:

  ```
  {"dryRun","validateOnly","results":[{"context","action","objectId","schemaId","scope","summary","error"}]}
  ```

  - `action` is one of `created`, `updated`, `unchanged`, `deleted`,
    `validated`, `failed`;
  - a dry run adds `current` and `value` per object;
  - people get a table.
- **Multi-environment:** `apply` and `create settings` take `--env "a;b"` /
  `ALL_ENVIRONMENTS`. Every context's checks run before any write;
  `delete settings` and `edit setting` with several contexts exit 2.
- **Cluster contexts:** write commands refuse them (exit 2, as
  `checkEnvironmentContexts` does). Cluster writes are phase 3.
- **Scopes:** write commands declare `settings.read` and `settings.write`.
  The README scope list gains `settings.write`, and its `DataExport` line
  names the `--validate-only` version check.
- **`-vv`** must not dump request bodies; the client prints none today, and
  a test keeps it so.
- **Repository rules:**
  - no real host names or IDs in repository files; fixtures use synthetic
    IDs;
  - error tests use 400/403/409, never 5xx, except the no-retry tests, which
    need a 5xx and pass because there is no retry;
  - `gocognit` ≤ 15;
  - `make docs` after any command change;
  - `go test ./...` and `make lint` pass at every commit.
- **Live writes:** only the `stage` environment (the maintainer allowed
  careful writes on `stage` and `stage-cluster`; phase 2 has no cluster
  writes).
  - Create and delete only objects named `dtmgd-it-…`.
  - Never change an existing object; existing objects get only
    `--dry-run` and `--validate-only`.
  - Never modify the maintainer's config file: the suite writes a temporary
    copy with the write level raised.

## Review Focus

1. **Applying `get settings -o yaml` output as it is.** The file is a list
   response (`items`, `totalCount`, `nextPageKey`). Every item is applied,
   and the extra top-level keys are ignored. Pinned by
   `TestApplyTakesAListResponse` (Task 4).
2. **An objectId the target environment answers with 403 or 400.** Only
   404 means "create instead"; any other error fails that object and
   creates nothing. Pinned by `TestApplyCreatesOnlyAfter404` (Task 4).
3. **A 409 in the middle of a multi-object file.** That object is `failed`,
   the others still run, every result is reported, and the exit code is
   non-zero. Pinned by `TestApplyConflictFailsOneObject` (Task 4).
4. **A typo in the config's `safety-level`** (`read-write`). Writes refuse
   with exit 2 naming the four values, before any request; reads are
   unaffected. Pinned by `TestUnknownSafetyLevelRefusesWrites` (Task 3).
5. **A POST whose connection drops after sending.** No retry, the object is
   `failed`, and rerunning `apply` is safe because the lookup finds the
   object if it was created. Pinned by `TestWritesAreNotRetried` (Task 2).

---

### Task 1: Write levels in the config, the context commands, listings and doctor

**Files:**
- Create: `pkg/config/safety.go`, `pkg/config/safety_test.go`
- Modify:
  - `pkg/config/config.go`: `Context.SafetyLevel` and `SetSafetyLevel`;
  - `cmd/ctx.go`: `--safety-level` on `ctx set`, and a SAFETY-LEVEL
    column;
  - `cmd/config.go`: `--safety-level` on `config set-context`, and its
    help;
  - `cmd/doctor.go`: `DoctorRow.SafetyLevel`;
  - goldens touched by the new columns;
  - `docs/configuration.md`: a "Write levels" section;
  - `docs/reference/`.
- Test: `cmd/safety_level_test.go`

**Interfaces:**
- Produces in `pkg/config`:
  - `type SafetyLevel string` with constants `SafetyReadOnly
    ("readonly")`, `SafetyReadWriteMine ("readwrite-mine")`,
    `SafetyReadWriteAll ("readwrite-all")`, `SafetyUnrestricted
    ("dangerously-unrestricted")`;
  - `func ParseSafetyLevel(s string) (SafetyLevel, error)`: `""` gives
    readonly, and an unknown value gives an error listing the four;
  - `type Operation string` with `OpCreate`, `OpUpdate`, `OpDelete`;
  - `func (l SafetyLevel) Allows(op Operation) bool`;
  - `Context.SafetyLevel string json:"safety-level,omitempty" yaml:"safety-level,omitempty"`;
  - `func (c *Config) SetSafetyLevel(name string, level SafetyLevel) error`
    (unknown context gives an error);
  - `func (c Context) Level() (SafetyLevel, error)`, which is
    `ParseSafetyLevel(c.SafetyLevel)`.
- Produces in `cmd`:
  - `ContextListItem.SafetyLevel string json:"safety-level" yaml:"safety-level" table:"SAFETY-LEVEL"`,
    holding the effective level, or the raw value when it is invalid;
  - `DoctorRow.SafetyLevel string json:"safetyLevel" yaml:"safetyLevel" table:"SAFETY-LEVEL"`.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/config/safety_test.go
func TestParseSafetyLevel(t *testing.T) // "" → readonly; each of the four parses; "read-write" → error whose text contains all four names
func TestSafetyLevelAllows(t *testing.T) // full matrix: readonly none; readwrite-mine create only; readwrite-all and dangerously-unrestricted all three
func TestSetSafetyLevel(t *testing.T)   // sets on a named context; unknown context → error

// cmd/safety_level_test.go
func TestSetContextSafetyLevel(t *testing.T)
// `config set-context a --safety-level readwrite-all` then `ctx -o json`: a's "safety-level" == "readwrite-all";
// `ctx set a --safety-level readonly` works too; `--safety-level read-write` → exit 2 naming the four values, config unchanged.
func TestContextsDefaultToReadonly(t *testing.T) // a config without the key: ctx -o json shows "safety-level":"readonly"; table has SAFETY-LEVEL column
func TestDoctorReportsSafetyLevel(t *testing.T)  // doctor -o json rows carry "safetyLevel"
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/config/ ./cmd/ -run 'SafetyLevel|DefaultToReadonly|DoctorReportsSafety' -count=1`
Expected: FAIL (build: undefined `ParseSafetyLevel`).

- [ ] **Step 3: Implement the `pkg/config` API above.**

- [ ] **Step 4: Wire the flags and columns.**
  - `setContext` gains a `safetyLevel string` parameter (empty means
    unchanged). It validates with `ParseSafetyLevel` before loading the
    config, so a typo writes nothing.
  - **Help text:** `config set-context`'s Long gains one paragraph. It says
    the levels gate dtmgd's own writes, that `readonly` is the default, and
    that the token's scopes are the real boundary.
  - **Goldens:** regenerate the ctx and doctor goldens, then read the
    diffs. Only the new column may change.

- [ ] **Step 5: Docs.**
  - **`docs/configuration.md`, "Write levels" section:**
    - the matrix;
    - the default;
    - how to set a level;
    - a local `.dtmgd.yaml` is not capped;
    - "a client-side guard, not a security boundary: a token without
      `settings.write` cannot write whatever the level".
  - Run `make docs`.

- [ ] **Step 6: Run the tests**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/config cmd docs
git commit -m "feat: per-context write levels, readonly by default"
```

---

### Task 2: Client writes — POST, PUT, DELETE without unsafe retries

**Files:**
- Modify:
  - `pkg/client/client.go`: write methods and `isRetryable`;
  - `pkg/client/multi.go`: `MultiRequestNamed` and `ResolveContexts`;
  - `pkg/client/exitcode.go`: `ExitSafetyBlocked`;
  - `pkg/client/version.go`: `VersionAtLeast`.
- Test: `pkg/client/write_test.go`

**Interfaces:**
- Produces:
  - **Write methods** (each returns `ErrClusterScoped` on a cluster-scoped
    client, and `APIError(status, body)` for a non-2xx answer; the body is
    complete, so a caller can read per-item results from a 400):
    - `func (c *Client) PostV2(path string, params map[string]string, body, result interface{}) error`;
    - `func (c *Client) PutV2(path string, params map[string]string, body, result interface{}) error`;
    - `func (c *Client) DeleteV2(path string, params map[string]string) error`.
  - **Retry rule:** `isRetryable` reads `r.Request.Method` when the
    response carries a request. For a method other than GET or HEAD it
    returns true only for `connectTimeout(err)` or status 429.
  - **Fan-out with names:**
    `func MultiRequestNamed(cfg *config.Config, envSpec string, maxConcurrency int, o Options, apiCall func(name string, c *Client) (interface{}, error)) ([]EnvResult, error)`.
    `MultiRequest` delegates to it.
  - **Context resolution:**
    `func ResolveContexts(cfg *config.Config, envSpec string) ([]config.NamedContext, error)`,
    the current `resolveContexts`, exported.
  - **Exit code:** `const ExitSafetyBlocked = 6`.
  - **Version check:** `func VersionAtLeast(v string, min [3]int) (ok bool, known bool)`,
    built on `ParseClusterVersion`.

- [ ] **Step 1: Write the failing tests**

```go
func TestWriteMethodsSendBodyAndQuery(t *testing.T) // PostV2/PutV2 JSON body arrives; params in query; DeleteV2 sends DELETE with query
func TestWritesAreNotRetried(t *testing.T)
// a server answering 503 to POST, PUT and DELETE: exactly 1 hit each; a handler that hijacks and closes the
// connection on POST: 1 hit, error returned (Review Focus 5)
func TestGetStillRetries(t *testing.T)      // GET 503 then 200: 2 hits, success (keeps today's behaviour)
func TestWriteOnClusterScopedClient(t *testing.T) // PostV2 on a client without env-id → ErrClusterScoped, no request
func TestVersionAtLeast(t *testing.T)       // "1.288.0.2024" ≥ {1,288,0}; "1.287.99" not; "garbage" → known=false
func TestMultiRequestNamedPassesNames(t *testing.T) // two contexts: each apiCall gets its own name
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/client/ -run 'Write|NotRetried|StillRetries|ClusterScopedClient|VersionAtLeast|MultiRequestNamed' -count=1`
Expected: FAIL (build: undefined `PostV2`).

- [ ] **Step 3: Implement** the interfaces above. The write methods use
  `c.http.R().SetBody(body)` with a JSON content type.

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/client/ -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client
git commit -m "feat(client): settings write methods that never resend a write"
```

---

### Task 3: The write runner, `create settings`, `--dry-run` and `--validate-only`

**Files:**
- Create:
  - `cmd/writes.go`: the runner, the safety check, the version gate, the
    report;
  - `cmd/create.go`: the `create` verb and `create settings`;
  - `cmd/writes_test.go`.
- Modify:
  - `cmd/errors.go`: `SafetyError` gives exit 6 and code `safety_blocked`;
  - `cmd/app.go` (`newRootCmd`'s `AddCommand`): register `create`;
  - `cmd/scopes.go`: `scopeSettingsWrite = "settings.write"`;
  - `cmd/commands.go`: the catalog marks write commands `"mutating": true`;
  - `README.md`: the verbs table row for `create`, the `settings.write`
    scope line, and the `DataExport` line;
  - `docs/reference/`.

**Interfaces:**
- Consumes: Task 1 `Context.Level`, `SafetyLevel.Allows`, `Operation`.
  Task 2 `PostV2`, `MultiRequestNamed`, `ResolveContexts`,
  `ExitSafetyBlocked`, `VersionAtLeast`. Phase 1 `settingsObjectsPath`,
  `settingsObject`.
- Produces:
  - **The result types:**
    - `type writeResult struct { Context string json:"context"; Action string json:"action"; ObjectID string json:"objectId,omitempty"; SchemaID string json:"schemaId"; Scope string json:"scope"; Summary string json:"summary,omitempty"; Error *writeError json:"error"; Current map[string]interface{} json:"current,omitempty"; Value map[string]interface{} json:"value,omitempty" }`;
    - `type writeError struct { Code int json:"code"; Message string json:"message"; ConstraintViolations []constraintViolation json:"constraintViolations,omitempty" }`;
    - `type constraintViolation struct { Path string json:"path"; Message string json:"message" }`;
    - `type writeReport struct { DryRun bool json:"dryRun"; ValidateOnly bool json:"validateOnly"; Results []writeResult json:"results" }`.
  - **One planned write:**
    `type plannedWrite struct { Op config.Operation; Action string; ObjectID, SchemaID, Scope, ExternalID, UpdateToken string; Value, Current map[string]interface{} }`.
    `Action` is `create`, `update`, `unchanged` or `delete`. `Op` is empty
    for `unchanged`.
  - **The command's description of its writes:**
    `type writeSpec struct { Verb, Resource string; DryRun, ValidateOnly bool; MultiEnv bool; Plan func(name string, c *client.Client) ([]plannedWrite, error) }`.
    `MultiEnv` false makes several contexts exit 2.
  - **The runner:** `func (a *App) runWrites(s writeSpec) error`. It:
    1. refuses cluster contexts, and several contexts when `!MultiEnv`;
    2. parses every selected context's level, where an unknown level is a
       `UsageError`;
    3. runs plan pass 1 with `MultiRequestNamed`;
    4. on any plan error, reports it and stops;
    5. checks levels (`SafetyError`) and, for `ValidateOnly`, the version
       gate, for every context. The gate covers creates too: the floor
       from which POST honours `validateOnly` was not confirmed, and one
       floor is simpler;
    6. stops with nothing written if one fails;
    7. otherwise, unless `DryRun`, runs execute pass 2 with
       `MultiRequestNamed`, looking up each context's plan by name;
    8. renders the `writeReport`.
  - **Per-object execution:** `func executeWrite(c *client.Client, w plannedWrite, validateOnly bool) writeResult`:
    - `create`: POST a single-element array with `?validateOnly=true` when
      validating. The response item's `objectId` and `code` fill the
      result.
    - `update`: PUT `{value, updateToken}`.
    - `delete`: DELETE `?updateToken=`.
    - `unchanged`: nothing is sent.
    - Failures read the `ErrAPI` body (the per-item array, or the standard
      error envelope) into `writeError`.
  - **The block error:**
    `type SafetyError struct { Context string; Level config.SafetyLevel; Op config.Operation }`.
    Its `Error()` is
    `context "<c>" is <level>: it does not allow <op>`, and its hint is
    `dtmgd config set-context <c> --safety-level readwrite-all`.
  - **The command:** `newCreateCmd(a *App)` (the verb group) and
    `newCreateSettingsCmd(a *App)`, Use
    `settings -f <value-file> --schema <id> --scope <scope>`.
    - Its plan GETs the schema (`multiObject`). For a single-object schema
      it lists the object at the scope and refuses (exit 1) if one exists,
      with the hint "use apply to change it".
    - The value file is YAML or JSON holding the bare value.

- [ ] **Step 1: Write the failing tests** in `cmd/writes_test.go`. They use a
  temp config whose contexts carry `safety-level`; extend `withFakeEnvs`
  with a variant `withFakeEnvLevels(t, levels map[string]string, envs...)`.

```go
func TestReadonlyBlocksWritesBeforeAnyRequest(t *testing.T)
// default level: `create settings -f v.yaml --schema s --scope environment` → exit 6, stderr names the context,
// "readonly" and the set-context hint; api.totalHits() == 0. CLAUDECODE=1: envelope error.code == "safety_blocked".
func TestUnknownSafetyLevelRefusesWrites(t *testing.T) // Review Focus 4: level "read-write" → exit 2 naming the four; 0 hits; `get settings` still works
func TestReadwriteMineMayCreate(t *testing.T)   // readwrite-mine: create succeeds (POST sent), result action "created", objectId from the response
func TestCreateRefusesAnExistingSingleton(t *testing.T) // single-object schema with an object at the scope → exit 1, hint names apply, no POST
func TestDryRunSendsOnlyGets(t *testing.T)      // readonly context + --dry-run: exit 0, result dryRun true, action "create", value set; no POST/PUT/DELETE hit
func TestValidateOnlyGate(t *testing.T)
// readwrite-all; cluster version 1.287 → exit 2 naming 1.288, no POST; version unreadable (403) → exit 2 naming DataExport
// and --dry-run; version 1.300 → POST carries validateOnly=true, action "validated".
func TestValidateOnlyIsAllowedAtReadonly(t *testing.T) // readonly + --validate-only on version 1.300 → exit 0 (counts as a read)
func TestMultiEnvChecksEveryContextFirst(t *testing.T)
// --env "a;b", a readwrite-all, b readonly → exit 6, zero POSTs on a and b; both readwrite-all → two POSTs,
// results carry context "a" and "b".
func TestWriteRefusesClusterContext(t *testing.T) // a cluster context → exit 2 before any request
func TestWriteFailureReport(t *testing.T)
// POST answers 400 with a per-item array holding constraintViolations → action "failed", error.constraintViolations[0].path set,
// exit 1; CLAUDECODE=1 → ok false and result.results still present.
func TestVerboseDumpsNoRequestBody(t *testing.T) // -vv create: stderr never contains a value field's text from the file
func TestCatalogMarksWriteCommands(t *testing.T)  // commands -o json: create settings has "mutating": true; get settings has none
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'Readonly|UnknownSafetyLevel|ReadwriteMine|CreateRefuses|DryRun|ValidateOnly|MultiEnvChecks|WriteRefusesCluster|WriteFailureReport|VerboseDumpsNoRequestBody|CatalogMarksWrite' -count=1`
Expected: FAIL (unknown command "create").

- [ ] **Step 3: Implement** `SafetyError` with its exit code and agent
  code, then `writeReport` rendering:
  - **Agent mode:** a success envelope, or `output.WriteAgentPartial`
    (`ok:false` with the results) followed by
    `fmt.Errorf("%w: %w", errReported, err)` when any object failed.
  - **json and yaml:** the report as is.
  - **A person:**
    - a table `ACTION OBJECT-ID SCHEMA SCOPE SUMMARY`, with CONTEXT in
      front when several contexts were selected;
    - one stderr line per failure:
      `✗ <context> <schemaId> <objectId or scope>: <message>`, plus one
      indented line per constraint violation `<path>: <message>`.
  - **Exit code when objects failed:** build a `client.PartialFailureError`
    whose `Failed` holds one `EnvResult` per failed object
    (`Name: context`, `Error: the ErrAPI`) and whose `Total` is the number
    of objects. Its `Code()` gives the shared code, or 1.

- [ ] **Step 4: Implement `runWrites`, `executeWrite`, `create settings`,
  the catalog's `mutating` mark, and `withScopes(cmd, scopeSettingsRead,
  scopeSettingsWrite)`.**
  - Register `create` in `newRootCmd` (`cmd/app.go`). Task 4 adds
    `apply`, Task 5 `delete`, Task 6 `edit`, in the same place.
  - Add the README verbs row, the scope lines, then run `make docs`.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./cmd/ -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd README.md docs/reference
git commit -m "feat: create settings, behind write levels, with --dry-run and --validate-only"
```

---

### Task 4: `apply -f`

**Files:**
- Create: `cmd/apply.go`, `cmd/apply_test.go`
- Create fixtures:
  - `cmd/testdata/apply/window_new.yaml`: one object, no `objectId`,
    update-windows schema;
  - `cmd/testdata/apply/export_list.yaml`: a `get settings -o yaml` list
    response with 2 items, `totalCount` and `nextPageKey`;
  - `cmd/testdata/apply/mixed.json`: an array of three objects.
- Modify: `README.md` (verbs row `apply`), `docs/reference/`

**Interfaces:**
- Consumes: Task 3 `writeSpec`, `plannedWrite`, `runWrites`.
- Produces:
  - `type applyObject struct { ObjectID string json:"objectId"; ExternalID string json:"externalId"; SchemaID string json:"schemaId"; Scope string json:"scope"; Value map[string]interface{} json:"value"; UpdateToken string json:"updateToken" }`;
  - `func readApplyFile(path string, stdin io.Reader) ([]applyObject, error)`:
    - YAML or JSON;
    - one object, an array, or a body with `items`;
    - `-` reads stdin;
    - an object without `schemaId`, or without `scope` when it has no
      `objectId`, gives a `UsageError` naming its index.
  - `func planApply(c *client.Client, name string, objs []applyObject) ([]plannedWrite, error)`,
    the spec's decision procedure:
    1. **With `objectId`:** GET it.
       - Found: `update`, with the file's `updateToken` or the GET's.
       - 404: `create` (the new objectId is reported).
       - Any other error fails the plan for that object.
    2. **Without:** GET the schema definition, cached per schema, and list
       the objects at schema and scope
       (`fields=objectId,externalId,value,updateToken`). Match in order:
       - for a single-object schema, the object at that scope;
       - otherwise the same `externalId`;
       - otherwise the same value of the schema's `keyProperty`, if the
         definition has that field.

       A match gives `update`, anything else `create`.
    3. **Unchanged:** an `update` whose value deep-equals the current value
       (after a JSON round-trip of both) becomes `unchanged`.
  - **A create that overwrote:** when a POST answers with an `objectId`
    that was already among the listed objects, the result's action is
    `updated`.
  - **The command:** `newApplyCmd(a *App)` with `-f/--filename` (required),
    `--dry-run` and `--validate-only`, `MultiEnv: true`.

- [ ] **Step 1: Write the failing tests** in `cmd/apply_test.go`:

```go
func TestApplyCreatesANewObject(t *testing.T)      // window_new.yaml at readwrite-all → one POST with schemaId, scope, value; action "created"
func TestApplyUpdatesByObjectID(t *testing.T)      // file objectId found → PUT body {value, updateToken} with the file's token when present, else the GET's
func TestApplyCreatesOnlyAfter404(t *testing.T)    // Review Focus 2: GET 404 → POST; GET 403 → action "failed", no POST
func TestApplyMatchesSingletonByScope(t *testing.T) // oneagent.updates without objectId, an object at the scope → PUT to its objectId (not POST)
func TestApplyMatchesExternalID(t *testing.T)      // multi-object schema, listed object with the same externalId → PUT
func TestApplyUnchangedSendsNothing(t *testing.T)  // value equals current → action "unchanged", no PUT
func TestApplyTakesAListResponse(t *testing.T)     // Review Focus 1: export_list.yaml → 2 results; totalCount/nextPageKey ignored
func TestApplyConflictFailsOneObject(t *testing.T) // Review Focus 3: mixed.json, second PUT answers 409 → results [updated, failed, created], exit 1, hint "re-read"
func TestApplyReadwriteMineBlocksAnUpdate(t *testing.T) // readwrite-mine, plan resolves an update → exit 6, nothing written
func TestApplyReportsAnOverwriteAsUpdated(t *testing.T) // no match found, but the POST answers with an objectId that was listed → action "updated"
func TestApplyStdin(t *testing.T)                  // `-f -` reads the file from stdin (swap os.Stdin with a pipe in the test)
func TestApplyBadFile(t *testing.T)                // object without schemaId → exit 2 naming index 0; unparsable YAML → exit 2
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'Apply' -count=1`
Expected: FAIL (unknown command "apply").

- [ ] **Step 3: Implement** `readApplyFile`, `planApply` and `newApplyCmd`.
  - Keep `planApply` under `gocognit` 15 by splitting it:
    `planByObjectID` and `planByLookup`.
  - Register `apply` at the root, add the README verbs row, then run
    `make docs`.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./cmd/ -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd README.md docs/reference
git commit -m "feat: apply -f creates or updates the settings objects in a file"
```

---

### Task 5: `delete settings`, with the update-window reference check and confirmation

**Files:**
- Create: `cmd/delete.go`, `cmd/delete_test.go`
- Modify:
  - `cmd/app.go`: an `App.stdin io.Reader` and `App.interactive func() bool`
    test hook. When nil, they default to `os.Stdin` and
    `term.IsTerminal(stdin)`.
  - `README.md` (verbs row), `docs/reference/`.

**Interfaces:**
- Consumes: Task 3 runner (`MultiEnv: false`), `plannedWrite{Op: OpDelete}`.
- Produces:
  - `newDeleteCmd(a *App)` (the verb group) and
    `newDeleteSettingsCmd(a *App)`, Use `settings <object-id>`, with
    `-y/--yes` and `--dry-run`;
  - `func windowReferences(c *client.Client, windowID string) ([]settingsObject, error)`;
  - `func (a *App) confirmDelete(prompt string, yes bool) error`. It returns
    a `UsageError` when neither `--yes` nor an interactive terminal is
    available (agent mode counts as not interactive), and `errCancelled`
    for an answer other than `y`/`yes`.

- [ ] **Step 1: Write the failing tests**

```go
func TestDeleteNeedsYesWithoutATerminal(t *testing.T) // readwrite-all, no --yes → exit 2 naming --yes; no DELETE; CLAUDECODE=1 likewise
func TestDeleteSendsTheUpdateToken(t *testing.T)      // --yes → GET then DELETE ?updateToken=<from GET>; action "deleted"
func TestDeleteRefusesAReferencedWindow(t *testing.T)
// object is an update window; oneagent.updates at HOST_GROUP-… references it → exit 1 naming the schema and scope; no DELETE
func TestDeleteUnreferencedWindow(t *testing.T)       // no references → DELETE sent
func TestDeleteDryRun(t *testing.T)                   // readonly + --dry-run → exit 0, action "delete" planned, no DELETE
func TestDeleteOneContextOnly(t *testing.T)           // --env "a;b" → exit 2
func TestConfirmDeletePrompt(t *testing.T)            // newTestApp with interactive=true and stdin "y\n" → nil; "n\n" → errCancelled; stdin "" → errCancelled
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'Delete|ConfirmDelete' -count=1`
Expected: FAIL (unknown command "delete").

- [ ] **Step 3: Implement**
  - **The plan:** GET the object (404 exits 4). If its schema is
    update-windows, run the reference check. The confirmation runs after
    the plan and before execution; a dry run skips it.
  - **The reference check** lists both update schemas with `schemaIds`
    only (every scope), `fields=objectId,schemaId,scope,value`, paging
    through `nextPageKey`.
  - **A cancelled delete** exits 0 with stderr "Delete cancelled" and an
    empty `results`.
  - Register `delete`, add the README row, then run `make docs`.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./cmd/ -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd README.md docs/reference
git commit -m "feat: delete settings, refusing update windows still in use"
```

---

### Task 6: `edit setting`

**Files:**
- Create: `cmd/edit.go`, `cmd/edit_test.go`
- Modify: `README.md` (verbs row), `docs/reference/`

**Interfaces:**
- Consumes: Task 3 runner (`MultiEnv: false`), Task 5 `App.interactive`
  and `App.stdin`.
- Produces:
  - `newEditCmd(a *App)` and `newEditSettingCmd(a *App)`, Use
    `setting <object-id>`, alias `settings`, with `--validate-only`;
  - `func editorCommand() []string`: `$EDITOR`, else `$VISUAL`, else `vi`
    (`notepad` on Windows), split on spaces;
  - `func (a *App) editValue(value map[string]interface{}) (map[string]interface{}, bool, error)`:
    writes the value as YAML to a temp file, runs the editor, reads it
    back, and reports whether it changed.

- [ ] **Step 1: Write the failing tests**

```go
func TestEditRefusesWithoutATerminal(t *testing.T) // CLAUDECODE=1, and a plain run (no terminal) → exit 2 with the hint "use apply"
func TestEditUpdatesTheObject(t *testing.T)
// newTestApp, interactive=true, EDITOR = a temp sh script that rewrites the file to `name: changed` (skip on Windows):
// PUT body value.name == "changed" with the GET's updateToken; action "updated"
func TestEditUnchangedSendsNothing(t *testing.T)   // EDITOR=true (leaves the file) → stderr "Edit cancelled", no PUT
func TestEditValidateOnly(t *testing.T)            // with version 1.300 → PUT ?validateOnly=true; version 1.287 → exit 2
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ -run 'Edit' -count=1`
Expected: FAIL (unknown command "edit").

- [ ] **Step 3: Implement.** The plan GETs the object, edits its value, and
  yields one `update` (or `unchanged`). Register `edit`, add the README
  row, then run `make docs`.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./cmd/ -count=1`, then `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd README.md docs/reference
git commit -m "feat: edit setting opens an object's value in an editor"
```

---

### Task 7: Docs, skill and knowledge base: the read-only promise becomes write levels

**Files:**
- Modify `README.md`:
  - line 3: "read-only CLI" becomes "reads Dynatrace Managed, and writes
    Settings 2.0 where a context's write level allows it";
  - replace the "`create`, `edit`, `delete` and `apply` are absent on
    purpose" paragraph with the write verbs and the default `readonly`;
  - one write example.
- Modify `AGENTS.md`:
  - "What dtmgd is": the read-only sentence becomes the new rule;
  - "Adding a command": a "write command" paragraph (`runWrites`, the
    write level, no retries, the report, `--dry-run`);
  - Rules for agents: live writes only to `stage` (and `stage-cluster` in
    phase 3), only test-named objects, with the go-ahead.
- Modify `skills/dtmgd/SKILL.md`:
  - the Settings section gains `apply`, `create`, `edit`, `delete`;
  - the exit codes gain 6;
  - drop any read-only claim.
- Modify `skills/dtmgd/references/settings.md`: a "Writing" section:
  - write levels, and that agents may raise them, the token being the
    customer's control;
  - always `--dry-run` first;
  - the apply file and round-trip;
  - `externalId` for repeatable applies;
  - the update-window reference check;
  - secret properties: check `describe settings-schema` for type `secret`
    before applying an export (dtmgd does not treat them specially);
  - conflicts (409: re-read, re-apply);
  - multi-environment apply.
- Modify `skills/dtmgd/references/output.md`: exit code 6 and
  `safety_blocked`.
- Modify `docs/output.md`: the exit-code list.
- Modify `.claude/CLAUDE.md`: a knowledge-base entry, "Settings 2.0
  writes":
  - the 1.288 gate and why;
  - `updateToken`, not `If-Match`;
  - non-atomic batches, so one request per object;
  - no retries on writes;
  - dangling window references;
  - the temp config copy in the live write suite.

**Interfaces:**
- Consumes: every command and flag from Tasks 1–6.

- [ ] **Step 1: Write the docs** as listed. Use only synthetic IDs and
  `example.com`.

- [ ] **Step 2: Check the skill against the CLI.** Use the same script as
  phase 1 Task 5 Step 2, with the verb pattern extended to
  `(get|describe|query|apply|create|edit|delete)`.
  Expected: no `MISSING` or `UNKNOWN FLAG` lines. For `apply`, which has no
  resource, check that `.verbs` or the catalog's top-level commands list it.

- [ ] **Step 3: Run the guards**

Run: `go test ./... -count=1 && make lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add README.md AGENTS.md docs skills
git add -f .claude/CLAUDE.md
git commit -m "docs: settings writes, write levels and exit code 6 in the README, AGENTS.md and skill"
```

---

### Task 8: Live write checks on `stage`

**Files:**
- Create: `test/integration/settings_write_test.go`
- Modify: `test/integration/harness_test.go` (`writeRun`), and
  `.integrationtests.env.example` (`DTMGD_IT_WRITE_ENV`).

**Interfaces:**
- Consumes: `run`, `brief`, `requireEnv`. Task 1 `config.LoadFrom`,
  `SetSafetyLevel`, `SaveTo`.
- Produces:
  - `func writeConfig(t *testing.T) (path, ctx string)`:
    - requires `DTMGD_IT_CONFIG` and `DTMGD_IT_WRITE_ENV`;
    - loads the config and sets `readwrite-all` on that context only;
    - saves to `t.TempDir()`;
    - never writes the original.
  - `func writeRun(t testing.TB, cfg, ctx string, args ...string) result`:
    - refuses any verb but `apply`, `create`, `delete`, and any context but
      `ctx`;
    - always adds `--context ctx --config cfg --no-agent --plain`.
  - The read `run` helper still refuses write verbs.

- [ ] **Step 1: Write the checks:**

```go
func TestReadonlyContextBlocksALiveWrite(t *testing.T)
// the ORIGINAL config (write context readonly): apply of a dtmgd-it window → exit 6 (nothing is sent)
func TestUpdateWindowLifecycle(t *testing.T)
// sweep: delete --yes every update window named dtmgd-it-* left by a crashed run;
// apply a disabled DAILY window named dtmgd-it-<unix time> (range 2030-01-01..2031-01-01) → created;
// get settings -o json finds it by name; apply it again with duration changed → updated; apply unchanged → unchanged;
// get settings --effective --scope environment lists it; delete --yes → deleted; get settings no longer lists it.
// t.Cleanup deletes it by objectId if any step failed.
func TestUpdateSettingsAreOnlyValidated(t *testing.T)
// read the effective OneAgent and ActiveGate update values at environment; build an apply file at scope environment
// with updateMode changed (AUTOMATIC ↔ MANUAL); --validate-only → exit 0, action "validated";
// --dry-run → exit 0, dryRun true; afterwards the effective values are unchanged.
func TestWriteRefusesTheClusterContext(t *testing.T) // apply against DTMGD_IT_CLUSTER → exit 2 (skips without it)
```

- [ ] **Step 2: Vet offline**

Run: `go vet -tags integration ./test/integration/ && env -u DTMGD_IT_CONFIG go test -tags integration ./test/integration/ -count=1`
Expected: vet clean; the write checks skip, and the harness self-tests pass
(add one: `writeRun` refuses `get` and a context other than the write
context).

- [ ] **Step 3: Commit**

```bash
git add test/integration .integrationtests.env.example
git commit -m "test: live write checks on the stage environment"
```

- [ ] **Step 4: The live run** (the maintainer allowed careful writes on
  `stage` on 2026-11-01). Run:

  ```bash
  DTMGD_IT_CONFIG=~/.config/dtmgd/config DTMGD_IT_ENV=stage DTMGD_IT_WRITE_ENV=stage DTMGD_IT_CLUSTER=stage-cluster make integration
  ```

  Report pass, skip and fail counts only. Afterwards, check that no
  `dtmgd-it-` window is left (a count only). A failure gets its own fixture
  and failing test before the fix.
