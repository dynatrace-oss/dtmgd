# CLAUDE.md

This file provides guidance to Claude Code when working in the `dtmgd` repository (Dynatrace Managed CLI, Go).

For `dtmgd` CLI usage patterns and command reference, see `skills/dtmgd/SKILL.md`.

## Knowledge Base

### Go coverage: reach 40% without touching RunE functions
- **Context**: `cmd` package has low coverage (11%) because `RunE` functions require real API calls; `pkg/config` had 0%
- **Insight**: Focus coverage work on pure-function helpers: `APIBaseURL()`, `ClusterAPIBaseURL()`, `CurrentContextObj()`, `GetContext()`, `SetContext()`, `DeleteContext()`, config round-trip (SaveTo/LoadFrom), and cmd package helpers (`parseColumns`, `effectiveMaxPages`, `isMultiEnv`). These alone lift coverage to ~44.7%.
- **When it applies**: Any Go CLI project where command handlers need a live external API and cannot be unit-tested without heavy mocking
- **Date**: 2026-04-15
- **Ref**: impl: update README/SKILL and add tests covering changes since v0.0.1

### GetV2Paged is a method, not a standalone generic function
- **Context**: Tried `GetV2Paged[T](c, path, params, pages)` standalone generic syntax — doesn't exist
- **Insight**: `GetV2Paged` is `(c *Client) GetV2Paged(path, params, maxPages) (map[string]interface{}, error)` — returns raw map; use `DecodePaged(raw, &result)` for typed decode
- **When it applies**: Writing tests for pagination in `pkg/client`
- **Date**: 2025-07-14
- **Ref**: increase test coverage to 60%, make release v0.0.14

### strings.Contains catches "://" in protocol prefix
- **Context**: `TestNewTrailingSlash` checked `strings.Contains(url, "//")` to catch double-slash paths
- **Insight**: The `https://` protocol always matches `//` — strip the protocol prefix first before checking for double-slash path segments
- **When it applies**: URL path validation in tests
- **Date**: 2025-07-14
- **Ref**: increase test coverage to 60%, make release v0.0.14

### Use 400/403 for error cases in HTTP tests, never 5xx
- **Context**: resty client has `SetRetryCount(3)` + retry on 5xx; using 500 in httptest adds 3-second waits
- **Insight**: Always use 400 (Bad Request) or 403 (Forbidden) for error test cases — they are NOT retried
- **When it applies**: Any test using `httptest.NewServer` to simulate errors
- **Date**: 2025-07-14
- **Ref**: increase test coverage to 60%, make release v0.0.14

### SetInterspersed(false) breaks persistent flags after positional args in Cobra
- **Context**: `describe problem` used `SetInterspersed(false)` to allow negative problem IDs. This prevents cobra from parsing persistent flags (`--env`, `-c`, `-o`) after the positional argument.
- **Insight**: Never use `SetInterspersed(false)` in cobra commands that also use persistent flags from parent commands. Use `--` (end-of-flags separator) instead: `dtmgd describe problem -- -12345_V2`
- **When it applies**: Any cobra command using `SetInterspersed(false)` with persistent parent flags (common in kubectl-like CLIs)
- **Date**: 2026-04-14
- **Ref**: impl: check for errors in dtmgd, cluster repo, docs

### GitHub releases/latest CDN lag
- **Context**: Dockerfile used `curl -sI -L .../releases/latest` (HTTP redirect) to detect latest release. Redirect can lag 30+ minutes after publishing.
- **Insight**: Use `curl -fsSL https://api.github.com/repos/{owner}/{repo}/releases/latest | grep '"tag_name"'` — the REST API propagates immediately
- **When it applies**: Any Dockerfile or script that autodetects the latest GitHub release version
- **Date**: 2026-04-14
- **Ref**: impl: why Dockerfile installed v0.0.1

### dtmgd security-problem fields — valid additional fields (DT Managed API)
- **Context**: v0.0.1 used `+codeLocations` which is not a valid field, causing 400 errors on `describe security-problem`
- **Insight**: Valid `fields` values for `/api/v2/securityProblems/{id}`: `+riskAssessment`, `+managementZones`, `+codeLevelVulnerabilityDetails`, `+vulnerableComponents`, `+affectedEntities`, `+exposedEntities`, `+description`. NOT `+codeLocations`.
- **When it applies**: Extending dtmgd security problem describe commands
- **Date**: 2026-04-14
- **Ref**: impl: check for errors in dtmgd, cluster repo, docs

### SLO sloSelector DSL for filtering
- **Context**: Needed to filter SLOs by management zone when `get slos` had no filter option
- **Insight**: `/api/v2/slo` supports `sloSelector` DSL: `name("...")`, `text("...")`, `managementZone("...")`, `managementZoneID("...")`, `entityIDs("...")`, `healthState("HEALTHY"|"UNHEALTHY")`. No OR/disjunction.
- **When it applies**: Any server-side SLO filtering in dtmgd
- **Date**: 2026-04-15
- **Ref**: impl: check SLOs of BookStore services, Dynatrace Managed prod

### SLO evaluate=true API limit — force pageSize=25
- **Context**: `dtmgd get slos --evaluate` fails with 400 "Exceeded the limit of 25 SLOs that can be evaluated at once"
- **Insight**: Decouple API page size from user result limit. When `--evaluate`, force `apiPageSize=25`. The `nextPageKey` cursor encodes evaluate=true state for subsequent pages.
- **When it applies**: Any `get slos --evaluate` with default pageSize > 25
- **Date**: 2026-04-15
- **Ref**: impl: check SLOs of BookStore services, Dynatrace Managed prod

