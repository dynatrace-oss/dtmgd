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
main.go            → calls cmd.Execute()
cmd/               → Cobra CLI; one file per resource/operation
  root.go          → persistent flags, LoadConfig, NewPrinter*, multiExec
  get_problems.go  → dtmgd get problems
  describe_*.go    → dtmgd describe <resource>
  query_*.go       → dtmgd query metrics/logs
  watch.go         → --watch flag support (watchOrRun helper)
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
raw, err := c.GetV2Paged("/problems", params, effectiveMaxPages(singlePageSuffices))
var resp ProblemsResponse
client.DecodePaged(raw, &resp)
```

Use `effectiveMaxPages(hasLimit bool)` — returns 1 when a single page satisfies the limit, otherwise `maxPages` from the flag.

### Multi-environment fan-out

```go
if isMultiEnv() {
    data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
        return c.GetV2Paged("/problems", params, ...)
    })
    return NewPrinterForResource("problems").Print(data)
}
// single-env path below
```

`multiExec` fans out in parallel goroutines. Single-env returns the raw value; multi-env returns `map[envName]data`.

### Agent mode (AI envelope output)

Auto-detected from env vars (`CLAUDECODE`, `GITHUB_COPILOT`, `CURSOR_AGENT`, etc.). Always use the resource-aware printer for data commands:

```go
NewPrinterForResource("problems")  // get/describe/query commands
NewPrinter()                       // config/management commands
```

Output is wrapped: `{"ok": true, "result": {...}, "context": {"resource": "problems"}}`.

### Adding a new resource

1. Create `cmd/get_foos.go` — define response types, flags, `RunE`, register with `getCmd.AddCommand()`
2. Wrap `RunE` body in `watchOrRun(func() error { ... })` to get `--watch` for free
3. Check `isMultiEnv()` and branch: call `multiExec` or `NewClientFromConfig` + typed decode
4. Add `cmd/get_foos_test.go` for any pure helpers; avoid testing `RunE` directly

### HTTP test patterns

```go
// Use 400/403 for error cases — resty retries 5xx (adds 3-second waits)
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

`cmd` package `RunE` functions require a live API; focus coverage on pure helpers:
- `pkg/client`: `GetV2Paged`, `MultiRequest`, `DecodePaged`, URL construction
- `pkg/config`: round-trip `SaveTo`/`LoadFrom`, `CurrentContextObj`, `GetContext`/`SetContext`/`DeleteContext`
- `cmd`: `parseColumns`, `effectiveMaxPages`, `isMultiEnv`, `rewriteNegativeArgs`
