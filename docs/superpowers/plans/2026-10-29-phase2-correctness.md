# Phase 2: Correctness Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task by task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every command's output, limits, exit codes and agent envelope are correct and consistent across formats and environments, locked down by command-level tests. Ships as v0.1.0.

**Architecture:**
- Replace package-level command and flag state with an `App` that is built per invocation. Commands are constructors over `App`.
- Add one shared runner, `App.runFetch`. It fetches from one or more environments through a single client factory, applies the limit, and renders table, json, yaml or agent output with explicit truncation and per-environment errors.
- Errors map to dtctl's exit codes 0–5 and to dtctl's `sdk/agentmode` envelope.
- A command-level test harness runs the real CLI against `httptest` with golden files.

**Tech Stack:** Go 1.26 (toolchain go1.26.10), Cobra, resty v2, tablewriter, `gopkg.in/yaml.v3`, `github.com/dynatrace-oss/dtctl/sdk` v0.45.0 (`agentmode` only).

**Spec:** `docs/superpowers/specs/2026-10-29-dtmgd-catch-up-design.md` (Phase 2 section, plus Goals, Anti-goals and Constraints).

## Global Constraints

- **Branches and PRs.** Branch `feat/phase2-correctness` from `main`. The work ships as two PRs:
  - **PR 1:** Tasks 1–2, the refactor with no behaviour change.
  - **PR 2:** Tasks 3–14.

  Each PR must leave `main` releasable. Get the owner's go-ahead at the time of every push, PR, merge request and tag. PRs are opened with `gh` (as the owner), never with a bot token. `main` has a ruleset: required checks are `test (ubuntu-latest)`, `test (macos-latest)`, `test (windows-latest)`, `lint`, `golangci-lint` and `vulncheck`.