### SLO evaluatedPercentage = -1 with evaluate=true
- **Context**: Some SLOs show `evaluatedPercentage = -1.00%` even with `--evaluate`; still show FAILURE status
- **Insight**: These SLOs reference entities that no longer exist or have no metric data. The cached `status` field reflects last successful evaluation; `evaluatedPercentage` cannot be recomputed. Not a bug in dtmgd.
- **When it applies**: Presenting SLO data — surface as "N/A evaluation (stale entity)" not an error
- **Date**: 2026-04-15
- **Ref**: impl: check SLOs of BookStore services, Dynatrace Managed prod

### DT Managed Classic: logs attributed to PROCESS_GROUP, not SERVICE
- **Context**: `query log-counts` with `groupBy=dt.entity.service` returned empty results
- **Insight**: On DT Managed Classic, log records contain `dt.entity.process_group`, not `dt.entity.service`. Use `groupBy=dt.entity.process_group`. Auto-convert `type(SERVICE)` selectors to `type(PROCESS_GROUP)` for log queries.
- **When it applies**: Any log aggregation or log search scoped to services on DT Managed Classic
- **Date**: 2026-04-15
- **Ref**: impl: check logs of BookStore services using dtmgd

### DT Managed Classic: logs aggregate entitySelector is hidden/non-functional
- **Context**: `/api/v2/logs/aggregate` has `entitySelector` marked as `hidden=true` in server source. Passing it returns `{"aggregationResult":{}}`.
- **Insight**: Never pass `entitySelector` to logs aggregate. Instead: (1) fetch entity IDs via `/api/v2/entities`, (2) aggregate without entitySelector, (3) filter results client-side by matching entity IDs.
- **When it applies**: Any call to `/api/v2/logs/aggregate` intended to filter by entity
- **Date**: 2026-04-15
- **Ref**: impl: check logs of BookStore services using dtmgd

### DT Managed Classic: entity API uppercase IDs; logs aggregate lowercase IDs
- **Context**: `GET /entities` returns `PROCESS_GROUP-CAA2A66AE22043F9` (uppercase); `/api/v2/logs/aggregate` returns `process_group-caa2a66ae22043f9` (lowercase) for the same entity
- **Insight**: Normalize entity IDs to lowercase (`strings.ToLower`) when building lookup maps from the entities API, so they match IDs from the logs aggregate API
- **When it applies**: Any code that stores entity IDs from the entities API and looks them up against logs aggregate API results
- **Date**: 2026-04-15
- **Ref**: impl: check logs of BookStore services using dtmgd

### pkg/skills tests fail locally but pass on CI — not a real failure
- **Context**: `go test ./...` fails `TestInstallAndStatusAndUninstall` and `TestStatusAll` with "expected not installed in fresh dir" on any machine that has dtmgd skills installed. Reproduces on clean `main`, so it looks like a regression when it is not.
- **Insight**: The tests pass a `t.TempDir()` as `baseDir`, but `Status()` also consults `os.UserHomeDir()` (`pkg/skills/installer.go:234`), so the "fresh dir" is not fresh once you have run `dtmgd skills install`. CI runners have no skills installed, which is why every check is green. Use `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...` for a clean local signal, and do not "fix" it as part of unrelated work.
- **When it applies**: Any local full-suite run, especially before concluding that a change broke something
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)

### Keyring-touching tests must install a mock, or they eat real credentials
- **Context**: A test called `setCredentials("prod-token", ...)`, which reaches `config.SetToken` → `keyring.Set("dtmgd", "prod-token", ...)` whenever `IsKeyringAvailable()` is true. `prod-token` is the credential name used in the README quick start, so `go test ./cmd/...` silently overwrote a developer's live API token. Headless CI takes the other branch, so CI never surfaced it.
- **Insight**: `pkg/config` and `cmd` both have a `TestMain` calling `keyring.MockInit()`. Keep it that way, and never add a keyring-touching test to another package without one. Side effect: `IsKeyringAvailable()` then reports true for the whole binary, so the plaintext (no-keyring) branch needs `keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)` with a `t.Cleanup(keyring.MockInit)` restore — see `withoutKeyring` in `pkg/config/keyring_test.go`.
- **When it applies**: Adding or moving any test that reaches `SetToken`, `GetToken`, or `MigrateTokensToKeyring`
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)

### The `sonar` job always fails and does not gate; `snyk` does gate
- **Context**: PR checks show three similarly named entries — `sonar`, `SonarCloud`, and `SonarCloud Code Analysis`. The `sonar` job fails on every run with "Not authorized or project not found".
- **Insight**: `sonar.yml` sets `continue-on-error: true` because SONAR_TOKEN lacks Execute Analysis permission, so the *job* shows failed while the *workflow run* succeeds — it does not block merges, and it never actually analyses the code. Real Sonar findings arrive via the `SonarCloud Code Analysis` app integration, which posts to the PR. By contrast `snyk` has no `continue-on-error` (removed in 8b31961) and genuinely gates, so even an infrastructure flake there blocks until re-run.
- **When it applies**: Triaging red checks on a dtmgd PR before assuming the code is at fault
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)
