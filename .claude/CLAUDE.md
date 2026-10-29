# CLAUDE.md

This file provides guidance to Claude Code when working in the `dtmgd` repository (Dynatrace Managed CLI, Go).

For `dtmgd` CLI usage patterns and command reference, see `skills/dtmgd/SKILL.md`.

## Knowledge Base

### GetV2Paged is a method, not a standalone generic function
- **Context**: Tried `GetV2Paged[T](c, path, params, pages)` standalone generic syntax — doesn't exist
- **Insight**: `GetV2Paged` is `(c *Client) GetV2Paged(path, params, client.Paging{MaxPages, Limit, ListKey}) (map[string]interface{}, error)` — returns raw map; use `DecodePaged(raw, &result)` for typed decode
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
- **Context**: resty client has `SetRetryCount(2)` + retry on 429/5xx with waits of 1–5s; using 500 in httptest adds seconds of retry waits
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
- **Insight**: The tests pass a `t.TempDir()` as `baseDir`, but `Status()` also consults `os.UserHomeDir()` (`pkg/skills/installer.go:327`), so the "fresh dir" is not fresh once you have run `dtmgd skills install`. CI runners have no skills installed, which is why every check is green. Use `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...` for a clean local signal, and do not "fix" it as part of unrelated work.
- **When it applies**: Any local full-suite run, especially before concluding that a change broke something
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)

### Keyring-touching tests must install a mock, or they eat real credentials
- **Context**: A test called `setCredentials("prod-token", ...)`, which reaches `config.SetToken` → `keyring.Set("dtmgd", "prod-token", ...)` whenever `IsKeyringAvailable()` is true. `prod-token` is the credential name used in the README quick start, so `go test ./cmd/...` silently overwrote a developer's live API token. Headless CI takes the other branch, so CI never surfaced it.
- **Insight**: `pkg/config` and `cmd` both have a `TestMain` calling `keyring.MockInit()`. Keep it that way, and never add a keyring-touching test to another package without one. Side effect: `IsKeyringAvailable()` then reports true for the whole binary, so the plaintext (no-keyring) branch needs `keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)` with a `t.Cleanup(keyring.MockInit)` restore — see `withoutKeyring` in `pkg/config/keyring_test.go`.
- **When it applies**: Adding or moving any test that reaches `SetToken`, `GetToken`, or `MigrateTokensToKeyring`
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)

