# Settings 2.0, phase 3: the cluster — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** the settings commands of phases 1 and 2 also work on cluster
contexts, through the Cluster API's public settings endpoints. New commands
read and write cluster ActiveGate auto-update.

**Architecture:** One small routing layer, `cmd/settings_api.go`, picks the
environment API (`/api/v2/settings/…`) or the Cluster API
(`/api/cluster/v2/settings/…`) from the client it is given
(`c.ClusterScoped()`). Every settings call goes through it, so the existing
commands, the write runner and the lookups need no second code path; only
context selection changes.
- **Reads:** a cluster context runs through `runClusterFetch`.
- **Writes:** `runWrites` accepts one cluster context.
- **Cluster ActiveGate auto-update:** `get`/`describe`/`update`/`edit
  activegate-updates`. Its writes go through `runWrites` with a per-write
  executor.

**Tech Stack:** Go, Cobra, resty (`pkg/client`), the CLI harness, the live
suite.

**Spec:** `docs/superpowers/specs/2026-10-31-settings-2-design.md`, "Cluster
contexts" (phase 3 of 3). Phases 1 (#72) and 2 (#73) are merged.

## Global Constraints

- **Branch** `feat/settings-cluster` from `main`.
- **Public APIs only.**
  - The cluster's own automatic-update window and the hidden
    `builtin:deployment.cluster.activegate.updates` schema stay out.
  - Cluster ActiveGate auto-update uses only `/api/cluster/v2/activeGates`,
    `/activeGates/autoUpdate` and `/activeGates/{agId}/autoUpdate` (and
    their `/validator`).
- **Cluster settings paths** are `{host}/api/cluster/v2/settings/{schemas,objects,effectiveValues}`:
  - `GetCluster`'s path is relative to `{host}/api`, so these are
    `/cluster/v2/settings/…`;
  - the scope is `cluster` in responses; requests accept `cluster`,
    `environment` and `tenant`;
  - paging and `fields` work as in the environment API.
- **Token scopes:**
  - cluster settings reads need `settings.read`;
  - cluster settings writes need `settings.write`, and creates (POST) also
    `ServiceProviderAPI`;
  - cluster ActiveGate auto-update needs `ServiceProviderAPI`, read and
    write.
- **Cluster contexts never fan out.** A cluster context named with
  environments in `--env` stays an error (as `client.resolveContexts`
  already makes it).
- **Write levels** apply unchanged: updating the ActiveGate auto-update
  configuration is an `update`.
- **The 1.288 version gate** applies to cluster settings validations. The
  version comes from the node list on a cluster context. ActiveGate
  auto-update's `--validate-only` uses `POST …/validator`, which never saves,
  so it has no version gate.
- **Target version and update windows:** cluster ActiveGate auto-update
  honours `targetVersion` and `updateWindows` only where its GET returns
  them (a server feature flag, off by default). dtmgd:
  - shows those fields only when returned;
  - refuses (exit 2) a file that sets them when the GET did not return
    them.
- **Deleting a cluster update window** first checks the cluster ActiveGate
  auto-update configuration, global and per ActiveGate, for references,
  when those configurations carry `updateWindows`. It refuses (exit 1) if
  one references the window.
- **Repository rules:** synthetic IDs, `example.com`; tests never use the
  default config; `gocognit` ≤ 15; `make docs`; `go test ./...` and
  `make lint` at every commit.
- **Live checks:**
  - reads on `prod-cluster` and `stage-cluster`;
  - writes only on `stage-cluster` (the maintainer allowed careful writes
    there on 2026-11-01), only to a disabled `dtmgd-it-…` cluster update
    window that the run creates and deletes;
  - cluster ActiveGate auto-update gets only `--validate-only` and
    `--dry-run`;
  - output stays filtered: no host names, ActiveGate host names or IDs.

## Review Focus

1. **A cluster context whose token lacks `settings.read`**, which is likely:
   tokens made for phases 1–4 carry only `ServiceProviderAPI`. The 403 hint
   names `settings.read` and says a cluster token needs it too. Pinned by
   `TestClusterSettings403NamesTheScope` (Task 1).
