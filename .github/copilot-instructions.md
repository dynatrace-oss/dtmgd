# GitHub Copilot Instructions — dtmgd

`dtmgd` is a `kubectl`-inspired read-only CLI for **Dynatrace Managed (self-hosted)** clusters, written in Go. It wraps the Dynatrace Managed v2 API and cluster management API.

## Build, Test, Lint

```bash
make build          # produces ./dtmgd (CGO_ENABLED=0)
make test           # go test -race ./...
make lint           # go vet + gofmt check (fails on unformatted files)
make fmt            # gofmt -w . (auto-format)

# Run a single test
go test -run TestName ./pkg/client/
go test -run TestName ./cmd/

# Check coverage (CI enforces ≥ 60%)
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

## Architecture

```
main.go            → os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))
cmd/               → Cobra CLI; one file per resource/operation
  app.go           → App (one per invocation): Run, global flags, loadConfig, printer
  runner.go        → runFetch: fetch from one or more environments, apply --limit, render
  root.go          → rewriteNegativeArgs, warnBareRefs
  get_problems.go  → dtmgd get problems
  describe_*.go    → dtmgd describe <resource>
  query_*.go       → dtmgd query metrics/logs
  watch.go         → --watch flag support (addWatchFlags, watchOrRun; get problems/events only)
pkg/client/        → HTTP client (resty); GetV2, GetV1, GetCluster, GetV2Paged
pkg/config/        → YAML config (~/.config/dtmgd/config); contexts, tokens, keyring
pkg/output/        → Printer interface: TablePrinter, JSONPrinter, YAMLPrinter, AgentPrinter
pkg/version/       → version/commit/date injected via ldflags at build time
```

## Key Conventions

### Table output uses struct tags

```go
type FooListItem struct {
    Name string `table:"NAME"`
    Desc string `table:"DESC,wide"` // only shown with -o wide or --columns DESC
}
```
`TablePrinter` reflects over the struct at runtime. The `Printer` interface has `Print(v)` and `PrintList(v)`.

### Pagination

`GetV2Paged` is a method (not a generic function) — returns `map[string]interface{}`:

```go
raw, err := c.GetV2Paged("/problems", params, client.Paging{MaxPages: maxPages, Limit: limit, ListKey: "problems"})
var resp ProblemsResponse
client.DecodePaged(raw, &resp)
```

`GetV2Paged` stops once `Limit` items are in hand and keeps the last `nextPageKey` when it stops with pages remaining. Commands normally use `pagedFetch(path, params, listKey, pageCap, a.Opts.MaxPages)` in `cmd/runner.go` rather than calling it directly.

### Multi-environment fan-out

Every `get`, `describe` and `query` command (except `get environments`) calls `a.runFetch(fetchSpec{...})`, which runs the fetch through `client.MultiRequest` for one environment or, with `--env`, several in parallel. Single-env prints the API's own response; multi-env prints a `map[contextName]data` (json/yaml/agent) or a `== name ==` section per environment (table), and fails with a `PartialFailureError` if any environment failed. Do not branch on `isMultiEnv()` in a command.

### Agent mode (AI envelope output)

Auto-detected from env vars (`CLAUDECODE`, `GITHUB_COPILOT`, `CURSOR_AGENT`, etc.) by dtctl's `sdk/agentmode`. `runFetch` handles agent output for data commands; other commands use `a.printer("resource")`, which wraps output in the envelope in agent mode.

Output is wrapped: `{"ok": true, "result": {...}, "context": {"verb": "get", "resource": "problems", "total": 5}}`.

### Adding a new resource

1. Create `cmd/get_foos.go` with a constructor `newGetFoosCmd(a *App) *cobra.Command`: response types, flags bound to a local options struct (no package-level flag variables), and a `RunE` that calls `a.runFetch(fetchSpec{...})`; register it in `newGetCmd`'s `AddCommand`
2. Declare its token scopes with `withScopes(cmd, "foos.read")` (a test keeps the README scope list in sync)
3. Add `--watch` only if re-running is meaningful: `addWatchFlags(cmd, &watch)` and wrap the body in `a.watchOrRun(watch, func() error { ... })`
4. Test it end to end in `cmd/get_foos_test.go` with the harness in `cmd/harness_test.go` (`runCLI` against a `newFakeAPI`, goldens under `cmd/testdata/golden`, `-update` to regenerate)

### HTTP test patterns

```go
// Use 400/403 for error cases — resty retries 429/5xx (adds seconds of waits)
ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusBadRequest)
}))
```

### Negative problem IDs

Problem IDs can be negative integers (e.g. `-6546711275898328738_1776193140000V2`). `root.go` contains `rewriteNegativeArgs` which inserts `--` before these automatically. Never use `SetInterspersed(false)` on cobra commands — it breaks persistent flags from parent commands.

## DT Managed API Specifics

| Topic | Detail |
|---|---|
| Environment API | `{host}/e/{env-id}/api/v2/` |
| Cluster API | `{host}/api/v1.0/onpremise/` |
| Logs entity attribution | Logs are attributed to `PROCESS_GROUP`, not `SERVICE`. Auto-convert `type(SERVICE)` → `type(PROCESS_GROUP)` in log queries. |
| `logs/aggregate` entitySelector | Hidden/non-functional server-side. Fetch entity IDs via `/entities`, aggregate without `entitySelector`, filter client-side. |
| Entity ID case | `/entities` returns uppercase IDs; logs aggregate returns lowercase. Normalize with `strings.ToLower` for lookups. |
| Security problem `fields` | Valid: `+riskAssessment`, `+managementZones`, `+codeLevelVulnerabilityDetails`, `+vulnerableComponents`, `+affectedEntities`, `+exposedEntities`, `+description`. **NOT** `+codeLocations`. |
| SLO evaluation page cap | `--evaluate` forces `apiPageSize=25` (API limit). Decouple API page size from user `--limit`. |

## Coverage Strategy

Commands are tested end to end through `Run` with the harness in `cmd/harness_test.go` (an httptest fake of the API), so `RunE` code is covered like any other. `go tool cover -func`, which the CI gate uses, counts only statements inside named functions: keep command code in constructors, not in closures assigned to package-level vars. Lower-level packages are unit-tested directly:
- `pkg/client`: `GetV2Paged`, `MultiRequest`, `DecodePaged`, URL construction
- `pkg/config`: round-trip `SaveTo`/`LoadFrom`, `CurrentContextObj`, `GetContext`/`SetContext`/`DeleteContext`