### The `sonar` job always fails and does not gate; `snyk` does not gate either
- **Context**: PR checks show three similarly named entries — `sonar`, `SonarCloud`, and `SonarCloud Code Analysis`. The `sonar` job fails on every run with "Not authorized or project not found".
- **Insight**: `sonar.yml` sets `continue-on-error: true` because SONAR_TOKEN lacks Execute Analysis permission, so the *job* shows failed while the *workflow run* succeeds — it does not block merges, and it never actually analyses the code. Real Sonar findings arrive via the `SonarCloud Code Analysis` app integration, which posts to the PR. `snyk` is skipped with a `::notice::` when `SNYK_API_TOKEN` is unavailable (Dependabot and fork PRs cannot read repository secrets) and is **not** a required check. What gates a merge is the `main` ruleset's required checks: `test` on ubuntu, macOS and Windows, `lint`, `golangci-lint` and `vulncheck`.
- **When it applies**: Triaging red checks on a dtmgd PR before assuming the code is at fault
- **Date**: 2026-08-06 (snyk and required checks updated 2026-10-29)
- **Ref**: impl: resolve config env vars at point of use (#29); Phase 1/Phase 2 catch-up (#49, #50)

### Snyk, Dependabot and govulncheck resolve different dependency sets
- **Context**: Six Snyk-sourced Jira tickets (PRISM-14120..14125) reported `golang.org/x/crypto` v0.51.0 CVEs against dtmgd. Dependabot never alerted and govulncheck never mentioned the module, which looks like two scanners missing something. They did not.
- **Insight**: All three answer different questions and all three were right. `x/crypto` was a *phantom* module-graph entry contributed by `golang.org/x/net`'s go.mod: present in `go list -m all`, but with **no `go.sum` entry**, so no package of it was ever downloaded or linked. Snyk resolves the MVS module graph, so it reported it. GitHub's dependency graph is built from go.mod + go.sum, so it excluded it — and Dependabot cannot alert on a package it does not think you have. (Not an advisory-coverage gap: all six CVEs were in GitHub's Advisory Database from 2026-06-25, correctly attributed. The advisories were there; the package was not.) govulncheck is build-and-call-graph based, so it was silent too.
- **When it applies**: Triaging any Snyk Go finding before spending time on it. `grep '<module>' go.sum || echo "graph-only, not in the build"` settles reachability in one command; confirm with `go list -deps ./...`. Beware one trap there: `vendor/golang.org/x/crypto/...` entries are the Go toolchain's own internal copy inside `crypto/tls`, not your dependency.
- **Date**: 2026-08-06
- **Ref**: fix(deps): raise golang.org/x/crypto floor past the ssh CVEs (#32)

### Dependabot does not bump indirect Go deps on schedule — only via alerts, to the minimum version
- **Context**: Asked why Snyk filed vulnerability tickets when Dependabot runs weekly. Dependabot was working — 16 PRs — yet `x/net` sat at v0.55.0 for a month with v0.56.0 and v0.57.0 published, and `x/sys` at v0.45.0 with v0.47.0 published.
- **Insight**: Both stale modules are `// indirect`. Scheduled `gomod` version updates cover direct dependencies; every direct dep and GitHub Action got a prompt PR over the same period. An indirect dep moves only when a Dependabot **security alert** fires, and then only to the **minimum safe version** — PR #20 bumped `x/net` to exactly v0.55.0, the `first_patched_version` of GHSA-5cv4-jp36-h3mw, while v0.57.0 already existed. So the indirect floor drifts upward-never, and Snyk keeps reporting transitive CVEs for which no PR will ever arrive.
- **When it applies**: Any "why didn't Dependabot catch this?" question, and any Go transitive-CVE remediation here. Fix the cause by bumping the *real* dependency whose go.mod contributes the requirement — a direct `require` on a module no package imports is dropped again by the next `go mod tidy`. A scheduled `go get -u ./... && go mod tidy` PR is the only thing that keeps indirect deps current.
- **Date**: 2026-08-06
- **Ref**: fix(deps): raise golang.org/x/crypto floor past the ssh CVEs (#32)

### setup-go ignores go.mod's `toolchain` line when GOTOOLCHAIN=local is preset
- **Context**: With `GOTOOLCHAIN: local` set in a CI job's `env`, `actions/setup-go` v7 with `go-version-file: go.mod` installed the `go` directive's version instead of the one on the `toolchain` line, and nothing in the log said so — CI silently built and tested with an older Go than go.mod asked for.
- **Insight**: `parseGoVersionFile` (`src/installer.ts:652-662`) reads the `toolchain` line only when `GOTOOLCHAIN` is not `local`; otherwise it falls back to the `go` directive. Never preset `GOTOOLCHAIN` in a job: setup-go exports `GOTOOLCHAIN=local` itself once it has resolved the version, so later `go` commands cannot switch toolchains anyway. go.mod currently has no `toolchain` line (`go 1.26.10`), but the trap returns the moment one is added.
- **When it applies**: Any workflow using `actions/setup-go` with `go-version-file`, and any go.mod that has (or gains) a `toolchain` line
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### goreleaser's tap PR gets 403; open it by hand
- **Context**: The `homebrew_casks` step pushes the cask to branch `dtmgd-<version>` of the `ivan-gudak/homebrew-tap` fork, then opens a PR to `dynatrace-oss/homebrew-tap`. That PR call fails with 403 "Resource not accessible" with the current `HOMEBREW_TAP_GITHUB_TOKEN`, which can push only to the fork.
- **Insight**: The release itself publishes; only the tap PR is missing. Open it by hand: `gh pr create --repo dynatrace-oss/homebrew-tap --head ivan-gudak:dtmgd-<version> --base main` (`<version>` without the `v`, as in `.goreleaser.yaml`'s `{{ .ProjectName }}-{{ .Version }}` branch).
- **When it applies**: Every dtmgd release until the token can open PRs on `dynatrace-oss/homebrew-tap`
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### Release-asset upload "408 Upload body timed out" is a GitHub flake
- **Context**: A release job failed uploading assets with "408 Upload body timed out due to inactivity".
- **Insight**: GitHub-side, not a goreleaser or workflow bug. goreleaser leaves a draft release behind holding only `checksums.txt`; delete that draft, then re-run the job.
- **When it applies**: Any failed release job whose error is a 408 on an asset upload
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### CI tools come from the checksum-locked `tools/` module, never `go install …@version`
- **Context**: SonarCloud's `githubactions:S8545` ("Dependency versions are not predictable") flagged `go install golang.org/x/vuln/cmd/govulncheck@v1.9.0` in `security.yml` and `drift.yml`, which turned `main`'s quality gate red. The install was in fact pinned and verified against sum.golang.org, but the owner chose to remove the pattern rather than dismiss the finding.
- **Insight**: govulncheck is a `tool` directive in `tools/go.mod`, locked by `tools/go.sum`. Workflows build it with `go -C tools build -o "$RUNNER_TEMP/govulncheck" golang.org/x/vuln/cmd/govulncheck` and run the binary, not `go tool …`, so its exit code 3 still fails the job; `make vulncheck` does the same locally. To add a tool: `go -C tools get -tool <pkg>@<version>`. To bump one: `go -C tools get <module>@<version> && go -C tools mod tidy`; `lint.yml` fails if `tools/` is not tidy. `go get -tool` marks the module `// indirect`, which Dependabot skips on schedule (see the indirect-deps entry), so the `/tools` Dependabot entry sets `allow: dependency-type: all`. A newer tool may raise `tools/go.mod`'s `go` line above the root's; setup-go then fails the build (`GOTOOLCHAIN=local`), so bump the root's Go first. `go -C` changes directory before reading `-o`, so a relative `-o` path lands under `tools/`. The root module's `go list ./...` does not include `tools/`.
- **When it applies**: Adding or bumping a CI tool, or seeing `go install` reappear in a workflow
- **Date**: 2026-10-29
- **Ref**: Phase 3 (1/2) code health

### Command behaviour is tested end to end with the harness in cmd/harness_test.go
- **Context**: Phase 2 replaced `Execute()` with `Run(args, stdout, stderr) int` and builds the command tree per invocation (`newRootCmd(a *App)`), so tests can run the real CLI and see its exit code.
- **Insight**: `runCLI(t, cfgPath, args...)` and `runCLIWithEnv(t, env, cfgPath, args...)` run the CLI through `Run` against an httptest `fakeAPI` (`newFakeAPI` with a route table; `withFakeEnvs` writes a temp config pointing at it) and return stdout, stderr and the exit code. `assertGolden` compares with `cmd/testdata/golden/<name>.golden`; regenerate with `go test ./cmd/ -run <Test> -update` and read the diff — a golden change is a behaviour change. `runCLI` clears every agent-detection variable (`agentmode.EnvVars()`), so output is human unless a test opts in through `runCLIWithEnv` (e.g. `CLAUDECODE=1`).
- **When it applies**: Adding or changing any command, and before calling a behaviour change safe
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### `go tool cover -func` cannot see closures in package-level vars
- **Context**: `main` reported 75.1% total coverage while `cmd` was at 32.9%: every command's `RunE` was a closure in a package-level var (`var xCmd = &cobra.Command{RunE: func…}`).
- **Insight**: `go tool cover -func` totals count only statements inside named functions, so those closures were missing from the total altogether and it over-reported. The CI coverage gate (`test.yml`, threshold 60) uses that total. Commands are now constructors (`newXCmd(a *App)`), so their closures count; keep it that way, or the gate measures less code than it seems to.
- **When it applies**: Reading or gating on coverage numbers; adding code in closures outside named functions
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### Never point a test at the default config path
- **Context**: `~/.config/dtmgd/config` on a developer machine may hold live tokens for real clusters. A test that runs a command without `--config` (and without `DTMGD_CONFIG`) reads that file, and one that saves config would rewrite it.
- **Insight**: Tests pass `--config` with a temp file (`withFakeEnvs` writes one; `runCLI` appends `--config` unless `cfgPath` is `""`). `DTMGD_CONFIG` is tested at path-resolution level only (`TestDTMGDConfigSelectsTheConfigFile` checks `ResolveSource`, never runs a command against the file), so a regression cannot fall back to the real config.
- **When it applies**: Writing any test that runs a command or loads or saves config
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### SonarCloud automatic analysis reads `.sonarcloud.properties`, and S3776 still runs on tests
- **Context**: `sonar-project.properties` declared `sonar.tests`, yet SonarCloud kept analysing `_test.go` files as production code (qualifier `FIL`) and reporting cognitive-complexity smells on table-driven tests.
- **Insight**: SonarCloud's automatic analysis (the `SonarCloud Code Analysis` app) reads only `.sonarcloud.properties`; `sonar-project.properties` is for scanner-based analysis, which the `sonar` job cannot run. With `.sonarcloud.properties` in place, test files show qualifier `UTS` (check with `curl -s "https://sonarcloud.io/api/components/show?component=dynatrace-oss_dtmgd:<path>"`). Even so, Go's `go:S3776` still reports test functions above 15, so keep new tests under the limit with helpers. Locally, `gocognit` in `.golangci.yml` checks only non-test code; `go run github.com/uudashr/gocognit/cmd/gocognit@v1.2.0 -over 15 <file>_test.go` measures a test (its counts run a little higher than Sonar's, which counts `t.Run` closures separately).
- **When it applies**: Changing Sonar configuration, or a SonarCloud PR comment flags a test
- **Date**: 2026-10-29
- **Ref**: Phase 3 (1/2) code health (#52)

### `/securityProblems` has no `entitySelector`; scope with `securityProblemSelector`
- **Context**: The Dynatrace Managed MCP server's `list_security_problems` sends a top-level `entitySelector`, and a `--entity` flag for `get security-problems` looked like a parity gap.
- **Insight**: `GET /api/v2/securityProblems` documents only `nextPageKey`, `pageSize`, `securityProblemSelector`, `sort`, `fields`, `from` and `to` (`env2temp.json`). Entity scoping goes through selector criteria: `relatedHostIds("HOST-…")`, `affectedPgIds("PROCESS_GROUP-…")`, `managementZoneIds("…")`, `relatedKubernetesClusterIds(…)`, and so on. So dtmgd adds no `--entity` there; `get security-problems --help` shows the `--selector` forms. Without `from` the API returns the last 30 days.
- **When it applies**: Adding filters to security-problem commands, or comparing with the MCP server
- **Date**: 2026-10-29
- **Ref**: Phase 3 (2/2) MCP parity

### `/logs/search` returns no `totalCount` and does not document `entitySelector`
- **Context**: Phase 3 planned to verify `query logs --entity` by comparing `jq .totalCount` with and without a nonexistent-host selector on a live environment.
- **Insight**: `/logs/search` returns `{results, sliceSize, nextSliceKey}` with no `totalCount`, and its documented parameters are only `from`, `to`, `limit`, `query`, `sort` and `nextSliceKey`. Count `.results | length` instead. The 2026-10-29 check on the owner's `stage` environment was inconclusive: `--query error` matched 0 records over both 1 hour and 24 hours, with and without `--entity`. `--entity` was therefore kept, and its help says the parameter is undocumented. To settle it, repeat the comparison against a query and time range that return records. `sort` takes only `+timestamp` and `-timestamp`; dtmgd now always sends `-timestamp` unless `--sort` says otherwise.
- **When it applies**: Changing `query logs`, or deciding whether `--entity` can be trusted
- **Date**: 2026-10-29
- **Ref**: Phase 3 (2/2) MCP parity

### Change the keyring only after the config save succeeds
- **Context**: `config delete-credentials`, `delete-context --delete-credentials` and `config import` (when a `${VAR}` reference replaces a keyring token) deleted the keyring entry first and saved the config afterwards. When the save failed (for example, a read-only config), the token was gone while the config still pointed at it.
- **Insight**: `Config.DeleteToken` only edits the in-memory config and returns `deleteFromKeyring func() error`; callers save first and run it only after the save succeeds. Tests reproduce a failing save with `readOnlyConfig` (`cmd/config_housekeeping_test.go`), which makes the file and its directory read-only. That defeats `writeFileAtomic` and its in-place fallback alike; skip on Windows and as root.
- **When it applies**: Any command that changes both the keyring and the config file
- **Date**: 2026-10-29
- **Ref**: Phase 3 (2/2) MCP parity

### Parameters the MCP server and dtmgd send that the API does not document
- **Context**: Comparing the Dynatrace Managed MCP server (2.0.0) with dtmgd request by request for the Phase 3 parity table, against `env2temp.json`.
- **Insight**:
  - The MCP server alone sends top-level `impactLevel` on `/problems`, and `riskLevel` and `entitySelector` on `/securityProblems`. dtmgd uses `problemSelector` / `securityProblemSelector` criteria instead.
  - Both send `entitySelector` on `/metrics` (`get metrics --entity`), `eventType` on `/events` (`get events --type`; the spec documents `eventSelector=eventType("…")`) and `mzSelector` on `/entities` (`get entities --mz`).
  - dtmgd alone sends `entitySelector` on `/logs/search` (`query logs --entity`) and `fields=+affectedEntities,+managementZones` on `/problems`.
  - Whether a cluster honours any of these is unverified. Before trusting one, compare a filtered and an unfiltered call on a live environment, choosing a query and time range that return results. The parity table in README and SKILL.md marks them "undocumented".
- **When it applies**: Adding or debugging a filter flag; deciding whether dtmgd's own undocumented parameters should move to documented selectors
- **Date**: 2026-10-29
- **Ref**: Phase 3 (2/2) MCP parity