2. **`--context <cluster>` combined with `--env` naming environments.**
   Refused with exit 2 before any request, for reads and writes. Pinned by
   `TestClusterSettingsRefuseFanOut` (Task 1).
3. **An ActiveGate auto-update file holding `setting: INHERITED` for the
   global configuration** (only per-ActiveGate configurations take
   `INHERITED`). Refused, exit 2, before any write. Pinned by
   `TestGlobalAutoUpdateRefusesInherited` (Task 3).
4. **An ActiveGate ID that does not exist.** Exit 4, with the hint to list
   them with `get activegate-updates`. Pinned by
   `TestActiveGateUpdatesUnknownID` (Task 2).
5. **An environment ActiveGate's ID given to the cluster commands**
   (`type ENVIRONMENT`). The list shows only cluster ActiveGates
   (`type=CLUSTER`). Whatever a per-ID call returns is passed through, not
   reinterpreted. Pinned by `TestActiveGateUpdatesListsClusterActiveGates`
   (Task 2).

---

### Task 1: Settings commands on cluster contexts

**Files:**
- Create: `cmd/settings_api.go`, `cmd/settings_cluster_test.go`
- Modify:
  - `pkg/client/client.go`: `PostCluster`, `PutCluster`, `DeleteCluster`;
  - every settings call site: `cmd/get_settings.go`,
    `cmd/get_settings_schemas.go`, `cmd/describe_settings_schema.go`,
    `cmd/describe_settings.go`, `cmd/settings_views.go`, `cmd/create.go`,
    `cmd/apply.go`, `cmd/delete.go`, `cmd/edit.go`, `cmd/writes.go`;
  - `README.md`: scope list text for cluster tokens;
  - `docs/reference/`.
- Test: `pkg/client/write_test.go` (cluster write methods)

**Interfaces:**
- Produces in `pkg/client`, each on the cluster client, relative to
  `{host}/api`, with the same no-retry rule (it lives in `isRetryable`):
  - `func (c *Client) PostCluster(path string, params map[string]string, body, result interface{}) error`;
  - `PutCluster` with the same signature;
  - `DeleteCluster(path string, params map[string]string) error`.
- Produces in `cmd/settings_api.go`. Each function routes on
  `c.ClusterScoped()`: the environment API with the path as given, the
  Cluster API with `"/cluster/v2" + path`.
  - `func settingsGet(c *client.Client, path string, params map[string]string, out interface{}) error`;
  - `func settingsPaged(c *client.Client, path string, params map[string]string, p client.Paging) (map[string]interface{}, error)`;
  - `func settingsPagedFetch(path string, params map[string]string, listKey string, pageCap, maxPages int) func(*client.Client, int) (fetched, error)`;
  - `func settingsObjectFetch(path string) func(*client.Client, int) (fetched, error)`;
  - `func settingsPost(c *client.Client, path string, params map[string]string, body, out interface{}) error`;
  - `settingsPut` with the same signature;
  - `settingsDelete(c *client.Client, path string, params map[string]string) error`.
- Produces in `cmd`:
  - `func (a *App) onClusterContext(cfg *config.Config) bool`: true when no
    multi-environment `--env` is given and the current context is a
    cluster context;
  - `func (a *App) runSettingsFetch(s fetchSpec) error`: `runClusterFetch`
    on a cluster context, `runFetch` otherwise;
  - `writeSpec.ClusterToo bool`. When true, `runWrites` accepts one
    cluster context; every settings write command sets it.

- [ ] **Step 1: Write the failing tests** in `cmd/settings_cluster_test.go`.
  Use `withFakeContexts(t, "c", nil, []fakeEnv{{"c", api}})` and routes
  under `/api/cluster/v2/settings/…`.

