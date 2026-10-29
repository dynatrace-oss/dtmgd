# Phase 4 — Cluster Management API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give dtmgd read-only coverage of the Cluster Management API for the four areas the architecture names: environments, nodes and cluster, license, and backups. It works through cluster-scoped contexts, contexts without an `env-id`. Ships as **v0.3.0**. It is breaking: `get environments` becomes the cluster's environment list.

**Architecture:**
- A context without `env-id` is a cluster context. `client.New` accepts an empty env-id and builds a cluster-scoped client. That client refuses environment-API calls and has no UI base URL.
- Environment commands keep `runFetch` and refuse cluster contexts. Cluster commands go through a new `runClusterFetch`, which shares `renderOne`, the client factory, the proxy, retry and error trimming. It runs against one context and has no `--env` fan-out.
- `--env ALL_ENVIRONMENTS` selects environment contexts only.
- `doctor` checks both kinds of context. The cluster version now comes from the node list's `buildVersion`.

**Tech Stack:** Go 1.26.10, Cobra, resty v2, `dtctl/sdk/agentmode`, golangci-lint v2.15 (`gocognit` ≤ 15). No new module dependencies.

**Spec:** `docs/superpowers/specs/2026-10-29-dtmgd-catch-up-design.md`, the Phase 4 section plus Goals, Non-goals, Anti-goals and Constraints. The architecture source is `../cli-tool-architecture/README.md`, its dtmgd section and decision log. API shapes come from `../docs/managed/_data/openapi/cluster.json` (`/api/cluster/v2`) and `clusterv1.json` (`/api/v1.0/onpremise`), checked live in Task 1.

## Global Constraints

- **Version.** This releases as **v0.3.0**, not the spec's v0.2.0, because Phase 3 already used v0.2.0 (see the Phase 3 plan's Global Constraints).
- **One branch, one PR.** `feat/phase4-cluster-api` from `main` holds Tasks 2–9 plus this plan. Task 1 changes no repository file.
- **Read-only.** dtmgd sends only `GET` requests to the Cluster API, and only to these paths, relative to the context's host:
  - `/api/cluster/v2/environments`
  - `/api/cluster/v2/environments/{id}`
  - `/api/cluster/v2/clusterLicense`
  - `/api/cluster/v2/clusterLicense/environment/total`
  - `/api/v1.0/onpremise/cluster`
  - `/api/v1.0/onpremise/cluster/configuration`
  - `/api/v1.0/onpremise/cluster/metadata`
  - `/api/v1.0/onpremise/backup/config`

  `/backup/config/status` is not used: it reports the status of a configuration change request, not of backups.
- **Context kinds.**
  - A context is a cluster context exactly when its `env-id` field is empty in the file, before `${VAR}` expansion. `env-id: ${DT_ENV_ID}` is an environment context.
  - Never pair environment and cluster contexts by host. An environment context's host is usually a node on port 9999, where the Cluster API answers 404.
  - The cluster token needs the `ServiceProviderAPI` scope.
- **Outward actions.** Get the owner's go-ahead at the time of every push, PR, merge, tag and live-cluster call. PRs are opened with `gh` as the owner.
- **Tokens and secrets.**
  - Never read token values from `~/.config/dtmgd/config`; filter out `token:` lines.
  - Tokens are used only through dtmgd's own code (`config.GetToken`) and are never printed.
  - Tests never use the default config path.
  - dtmgd masks `licenseKey` in every output format as `****` + its last 4 characters. No live check prints license keys, contact emails, node IP addresses or storage paths in full.
- **Tests.**
  - Local suite: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...`. Never `./pkg/skills`.
  - HTTP error tests use 400, 401, 403, 404 or 422, never 5xx.
  - Command tests go through the harness in `cmd/harness_test.go`.
  - Fixtures are synthetic, shaped like the API. Never copy live data into `cmd/testdata/`.
  - Regenerate goldens with `-update`, read the diff, and name every golden change in its commit.
  - Keep every new test function under cognitive complexity 15 (Sonar S3776); use helpers.
- **Placeholders used in steps.**
  - `<scratchpad>` = `/tmp/claude-502/-workspace-dtmgd/879c970e-7206-4485-81c4-1f96759bf4c3/scratchpad`.
  - `<workspace>` = the plan's SDD workspace printed by `sdd-workspace`, `.superpowers/sdd/2026-10-29-phase4-cluster-api/`, which git ignores.
  - `<lint>` = `<scratchpad>/golangci-lint` (v2.15.0).
- **Lint.** Run `<lint> run ./...` and check its **exit code**, not a piped tail.
- **Commit trailer.** Every commit ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Output shape stays Phase 2's.**
  - json, yaml and agent output carry the API's own response shape. The only additions are keys dtmgd names in this plan: `configuration` on each node, `nodes` on the cluster, and the masked `licenseKey`.
  - Human notes go to stderr, and exit codes are 0–5.
  - A wrong-kind context is a `UsageError`: exit code 2, agent code `validation_error`.

## Review Focus

1. **A cluster context whose host is a node address** (`https://node:9999`) or ends in `/e/<env-id>`. Every cluster call then returns 404. The user should get a hint that names the context and says which address to use, not "check the ID". Owner: Task 4, `TestClusterCommand404Hint`.
2. **An environment API token stored for a cluster context.** The call returns 401 (or 403). The hint must say that a Cluster API token with `ServiceProviderAPI` is needed, both from a command and from `doctor`. Owner: Task 4, `TestClusterCommand401Hint`; Task 8, `TestDoctorClusterContextWrongToken`.
3. **A config holding both kinds of context, or only cluster contexts, with `--env ALL_ENVIRONMENTS`.** Fan-out must never call a cluster context. With no environment contexts, the error must say so. The owner's own config has both kinds today, so `ALL_ENVIRONMENTS` currently fails partly with "env-id is required". Owner: Task 2, `TestAllEnvironmentsSkipsClusterContexts` and `TestAllEnvironmentsOnlyClusterContexts`; Task 3, `TestAllEnvironmentsIgnoresClusterContexts`.
4. **A cluster on the Dynatrace Platform Subscription (DPS).**
   - `describe license` and `get license-usage` must say that the license API works only with classic licensing.
   - `get environments --consumption` must say that consumption is unavailable on DPS.
   - The exit code must stay the API's.

   Owner: Task 4, `TestGetEnvironmentsConsumptionOnDPS`; Task 6, `TestDescribeLicenseOnDPS`.
5. **A cluster in the middle of an upgrade, with nodes on different versions.** `doctor` and `describe cluster` must judge and show the lowest version and say the versions are mixed. An empty node list must not crash. Owner: Task 2, `TestClusterVersionIsLowestNode` and `TestClusterVersionNoNodes`; Task 5, `TestDescribeClusterMixedVersions`.

---

### Task 1: Live shape spike (read-only, owner-gated)

Settles the spec's open unknowns before any code depends on them:
- the response shapes (above all, whether `/cluster` is an array),
- the paging protocol,
- each cluster's licensing model, and what the API returns for consumption on DPS,
- what an environment token gets from the Cluster API.