- **Commit messages** end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. The PR body ends with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- **Tests.**
  - Local test signal: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...`. The `pkg/skills` home-directory failures are known; leave them.
  - HTTP error tests use 400, 401, 403 or 404, never 5xx (resty retries 5xx).
  - Keyring tests rely on `keyring.MockInit()` in `TestMain` (`cmd/config_test.go`, `pkg/config/keyring_test.go`).
- **Security posture stays as it is:**
  - API error bodies are trimmed to `error.message`, at most 200 characters (`pkg/client/errors.go`).
  - `${VAR}` is resolved only at point of use.
  - Tokens are never printed. When inspecting `~/.config/dtmgd/config`, filter out `^\s*token:` lines.
- **Output shape (spec ruling):**
  - `-o json`, `-o yaml` and the envelope's `result` carry the **API's own response shape**, for example `{"problems":[…],"totalCount":N}` with camelCase keys. `--limit` truncates the list inside it.
  - An empty result prints that object with an empty list, never nothing.
  - Multi-environment output is a map keyed by context name, with `{"error": "<message>"}` for each failed environment.
- **Streams:** stdout carries only data. Human notes (empty result, "Showing N of M", warnings, errors, hints) go to stderr.
- **Exit codes:** 0 success, 1 general, 2 usage, 3 auth (401), 4 not found (404), 5 permission (403). In a multi-environment call, any failed environment means a non-zero exit, and partial results are still printed.
- **Agent mode:**
  - Default limit for lists is 50 when `--limit` isn't given. Human table mode keeps fetch-all.
  - The envelope comes from `agentmode` (`ok`, `result`, `error{code,message,status_code,suggestions}`, `context{verb,resource,total,has_more,warnings,suggestions}`).
- **Token scopes**, declared once per command (Task 6):

  | Scope | Commands |
  |---|---|
  | `problems.read` | problems |
  | `entities.read` | entities, entity-types, entity-relations, and name lookups in `query metrics` and `query log-counts` |
  | `events.read` | events |
  | `metrics.read` | metrics, `query metrics` |
  | `logs.read` | `query logs`, `query log-counts` |
  | `slo.read` | slos |
  | `securityProblems.read` | security-problems |

- **Golden files** live in `cmd/testdata/golden/*.golden` and are regenerated with `go test ./cmd/ -run <Test> -update`. Every golden change in a later task must be deliberate and named in that task's commit message.

## Review Focus

1. **`-o yaml` output is valid YAML for every list and query command**, with no human header lines on stdout. Today `query log-counts -o yaml` breaks this. Owner: Task 10, test `TestYAMLOutputParses`.
2. **Argument-parse failures in agent mode** (unknown flag or resource) give one JSON envelope on stdout with code `unknown_command` or `validation_error` and exit 2. `--no-agent` gives a human error. Owner: Task 6c, test `TestParseFailureEnvelope`.
3. **One unreachable environment in a multi-environment call** (connection refused): the other environments still print, that environment appears as `{"error": …}`, and the exit is non-zero within a few seconds, with no retry storm. Owner: Task 7, test `TestMultiEnvOneEnvUnreachable`.
4. **`--limit` smaller than the full result** stops paging once the limit is reached, rather than fetching every page and truncating afterwards. Owner: Task 5, test `TestGetV2PagedStopsAtLimit`.
5. **Empty results in json and agent mode** give the API object with an empty list (and an `ok:true` envelope) and exit 0. Owner: Task 7, test `TestEmptyResultJSONAndAgent`.

## File structure (new and reshaped)

| File | Responsibility |
|---|---|
| `cmd/app.go` | `App`, `GlobalOptions`, `Run`, `newRootCmd`, `App` helpers (config, client, printer, agent mode, plain mode) |
| `cmd/runner.go` | `fetchSpec`, `fetched`, `App.runFetch` |
| `cmd/errors.go` | `UsageError`, `exitCode`, `App.reportError`, cobra error classification, "did you mean" |
| `cmd/scopes.go` | scope annotation helpers, `requiredScopes` |
| `cmd/commands.go`, `cmd/doctor.go` | new commands (Task 13) |
| `cmd/harness_test.go` | `runCLI`, `fakeAPI`, `withFakeEnvs`, `assertGolden` |
| `cmd/testdata/api/*.json`, `cmd/testdata/golden/*.golden` | fixtures and goldens |
| `pkg/client/paging.go` (was `pagination.go`) | `Paging`, `GetV2Paged` with limit and truncation signal |
| `pkg/client/exitcode.go` | exit-code constants, `ExitCodeForStatus`, `PartialFailureError` |
| `pkg/output/format.go` | `Format`, `ParseFormat` |
| `pkg/output/agent.go` | thin wrapper over `agentmode` |

Existing per-command files (`cmd/get_*.go`, `cmd/describe_*.go`, `cmd/query_*.go`, `cmd/config.go`, `cmd/ctx.go`, `cmd/skills.go`) keep their names and become constructors.

---

## PR 1: test harness and constructors (no behaviour change)

### Task 1: `Run` seam and the command test harness

**Files:**
- Modify: `cmd/root.go` (replace `Execute` with `Run`), `main.go`
- Create: `cmd/harness_test.go`, `cmd/characterize_test.go`, `cmd/testdata/api/*.json`, `cmd/testdata/golden/*.golden`

**Interfaces:**
- Produces:
  - `func Run(args []string, stdout, stderr io.Writer) int` in `cmd`. It runs dtmgd with `args` (program name excluded) and returns the exit code. In this task, behaviour is identical to `Execute`: errors print as today and the code is 1.
  - `main.go` becomes `os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))`.
- Produces (test helpers in `cmd/harness_test.go`):
  - `type route struct{ Status int; File string; Body string }`. `File` is relative to `testdata/api/`. `Status` 0 means 200.
  - `type fakeAPI struct{ URL string; Hits map[string]int }`.
  - `func newFakeAPI(t *testing.T, routes map[string]route) *fakeAPI`. The key is `"GET /e/env1/api/v2/problems"`; the query string is ignored for matching, and every hit is counted. An unknown path returns 404 with `{"error":{"code":404,"message":"not found"}}`.
  - `type fakeEnv struct{ Name string; API *fakeAPI }`.
  - `func withFakeEnvs(t *testing.T, envs ...fakeEnv) string`. It writes a temp config with one context per env (`host: <API.URL>`, `env-id: env1`, a plaintext token `dt0c01.TEST`, `current-context` = first env) and returns its path.
  - `type cliResult struct{ Stdout, Stderr string; Code int }`.
  - `func runCLI(t *testing.T, cfgPath string, args ...string) cliResult`:
    - prepends `--config cfgPath`
    - clears every agent-detection env var with `t.Setenv(v, "")` (until Task 6b that list is `output.AgentEnvVars`)
    - swaps `os.Stdout`/`os.Stderr` for pipes and passes those pipes to `Run`
    - resets all flags on the command tree to their defaults before running: `Set(DefValue)` and `Changed=false`, with `pflag.SliceValue.Replace(nil)` for slices
    - replaces every occurrence of the fake server URL in the output with `http://FAKE`
  - `func assertGolden(t *testing.T, name, got string)` with `var update = flag.Bool("update", false, "rewrite golden files")`.

- [ ] **Step 1: Write the characterization tests (`cmd/characterize_test.go`).**

Each test runs the CLI against fixtures and pins **today's** output in goldens, including today's bugs. The fixtures are trimmed real API shapes:
- `problems_page1.json`: 10 problems, `nextPageKey`, `totalCount` 15
- `problems_page2.json`: 5 problems
- `problems_empty.json`
- `entity.json`
- `logs.json`

| Test | Args | Pins |
|---|---|---|
| `TestCharGetProblemsTable` | `get problems` | stdout table, stderr, code 0 |
| `TestCharGetProblemsJSON` | `get problems -o json` | stdout |
| `TestCharGetProblemsAgent` | `get problems -A` | envelope |
| `TestCharGetProblemsEmptyJSON` | `get problems -o json` with the empty fixture | stdout is `""` (today's bug), code 0 |
| `TestCharDescribeEntityTable` | `describe entity HOST-1` | stdout contains `map[` (today's bug) |
| `TestCharMultiEnvJSON` | `get problems --env "a;b" -o json` | map keyed `a`/`b` |
| `TestCharMultiEnvAllFail` | `--env "a;b"` with both returning 403 | code 0 (today's bug) |
| `TestCharQueryLogsTable` | `query logs --query error --from now-1h` | stdout and stderr |
| `TestCharUnknownResource` | `get prblems` | code 1, help on stdout |

- [ ] **Step 2: Run them before `Run` exists.**

Run: `go test ./cmd/ -run TestChar -v`
Expected: build failure, `undefined: Run`.

- [ ] **Step 3: Implement `Run`, update `main.go`, then generate the goldens.**

Run: `go test ./cmd/ -run TestChar -update && go test ./cmd/ -run TestChar -v`
Expected: all PASS. Read each golden and confirm it shows the behaviour described in the table, including the bugs.

- [ ] **Step 4: Full package run.**

Run: `go test ./cmd/... ./pkg/output/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `test(cmd): command-level harness and characterization goldens`.

---

### Task 2: `App` and command constructors (remove package-level command and flag state)

**Files:**
- Create: `cmd/app.go`
- Modify: every `cmd/*.go` that declares `var xCmd = &cobra.Command{…}` and/or flag variables, plus `cmd/watch.go`
- Modify tests that touch package variables: `cmd/root_test.go`, `cmd/config_test.go`, `cmd/ctx_test.go`, `cmd/configsource_test.go`, `cmd/configview_test.go`, `cmd/skills_test.go`

**Interfaces:**
- Produces:

```go
type GlobalOptions struct {
	ConfigPath, Context, Output, Env, Columns string
	Verbose, MaxPages, Concurrency            int
	Agent, NoAgent                            bool
	Watch                                     bool          // moves to get problems/events in Task 7
	WatchInterval                             time.Duration // same
}
type App struct {
	Out, Err io.Writer
	Opts     GlobalOptions
}
func newRootCmd(a *App) *cobra.Command
```

- Method replacements, one for one. Behaviour must not change:

| Today | After |
|---|---|
| `agentMode()` | `a.agentMode()` |
| `agentNotice()` | `a.agentNotice()` |
| `LoadConfig()` | `a.loadConfig()` |
| `loadConfigRaw()` / `loadConfigForUpdate()` / `loadFromSource()` | `a.loadConfigRaw()` / `a.loadConfigForUpdate()` / `a.loadFromSource()` |
| `saveConfig(cfg)` | `a.saveConfig(cfg)` |
| `reportConfigSource(src)` | `a.reportConfigSource(src)` |
| `NewClientFromConfig(cfg)` | `a.newClient(cfg)` |
| `NewClientWithHostEnv(...)` | `a.newClientWithHostEnv(...)` |
| `NewPrinter()` / `NewPrinterForResource(r)` | `a.printer(r)`, writing to `a.Out` |
| `isMultiEnv()` | `a.isMultiEnv()` |
| `effectiveMaxPages(b)` | `a.maxPagesFor(b)` |
| `multiExec(cfg, fn)` | `a.multiExec(cfg, fn)` |
| `parseColumns()` | `a.columns()` |
| `watchOrRun(fn)` | `a.watchOrRun(fn)` |
| `setContext(...)` / `setCredentials(...)` / `useContext(...)` | `a.setContext(...)` / `a.setCredentials(...)` / `a.useContext(...)` |

- Every command becomes `func newXCmd(a *App) *cobra.Command`, with its flags bound to a local `opts` struct captured by the closure. `RunE` never assigns to an options field: defaults such as `from="now-24h"` go into local variables.
- Every `fmt.Print*`/`fmt.Println` in `cmd/` becomes `fmt.Fprint*(a.Out, …)`; there are 68 call sites, listed in the inventory. `cobra` output uses `cmd.SetOut(a.Out)`/`SetErr(a.Err)`.
- `Run` builds `a := &App{Out: stdout, Err: stderr}` and `newRootCmd(a)` on every call.

- [ ] **Step 1: Convert, command group by command group.**

Groups, in order: root and globals, then get, describe, query, config/ctx, skills/version/completion. After each group, run `go build ./... && go test ./cmd/ -run TestChar`.
Expected: goldens unchanged.

- [ ] **Step 2: Remove the flag-reset helper from `runCLI`.**

It isn't needed now that `Run` builds a fresh tree.

- [ ] **Step 3: Verify that no package-level command or flag state remains.**

Run: `grep -nE '^var' cmd/*.go | grep -v _test.go; grep -c 'fmt\.Print' cmd/*.go | grep -v ':0'`
Expected:
- The first command lists only non-state values such as `displayIDRegexp = regexp.MustCompile(…)`: no `var (` blocks, no `&cobra.Command`, no flag variables.
- The second command produces no output.

- [ ] **Step 4: Full run.**

Run: `go test -race ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... && go vet ./...`
Expected: all `ok`, goldens unchanged.

- [ ] **Step 5: Commit, and open PR 1 (owner go-ahead).**

Message: `refactor(cmd): build the command tree per invocation; no package-level flag state`. PR title: `Phase 2 (1/2): command test harness and per-invocation command tree`. After CI is green and the owner merges, rebase `feat/phase2-correctness` on `main`.

---

## PR 2: behaviour

### Task 3: Output layer: format validation, readable fallback, plain mode

**Files:**
- Create: `pkg/output/format.go`, `pkg/output/format_test.go`
- Modify: `pkg/output/output.go`, `pkg/output/table.go`, `cmd/app.go`, `cmd/watch.go`
- Tests: `pkg/output/table_test.go`, `pkg/output/output_test.go`, `cmd/output_test.go` (create)

**Interfaces:**
- Produces in `pkg/output`:
  - `type Format string` with consts `FormatTable="table"`, `FormatWide="wide"`, `FormatJSON="json"`, `FormatYAML="yaml"`.
  - `func ParseFormat(s string) (Format, error)`. `""` → table; `yml` → yaml; anything else → error `unknown output format "xml" (want one of: table, wide, json, yaml)`.
  - `func SetStderr(w io.Writer)` and `func SetPlain(plain bool)`. These are process-wide presentation settings; `Run` sets them once.
  - `PrintWarning` / `PrintHumanError` / `PrintSuccess`: glyphs `⚠ ` / `✗ ` / `✓ ` when not plain; `Warning: ` / `Error: ` / nothing when plain.
  - `func PrintHint(format string, args ...interface{})`: `💡 ` or `Hint: `.
  - `func IsTerminal(w io.Writer) bool`.
  - `TablePrinter` renders any non-struct value (maps, slices of maps, scalars), and structs without `table` tags, as YAML. It never uses `fmt.Println(v)`.
  - Delete `DescribeKV` (no callers).
- Produces in `cmd`:
  - `--plain` (bool, persistent), stored in a new `GlobalOptions.Plain bool` field.
  - `a.plain() bool` = `Opts.Plain || NO_COLOR != "" || (!IsTerminal(a.Out) && FORCE_COLOR == "")`.
  - `a.format() output.Format`, validated in `PersistentPreRunE` (which also prints the agent notice). An invalid `-o` is a usage error (exit 2 once Task 6 lands; exit 1 until then).
  - `a.watchOrRun` clears the screen only when `format` is table or wide, not agent, and not plain.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestParseFormat` (table-driven) | `""`→table, `"yml"`→yaml, `"wide"`→wide, `"xml"`→error containing `want one of: table, wide, json, yaml` |
| `TestTableFallbackRendersYAML` | `PrintList(map[string]interface{}{"entityId":"HOST-1","properties":map[string]interface{}{"a":1}})` output contains `entityId: HOST-1` and not `map[` |
| `TestPlainPrefixes` | with `SetPlain(true)` and `SetStderr(&buf)`, `PrintWarning("x")` gives `"Warning: x\n"` and `PrintHumanError("y")` gives `"Error: y\n"` |
| `TestInvalidOutputFormatIsRejected` (cmd) | `runCLI(..., "get problems", "-o", "xml")` gives a non-zero code, stderr contains `unknown output format "xml"`, and zero API hits |
| `TestYmlAliasPrintsAPIShapedYAML` (cmd) | `-o yml` output equals `-o yaml` output |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./pkg/output/ ./cmd/ -run 'ParseFormat|TableFallback|PlainPrefixes|InvalidOutputFormat|YmlAlias' -v`
Expected: build failure (undefined `ParseFormat`, `SetPlain`), or assertion failures.

- [ ] **Step 3: Implement.**

Then regenerate goldens and check the diff: `TestCharDescribeEntityTable` now shows YAML, and no glyphs appear (harness stdout is a pipe, so plain mode is on). Rename the `TestChar…` tests whose bug is now fixed to names describing the new behaviour.

- [ ] **Step 4: Run the tests.**

Run: `go test ./pkg/output/... ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `fix(output): validate -o, render non-struct values as YAML, add --plain` (list the goldens changed).

---

### Task 4: One client factory; network behaviour

**Files:**
- Modify: `pkg/client/client.go`, `pkg/client/multi.go`, `cmd/app.go`, `cmd/get_environments.go`, `go.mod`/`go.sum` (drop logrus)
- Tests: `pkg/client/client_test.go`, `pkg/client/multi_test.go`

**Interfaces:**
- Produces in `pkg/client`:
  - `type Options struct{ Verbosity int }`.
  - `func NewForContext(cfg *config.Config, name string, o Options) (*Client, error)`. It resolves the named context (`UnresolvedVarsError.InContext(name)`), gets the token, calls `New`, `SetProxy` and `SetVerbosity`. `NewFromConfig(cfg)` becomes `NewForContext(cfg, cfg.CurrentContext, Options{})`.
  - `MultiRequest(cfg *config.Config, envSpec string, maxConcurrency int, o Options, apiCall func(c *Client) (interface{}, error)) ([]EnvResult, error)` builds each client with `NewForContext`.
  - Retry policy (`isRetryable`): retry on 429 and ≥ 500, and on transport errors only when they are timeouts (`net.Error.Timeout()`), `syscall.ECONNRESET` or `io.ErrUnexpectedEOF`. Never retry `*net.DNSError`, connection refused, TLS/x509, or proxy errors. Use `SetRetryCount(2)`, `SetRetryMaxWaitTime(5*time.Second)`, and a 30-second timeout for both clients.
  - resty's own logger is silent unless verbosity > 0 (`SetLogger` with a no-op `resty.Logger`).
  - `getV2Raw` wraps transport errors with `c.requestError`, so the proxy annotation is kept.
  - Remove the `logrus` dependency, `Client.logger` and `Client.token`.
- `cmd`: `a.newClient(cfg)` = `client.NewForContext(cfg, cfg.CurrentContext, client.Options{Verbosity: a.Opts.Verbose})`. `get environments` uses `NewForContext` for each context, so it gets the proxy.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestDeadHostFailsFast` | `l, _ := net.Listen("tcp","127.0.0.1:0"); addr := l.Addr(); l.Close()`, then `GetV2` against `http://`+addr returns an error in under 2s, after a single dial |
| `TestNoRetryOnDNSFailure` | host `http://nonexistent.invalid`: error in under 5s, and the error is a `*net.DNSError` via `errors.As` |
| `TestMultiRequestAppliesVerbosity` | with `Options{Verbosity:1}` and two contexts on a fake server, stderr (captured) contains two `==> GET` lines |
| `TestNewForContextAppliesProxy` | a context with `https-proxy: http://127.0.0.1:9` makes the request error mention `via proxy http://127.0.0.1:9` |
| `TestGetV2PagedKeepsProxyAnnotation` | the same proxy setting makes `GetV2Paged`'s error mention `via proxy` |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./pkg/client/ -run 'DeadHost|NoRetryOnDNS|AppliesVerbosity|AppliesProxy|KeepsProxyAnnotation' -v`
Expected:
- `TestDeadHostFailsFast` FAIL (takes more than 2s because of retries).
- `TestMultiRequestAppliesVerbosity` FAIL (no output).
- Signature errors for `NewForContext`/`Options`.

- [ ] **Step 3: Implement, then run `go mod tidy`.**

Expected: `logrus` gone from `go.mod`.

- [ ] **Step 4: Run the tests.**

Run: `go test ./pkg/client/... ./cmd/...`
Expected: `ok`. `TestSetProxyReachesClusterClient` is no longer slow.

- [ ] **Step 5: Commit**

Message: `fix(client): one client factory for all paths; fail fast on dead hosts; drop logrus`.

---

### Task 5: Paging that honours the limit and keeps the truncation signal

**Files:**
- Rename: `pkg/client/pagination.go` → `pkg/client/paging.go`
- Tests: `pkg/client/client_test.go` (paging tests move to `pkg/client/paging_test.go`)

**Interfaces:**
- Produces:
  - `type Paging struct{ MaxPages, Limit int; ListKey string }`.
  - `func (c *Client) GetV2Paged(path string, params map[string]string, p Paging) (map[string]interface{}, error)`. It stops when the next page key is empty, when `MaxPages > 0 && pages == MaxPages`, or when `Limit > 0 && len(result[ListKey]) >= Limit`.
  - It truncates `result[ListKey]` to `Limit`.
  - It **keeps** `nextPageKey` in the result whenever it stopped with pages remaining, and deletes it only when the API returned none.
  - `totalCount` stays as page 1 reported it.
- Callers: every existing `GetV2Paged(path, params, n)` call becomes `GetV2Paged(path, params, client.Paging{MaxPages: n})` in this task. Task 7 rewires them.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestGetV2PagedStopsAtLimit` (Review Focus 4) | 3 pages of 10 with `Limit: 12, ListKey: "problems"`: 2 requests, 12 items, `nextPageKey` present |
| `TestGetV2PagedExhaustedDropsKey` | 2 pages and no limit: 2 requests, 20 items, no `nextPageKey` |
| `TestGetV2PagedMaxPagesKeepsKey` | `MaxPages: 1`: 1 request, `nextPageKey` present |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./pkg/client/ -run GetV2Paged -v`
Expected: build failure (`Paging` undefined).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./pkg/client/... ./cmd/...`
Expected: `ok`. Goldens may change only where JSON now keeps `nextPageKey`; name those goldens in the commit message.

- [ ] **Step 5: Commit**

Message: `fix(client): stop paging at --limit and keep nextPageKey when results are truncated`.

---

### Task 6a: Exit codes and multi-environment failures

**Files:**
- Create: `pkg/client/exitcode.go`, `pkg/client/exitcode_test.go`, `cmd/errors.go`, `cmd/errors_test.go`
- Modify: `pkg/client/multi.go` (`UnwrapSingle`), `pkg/client/diagnostic.go`, `cmd/app.go` (`Run`)

**Interfaces:**
- Produces in `pkg/client`:
  - Constants `ExitSuccess=0`, `ExitError=1`, `ExitUsage=2`, `ExitAuth=3`, `ExitNotFound=4`, `ExitPermission=5`.
  - `func ExitCodeForStatus(status int) int`: 401→3, 403→5, 404→4, else 1.
  - `type PartialFailureError struct{ Failed []EnvResult; Total int }`.
    - `Error()` gives `"<n> of <Total> environments failed: <name>: <err>; …"`, in config order.
    - `Code() int`: when every environment failed and all of them map to the same `ExitCodeForStatus`, that code; otherwise 1.
  - `UnwrapSingle` returns `(merged, *PartialFailureError)` when any environment failed. A single-environment result is unchanged.
  - `func Diagnose(err error) string` returns the hint text (today's `DiagnoseError`). `WrapWithDiagnosis` is deleted: hints are no longer glued into the error text.
- Produces in `cmd`:
  - `type UsageError struct{ Err error; Suggestions []string }` with `Error()` and `Unwrap()`.
  - `func exitCode(err error) int`: nil→0, `*UsageError`→2, `*client.PartialFailureError`→`Code()`, `*client.ErrAPI`→`ExitCodeForStatus`, otherwise 1.
  - `var errReported = errors.New("already reported")`, which a command returns, wrapped, when it has already printed its own error output.
  - `func (a *App) reportError(cmd *cobra.Command, err error)`. In human mode it does `PrintHumanError(err)`, then `PrintHint(client.Diagnose(err))` when the hint is non-empty. Agent mode is Task 6b.
  - `Run` returns `exitCode(err)`.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestExitCodeForStatus` | 401→3, 403→5, 404→4, 400→1 |
| `TestPartialFailureCode` | all 403 → 5; one 403 and one 401 → 1; one ok and one 403 → 1 |
| `TestExitCodes` (cmd, harness) | 401 fixture → code 3; 404 → 4; 403 → 5; `--env "a;b"` both 403 → code 5 with stderr naming `a` and `b`; one ok and one 403 → code 1 |

Rename `TestCharMultiEnvAllFail` to `TestMultiEnvAllFailExitsNonZero` and update its golden.

Until the runner exists (Task 7), a multi-environment call with a failed environment returns the `PartialFailureError` from the command's existing `multiExec` branch, so only the error prints. Printing the other environments' partial results arrives with the runner, and `TestMultiEnvOneEnvUnreachable` pins it.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./pkg/client/ ./cmd/ -run 'ExitCode|PartialFailure|MultiEnvAllFail' -v`
Expected: build failures, then code assertions failing (today everything exits 1, or 0 for multi-env).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./pkg/client/... ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `fix(cli): exit codes 0-5; non-zero exit when any environment fails`.

---

### Task 6b: Agent envelope from `dtctl/sdk/agentmode`

**Files:**
- Modify: `go.mod`/`go.sum`, `pkg/output/agent.go`, `pkg/output/output_test.go`, `cmd/app.go`, `cmd/errors.go`, `cmd/root_test.go`, `pkg/client/client.go` (User-Agent)
- Docs: `README.md` (agent-mode section), `skills/dtmgd/SKILL.md` (envelope example), `SECURITY.md` (notice wording)

**Interfaces:**
- Consumes: `agentmode.Response`, `ResponseContext`, `ErrorDetail`, `WriteSuccess(w, result, ctx)`, `WriteError(w, detail)`, `ClassifyHTTPError(status int) string`, `Detect() AgentInfo{Detected bool; Name string}`, `EnvVars() []string`, `UserAgentSuffix() string`.
- `go get github.com/dynatrace-oss/dtctl/sdk@v0.45.0`. Use exactly `@v0.45.0`: `@sdk/v0.45.0` is rejected.
  - Expected side effect: `go 1.26.0` + `toolchain go1.26.10` becomes `go 1.26.10` with no toolchain line. `scripts/drift-check.sh` already falls back to `"go"+.Go`.
  - Module-graph-only additions: kr/pretty, pkg/browser, rogpeppe/go-internal, check.v1. Snyk may report them; see the Snyk knowledge-base entry.
- Produces in `pkg/output`:
  - `func NewAgentPrinter(w io.Writer, ctx *agentmode.ResponseContext) Printer`. `Print(v)` = `agentmode.WriteSuccess(w, v, ctx)`.
  - `func WriteAgentError(w io.Writer, d *agentmode.ErrorDetail) error`.
  - `func WriteAgentPartial(w io.Writer, result interface{}, d *agentmode.ErrorDetail, ctx *agentmode.ResponseContext) error`, which encodes `agentmode.Response{OK:false, Result:result, Error:d, Context:ctx}` with a 2-space indent.
  - `DetectAgent()` = `agentmode.Detect().Detected`. Delete `AgentEnvVars` and `DetectedAgentVar`, along with Phase 1's `TestDetectedAgentVar`. `cmd/skills_test.go` and `cmd/root_test.go` clear `agentmode.EnvVars()` instead.
- Produces in `cmd`:
  - `func errorDetail(err error, hint string, scopes []string) *agentmode.ErrorDetail`, with codes:

    | Error | Code |
    |---|---|
    | `*client.ErrAPI` | `ClassifyHTTPError(status)`, plus `StatusCode` |
    | `*UsageError` | `unknown_command` when it is an unknown command or resource, otherwise `validation_error` |
    | `*config.NotFoundError`, `*config.UnresolvedVarsError`, "no current context" | `config_error` |
    | `net.Error` timeout or `context.DeadlineExceeded` | `timeout` |
    | `*client.PartialFailureError` | `partial_failure` |
    | anything else | `error` |

    `Suggestions` = the hint (when non-empty) + `UsageError.Suggestions`.
  - `reportError` in agent mode writes `WriteAgentError` to `a.Out`.
  - Agent notice text (exact): `Agent mode is on (detected <Name>). Use --no-agent for human-readable output.`
  - User-Agent: `dtmgd/<version>` + `agentmode.UserAgentSuffix()`.
  - The harness now clears `agentmode.EnvVars()`.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestAgentErrorEnvelope` | 403 fixture with `-A` → stdout parses as `agentmode.Response`, `ok == false`, `error.code == "permission_denied"`, `error.status_code == 403`, `len(error.suggestions) == 1`, and code 5 |
| `TestAgentNoticeNamesAgent` | `CLAUDECODE=1` set inside the test → stderr contains `Agent mode is on (detected claude` |
| `TestUserAgentHasAgentSuffix` | with `CLAUDECODE=1`, the fake API sees a `User-Agent` containing `AI-Agent:` |

Update `TestAgentNoticeOnlyWhenAutoDetected` to the new text.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./pkg/output/ ./cmd/ -run 'AgentErrorEnvelope|AgentNotice|UserAgentHasAgentSuffix' -v`
Expected: FAIL (`code` is `"error"`, no `status_code`, old notice text).

- [ ] **Step 3: Implement. Update README, SKILL.md and SECURITY.md wording to the new envelope and notice.**

- [ ] **Step 4: Run the tests and the vulnerability check.**

Run: `go test ./pkg/output/... ./cmd/... && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`
Expected: `ok`; `No vulnerabilities found.`

- [ ] **Step 5: Commit**

Message: `feat(agent): adopt dtctl sdk/agentmode envelope (classified codes, suggestions, status_code)`.

---

### Task 6c: Usage errors, "did you mean", `--no-agent` on parse failures

**Files:**
- Modify: `cmd/errors.go`, `cmd/app.go`, `cmd/get.go`, `cmd/describe.go`, `cmd/query.go` (`requireSubcommand`)
- Test: `cmd/errors_test.go`

**Interfaces:**
- Produces:
  - `func closest(input string, candidates []string) (string, bool)`: case-insensitive Levenshtein distance ≤ 2, ties broken alphabetically.
  - `requireSubcommand`:
    - With an argument, it returns `&UsageError{Err: fmt.Errorf("unknown resource type %q for %q", arg, cmd.CommandPath()), Suggestions: []string{"dtmgd " + verb + " " + match}}` when there is a match.
    - With no match, the suggestion is `Run 'dtmgd <verb> --help' for available resources`.
    - With no argument: `&UsageError{Err: errors.New("a resource type is required")}`.
    - It no longer prints help to stdout.
  - Root `SetFlagErrorFunc`: an unknown flag becomes `&UsageError{Err: fmt.Errorf("unknown flag --%s, did you mean --%s?", got, match)}`, with no suggestion when nothing matches. Candidates are the local and inherited flag names.
  - `func classifyCobraError(err error) error`: messages starting with `unknown command`, `unknown flag`, `unknown shorthand flag`, `accepts`, `requires at least`, `invalid argument`, `flag needs an argument` or `required flag(s)` become `*UsageError`.
  - `func rawAgentMode(args []string) bool`, used by `Run` only when execution failed before flags were parsed:
    - `--no-agent` (or `--no-agent=true`) before `--` → false
    - otherwise `--agent`, `-A` or an agent detected from the environment → true

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestDidYouMeanResource` | `get prblems` → code 2, stderr contains `unknown resource type "prblems"` and `dtmgd get problems`, stdout is empty |
| `TestDidYouMeanFlag` | `get problems --statsu OPEN` → code 2, stderr contains `did you mean --status?` |
| `TestParseFailureEnvelope` (Review Focus 2) | `get problems --bogus -A` → stdout is one envelope with `error.code == "validation_error"`, code 2. `get prblems -A` gives `unknown_command`. With `CLAUDECODE=1`, `get problems --bogus --no-agent` → stdout empty, stderr human `Error:` line, code 2 |

Rename `TestCharUnknownResource` to `TestUnknownResourceIsUsageError` and update its golden.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'DidYouMean|ParseFailureEnvelope|UnknownResource' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `feat(cli): usage errors exit 2 with did-you-mean suggestions; honour --no-agent on parse failures`.

---

### Task 6d: Token scopes declared once

**Files:**
- Create: `cmd/scopes.go`, `cmd/scopes_test.go`
- Modify: every command constructor (`withScopes(...)`), `cmd/errors.go` (403 hint), `README.md` (scope list between markers)

**Interfaces:**
- Produces:
  - `const scopesAnnotation = "dtmgd.io/scopes"`.
  - `func withScopes(cmd *cobra.Command, scopes ...string) *cobra.Command`.
  - `func requiredScopes(cmd *cobra.Command) []string`.
  - `func allScopes(root *cobra.Command) []string`: sorted and unique.
  - Values are the table in Global Constraints.
  - `reportError` on a 403 adds the hint `This command needs the token scope(s): <comma list>.`, replacing the stale list in `pkg/client/diagnostic.go:63` (that line is deleted).
  - README: the required-scopes list sits between `<!-- scopes:start -->` and `<!-- scopes:end -->`, one bullet per scope.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestEveryAPICommandDeclaresScopes` | walk the tree; every runnable command under `get`/`describe`/`query` has a non-empty `requiredScopes` |
| `TestReadmeScopeListMatchesCommands` | the README bullets between the markers equal `allScopes(newRootCmd(...))` |
| `Test403HintNamesScopes` | 403 fixture on `get slos` → stderr contains `slo.read` |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'DeclaresScopes|ReadmeScopeList|403HintNamesScopes' -v`
Expected: FAIL.

- [ ] **Step 3: Implement, and rewrite the README scope list between the markers.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `feat(cli): declare token scopes per command; 403 hint and README list come from one source`.

---

### Task 7: The shared runner, piloted on `get problems`

**Files:**
- Create: `cmd/runner.go`, `cmd/runner_test.go`
- Modify: `cmd/get_problems.go`, `cmd/get_events.go` (watch flags only), `cmd/watch.go`, `cmd/app.go` (remove the global watch options)

**Interfaces:**
- Produces:

```go
// fetched is one environment's API-shaped result.
type fetched struct {
	Body    interface{} // printed as-is for json/yaml/agent
	Total   int         // -1 when unknown
	HasMore bool
}

type fetchSpec struct {
	Verb, Resource string                                  // agent context and messages, e.g. "get", "problems"
	Fetch          func(c *client.Client, limit int) (fetched, error)
	Rows           func(body interface{}) (rows interface{}, n int, err error) // table rows for one env; nil → YAML of Body
	Human          func(w io.Writer, body interface{}) error // custom human renderer (describe); overrides Rows
	Limit          int                                       // user --limit; 0 = none
	List           bool                                      // applies the agent default limit of 50
	Empty          string                                    // stderr note when n == 0, e.g. "No problems found for the given filters."
}

func (a *App) runFetch(s fetchSpec) error

// pagedFetch is the default Fetch for a v2 list endpoint.
func pagedFetch(path string, params map[string]string, listKey string, maxPages int) func(*client.Client, int) (fetched, error)
```

- `runFetch` behaviour, as the spec's output rules require:
  1. **Limit and fan-out.** The effective limit is `s.Limit`, or 50 when it is 0 and `s.List && a.agentMode()`. The fan-out is always `client.MultiRequest(cfg, a.Opts.Env, a.Opts.Concurrency, client.Options{Verbosity: a.Opts.Verbose}, …)`; a single environment is a result of length 1.
  2. **Agent mode.**
     - **Single environment:** `WriteSuccess(Body, ctx{Verb, Resource, Total (when ≥ 0), HasMore})`. When `HasMore`, add the suggestion `Showing <n> of <Total> <resource>; use --limit to see more` (drop "of <Total>" when Total < 0).
     - **Multi-environment:** `result` = a map name → Body, or `{"error": msg}` for a failed environment; `ctx.warnings` = `"<name>: <err>"` for each failure. If any environment failed, write `WriteAgentPartial` with code `partial_failure` and return `fmt.Errorf("%w: %w", errReported, partialErr)`.
  3. **json/yaml.** Print `Body`, or the multi-environment map with the same `{"error":…}` entries. Failures print to stderr as `PrintHumanError`, and the runner returns the `PartialFailureError`.
  4. **table/wide.**
     - For each environment, in config order: when there is more than one environment, print a `== <name> ==` header line to stdout. A failed environment prints its error to stderr.
     - Otherwise render `Human`, else `Rows` (with `n == 0`, print the stderr note `Empty`), else YAML of `Body`.
     - When `HasMore`, print the stderr note `Showing <n> of <Total> <resource>. Use --limit or narrower filters.`
  5. **Errors.** A single environment's error is returned unchanged; `Run` reports it.
- `pagedFetch`: `c.GetV2Paged(path, params, client.Paging{MaxPages: maxPages, Limit: limit, ListKey: listKey})`. `Total` = `totalCount` when it is present, else the list length when no `nextPageKey`, else -1. `HasMore` = `nextPageKey` present, or `totalCount > len(list)`.
- `get problems`: `pageSize` = min(limit, 10), or 10 when there is no limit. Rows map `ProblemEntry` to `ProblemListItem` as today.
- **Watch:** `--watch/-w` and `--watch-interval` become local flags on `get problems` and `get events` only. Required-flag validation (events `--from`) happens before the loop.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestLimitHonouredInEveryFormat` (table-driven) | `get problems --limit 12` with 15 available, for table/json/yaml/agent: json/yaml/agent `problems` has length 12; the table has 12 data rows; the fake API saw 2 page requests; stderr (human) contains `Showing 12 of 15 problems` |
| `TestAgentDefaultLimit` | 60 problems available, `-A` with no `--limit` → `len(result.problems) == 50`, `context.has_more == true`, `context.total == 60` |
| `TestEmptyResultJSONAndAgent` (Review Focus 5) | empty fixture: `-o json` stdout parses with `problems == []`, code 0; `-A` gives `ok:true` and an empty list; table: stdout empty, stderr contains `No problems found` |
| `TestMultiEnvTableSections` | `--env "a;b"` → stdout contains `== a ==` and `== b ==` and two tables |
| `TestMultiEnvOneEnvUnreachable` (Review Focus 3) | env `b` on a closed port: `-o json` → stdout has `a` data and `b.error`, code 1, wall time under 5s |
| `TestWatchFlagOnlyOnWatchableCommands` | `get slos -w` → code 2, `unknown shorthand flag` |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'LimitHonoured|AgentDefaultLimit|EmptyResultJSON|MultiEnvTableSections|OneEnvUnreachable|WatchFlagOnly' -v`
Expected: FAIL.

- [ ] **Step 3: Implement `runner.go`, migrate `get problems`, move the watch flags, and update the goldens for get problems.**

- [ ] **Step 4: Run the tests.**

Run: `go test -race ./cmd/... ./pkg/...`
Expected: `ok` (the known `pkg/skills` failures are allowed locally).

- [ ] **Step 5: Commit**

Message: `feat(cli): shared fetch/limit/render runner; migrate get problems; --watch only where supported`.

---

### Task 8: Migrate the other `get` commands

**Files:**
- Modify: `cmd/get_entities.go` (entities, entity-types), `cmd/get_events.go`, `cmd/get_metrics.go`, `cmd/get_slos.go`, `cmd/get_security.go`
- Fixtures and goldens: one list fixture per resource

**Interfaces:**
- Consumes: `fetchSpec`, `pagedFetch` and `runFetch` from Task 7.
- List keys and page sizes:

  | Resource | List key | Page size | Notes |
  |---|---|---|---|
  | entities | `entities` | limit, max 100 | |
  | entity-types | `types` | 500 | gains `--limit` |
  | events | `events` | limit, max 100 | |
  | metrics | `metrics` | limit, max 500 | |
  | slos | `slo` | 25 with `--evaluate`, else limit, max 200 | `--evaluate --limit 50` pages through 2×25 |
  | security-problems | `securityProblems` | limit, max 200 | |

- `get slos --evaluate` keeps its stderr status summary in human mode.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestGetListCommandsHonourLimit` (table-driven over the six commands) | `--limit 3 -o json`: the list has length 3 |
| `TestSlosEvaluateLimitAbove25` | 60 SLOs: `--evaluate --limit 50 -o json` gives 50 SLOs and 2 requests, each with `pageSize=25` |
| `TestEntitiesWithoutSelectorIsUsageError` | code 2 |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'GetListCommandsHonourLimit|SlosEvaluateLimitAbove25|EntitiesWithoutSelector' -v`
Expected: FAIL.

- [ ] **Step 3: Migrate each command onto `runFetch`, delete its `isMultiEnv` branch, and regenerate the goldens.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`. `grep -c 'isMultiEnv()' cmd/get_*.go` is 0 in every file.

- [ ] **Step 5: Commit**

Message: `feat(cli): migrate get commands onto the shared runner (--limit honoured everywhere)`.

---

### Task 9: Migrate the `describe` commands

**Files:**
- Modify: `cmd/describe_problem.go`, `cmd/describe_entity.go` (entity, entity-type, entity-relations), `cmd/describe_event.go`, `cmd/describe_metric.go`, `cmd/describe_slo.go`, `cmd/describe_security.go`, `cmd/describe.go` (help text)

**Interfaces:**
- Consumes: `fetchSpec` with `Fetch` returning `fetched{Body: map, Total: -1}`.
- Human renderers become `printProblemDetail(w io.Writer, d ProblemDetail)`, `printSLODetail(w io.Writer, …)` and `printSecurityProblemDetail(w io.Writer, …)`, writing to `w` and wired through `fetchSpec.Human`. Describe commands without a custom renderer print YAML in table mode.
- `describe slo`: when `--from` or `--to` is set and `--timeframe` isn't, send `timeFrame=GTF` (spec Phase 3 item, cheap to fold in here).
- `describe.go` `Long`: "Detail commands print a readable summary (or YAML) by default; use -o json for the raw API object."

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestDescribeTableIsReadable` (table-driven over entity, entity-type, entity-relations, event, metric) | stdout has no `map[` and contains the fixture's ID as `key: value` YAML |
| `TestDescribeSLOSetsGTF` | `describe slo X --from now-7d` → the fake API received `timeFrame=GTF` |
| `TestDescribeMultiEnvJSON` | `describe problem P --env "a;b" -o json` → a map keyed `a`/`b` |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'DescribeTableIsReadable|DescribeSLOSetsGTF|DescribeMultiEnvJSON' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `feat(cli): migrate describe commands; readable default output; describe slo sends GTF with explicit dates`.

---

### Task 10: Migrate the `query` commands; fix `query log-counts`

**Files:**
- Modify: `cmd/query_metrics.go`, `cmd/query_logs.go`, `cmd/query_log_counts.go`, `cmd/query.go` (help lists `log-counts`)

**Interfaces:**
- **`query metrics`:**
  - `Fetch` does `GetV2("/metrics/query")`.
  - The summary and series renderer becomes `Human(w, body)` and writes everything to `w` (no `PrintInfo` for table content).
  - Entity-name lookup errors print one stderr warning and do not fail the command.
  - `-o yml` behaves like yaml.
- **`query logs`:**
  - `Fetch` does `GetV2("/logs/search")` with `limit` = min(limit, 1000), defaulting to 100.
  - `HasMore` = `nextSliceKey` present. Total = `totalCount`.
  - Records render through `Human`.
  - Default `sort=-timestamp` is **not** in this task; it belongs to Phase 3.
- **`query log-counts`:**
  - Delete `cleanPGName`'s BookStore handling: the name shown is the entity's `displayName`.
  - An entity lookup error is returned, so a 401 exits 3.
  - It runs through `runFetch`, so `--env` works.
  - Its header lines go to stderr, so `-o yaml` stdout is pure YAML.
  - **Top-N window rule:** `/logs/aggregate` returns at most `maxGroupValues` groups. When a level's result has exactly `maxGroupValues` groups, an entity missing from it is **unknown**: show `-` and print a stderr note `Some process groups are outside the top <N> for <LEVEL>; their counts are shown as "-".`. When the result has fewer groups, a missing entity's count is 0.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestYAMLOutputParses` (Review Focus 1, table-driven over every list, describe and query command) | `-o yaml` stdout unmarshals with `yaml.Unmarshal` into `interface{}` with no error, and the first line is not a human sentence |
| `TestLogCountsEntityLookupErrorFails` | `/entities` returns 401 → code 3 |
| `TestLogCountsTopNWindowShowsUnknown` | aggregate fixture with `maxGroupValues=2` and 2 groups, 3 entities → the third entity's cell is `-`, stderr has the note |
| `TestLogCountsNoDemoNames` | `grep -c 'BookStore\|bookstore' cmd/query_log_counts.go` is 0 (assert via `os.ReadFile`) |
| `TestQueryLogsHasMoreFromSliceKey` | fixture with `nextSliceKey` and `-A` → `context.has_more == true` |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'YAMLOutputParses|LogCounts|QueryLogsHasMore' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`. `grep -c 'isMultiEnv' cmd/*.go | grep -v ':0'` gives no output. Delete `isMultiEnv` and `multiExec`.

- [ ] **Step 5: Commit**

Message: `fix(query): migrate query commands; log-counts honours --env, surfaces lookup errors, marks top-N gaps`.

---

### Task 11: Security display-ID lookup (`S-NNN`)

**Files:**
- Modify: `cmd/describe_security.go` (`resolveSecurityProblemID`)
- Test: `cmd/describe_security_test.go`

**Interfaces:**
- `func resolveSecurityProblemID(c *client.Client, id string, verbose int, errw io.Writer) (string, error)`:
  - It pages `/securityProblems` with `pageSize=500` by hand (`GetV2` with `nextPageKey`) and **stops at the first page that contains the match**.
  - At `verbose > 0` it writes `Resolving <id> by listing security problems…` to `errw`.
  - It returns any API error.
  - When it isn't found, it returns `fmt.Errorf("security problem %s not found", id)`, which is a not-found error in exit-code terms: wrap it so `exitCode` gives 4.
  - Display IDs are per environment, so resolution runs once per environment and never more than once per invocation.

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestResolveStopsAtFirstMatchingPage` | 3 pages, match on page 1 → 1 request |
| `TestResolvePropagatesErrors` | 403 → error returned, not the original ID |
| `TestResolveNotFoundExits4` | harness: `describe security-problem S-999` with no match → code 4 |

Update the existing `TestResolveSecurityProblemID_*` tests to the new signature.

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'Resolve' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `fix(security): S-NNN lookup stops at the first match and reports errors`.

---

### Task 12: Config housekeeping, environment variables, credential deletion

**Files:**
- Modify: `pkg/config/config.go` (`SaveTo`, `GetToken`, remove `Load`), `pkg/config/keyring.go` (probe once), `pkg/config/resolve.go` (remove `APIBaseURL`/`ClusterAPIBaseURL`), `pkg/client/multi.go` (remove `IsSingleEnv`), `pkg/config/source.go` + `cmd/app.go` (env vars), `cmd/config.go`, `cmd/ctx.go`
- Docs: `README.md` (env vars, credential deletion), `SECURITY.md` (0600 wording)

**Interfaces:**
- **`SaveTo`** writes atomically: a temp file in the same directory, `Chmod(0600)`, then `Rename`. The saved file is always 0600, including a file that was looser before. `SECURITY.md` item 2 becomes "dtmgd writes the config with mode 0600."
- **`IsKeyringAvailable`** probes once per process (`sync.Once`).
- **`GetToken`**: a keyring error other than `keyring.ErrNotFound` returns `fmt.Errorf("keyring: %w", err)` instead of falling through to "token not found".
- **Environment variables.** `DTMGD_CONFIG` is used when `--config` is empty, with `SourceExplicit` semantics. `DTMGD_CONTEXT` is used when `-c` is empty and `--env` doesn't name a context. Precedence: flag > env var > file.
- **`config delete-credentials <name>`** removes the token from the keyring and from the config's `tokens` list. It errors if no such token exists. It prints a warning naming every context whose `token-ref` still points at it.
- **`config delete-context <name> --delete-credentials`** (and `ctx delete … --delete-credentials`) also deletes the context's `token-ref` when no other context uses it.
- **Delete dead code:** `config.Load`, `ResolvedContext.APIBaseURL`, `ResolvedContext.ClusterAPIBaseURL`, `client.IsSingleEnv`, and their tests. Keep `expandedFrom` and the `SaveTo` guard (deliberate backstop).

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestSaveToIsAtomicAnd0600` | an existing 0644 file is 0600 after `SaveTo`; no temp files remain in the dir (skip the mode check on windows) |
| `TestGetTokenSurfacesKeyringErrors` | `keyring.MockInitWithError(errors.New("locked"))` → error contains `keyring: locked` (restore with `t.Cleanup(keyring.MockInit)`) |
| `TestEnvVarConfigAndContext` (harness) | `DTMGD_CONFIG=<path>` with no `--config` works; `DTMGD_CONTEXT=b` targets env `b`; `-c a` overrides `DTMGD_CONTEXT` |
| `TestDeleteCredentials` | after `config delete-credentials t1`, the keyring mock has no `t1`, the config has no `t1` entry, and stderr warns about a context still using it |
| `TestDeleteContextWithCredentials` | a shared token-ref is kept; an unshared one is deleted |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./pkg/config/ ./cmd/ -run 'Atomic|SurfacesKeyring|EnvVarConfig|DeleteCredentials|DeleteContextWithCredentials' -v`
Expected: FAIL.

- [ ] **Step 3: Implement and remove the dead code.**

- [ ] **Step 4: Run the tests and lint.**

Run: `go test ./pkg/config/... ./cmd/... && go vet ./... && golangci-lint run`
Expected: `ok`; `0 issues.`

- [ ] **Step 5: Commit**

Message: `fix(config): atomic 0600 saves, surface keyring errors, DTMGD_CONFIG/DTMGD_CONTEXT, credential deletion`.

---

### Task 13: `commands` catalog, `doctor`, `get environments` deprecation note

**Files:**
- Create: `cmd/commands.go`, `cmd/doctor.go`, `cmd/commands_test.go`, `cmd/doctor_test.go`
- Modify: `cmd/get_environments.go`, `cmd/app.go`
- Docs: `README.md`, `skills/dtmgd/SKILL.md` (agents start with `dtmgd commands --brief`; `doctor` replaces `get environments` for connectivity)

**Interfaces:**
- **`dtmgd commands [--brief]`.** JSON by default; `-o yaml` gives YAML. Never wrapped in an envelope.
  - Full catalog:

    ```
    {"schema_version":1,"tool":"dtmgd","version":"<v>","command_model":"verb-noun",
     "global_flags":{"--output":{"type":"string","default":"","description":"…"},…},
     "commands":[{"path":"get problems","aliases":["prob","problem"],"short":"…",
                  "required_scopes":["problems.read"],
                  "flags":{"--status":{"type":"string","default":"","description":"…"},…}},…]}
    ```

  - `--brief`: `{"schema_version":1,"tool":"dtmgd","version":"<v>","verbs":{"get":["entities","entity-types",…],"describe":[…],"query":[…]}}`, with sorted names.
  - Excludes hidden commands, `help` and `completion`.
- **`dtmgd doctor`.** For each configured context (or only `-c`), one row: `DoctorRow{Context, Host, EnvID, Status ("ok"|"warn"|"fail"), Version, Detail}` with table tags `CONTEXT HOST ENV-ID STATUS VERSION DETAIL`.
  - Checks: context resolves, token found, then `ClusterVersion()`.
  - **Version fallback.** On 401 or 403 from the version call, `GET /api/v2/metrics?pageSize=1`. If that succeeds: `warn`, Version `unknown`, Detail `cluster version not readable with this token; assumed ≥ 1.328.0`.
  - A version below `1.328.0` is `warn`, with Detail `below 1.328.0, the minimum the Dynatrace Managed MCP server supports`.
  - Any other error is `fail`, with Detail from `client.Diagnose`.
  - Exit 1 when any row is `fail`, as a returned error that has already been reported.
  - json/yaml/agent print the rows.
- **`get environments`** keeps today's per-context check, using doctor's code, and prints the stderr note: `Note: "get environments" will list environments from the Cluster API in a coming release. Use "dtmgd doctor" for this connectivity check.`

- [ ] **Step 1: Write the failing tests.**

| Test | Assertion |
|---|---|
| `TestCommandsBriefListsResources` | `commands --brief` → JSON with `verbs.get` containing `problems` and `security-problems` and not containing `help` |
| `TestCommandsFullHasScopesAndFlags` | `commands` → the entry `get problems` has `required_scopes == ["problems.read"]` and `flags["--status"]` |
| `TestCommandsNotEnveloped` | `commands -A` → top-level key `schema_version`, no `ok` |
| `TestDoctorStatuses` | three contexts (version 1.330.0 ok; version 403 + metrics 200 → warn `assumed`; 401 → fail) → rows ok/warn/fail, code 1 |
| `TestDoctorOldVersionWarns` | version 1.320.0 → warn with `1.328.0` in Detail |
| `TestGetEnvironmentsDeprecationNote` | stderr contains `Use "dtmgd doctor"` |

- [ ] **Step 2: Run them to see them fail.**

Run: `go test ./cmd/ -run 'Commands|Doctor|GetEnvironmentsDeprecation' -v`
Expected: FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests.**

Run: `go test ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

Message: `feat(cli): dtmgd commands catalog and dtmgd doctor; deprecate get environments' connectivity role`.

---

### Task 14: Docs, knowledge base, PR 2, release v0.1.0

**Files:**
- Modify: `README.md`, `skills/dtmgd/SKILL.md`, `.claude/CLAUDE.md`, `.github/copilot-instructions.md` (only remove advice this phase made false)

**Interfaces:** none.

- [ ] **Step 1: Bring README and SKILL.md in line with behaviour.**

Cover: exit codes table, `--plain`, `-o` values, `--limit` semantics and the agent default of 50, envelope example (new codes), `--watch` only on problems/events, `commands`, `doctor`, `DTMGD_CONFIG`/`DTMGD_CONTEXT`, credential deletion, multi-environment output rules, the `--env` claim narrowed to what's true.

Verify with:

```bash
grep -n "defaults to JSON\|works with all commands\|caps to a single page\|Go 1.25" README.md skills/dtmgd/SKILL.md
```

Expected: no output.

- [ ] **Step 2: Update `.claude/CLAUDE.md`'s knowledge base.**

Delete or replace:
- "Go coverage: reach 40% without touching RunE"
- the `pkg/skills` line reference (`installer.go:327`)
- "snyk does gate"

Add entries, in the file's format (Context / Insight / When it applies / Date / Ref):
- setup-go v7 ignores go.mod's `toolchain` line when `GOTOOLCHAIN=local` is preset
- goreleaser's tap PR now gets 403 on `dynatrace-oss/homebrew-tap`; open it by hand from the fork branch
- GitHub release-asset 408 "Upload body timed out": delete the draft release and re-run
- SonarCloud `githubactions:S8545` on pinned `go install …@vX` is a false positive
- the command harness (`runCLI`, goldens, `-update`) is how command behaviour is tested now

- [ ] **Step 3: Full verification.**

Run:

```bash
gofmt -l . && go vet ./... && golangci-lint run \
  && go test -race ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... \
  && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Expected: no gofmt output, `0 issues.`, all `ok`, `No vulnerabilities found.`

- [ ] **Step 4: Final whole-branch review, then push and open PR 2 (owner go-ahead).**

Title: `Phase 2 (2/2): correct output, limits, exit codes and agent envelope`. The body lists every breaking change with its replacement:
- exit codes 2–5 and non-zero exit on partial multi-environment failure
- `-o` validation
- empty JSON prints `{…,"problems":[]}`
- JSON/YAML keep `nextPageKey` when truncated
- agent default limit 50
- the envelope error codes
- `--watch` scope
- usage errors no longer print help
- the agent notice text
- the `go` directive at 1.26.10

- [ ] **Step 5: After merge, tag v0.1.0 (owner go-ahead).**

Follow the Phase 1 release steps. If the tap step 403s, open the tap PR by hand from `ivan-gudak:dtmgd-0.1.0`. Then prepend the breaking-changes list to the release notes with `gh release edit v0.1.0 --notes-file …`.