```go
func TestClusterSettingsSchemas(t *testing.T)   // get settings-schemas --context c hits GET /api/cluster/v2/settings/schemas; one row per item
func TestClusterGetSettings(t *testing.T)       // --schema builtin:deployment.management.update-windows --scope cluster: query schemaIds, scopes=cluster, fields; update-window table
func TestClusterEffectiveValues(t *testing.T)   // --effective --scope cluster → GET /api/cluster/v2/settings/effectiveValues with scope=cluster
func TestClusterDescribeSettings(t *testing.T)  // describe settings <id> --context c → GET /api/cluster/v2/settings/objects/<id>
func TestClusterSettings403NamesTheScope(t *testing.T) // Review Focus 1: 403 → exit 5, stderr names settings.read and "cluster token"
func TestClusterSettingsRefuseFanOut(t *testing.T)     // Review Focus 2: --context c --env "a;b" → exit 2, 0 hits (get settings and apply)
func TestClusterApplyCreatesAWindow(t *testing.T)
// readwrite-all cluster context: apply of a cluster-scope window → POST /api/cluster/v2/settings/objects; action created
func TestClusterWritesKeepTheGuards(t *testing.T)
// readonly cluster context → exit 6, 0 hits; --validate-only with node version 1.287 → exit 2; delete --yes sends DELETE to the cluster path
```

  Add to `pkg/client/write_test.go`: `TestClusterWriteMethods`. A
  cluster-scoped client sends POST, PUT and DELETE to `/api` + path, and a
  503 is sent once.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./cmd/ ./pkg/client/ -run 'Cluster' -count=1`
Expected: FAIL (build: undefined `PostCluster`; then the commands refuse the
cluster context with exit 2).

- [ ] **Step 3: Implement the client methods and `settings_api.go`, then
  switch every call site.**
  - Change the call sites listed in Files from `GetV2`/`GetV2Paged`/
    `pagedFetch`/`describeFetch`/`PostV2`/`PutV2`/`DeleteV2` to the
    `settings*` functions.
  - Read commands call `a.runSettingsFetch`. Write commands set
    `ClusterToo: true`.
  - `checkWriteContexts` uses `checkEnvironmentContexts` only when the
    selection is not one cluster context.
  - **403 hint:** on a cluster context, `runClusterFetch`'s 403 hint gets
    the declared scope plus "a cluster token needs settings.read (and
    settings.write to write) besides ServiceProviderAPI". Add that wording
    to `clusterHint` for 403, via `fetchSpec.Forbidden`, a new optional
    hint.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./... -count=1`
Expected: PASS. Phases 1 and 2's tests stay green unchanged: the
environment path is the same.

- [ ] **Step 5: Docs and commit.**
  - **README scope list:** the `settings.read` and `settings.write` lines
    say "(on an environment token, or a cluster token for the cluster's
    own settings; creating cluster settings also needs
    ServiceProviderAPI)".
  - Run `make docs`.

```bash
git add pkg/client cmd README.md docs/reference
git commit -m "feat: the settings commands work on cluster contexts"
```

---

### Task 2: Cluster ActiveGate auto-update — reading

**Files:**
- Create: `cmd/activegate_updates.go`, `cmd/activegate_updates_test.go`
- Create fixtures:
  - `cmd/testdata/api/cluster_activegates.json`: 2 cluster ActiveGates
    with `id`, `hostname` (`ag1.example.com`, `ag2.example.com`), `type
    CLUSTER`, `version`, `autoUpdateSettings {setting, effectiveSetting}`
    and `autoUpdateStatus`;
  - `cluster_ag_autoupdate_global.json`: `{"globalSetting":"ENABLED"}`;
  - `cluster_ag_autoupdate_global_flagged.json`: the same, plus
    `targetVersion` `"latest"` and
    `updateWindows {windows:[{id:"uw-c1",name:"Cluster window"}]}`.
- Modify: `cmd/get.go`, `cmd/describe.go`, `README.md` (verbs table,
  cluster resources), `docs/reference/`