**Files:**
- Create (scratch, never committed): `/tmp/claude-502/-workspace-dtmgd/879c970e-7206-4485-81c4-1f96759bf4c3/scratchpad/spike/{go.mod,go.sum,main.go}`
- Create (in the plan's git-ignored SDD workspace): `<workspace>/spike.md`

**Interfaces:**
- Consumes: `config.LoadFrom`, `config.DefaultConfigPath`, `(*Config).GetContext`, `(*Context).Resolve`, `(*Config).GetToken`, `client.New` (with a placeholder env-id, since the current `New` requires one), `(*Client).GetCluster`.
- Produces: `spike.md` (shapes and statuses, no values) and ledger rulings that later tasks read.

- [ ] **Step 1: Confirm the owner's go-ahead.** It is requested at plan handoff, together with plan approval. Without it, skip to Task 2 and build on the OpenAPI shapes. Record `Task 1: Ruling: spike skipped — …` and repeat each check in Task 10.

- [ ] **Step 2: Write the spike program.**
  - `go.mod`: `module spike`, `require github.com/dynatrace-oss/dtmgd v0.0.0`, `replace github.com/dynatrace-oss/dtmgd => /workspace/dtmgd`. Copy `/workspace/dtmgd/go.sum` to `go.sum`.
  - For each context, `prod-cluster` and `stage-cluster`, it prints `<ctx> <path> -> ok|<err.Error()>` and then `shape(v)`. `err.Error()` is safe to print: it carries the status and the trimmed API message only.
  - `shape` prints an object as its sorted keys, each with a type. An array prints as `[n]` followed by the union of its elements' shapes. A number prints as `number`, a bool as its value, and a string as `string`.
  - A string's value is printed only for these keys: `state`, `trial`, `operationState`, `buildVersion`, `licenseType`, `licenseStatus`, `usageStatus`, `kubernetesRole`.
  - `totalCount` and `pageSize` print their values. `nextPageKey` prints `present` or `absent`.

- [ ] **Step 3: Run it.** Every request is a GET:
  1. `/cluster/v2/environments?pageSize=1`.
  2. If (1) has a `nextPageKey`, the same path with **only** `nextPageKey` (status only).
  3. `/cluster/v2/environments?pageSize=1&includeConsumptionInfo=true`
  4. `/cluster/v2/environments?pageSize=1&includeStorageInfo=true`
  5. `/cluster/v2/environments/<first id from (1)>`. The program uses the ID without printing it.
  6. `/cluster/v2/environments/dtmgd-spike-missing`: status and message.
  7. `/cluster/v2/clusterLicense`
  8. `/cluster/v2/clusterLicense/environment/total`
  9. `/v1.0/onpremise/cluster`
  10. `/v1.0/onpremise/cluster/configuration`
  11. `/v1.0/onpremise/cluster/metadata`
  12. `/v1.0/onpremise/backup/config`
  13. Request (1) again using the **environment** context's token (`prod`, then `stage`) against the cluster host. This records the status an environment token gets.

  Run: `cd <scratchpad>/spike && GOFLAGS=-mod=mod go run . > <workspace>/spike.md 2>&1; echo exit=$?`
  Expected: `exit=0`. Each line is a shape or a status, and no token, key, email, IP address or path appears. Check with `grep -cE 'dt0[cs]01|@|[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:' <workspace>/spike.md`; the expected count is 0, apart from `buildVersion` values.

- [ ] **Step 4: Compare with the plan's assumptions, and rule on each difference.**
  - The assumptions are:
    - (a) `/cluster` is an array of objects with `id`, `buildVersion`, `operationState`, `clusterMemberAddress` and `osInfo`.
    - (b) Page 2 accepts `nextPageKey` alone.
    - (c) The environment keys match the OpenAPI `Environment` schema.
    - (d) `/cluster/configuration` is `{"clusterNodes":[{id, ipAddress, webUI, agent, datacenter, kubernetesRole}]}`.
    - (e) `/cluster/metadata` is `{clusterName, clusterUuid}`.
    - (f) The backup config matches `backupConfigDto`.
    - (g) An environment token gets 401 from the Cluster API.
  - Record the licensing model and the consumption status for each cluster.
  - For each difference, write `Task 1: Ruling: <finding> — <the change to Task N> — <cost if wrong>` in the ledger. Pre-decided rulings:
    - If `/cluster` is an object with `version`, `ClusterVersion` decodes both shapes.
    - If page 2 needs `pageSize`, `GetClusterPaged` sends `pageSize` with `nextPageKey`.
    - If an environment token gets 403, the 401 hint also applies to 403 on cluster commands.
    - If `/backup/config` fails on both clusters with anything other than a transient error, drop Task 7. Ship the other three areas, and record the gap in the README and the release notes, as the spec says.
    - If `/cluster/configuration` or `/cluster/metadata` fails, Task 5 builds on the endpoint that works. The degraded paths it already has (warnings) become the documented behaviour.

- [ ] **Step 5: Clean up.** `rm -rf <scratchpad>/spike`. Keep `spike.md`; it holds shapes only.

---

### Task 2: Cluster-scoped contexts in `pkg/config` and `pkg/client`

**Files:**
- Modify: `pkg/config/config.go` (add `IsCluster`; update `Context`'s doc comment)
- Modify: `pkg/client/client.go` (`New`, `ErrClusterScoped`, `ClusterScoped`, the environment-call guard, `ClusterVersion`)
- Modify: `pkg/client/paging.go` (`GetClusterPaged`, with a shared `getPaged`)
- Modify: `pkg/client/multi.go` (`resolveContexts`)
- Create: `pkg/client/version.go` (`ParseClusterVersion` moved from `cmd/doctor.go`, plus `LowestClusterVersion`)
- Modify: `cmd/doctor.go` (`judgeVersion` uses `client.ParseClusterVersion`; delete `parseClusterVersion`)
- Test: `pkg/config/config_test.go`, `pkg/client/client_test.go`, `pkg/client/cluster_test.go`, `pkg/client/paging_test.go`, `pkg/client/multi_test.go`, `pkg/client/version_test.go`, `cmd/doctor_test.go` (fakes only)

**Interfaces:**
- Consumes: Task 1 rulings on the `/cluster` shape and on paging.
- Produces:
  - `func (c Context) IsCluster() bool`: `c.EnvID == ""`.
  - `func New(host, envID, token string) (*Client, error)`. An empty `envID` builds a cluster-scoped client with `apiBaseURL == ""` and `dashboardURL == ""`. host and token are still required.
  - `var ErrClusterScoped = errors.New("this is a cluster context (no env-id); environment API calls need an environment context")`
  - `func (c *Client) ClusterScoped() bool`
  - `GetV2`, `GetV2WithValues`, `GetV1` and `GetV2Paged` return `ErrClusterScoped` on a cluster-scoped client without sending a request.
  - `func (c *Client) GetClusterPaged(path string, params map[string]string, p Paging) (map[string]interface{}, error)`. `path` is relative to `{host}/api`, for example `/cluster/v2/environments`. Paging semantics are `GetV2Paged`'s.
  - `func ParseClusterVersion(v string) ([3]int, bool)`, with the same behaviour as the old `cmd` function.
  - `func LowestClusterVersion(versions []string) string`. It returns the lowest version that parses, ignoring the rest, and `""` when none parses.
  - `func (c *Client) ClusterVersion() (string, error)`:
    - It returns `LowestClusterVersion` of the `buildVersion`s from `GET /v1.0/onpremise/cluster`.
    - An environment-scoped client falls back to `/v1/config/clusterversion` exactly as today: after an HTTP error, or when no version came back.
    - A cluster-scoped client never falls back; it returns the error, or `("", nil)` when no version came back.
  - `resolveContexts`:
    - `ALL_ENVIRONMENTS` returns environment contexts only. With none, it returns the error `no environment contexts configured (cluster contexts are not part of ALL_ENVIRONMENTS)`.
    - A `;` list that names a cluster context returns the error `context %q is a cluster context (no env-id); --env selects environment contexts`.

- [ ] **Step 1: Write the failing tests.**

```go
// pkg/config/config_test.go
func TestIsCluster(t *testing.T) {
	for envID, want := range map[string]bool{"": true, "abc12345": false, "${DT_ENV_ID}": false} {
		if got := (Context{EnvID: envID}).IsCluster(); got != want {
			t.Errorf("IsCluster(%q) = %v, want %v", envID, got, want)
		}
	}
}

// pkg/client/client_test.go: replaces the "empty envID should error" case.
func TestNewClusterScoped(t *testing.T) {
	c, err := New("https://cluster.example.com/", "", "tok")
	// err == nil; c.ClusterScoped(); c.DashboardURL() == ""; c.APIBaseURL() == "";
	// c.ClusterURL() == "https://cluster.example.com/api"
}
```

- `TestEnvironmentCallsRefusedOnClusterClient`: httptest server counting hits. On a cluster-scoped client, each of `GetV2`, `GetV2WithValues`, `GetV1` and `GetV2Paged` gives `errors.Is(err, ErrClusterScoped)`, and the server's hit count is 0.
- `TestGetClusterPagedFollowsNextPageKey`:
  - Page 1 at `/api/cluster/v2/environments` is `{"environments":[{"id":"a"}],"totalCount":2,"nextPageKey":"k"}`. Page 2, for `nextPageKey=k`, is `{"environments":[{"id":"b"}],"totalCount":2}`.
  - The merged result has 2 environments and no `nextPageKey`.
  - The second request's query is exactly `nextPageKey=k`, unless Task 1 ruled otherwise.
- `TestClusterVersionIsLowestNode`: `/api/v1.0/onpremise/cluster` returns `[{"buildVersion":"1.330.18.20251017-120000"},{"buildVersion":"1.328.5.20250901-090000"}]`. Both client kinds get `"1.328.5.20250901-090000"`.
- `TestClusterVersionClusterScopedNoFallback`: on a cluster-scoped client, `/cluster` answers 401. The result is an `*ErrAPI` with status 401, and the hit count on `/e//api/v1/config/clusterversion` (or any `/v1/config` path) is 0.
- `TestClusterVersionNoNodes`:
  - `/cluster` returns `[]`.
  - The environment client falls back to v1, which returns `{"version":"1.330.0"}`, and gets `"1.330.0"`.
  - The cluster client gets `("", nil)`.
- `TestLowestClusterVersion`:
  - `{"1.330.1.x","1.329.9.y"}` gives `"1.329.9.y"`.
  - `{"garbage","1.330.1.x"}` gives `"1.330.1.x"`.
  - `{}` gives `""`.
- Update the existing `ClusterVersion` tests and the `cmd/doctor_test.go` helper `versionAPI` so that `/cluster` serves `[{"buildVersion":v}]` instead of `{"version":v}`.
- `TestAllEnvironmentsSkipsClusterContexts`: config with environment context `a` and cluster context `c` (no `env-id`). `MultiRequest(cfg, "ALL_ENVIRONMENTS", …)` returns results named exactly `[a]`, and the call never runs for `c`.
- `TestAllEnvironmentsOnlyClusterContexts`: config with cluster contexts only. The error contains `no environment contexts configured`.
- `TestNamedClusterContextInEnvList`: `MultiRequest(cfg, "a;c", …)` returns an error containing `"c" is a cluster context`.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./pkg/config/ ./pkg/client/ -run 'IsCluster|ClusterScoped|ClusterClient|ClusterPaged|ClusterVersion|LowestClusterVersion|AllEnvironments|NamedClusterContext' 2>&1 | tail -30`
  Expected: compile errors (`IsCluster`, `ClusterScoped`, `ErrClusterScoped`, `GetClusterPaged` and `LowestClusterVersion` are undefined) and the `/cluster` array decode failure.

- [ ] **Step 3: Implement.**
  - Move the page loop of `GetV2Paged` into `getPaged(rc *resty.Client, fullPath string, params map[string]string, p Paging)`. `GetV2Paged` calls it with `c.http` and `"/v2"+path`; `GetClusterPaged` calls it with `c.cluster` and `path`.
  - `ClusterVersion` decodes `[]struct{ BuildVersion string `json:"buildVersion"` }`.
  - Keep `ClusterVersion` under gocognit 15 by extracting `nodesVersion() (string, error)`.

- [ ] **Step 4: Run the tests to verify they pass.**
  Run: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -15`
  Expected: `ok` for all four packages.

- [ ] **Step 5: Commit.**

```bash
git add pkg/config pkg/client cmd/doctor.go cmd/doctor_test.go
git commit -m "feat(client): cluster-scoped contexts: clients without env-id, cluster paging, version from nodes"
```

---

### Task 3: Environment commands refuse cluster contexts; `ctx set` creates them

**Files:**
- Create: `cmd/contextkind.go`
- Modify: `cmd/runner.go` (`runFetch` calls `checkEnvironmentContexts` after `checkEnvVarContext`)
- Modify: `cmd/ctx.go` (`setContext`, `ContextListItem`, `listContexts`)
- Modify: `cmd/config.go` (the `set-context` Long help)
- Modify: `cmd/harness_test.go` (`withFakeContexts`)
- Test: `cmd/contextkind_test.go`, `cmd/ctx_test.go`

**Interfaces:**
- Consumes: `Context.IsCluster`, and `resolveContexts`' `ALL_ENVIRONMENTS` filtering (Task 2).
- Produces:
  - `func contextNamesOfKind(cfg *config.Config, cluster bool) []string`, in config order.
  - `func (a *App) checkEnvironmentContexts(cfg *config.Config, s fetchSpec) error`. A single context is checked through `cfg.CurrentContext`. Each name in a `;` list is checked. `ALL_ENVIRONMENTS` is left to the client. A name that does not exist is left to the existing not-found errors.
  - `func (a *App) checkClusterContext(cfg *config.Config, s fetchSpec) error`. It is used by Task 4. It checks `cfg.CurrentContext`, and a missing context is left to `NewForContext`.
  - `func wrongContextError(name string, wantCluster bool, s fetchSpec, others []string) *UsageError`
  - Harness: `func withFakeContexts(t *testing.T, current string, envs, clusters []fakeEnv) string`. Environment contexts get `env-id: env1`, cluster contexts get no `env-id`, and every context gets the token `tok-<name>`.
- Fixed copy (`%s` is replaced by `dtmgd <Verb> <Resource>`, for example `dtmgd get problems`):
  - An environment command on a cluster context:
    - Error: `context "c" is a cluster context (no env-id); "dtmgd get problems" needs an environment context`
    - Suggestion: `Environment contexts: a, b. Pass one with --context <name>, or make one current with: dtmgd ctx <name>`
    - When none exist: `No environment contexts are configured. Create one with: dtmgd ctx set <name> --host <url> --env-id <id> --token-ref <ref>`
  - A cluster command on an environment context:
    - Error: `context "a" is an environment context; "dtmgd get nodes" needs a cluster context (one without env-id)`
    - Suggestion: `Cluster contexts: c. Pass one with --context <name>`
    - When none exist: `No cluster contexts are configured. Create one with: dtmgd ctx set <name> --host <cluster-url> --token-ref <ref> (no --env-id; its token needs the ServiceProviderAPI scope)`
  - `ctx set` / `config set-context` creating a context without `--env-id`:
    - stdout, the usual success line: `Context "c" set`
    - stderr info: `"c" is a cluster context (no --env-id): use it with cluster commands, e.g. dtmgd get environments --context c`
  - `ContextListItem` gains `Scope string `table:"SCOPE"``, placed after NAME, with the value `cluster` or `environment`.
  - The `set-context` Long help adds, after the `%SCOPES%` block: `Without --env-id the context is a cluster context, for the Cluster Management API (get environments, get nodes, describe cluster|license|backup). Its --host is the cluster's own address, as used by the Cluster Management Console, and its token needs the ServiceProviderAPI scope.`

- [ ] **Step 1: Write the failing tests** (`cmd/contextkind_test.go`, `cmd/ctx_test.go`).
  - `TestEnvCommandRefusesClusterContext`:
    - Setup: `withFakeContexts(t, "c", []fakeEnv{{"a", envAPI}}, []fakeEnv{{"c", clusterAPI}})`; run `get problems`.
    - `r.Code == 2`.
    - Stderr contains `context "c" is a cluster context` and `Environment contexts: a`.
    - `clusterAPI` received 0 requests.
  - `TestEnvCommandRefusesClusterContextInEnvList`: `get problems --env "a;c"`. `r.Code == 2`, stderr names `"c"`, and both APIs received 0 requests.
  - `TestAllEnvironmentsIgnoresClusterContexts`:
    - Run `get problems --env ALL_ENVIRONMENTS -o json` with `envAPI` serving `problems_small.json`.
    - `r.Code == 0`, and the JSON object's keys are exactly `["a"]`.
    - `clusterAPI` received 0 requests.
  - `TestWrongContextAgentEnvelope`: as the first test, under `CLAUDECODE=1`. The envelope has `ok:false`, `error.code == "validation_error"`, and a suggestion containing `Environment contexts: a`.
  - `TestCtxSetWithoutEnvIDCreatesClusterContext`:
    - Run `ctx set c --host https://cluster.example.com --token-ref t` on a temp config.
    - `r.Code == 0`, and stderr contains `is a cluster context`.
    - Loading the file gives context `c` with `EnvID == ""`.
  - `TestCtxListShowsScope`: on a config with both kinds, the `ctx` table has a `SCOPE` header, `cluster` on `c`'s row and `environment` on `a`'s row.
  - Update whatever pinned `--env-id is required for new contexts`.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./cmd/ -run 'RefusesClusterContext|IgnoresClusterContexts|WrongContextAgent|CtxSetWithoutEnvID|CtxListShowsScope' 2>&1 | tail -30`
  Expected: compile error: `withFakeContexts` is undefined. Once that helper exists, assertion failures follow: the `env-id is required` text, and no SCOPE column.

- [ ] **Step 3: Implement.** Add the helpers in `cmd/contextkind.go`, and the `runFetch` call right after `checkEnvVarContext`. In `setContext`, drop the `--env-id is required` check and print the cluster-context info line when a new context has no env-id.

- [ ] **Step 4: Run the tests and regenerate goldens.**
  Run: `go test ./cmd/ -run 'RefusesClusterContext|IgnoresClusterContexts|WrongContextAgent|CtxSetWithoutEnvID|CtxListShowsScope' && go test ./cmd/ -update && git diff --stat cmd/testdata && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8`
  Expected: PASS. Golden diffs only add the SCOPE column; read each one.

- [ ] **Step 5: Commit** (name every golden change in the message).

```bash
git add cmd
git commit -m "feat(cmd): environment commands refuse cluster contexts; ctx set without --env-id creates one"
```

---

### Task 4: Cluster command pipeline; `get environments` and `describe environment`

**Files:**
- Create: `cmd/cluster.go` (`runClusterFetch`, `clusterCommand`, `isClusterCommand`, `clusterPagedFetch`, `clusterObjectFetch`, the cluster hints)
- Rewrite: `cmd/get_environments.go`
- Create: `cmd/describe_environment.go`
- Modify: `cmd/runner.go` (`fetchSpec.NotFound`, `fetchSpec.WrongContext`)
- Modify: `cmd/errors.go` (`hintError`, `withHint`, `hintFor`; `reportError` uses `hintFor`)
- Modify: `cmd/scopes.go` (`scopeServiceProviderAPI`; `%SCOPES%` excludes it)
- Modify: `cmd/commands.go` (`catalogCommand.ClusterScoped`)
- Modify: `cmd/get.go`, `cmd/describe.go` (registration; a "Cluster resources" block in Long)
- Modify: `README.md` (scope list only; the rest of the docs is Task 9)
- Test: `cmd/get_environments_test.go`, `cmd/cluster_test.go`, `cmd/scopes_test.go`, `cmd/commands_test.go`. Delete `TestGetEnvironmentsDeprecationNote` from `cmd/doctor_test.go`.
- Fixtures: `cmd/testdata/api/cluster_environments_page1.json` (2 environments, `totalCount: 3`, `nextPageKey: "k2"`), `cluster_environments_page2.json` (1 environment), `cluster_environment.json` (one `Environment` with `quotas.hostUnits {currentUsage: 12, maxLimit: 100}`, `quotas.davisDataUnits {consumedThisYear: 5000.5, annualLimit: 100000}`, `storage`, `tags`), `cluster_error_422_dps.json` (`{"error":{"code":422,"message":"Consumption information is not available for the DPS licensing model."}}`).

**Interfaces:**
- Consumes: `checkClusterContext`, `wrongContextError` (Task 3); `GetClusterPaged`, `ClusterScoped` (Task 2).
- Produces:
  - `func (a *App) runClusterFetch(s fetchSpec) error`. In order, it:
    1. checks `limitFlag`;
    2. rejects multi-environment `--env` with a `UsageError`: `--env %q selects environments; "dtmgd %s %s" runs against one cluster context. Use --context <name>`;
    3. loads the config and runs `checkEnvVarContext`;
    4. runs `checkClusterContext`, appending `s.WrongContext` to its suggestions;
    5. builds the client with `client.NewForContext`;
    6. applies the agent default limit to lists;
    7. calls `Fetch`, wrapping its error with `clusterHint`;
    8. calls `renderOne`.
  - `fetchSpec` gains two fields. `NotFound string` is the hint for a 404 from a cluster command; `""` means the default hint. `WrongContext string` is an extra suggestion shown when the command runs against an environment context.
  - `func clusterHint(err error, ctxName string, s fetchSpec) error`. It applies `withHint` to:
    - a 401 (and a 403 too, if Task 1 ruled so), giving `hintClusterAuth`;
    - a 404, giving `s.NotFound` or, when that is empty, `fmt.Sprintf(hintNoClusterAPI, ctxName)`.

    Any other error passes through unchanged.
  - `type hintError struct{ err error; hint string }`, with `Error`, `Unwrap`, and `func withHint(err error, hint string) error`.
  - `func hintFor(err error) string`: the hint of the outermost `hintError` in the chain, else `client.Diagnose(err)`. `reportError` uses it, and still appends the scope line for a 403.
  - `const scopeServiceProviderAPI = "ServiceProviderAPI"`.
  - `func clusterCommand(cmd *cobra.Command) *cobra.Command`: `withScopes(cmd, scopeServiceProviderAPI)` plus the annotation `dtmgd.io/cluster: "true"`.
  - `func isClusterCommand(cmd *cobra.Command) bool`.
  - `fillScopeHelp` replaces `%SCOPES%` with `allScopes(root)` minus `scopeServiceProviderAPI`.
  - `catalogCommand.ClusterScoped bool `json:"cluster_scoped,omitempty" yaml:"cluster_scoped,omitempty"``. `catalogSchemaVersion` stays 1: the change is additive.
  - `func clusterPagedFetch(path string, params map[string]string, listKey string, pageCap, maxPages int) func(*client.Client, int) (fetched, error)`, which is `pagedFetch` over `GetClusterPaged`.
  - `func clusterObjectFetch(path string, params map[string]string) func(*client.Client, int) (fetched, error)`, which is `describeFetch` over `GetCluster`.
- Fixed copy:
  - `hintClusterAuth`: `Authentication failed. A cluster context needs a Cluster API token with the ServiceProviderAPI scope, created in the Cluster Management Console; an environment API token does not work here.\n  Store it with: dtmgd config set-credentials <token-ref>`
  - `hintNoClusterAPI`: `No Cluster API answered at context %q's host. A cluster context's host must be the cluster's own address, the one the Cluster Management Console uses: not a node address on port 9999, and without /e/<env-id>.`
  - DPS consumption, a hint applied by `get environments` and `describe environment` to a 422 when `--consumption` is set: `Consumption information is not available on a cluster licensed with the Dynatrace Platform Subscription (DPS). Run without --consumption.`
  - `get environments` WrongContext: `"get environments" now lists a cluster's environments. The per-context connectivity check it used to run is: dtmgd doctor`
  - `describe environment` NotFound: `No environment %q on this cluster, or no Cluster API at this context's host. List environments with: dtmgd get environments`
- `get environments` (aliases `envs`, `env`; `clusterCommand`; `List: true`; Resource `environments`; Empty `No environments match.`):
  - `--name <s>` sends `name(<s>)`. A value containing `,`, `(` or `)` is a `UsageError`: `--name cannot contain ',', '(' or ')'; use --filter`.
  - `--state` (`ENABLED`/`DISABLED`, through `enumFlag` with upper) sends `state(<S>)`.
  - `--filter <raw>` is appended as is.
  - The parts are joined with `,` into `filter`, in the order name, state, raw.
  - `--sort` (`name`, `-name`, `creationDate`, `-creationDate`; `enumFlag`, not upper) is sent as `sort`.
  - `--consumption` sends `includeConsumptionInfo=true`.
  - `--limit`. The page cap is 1000.
  - Rows are `environmentRow{ID "ID"; Name "NAME"; State "STATE"; Trial "TRIAL" (yes|no); Created "CREATED" (creationDate as given); Tags "TAGS,wide" (joined ", ")}`.
  - With `--consumption`, rows are `environmentUsageRow`: the same plus `HostUnits "HOST-UNITS"` and `DDUs "DDU-YEAR"`, formatted `"<usage> / <limit>"`. Numbers use `strconv.FormatFloat(v, 'f', -1, 64)`. A missing limit is `unlimited`, and a missing quota is `—`.
  - Delete `EnvListItem`, `envListItem` and `envDeprecationNote`.
- `describe environment <id>` (alias `env`; `clusterCommand`; Resource `environment`):
  - Path `/cluster/v2/environments/` + `url.PathEscape(id)`, always with `includeStorageInfo=true`. `--consumption` adds `includeConsumptionInfo=true`.
  - Human output is the YAML default: no `Human`, no `Rows`.
- README scope list: add `- Service Provider API (`ServiceProviderAPI`), on a cluster context's token: `get environments`, `describe environment`, `get nodes`, `describe cluster`, `describe license`, `get license-usage`, `describe backup``. Change the `DataExport` line's commands from `get environments` to `doctor`. Task 8 then adds `doctor` to the ServiceProviderAPI line.

- [ ] **Step 1: Write the failing tests.** Each uses `withFakeContexts(t, "c", []fakeEnv{{"a", envAPI}}, []fakeEnv{{"c", clusterAPI}})` unless it says otherwise.
  - `TestGetEnvironmentsListsClusterEnvironments`: page 1 and page 2 routes on `GET /api/cluster/v2/environments` (page 2 keyed `nextPageKey=k2`). Run `get environments`, and compare stdout and stderr with golden `get_environments_table`. `r.Code == 0`, and there are 2 hits.
  - `TestGetEnvironmentsFilterAndSort`: `--name shop --state enabled --filter 'tag(prod)' --sort -name`. The first query has `filter == "name(shop),state(ENABLED),tag(prod)"`, `sort == "-name"` and `pageSize == "1000"`.
  - `TestGetEnvironmentsRejectsReservedNameChars`: `--name 'a)b'`. `r.Code == 2`, and the API received 0 requests.
  - `TestGetEnvironmentsLimit`: `--limit 1`. `pageSize == "1"`, one data row, and stderr contains `Showing 1 of 3 environments`.
  - `TestGetEnvironmentsConsumptionColumns`: `--consumption`, with page 1 swapped for a single-page fixture built from `cluster_environment.json`. The query has `includeConsumptionInfo == "true"`, and stdout contains `HOST-UNITS` and `12 / 100`.
  - `TestGetEnvironmentsConsumptionOnDPS`: a 422 with `cluster_error_422_dps.json`, plus `--consumption`. `r.Code == 1`, and stderr contains `Dynatrace Platform Subscription`.
  - `TestGetEnvironmentsOnEnvironmentContext`: current context `a`. `r.Code == 2`, and stderr contains `needs a cluster context`, `Cluster contexts: c` and `dtmgd doctor`. Neither API received a request.
  - `TestClusterCommandRejectsMultiEnv`: `get environments --env ALL_ENVIRONMENTS`. `r.Code == 2`, and stderr contains `--env`.
  - `TestClusterCommand401Hint`: 401 with `error_401.json`. `r.Code == 3`, and stderr contains `ServiceProviderAPI` and `environment API token does not work here`.
  - `TestClusterCommand403NamesScope`: 403. `r.Code == 5`, and stderr contains `This command needs the token scope(s): ServiceProviderAPI.`
  - `TestClusterCommand404Hint`: no routes, so everything returns 404. `r.Code == 4`, and stderr contains `context "c"'s host` and `port 9999`.
  - `TestDescribeEnvironment`: `GET /api/cluster/v2/environments/env-1` serves `cluster_environment.json`. The query has `includeStorageInfo == "true"`. Stdout is YAML containing the fixture's `name`. With `-o json`, the decoded output equals the decoded fixture.
  - `TestDescribeEnvironmentNotFound`: 404. `r.Code == 4`, and stderr contains `dtmgd get environments`.
  - `TestGetEnvironmentsAgentEnvelope`: `CLAUDECODE=1`. `ok:true`, `context.resource == "environments"`, `context.total == 3`.
  - `TestCatalogMarksClusterCommands`: in `commands -o json`, the entry with `path == "dtmgd get environments"` has `cluster_scoped == true`, and `dtmgd get problems` has no `cluster_scoped` key.
  - `TestSetContextScopesExcludeClusterScope`: the set-context Long text between `Required API token scopes:` and the next blank line does not contain `ServiceProviderAPI`, while `allScopes(root)` does.
  - `TestREADMEListsEveryScope` (existing) now fails until the README line is added.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./cmd/ -run 'Environment|ClusterCommand|CatalogMarksCluster|SetContextScopes|READMEListsEveryScope' 2>&1 | tail -40`
  Expected: compile errors for the undefined `ClusterScoped` field and the fixtures. Then assertion failures: `get environments` still runs the doctor rows.

- [ ] **Step 3: Implement** the Produces list. `runClusterFetch` and `clusterHint` stay under gocognit 15 each.

- [ ] **Step 4: Run the tests, regenerate goldens and lint.**
  Run: `go test ./cmd/ -update -run 'GetEnvironmentsListsClusterEnvironments' && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8; <lint> run ./...; echo lint=$?`
  Expected: all `ok`, and `lint=0`.

- [ ] **Step 5: Commit.**

```bash
git add cmd README.md
git commit -m "feat(cmd)!: get environments lists the cluster's environments; describe environment; cluster command pipeline"
```

---

### Task 5: `get nodes` and `describe cluster`

**Files:**
- Create: `cmd/get_nodes.go`, `cmd/describe_cluster.go`
- Modify: `cmd/get.go`, `cmd/describe.go` (registration; Long)
- Test: `cmd/nodes_test.go`
- Fixtures:
  - `cmd/testdata/api/cluster_nodes.json`: 3 nodes, ids 1–3, `operationState` `RUNNING`, `RUNNING`, `OFFLINE`; node 3 has `buildVersion` `1.328.5.20250901-090000` and the others `1.330.18.20251017-120000`; `clusterMemberAddress` `10.0.0.N:5701`; synthetic `osInfo`.
  - `cluster_nodes_configuration.json`: `{"clusterNodes":[…]}` for ids 1–3, with `ipAddress` `10.0.0.N`, `webUI`, `agent`, `datacenter` `dc-1`, `kubernetesRole` `""`.
  - `cluster_metadata.json`: `{"clusterName":"test-cluster","clusterUuid":"00000000-0000-0000-0000-000000000001"}`.

**Interfaces:**
- Consumes: `runClusterFetch`, `clusterCommand` (Task 4); `LowestClusterVersion` (Task 2).
- Produces:
  - `get nodes` (alias `node`; `clusterCommand`; `List: true`; Resource `nodes`; Empty `The cluster reported no nodes.`):
    - Fetch is `GET /v1.0/onpremise/cluster` into `[]interface{}`, then `GET /v1.0/onpremise/cluster/configuration`.
    - Each node map gets a `"configuration"` key: the `clusterNodes` entry with the same numeric `id`. The key is omitted when there is no such entry.
    - If the configuration call fails, the node list is still returned, with the warning `node roles unavailable: <err>`.
    - The limit truncates the array, sets `HasMore`, and sets `Total` to the full length.
    - Rows are `nodeRow{ID "ID"; Address "ADDRESS"; State "STATE"; Version "VERSION"; Datacenter "DATACENTER"; Roles "ROLES"; OS "OS,wide"}`:
      - ADDRESS is `configuration.ipAddress`, else `clusterMemberAddress`.
      - ROLES is `web UI` and/or `agents`, from `webUI` and `agent`, joined with `", "`. It is `—` when neither is set or the configuration is unknown.
      - DATACENTER is `—` when unknown.
  - `describe cluster` (`clusterCommand`; Resource `cluster`; `noArgs`):
    - Fetch is `GET /v1.0/onpremise/cluster/metadata`, which must succeed, then `GET /v1.0/onpremise/cluster`. Body is the metadata map plus `"nodes": <array>`.
    - If the node call fails, Body has no `nodes` key and carries the warning `node list unavailable: <err>`.
  - The human output of `describe cluster` is `printClusterDetail`:

```text
Name:      test-cluster
UUID:      00000000-0000-0000-0000-000000000001
Version:   1.328.5.20250901-090000 (mixed: 1.330.18.20251017-120000 on 2 nodes, 1.328.5.20250901-090000 on 1 node)
Nodes:     3 (2 RUNNING, 1 OFFLINE)
```

  - With a single version, the Version line is just that version. Without nodes, the Version and Nodes lines read `unknown`.
  - The mixed list is ordered by version descending. State counts are ordered by count descending, then by name.

- [ ] **Step 1: Write the failing tests** (`cmd/nodes_test.go`).
  - `TestGetNodesTable`: golden `get_nodes_table`, which shows ADDRESS `10.0.0.1`, DATACENTER `dc-1`, and ROLES from the fixture.
  - `TestGetNodesJSONCarriesConfiguration`: with `-o json`, the element with `id == 1` has `configuration.datacenter == "dc-1"`, and the array has length 3.
  - `TestGetNodesConfigurationUnavailable`: the configuration route answers 403. `r.Code == 0`, the rows show `—` for DATACENTER and ROLES, and stderr contains `node roles unavailable`.
  - `TestGetNodesLimit`: `--limit 2`. Two rows, and stderr contains `Showing 2 of 3 nodes`.
  - `TestDescribeClusterMixedVersions`: golden `describe_cluster_human`, matching the block above.
  - `TestDescribeClusterJSON`: with `-o json`, the output has `clusterName == "test-cluster"` and `len(nodes) == 3`.
  - `TestDescribeClusterNodesUnavailable`: the node route answers 403. `r.Code == 0`, the Version line reads `unknown`, and stderr contains `node list unavailable`.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./cmd/ -run 'GetNodes|DescribeCluster' 2>&1 | tail -30`
  Expected: FAIL, with `unknown command "nodes"` / `"cluster"` (exit 2) and missing goldens.

- [ ] **Step 3: Implement.** Keep the node join and `printClusterDetail` under gocognit 15 by extracting `joinNodeConfiguration` and `versionSummary`.

- [ ] **Step 4: Run the tests, then the suite.**
  Run: `go test ./cmd/ -update -run 'GetNodesTable|DescribeClusterMixedVersions' && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8`
  Expected: all `ok`. Read the two new goldens.

- [ ] **Step 5: Commit.**

```bash
git add cmd
git commit -m "feat(cmd): get nodes and describe cluster from the Cluster API"
```

---

### Task 6: `describe license` and `get license-usage`

**Files:**
- Create: `cmd/describe_license.go`, `cmd/get_license_usage.go`
- Modify: `cmd/get.go`, `cmd/describe.go`
- Test: `cmd/license_test.go`
- Fixtures:
  - `cmd/testdata/api/cluster_license.json`: a synthetic `ClusterLicense`. It has `licenseKey "TEST-KEY0-0000-9876"`, `licenseType "PAID"`, `licenseStatus "ACTIVE"`, and `usageOfHostUnits {quota: 100, usage: 42.5, usagePercent: 42.5, usageStatus: "USING_QUOTA"}`.
  - `cluster_license_usage.json`: two `TotalLicenseUsageForEnvironment` entries, whose `environmentUuid`s match IDs in `cluster_environments_page1.json`.
  - `cluster_error_400_classic.json`: `{"error":{"code":400,"message":"This operation is only compatible with Dynatrace classic licensing."}}`. This is a stand-in; use the real text if Task 1 observed one.

**Interfaces:**
- Consumes: `runClusterFetch`, `clusterObjectFetch`, `withHint`, `fetchSpec.NotFound` (Task 4).
- Produces:
  - `func maskLicenseKey(k string) string`: `"****" + k[len(k)-4:]` for keys longer than 4 characters, `"****"` otherwise, and `""` for an empty key. It is applied to the Body before any rendering, so every format shows the masked key.
  - `hintClassicLicensing`: `The cluster license API needs Dynatrace Managed 1.326+ and works only with Dynatrace classic licensing. A cluster on the Dynatrace Platform Subscription (DPS) has no classic license to show; see its usage in the Cluster Management Console.` Both commands' Fetch apply it with `withHint` to any `*client.ErrAPI` 4xx other than 401, 403, 404 and 429. It is also set as `NotFound` on both commands, so `runClusterFetch` gives the same hint for a 404.
  - `describe license` (`clusterCommand`; Resource `license`; `noArgs`). The human output is `printLicenseDetail`. Each line is omitted when its field is absent:

```text
License:     <licenseName> (<licenseType>, <licenseStatus>)
Key:         ****9876
Account:     <accountName>
Cluster ID:  <clusterId>
Expires:     <licenseExpirationTime>
Version:     <productVersion>
Billed at:   <lastBillingTime>
Host units:  42.5 of 100 (42.5%), USING_QUOTA
DEM units:   …
DDUs:        …
```

  - On a unit line, an `overageUsage.overageUsage` value above 0 adds `, overage <overageUsage>`.
  - `get license-usage` (`clusterCommand`; `List: true`; Resource `license-usage`; Empty `No license usage reported.`):
    - Fetch is `GET /cluster/v2/clusterLicense/environment/total`.
    - The limit truncates `totalLicenseUsageDataPerEnvironment` client-side.
    - Rows are `licenseUsageRow{Environment "ENVIRONMENT"; Name "NAME"; HostUnits "HOST-UNITS"; DEMUnits "DEM-UNITS"; DDUs "DDUS"}`, with numbers formatted as `FormatFloat(v,'f',-1,64)`.
    - NAME comes from one `GET /cluster/v2/environments?pageSize=1000` call, made only when `a.humanOutput()`. If that call fails, NAME is `—` and the warning reads `environment names unavailable: <err>`.

- [ ] **Step 1: Write the failing tests** (`cmd/license_test.go`).
  - `TestDescribeLicenseMasksKey`: human output, `-o json`, `-o yaml` and `CLAUDECODE=1`. None of their stdouts contains `TEST-KEY0-0000-9876`, and each contains `****9876`.
  - `TestDescribeLicenseHuman`: golden `describe_license_human`.
  - `TestDescribeLicenseOnDPS`, a table test:
    - a 400 with `cluster_error_400_classic.json` gives `r.Code == 1`;
    - a 404 gives `r.Code == 4`;
    - both have stderr containing `classic licensing`.
  - `TestGetLicenseUsageTableWithNames`: golden `get_license_usage_table`, where NAME comes from the environments fixture.
  - `TestGetLicenseUsageNameLookupFails`: the environments route answers 403. `r.Code == 0`, NAME is `—`, and stderr contains `environment names unavailable`.
  - `TestGetLicenseUsageJSONSkipsLookup`: with `-o json`, the environments route received 0 requests.
  - `TestMaskLicenseKey`:
    - `""` gives `""`.
    - `"abc"` gives `"****"`.
    - `"abcd"` gives `"****"`.
    - `"abcde"` gives `"****bcde"`.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./cmd/ -run 'License' 2>&1 | tail -30`
  Expected: FAIL (`maskLicenseKey` undefined; unknown commands).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests, then the suite.**
  Run: `go test ./cmd/ -update -run 'DescribeLicenseHuman|GetLicenseUsageTableWithNames' && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8`
  Expected: all `ok`. Confirm that the goldens contain no unmasked key: `grep -r 'TEST-KEY0' cmd/testdata/golden` prints nothing.

- [ ] **Step 5: Commit.**

```bash
git add cmd
git commit -m "feat(cmd): describe license and get license-usage, with the license key masked"
```

---

### Task 7: `describe backup`

**Files:**
- Create: `cmd/describe_backup.go`
- Modify: `cmd/describe.go`
- Test: `cmd/backup_test.go`
- Fixture: `cmd/testdata/api/cluster_backup_config.json`: a synthetic `backupConfigDto` with `enabled: true`, `pauseBackups: false`, `storagePath "/backup/test"`, `datacenter "dc-1"`, `cassandraScheduledTime: 2`, `bandwidthLimitMbits: 200`, `includeTsMetricData: true`, `includeRumData: false`, `includeLm20Data: true`, `maxEsSnapshotsToClean: 5`.

**Interfaces:**
- Consumes: `runClusterFetch`, `clusterObjectFetch`, `clusterCommand` (Task 4).
- Produces: `describe backup` (alias `backups`; `clusterCommand`; Resource `backup`; `noArgs`). Its Long says: `The Cluster API reports the backup configuration only. It has no last-backup time and no backup health; check those in the Cluster Management Console.` The human output is `printBackupDetail`, followed by that same sentence on stderr (`output.PrintInfo`):

```text
Enabled:                yes
Paused:                 no
Storage path:           /backup/test
Datacenter:             dc-1
Cassandra backup start: 02:00 (hour of day, cluster time)
Bandwidth limit:        200 Mbit/s
Includes:               time-series metric data: yes, user sessions: no, Log Monitoring v2 data: yes
Elasticsearch snapshots to clean: 5
```

- [ ] **Step 1: Write the failing tests.**
  - `TestDescribeBackupHuman`: golden `describe_backup_human`, covering stdout and the stderr note.
  - `TestDescribeBackupJSON`: with `-o json`, the decoded output equals the decoded fixture, and stderr does not contain the note.
  - `TestDescribeBackupDisabled`: a body with `enabled: false` shows `Enabled:                no`.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./cmd/ -run 'DescribeBackup' 2>&1 | tail -20`
  Expected: FAIL (unknown command `backup`).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests, then the suite.**
  Run: `go test ./cmd/ -update -run DescribeBackupHuman && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8`
  Expected: all `ok`.

- [ ] **Step 5: Commit.**

```bash
git add cmd
git commit -m "feat(cmd): describe backup shows the cluster's backup configuration"
```

---

### Task 8: `doctor` understands cluster contexts

**Files:**
- Modify: `cmd/doctor.go` (`DoctorRow.Scope`, `checkContext`, `failDetail` through `hintFor`, scopes, Long)
- Modify: `README.md` (add `doctor` to the ServiceProviderAPI scope line)
- Test: `cmd/doctor_test.go`

**Interfaces:**
- Consumes: `IsCluster`, `ClusterVersion`, `ClusterScoped` (Task 2); `hintFor`, `withHint`, `hintClusterAuth`, `scopeServiceProviderAPI` (Task 4).
- Produces:
  - `DoctorRow` gains `Scope string `json:"scope" yaml:"scope" table:"SCOPE"``, placed after Context, with the value `cluster` or `environment`.
  - `checkContext` on a cluster context:
    - Version comes from `ClusterVersion`, which reads the nodes.
    - There is no metrics fallback.
    - A 401 or 403 fails the row with detail from `withHint(err, hintClusterAuth)`.
  - `failDetail` uses `hintFor`.
  - doctor declares `scopeDataExport`, `scopeMetricsRead` and `scopeServiceProviderAPI`.
  - The Long help adds: `A cluster context (no env-id) is checked against the Cluster API: its token must have the ServiceProviderAPI scope.`

- [ ] **Step 1: Write the failing tests.**
  - `TestDoctorClusterContextOK`: cluster context `c` whose `/api/v1.0/onpremise/cluster` serves `cluster_nodes.json`. The row has `scope == "cluster"`, `status == "ok"`, `version == "1.328.5.20250901-090000"` and `envId == ""`.
  - `TestDoctorClusterContextWrongToken`: `/cluster` answers 401. The row has `status == "fail"` and a detail containing `ServiceProviderAPI`. `r.Code == 1`, and the metrics path received 0 requests.
  - `TestDoctorMixedContextsTable`: golden `doctor_mixed_table`, with one environment row and one cluster row.
  - Update `doctorRowJSON` with `Scope`, and update `TestDoctorDeclaresScopes`.

- [ ] **Step 2: Run them to verify they fail.**
  Run: `go test ./cmd/ -run 'Doctor' 2>&1 | tail -30`
  Expected: FAIL. `scope` is empty, and the cluster row currently tries the metrics fallback.

- [ ] **Step 3: Implement.** Split `checkContext` into `checkEnvironmentVersion` and `checkClusterVersion` to stay under gocognit 15.

- [ ] **Step 4: Run the tests, regenerate goldens, and lint.**
  Run: `go test ./cmd/ -update -run 'DoctorMixedContextsTable|DoctorTable' && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8; <lint> run ./...; echo lint=$?`
  Expected: all `ok`, and `lint=0`. The doctor golden diffs only add SCOPE.

- [ ] **Step 5: Commit.**

```bash
git add cmd README.md
git commit -m "feat(doctor): check cluster contexts against the Cluster API"
```

---

### Task 9: Documentation, knowledge base and release notes

**Files:**
- Modify: `README.md`, `skills/dtmgd/SKILL.md`, `cmd/app.go` (root Long), `pkg/client/diagnostic.go` (the "no current context" hint), `.claude/CLAUDE.md`
- Create (SDD workspace, not committed): `<workspace>/release-notes-v0.3.0.md`
- Test: `cmd/root_test.go`

**Interfaces:**
- Consumes: every command, flag and message from Tasks 2–8.
- Produces:
  - **README.**
    - A "Cluster contexts" section covering how to create one (`dtmgd ctx set prod-cluster --host https://cluster.example.com --token-ref prod-cluster-token`, then `dtmgd config set-credentials prod-cluster-token --token-stdin`).
    - The host rule (the CMC address, not a :9999 node) and the `ServiceProviderAPI` scope.
    - The seven cluster commands, with one example each.
    - The limits: read-only; backup configuration only; license only on classic licensing; no `--env` fan-out; `ALL_ENVIRONMENTS` skips cluster contexts.
    - The command list.
    - In the MCP parity table, a row stating that the cluster commands have no MCP counterpart (dtmgd-only), and a `get_environments_info` row that still maps to `doctor`.
  - **SKILL.md:** the same facts in its own voice, and the `get environments` semantics corrected.
  - **Root Long:** `Cluster API: {host}/api/cluster/v2/ and {host}/api/v1.0/onpremise/ (cluster contexts: no env-id)`.
  - **The "no current context" hint:** keeps the environment example and adds `or, for the Cluster API: dtmgd ctx set <name> --host <cluster-url> --token-ref <ref>`.
  - **CLAUDE.md knowledge-base entries**, in the file's format with Ref `Phase 4 Cluster API`:
    1. A cluster context's host is the CMC address, not the node host on :9999, where the Cluster API answers 404.
    2. `/api/v1.0/onpremise/cluster` returns an array of nodes with `buildVersion`. The old `{"version"}` fakes were wrong, and the version is the lowest node's.
    3. Licensing and DPS behaviour, as observed in Task 1.
    4. `ALL_ENVIRONMENTS` excludes cluster contexts, and the context kind is decided by the raw `env-id`.
  - **Release notes:**
    - Breaking: `get environments`, plus the replacement command for the old behaviour (`dtmgd doctor`).
    - Behaviour change: `ctx set` without `--env-id` now creates a cluster context instead of failing.
    - Behaviour change: `ALL_ENVIRONMENTS` skips cluster contexts.
    - New: the seven commands.
    - Additive: doctor and `ctx` gain a SCOPE column, and doctor json/yaml gains `scope`.
    - Fixed: the cluster version probe now decodes the node list.

- [ ] **Step 1: Write the failing test.** `TestRootHelpNamesClusterAPI`: `dtmgd --help` stdout contains `/api/cluster/v2/` and `cluster contexts`.

- [ ] **Step 2: Run it to verify it fails.**
  Run: `go test ./cmd/ -run RootHelpNamesClusterAPI`
  Expected: FAIL.

- [ ] **Step 3: Write the docs and the root Long.**

- [ ] **Step 4: Verify.**
  Run: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... 2>&1 | tail -8 && grep -nE 'connectivity|Configured Managed environments' README.md skills/dtmgd/SKILL.md cmd/get.go`
  Expected: all `ok`. The grep shows no line that describes `get environments` as a connectivity check or as "configured environments".

- [ ] **Step 5: Commit.**

```bash
git add README.md skills/dtmgd/SKILL.md cmd pkg/client/diagnostic.go .claude/CLAUDE.md
git commit -m "docs: cluster contexts and the Cluster API commands"
```

---

### Task 10: Live read-only verification (owner-gated)

**Files:**
- Modify: `<workspace>/progress.md` (results only)

**Interfaces:**
- Consumes: a binary built from the branch: `go build -o <scratchpad>/dtmgd-p4 .`

- [ ] **Step 1: Confirm the owner's go-ahead** for these read-only calls against the default config. Without it, record `Task 10: Ruling: live check not run` and state it in the PR.

- [ ] **Step 2: Run each command and compare.** The jq filters keep values out of the transcript. `B=<scratchpad>/dtmgd-p4`.

| Command | Expected |
|---|---|
| `$B get environments --context prod-cluster -o json \| jq '{n: (.environments\|length), total: .totalCount}'` | exit 0, n ≥ 1 |
| `$B get environments --context stage-cluster --limit 3 >/dev/null; echo $?` | 0 |
| `$B describe environment "$($B get environments --context prod-cluster -o json \| jq -r '.environments[0].id')" --context prod-cluster -o json \| jq 'keys'` | includes `name`, `state` |
| `$B get nodes --context prod-cluster -o json \| jq '[length, ([.[].operationState]\|unique), ([.[]\|has("configuration")]\|all)]'` | length ≥ 1, states listed, `true` |
| `$B describe cluster --context stage-cluster -o json \| jq 'keys'` | `clusterName`, `clusterUuid`, `nodes` |
| `$B describe license --context prod-cluster -o json \| jq '{t: .licenseType, s: .licenseStatus, masked: (.licenseKey\|startswith("****"))}'` | `masked: true`, or the classic-licensing hint with an exit code ≠ 0 on DPS |
| `$B get license-usage --context prod-cluster -o json \| jq '.totalLicenseUsageDataPerEnvironment\|length'` | ≥ 0, or the same DPS hint |
| `$B describe backup --context stage-cluster -o json \| jq '{enabled, pauseBackups}'` | booleans |
| `$B doctor -o json \| jq '[.[] \| {context, scope, status}]'` | 4 rows; prod-cluster and stage-cluster have `scope: cluster`, `status: ok` |
| `$B get problems --env ALL_ENVIRONMENTS -o json \| jq 'keys'` | `["prod","stage"]` |
| `$B get problems --context prod-cluster; echo $?` | stderr `is a cluster context`, exit 2 |
| `$B get nodes --context prod; echo $?` | stderr `needs a cluster context`, exit 2 |

- [ ] **Step 3: Ledger the results.** Any mismatch is a finding: fix it test-first under the task that owns the code, then re-run its row.

---

## After the tasks

- Final whole-branch review by a fresh reviewer on the most capable model, given this plan's Review Focus verbatim. Re-grade the findings by effect. Fix Critical and Important findings test-first. Then re-grade the deferred minors, and fix the cheap or mitigable ones before the PR (owner feedback, 2026-10-29).
- Push, open the PR with `gh` as the owner, wait for CI and check SonarCloud. Each of these needs the owner's go-ahead.
- Merge, tag `v0.3.0` on the merge commit, and follow the release steps in the catch-up program memory. Each of these needs the owner's go-ahead.
- Follow-ups outside this repo, for the owner: the `cli-tool-architecture` dtmgd section should document `get environments` semantics, the `/api/cluster/v2` surface and Open Question #4 (resolved as "absence of env-id").