**Interfaces:**
- Produces:
  - **Paths:** `const clusterAGPath = "/cluster/v2/activeGates"`;
    `clusterAGAutoUpdatePath = clusterAGPath + "/autoUpdate"`.
  - **Commands:**
    - `newGetActiveGateUpdatesCmd(a *App)`, Use `activegate-updates
      [activegate-id]`, alias `ag-updates`, wrapped in `clusterCommand`;
    - `newDescribeActiveGateUpdatesCmd(a *App)`, the same, also wrapped.
  - **`get` without an ID:**
    - fetches the global configuration and
      `GET /cluster/v2/activeGates?type=CLUSTER`;
    - the body is `{"global": <global object>, "activeGates": <list's
      activeGates>}`;
    - table rows `clusterAGUpdatesRow{ID, Hostname, Version, Setting,
      Effective, Status, TargetVersion, Windows}`: the first row is
      `ID "(global)"` with `Setting` = `globalSetting`, then one row per
      ActiveGate from its `autoUpdateSettings` and `autoUpdateStatus`;
    - TargetVersion and Windows are `—` unless the global configuration
      carries them.
  - **With an ID:** `GET …/activeGates/{id}/autoUpdate`, the object as
    returned, one row.
  - **`describe`:** the global object, or one ActiveGate's object, as a
    YAML summary.
  - **A 404 on an ID** gets the hint "List cluster ActiveGates with: dtmgd
    get activegate-updates".

- [ ] **Step 1: Write the failing tests**

```go
func TestActiveGateUpdatesTable(t *testing.T)       // golden; global row first; TARGET-VERSION and WINDOWS "—" without the flag
func TestActiveGateUpdatesFlagged(t *testing.T)     // flagged global → global row shows "latest" and "Cluster window"
func TestActiveGateUpdatesListsClusterActiveGates(t *testing.T) // Review Focus 5: the list query carries type=CLUSTER
func TestActiveGateUpdatesByID(t *testing.T)        // get activegate-updates ag-1 → GET /api/cluster/v2/activeGates/ag-1/autoUpdate
func TestActiveGateUpdatesUnknownID(t *testing.T)   // Review Focus 4: 404 → exit 4, hint names get activegate-updates
func TestActiveGateUpdatesNeedACluster(t *testing.T) // environment context → exit 2 (clusterCommand)
func TestActiveGateUpdatesJSONShape(t *testing.T)   // -o json → {"global":{...},"activeGates":[...]}
```

- [ ] **Step 2: Run them to verify they fail.**
  `go test ./cmd/ -run 'ActiveGateUpdates' -count=1` gives FAIL, unknown
  resource.

- [ ] **Step 3: Implement** with `runClusterFetch`. Register both commands,
  update the `get`/`describe` Long lists and the README cluster resources,
  then run `make docs`.

- [ ] **Step 4: Run the tests.**
  `go test -race ./cmd/ -count=1`, then `go test ./...`, gives PASS.

- [ ] **Step 5: Commit.**
  `git commit -m "feat: get and describe activegate-updates on cluster contexts"`

---

### Task 3: Cluster ActiveGate auto-update — writing, and cluster window references

**Files:**
- Create: `cmd/update.go` (the `update` verb), `cmd/activegate_updates_write_test.go`
- Modify:
  - `cmd/activegate_updates.go`: the update and edit commands;
  - `cmd/writes.go`: `plannedWrite.Exec`;
  - `cmd/delete.go`: cluster window references;
  - `cmd/app.go`: register `update`;
  - `README.md`, `docs/reference/`.

**Interfaces:**
- Consumes: Task 2 paths and row logic; phase 2 `runWrites`, `plannedWrite`,
  `writeResult`, `editValue`, `isInteractive`, `withWrites`-style marking.
- Produces:
  - **The per-write executor:**
    `plannedWrite.Exec func(c *client.Client, validateOnly bool) writeResult`.
    `executeWrite` calls it when set. The version gate skips writes with
    `Exec` (they validate their own way).
  - **The `update` verb:** `newUpdateCmd(a *App)`, holding
    `newUpdateActiveGateUpdatesCmd(a *App)`. Use `activegate-updates
    [activegate-id] -f <file>`, with `--dry-run` and `--validate-only`. It
    is a cluster command and is marked mutating.
  - **`newEditActiveGateUpdatesCmd(a *App)`** under `edit`, people only.
  - **The plan:** `func planActiveGateUpdate(c *client.Client, agID string, value map[string]interface{}) ([]plannedWrite, error)`.
    1. GET the current object.
    2. Refuse (a `UsageError`):
       - `targetVersion` or `updateWindows` in the file when the GET has
         neither;
       - for the global configuration, `setting` or `globalSetting:
         INHERITED`.
    3. The same value gives `unchanged`. Otherwise an `update` whose
       `Exec` PUTs the value, or POSTs it to `…/validator` with
       `--validate-only`: 204 gives `validated`, 400 gives `failed` with
       the constraint violations.
    4. Its `SchemaID` reads `cluster-activegate-autoupdate` and its `Scope`
       reads `global` or the ActiveGate ID, for the report.
  - **The cluster reference check:**
    `func clusterWindowReferences(c *client.Client, windowID string) ([]string, error)`.
    - Read the global configuration. If it has no `updateWindows`, return
      none: the cluster ignores them.
    - Otherwise check the global configuration and each cluster
      ActiveGate's configuration (`GET …/{id}/autoUpdate`) for
      `updateWindows.windows[].id == windowID`.
    - Return "global" or ActiveGate IDs.
    - `planDelete` calls it on a cluster context instead of
      `windowReferences`.

- [ ] **Step 1: Write the failing tests**

```go
func TestUpdateActiveGateUpdatesGlobal(t *testing.T)  // readwrite-all: file {globalSetting: DISABLED} → PUT /api/cluster/v2/activeGates/autoUpdate; action updated
func TestUpdateActiveGateUpdatesValidateOnly(t *testing.T) // → POST …/autoUpdate/validator, no PUT; 204 → validated; 400 body → failed with violations; no version read
func TestUpdateActiveGateUpdatesReadonly(t *testing.T) // readonly cluster context → exit 6, 0 hits
func TestUpdateRefusesUnhonouredFields(t *testing.T)  // GET without targetVersion, file with targetVersion → exit 2, no PUT; flagged GET → PUT carries it
func TestGlobalAutoUpdateRefusesInherited(t *testing.T) // Review Focus 3: global file with INHERITED → exit 2
func TestUpdateActiveGateUpdatesPerAG(t *testing.T)   // ag-1 file {setting: INHERITED} → PUT …/ag-1/autoUpdate
func TestUpdateActiveGateUpdatesUnchanged(t *testing.T) // same value → unchanged, no PUT
func TestEditActiveGateUpdates(t *testing.T)          // runInteractive + fake editor → PUT; agent mode → exit 2
func TestDeleteRefusesAClusterWindowInUse(t *testing.T) // cluster context, flagged global referencing uw-c1 → delete settings uw-c1 --yes exits 1, no DELETE
func TestDeleteClusterWindowWithoutTheFlag(t *testing.T) // global without updateWindows → DELETE sent
```

- [ ] **Step 2: Run them to verify they fail.** `go test ./cmd/ -run
  'ActiveGateUpdates|UnhonouredFields|Inherited|ClusterWindow' -count=1`
  gives FAIL.

- [ ] **Step 3: Implement.** Keep `planActiveGateUpdate` under `gocognit` 15:
  `checkAutoUpdateFile`, then build the write. Register `update`, add the
  README verbs row (`update`: `activegate-updates`, cluster contexts), then
  run `make docs`.

- [ ] **Step 4: Run the tests.** `go test -race ./... -count=1` gives PASS.

- [ ] **Step 5: Commit.**
  `git commit -m "feat: update and edit activegate-updates; cluster windows in use are not deleted"`

---

### Task 4: Docs, skill and knowledge base

**Files:**
- Modify `skills/dtmgd/references/cluster-api.md`:
  - cluster settings (reads, writes, scope `cluster`, token scopes);
  - `activegate-updates` (what the columns mean, the flag-dependent
    fields, `--validate-only` through the validator).
- Modify `skills/dtmgd/references/settings.md`: a "Cluster contexts"
  paragraph.
- Modify `skills/dtmgd/SKILL.md`: one line under Settings for cluster
  contexts; a References bullet update.
- Modify `docs/configuration.md` (Cluster Contexts):
  - "Read-only" becomes: the Cluster API is read except settings and
    ActiveGate auto-update, which follow the context's write level;
  - the cluster token scopes.
- Modify `AGENTS.md`: the "Cluster Management API is still read-only"
  sentence; architecture lines for `settings_api.go`.
- Modify `.claude/CLAUDE.md`: an entry, "Settings on cluster contexts and
  cluster ActiveGate auto-update":
  - the routing layer;
  - paths;
  - scopes;
  - the feature-flag fields;
  - the validator.

- [ ] **Step 1: Write them.** Check the skill against the CLI with the
  phase 2 script, adding `update` to the verbs. Expected: no `MISSING` and
  no `UNKNOWN FLAG`.
- [ ] **Step 2:** `go test ./... -count=1 && make lint`, then commit:
  `git commit -m "docs: settings and ActiveGate auto-update on cluster contexts"`

---

### Task 5: Live checks on the clusters

**Files:**
- Create: `test/integration/settings_cluster_test.go`
- Modify: `test/integration/harness_test.go`:
  - `writeConfig` takes the write context from `DTMGD_IT_WRITE_ENV` or the
    new `DTMGD_IT_WRITE_CLUSTER`;
  - `writeRun` accepts `update`;
  - the `.integrationtests.env.example` line.

**Interfaces:**
- Consumes: `run`, `decode`, `brief`, `writeConfig`, `writeRun`, `cluster(t)`.

- [ ] **Step 1: Write the checks**

```go
func TestClusterSettingsSchemasRead(t *testing.T)    // get settings-schemas --context <cluster> -o json: includes builtin:deployment.management.update-windows (skip on exit 5: token lacks settings.read)
func TestClusterUpdateWindowsDecode(t *testing.T)    // get settings --schema …update-windows --scope cluster: recurrence values valid
func TestActiveGateUpdatesRead(t *testing.T)         // get activegate-updates -o json: global.globalSetting is ENABLED or DISABLED; every activeGates[].type is CLUSTER
func TestClusterWindowLifecycle(t *testing.T)        // DTMGD_IT_WRITE_CLUSTER: sweep dtmgd-it-*, apply (disabled, 2030) → created, apply changed → updated, delete --yes → gone
func TestActiveGateUpdatesOnlyValidated(t *testing.T) // update activegate-updates with the current globalSetting flipped: --validate-only → validated; --dry-run → dryRun; GET unchanged afterwards
```

- [ ] **Step 2:** `go vet -tags integration ./test/integration/`, and the
  offline run skips them. Commit:
  `git commit -m "test: live checks for cluster settings and ActiveGate auto-update"`

- [ ] **Step 3: The live runs** (reads allowed on all four contexts, writes
  on `stage-cluster`):

  ```bash
  DTMGD_IT_CONFIG=~/.config/dtmgd/config DTMGD_IT_ENV=prod DTMGD_IT_CLUSTER=prod-cluster make integration
  DTMGD_IT_CONFIG=~/.config/dtmgd/config DTMGD_IT_ENV=stage DTMGD_IT_CLUSTER=stage-cluster DTMGD_IT_WRITE_ENV=stage DTMGD_IT_WRITE_CLUSTER=stage-cluster make integration
  ```

  - Report counts only.
  - Afterwards confirm, as counts, that `stage-cluster` holds the same
    update windows and the same ActiveGate auto-update `globalSetting` as
    before.
  - A failure on real data gets a fixture and a failing test first.
