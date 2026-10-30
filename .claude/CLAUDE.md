# CLAUDE.md

@../AGENTS.md

The knowledge base below records lessons learned while working on dtmgd: each
entry's context, insight and when it applies. Read the entries that match
your task.

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
- **Insight**: The tests pass a `t.TempDir()` as `baseDir`, but `Status()` in `pkg/skills/installer.go` also consults `os.UserHomeDir()`, so the "fresh dir" is not fresh once you have run `dtmgd skills install`. CI runners have no skills installed, which is why every check is green. Use `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... ./pkg/version/... ./skills/... .` for a clean local signal, and do not "fix" it as part of unrelated work.
- **When it applies**: Any local full-suite run, especially before concluding that a change broke something
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)

### Keyring-touching tests must install a mock, or they eat real credentials
- **Context**: A test called `setCredentials("prod-token", ...)`, which reaches `config.SetToken` → `keyring.Set("dtmgd", "prod-token", ...)` whenever `IsKeyringAvailable()` is true. `prod-token` is the credential name used in the README quick start, so `go test ./cmd/...` silently overwrote a developer's live API token. Headless CI takes the other branch, so CI never surfaced it.
- **Insight**: `pkg/config` and `cmd` both have a `TestMain` calling `keyring.MockInit()`. Keep it that way, and never add a keyring-touching test to another package without one. Side effect: `IsKeyringAvailable()` then reports true for the whole binary, so the plaintext (no-keyring) branch needs `keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)` with a `t.Cleanup(keyring.MockInit)` restore — see `withoutKeyring` in `pkg/config/keyring_test.go`.
- **When it applies**: Adding or moving any test that reaches `SetToken`, `GetToken`, or `MigrateTokensToKeyring`
- **Date**: 2026-08-06
- **Ref**: impl: resolve config env vars at point of use (#29)

### SonarCloud findings come only from the app, and neither SonarCloud nor `snyk` gates
- **Context**: PR checks show `SonarCloud` and `SonarCloud Code Analysis`. Until Phase 5 there was also a `sonar` job (`sonar.yml`) that failed on every run with "Not authorized or project not found": SONAR_TOKEN lacked Execute Analysis permission, so it never analysed anything, and `continue-on-error` kept it from blocking. It re-ran the whole test suite for nothing; Phase 5 deleted it together with `sonar-project.properties`.
- **Insight**: Sonar findings arrive only through the `SonarCloud Code Analysis` app integration, which reads `.sonarcloud.properties` and posts to the PR. Its quality gate does not block a merge, so check it on every PR (`api/qualitygates/project_status?projectKey=dynatrace-oss_dtmgd&pullRequest=<n>`) before calling the PR done; #59 merged with a red gate. `snyk` is skipped with a `::notice::` when `SNYK_API_TOKEN` is unavailable (Dependabot and fork PRs cannot read repository secrets), and it is not required. What gates a merge is the `main` ruleset's required checks: `test` on ubuntu, macOS and Windows, `lint`, `golangci-lint`, `vulncheck` and (since 2026-10-30) `goreleaser-check`, which reports a config error or deprecation.
- **When it applies**: Triaging checks on a dtmgd PR, or deciding whether a PR is ready
- **Date**: 2026-08-06 (sonar job removed and entry rewritten 2026-10-30)
- **Ref**: impl: resolve config env vars at point of use (#29); Phase 5 (#59)

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
- **Insight**: govulncheck and actionlint are `tool` directives in `tools/go.mod`, locked by `tools/go.sum`. Workflows build it with `go -C tools build -o "$RUNNER_TEMP/govulncheck" golang.org/x/vuln/cmd/govulncheck` and run the binary, not `go tool …`, so its exit code 3 still fails the job; `make vulncheck` does the same locally. To add a tool: `go -C tools get -tool <pkg>@<version>`. To bump one: `go -C tools get <module>@<version> && go -C tools mod tidy`; `lint.yml` fails if `tools/` is not tidy. `go get -tool` marks the module `// indirect`, which Dependabot skips on schedule (see the indirect-deps entry), so the `/tools` Dependabot entry sets `allow: dependency-type: all`. A newer tool may raise `tools/go.mod`'s `go` line above the root's; setup-go then fails the build (`GOTOOLCHAIN=local`), so bump the root's Go first. `go -C` changes directory before reading `-o`, so a relative `-o` path lands under `tools/`. The root module's `go list ./...` does not include `tools/`. golangci-lint and GoReleaser are not in `tools/` (both advise against building them from source): they are pinned by version as action inputs (`sast.yml`; `release.yml`, `lint.yml`, `build.yml`), each pin carrying a `# keep in step with …` comment, and the Makefile's `GOLANGCI_LINT_VERSION` names the golangci-lint version `make lint` expects (it only prints it in the install hint; it does not check the installed version). Dependabot cannot see those pins, so `scripts/drift-check.sh` compares them with the latest releases and reports a pin without its comment as an error.
- **When it applies**: Adding or bumping a CI tool, or seeing `go install` reappear in a workflow
- **Date**: 2026-10-29 (actionlint and the pinned action inputs added 2026-10-30)
- **Ref**: Phase 3 (1/2) code health; Phase 5 (#59)

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
- **Context**: The global config on a developer machine (`~/.config/dtmgd/config` on Linux, `~/Library/Application Support/dtmgd/config` on macOS, `%LOCALAPPDATA%\dtmgd\config` on Windows, or under `$XDG_CONFIG_HOME`) may hold live tokens for real clusters. A test that runs a command without `--config` (and without `DTMGD_CONFIG`) reads that file, and one that saves config would rewrite it.
- **Insight**: Tests pass `--config` with a temp file (`withFakeEnvs` writes one; `runCLI` appends `--config` unless `cfgPath` is `""`). `DTMGD_CONFIG` is tested at path-resolution level only (`TestDTMGDConfigSelectsTheConfigFile` checks `ResolveSource`, never runs a command against the file), so a regression cannot fall back to the real config.
- **When it applies**: Writing any test that runs a command or loads or saves config
- **Date**: 2026-10-29
- **Ref**: Phase 1/Phase 2 catch-up (#49, #50)

### SonarCloud automatic analysis reads `.sonarcloud.properties`, and S3776 still runs on tests
- **Context**: `sonar-project.properties` declared `sonar.tests`, yet SonarCloud kept analysing `_test.go` files as production code (qualifier `FIL`) and reporting cognitive-complexity smells on table-driven tests.
- **Insight**: SonarCloud's automatic analysis (the `SonarCloud Code Analysis` app) reads only `.sonarcloud.properties`; `sonar-project.properties` was for scanner-based analysis, which the old `sonar` job could never run (both were removed in Phase 5). With `.sonarcloud.properties` in place, test files show qualifier `UTS` (check with `curl -s "https://sonarcloud.io/api/components/show?component=dynatrace-oss_dtmgd:<path>"`). Even so, Go's `go:S3776` still reports test functions above 15, so keep new tests under the limit with helpers. Locally, `gocognit` in `.golangci.yml` checks only non-test code; `go run github.com/uudashr/gocognit/cmd/gocognit@v1.2.0 -over 15 <file>_test.go` measures a test (its counts run a little higher than Sonar's, which counts `t.Run` closures separately).
- **When it applies**: Changing Sonar configuration, or a SonarCloud PR comment flags a test
- **Date**: 2026-10-29
- **Ref**: Phase 3 (1/2) code health (#52)

### `/securityProblems` has no `entitySelector`; scope with `securityProblemSelector`
- **Context**: The Dynatrace Managed MCP server's `list_security_problems` sends a top-level `entitySelector`, and a `--entity` flag for `get security-problems` looked like a parity gap.
- **Insight**: `GET /api/v2/securityProblems` documents only `nextPageKey`, `pageSize`, `securityProblemSelector`, `sort`, `fields`, `from` and `to` (`env2temp.json`). Entity scoping goes through selector criteria: `relatedHostIds("HOST-…")`, `affectedPgIds("PROCESS_GROUP-…")`, `managementZoneIds("…")`, `relatedKubernetesClusterIds(…)`, and so on. So dtmgd adds no `--entity` there; `get security-problems --help` shows the `--selector` forms. Without `from` the API returns the last 30 days.
- **When it applies**: Adding filters to security-problem commands, or comparing with the MCP server
- **Date**: 2026-10-29
- **Ref**: Phase 3 (2/2) MCP parity

### `/logs/search` returns no `totalCount`; its undocumented `entitySelector` is honoured
- **Context**: Phase 3 planned to verify `query logs --entity` by comparing counts with and without a nonexistent-host selector on a live environment. The 2026-10-29 attempt on `stage` was inconclusive, because no query matched any records there.
- **Insight**:
  - `/logs/search` returns `{results, sliceSize, nextSliceKey}` with no `totalCount`, so count `.results | length`.
  - Its documented parameters are only `from`, `to`, `limit`, `query`, `sort` and `nextSliceKey`. Even so, `entitySelector` filters. On 2026-10-30, on `prod` (Dynatrace Managed 1.348), `--query exception --from now-24h` matched 167 records, 68 with `--entity 'type(PROCESS_GROUP)'`, and 0 with an impossible selector (a nonexistent entity ID or name).
  - `sort` takes only `+timestamp` and `-timestamp`; dtmgd always sends `-timestamp` unless `--sort` says otherwise.
  - To repeat a check, first find a query that returns records: `stage` had none for `error`, `info`, `warn`, `GET` or `exception`.
- **When it applies**: Changing `query logs`, or deciding whether `--entity` can be trusted
- **Date**: 2026-10-29 (verified live 2026-10-30)
- **Ref**: Phase 3 (2/2) MCP parity; filter fixes after v0.3.1

### Change the keyring only after the config save succeeds
- **Context**: `config delete-credentials`, `delete-context --delete-credentials` and `config import` (when a `${VAR}` reference replaces a keyring token) deleted the keyring entry first and saved the config afterwards. When the save failed (for example, a read-only config), the token was gone while the config still pointed at it.
- **Insight**: `Config.DeleteToken` only edits the in-memory config and returns `deleteFromKeyring func() error`; callers save first and run it only after the save succeeds. Writes go the other way round: `Config.SetTokenUndoable` stores the token in the keyring at once and returns `undo`, which puts the previous keyring value back (or removes the entry); `set-credentials` and `config import` run it when the save fails (v0.2.1). Without it, a failed `import --overwrite` left an old context pointing at its old host with the new host's token. Tests reproduce a failing save with `readOnlyConfig` (`cmd/config_housekeeping_test.go`), which makes the file and its directory read-only. That defeats `writeFileAtomic` and its in-place fallback alike; skip on Windows and as root.
- **When it applies**: Any command that changes both the keyring and the config file
- **Date**: 2026-10-29
- **Ref**: Phase 3 (2/2) MCP parity (#53); v0.2.1

### Undocumented filter parameters: three are ignored by the cluster, one is honoured
- **Context**: The Phase 3 parity review found parameters that the Dynatrace Managed MCP server (2.0.0) and dtmgd send but `env2temp.json` does not document. On 2026-10-30 each was checked live on `prod` and `stage` (Dynatrace Managed 1.348). The check uses a value that cannot match anything: a filter that is honoured returns nothing (or a 400), and an ignored one returns the unfiltered count.
- **Insight**:
  - `eventType` on `/events` is **ignored**: 864 events with and without `eventType=ZZ_NOT_A_TYPE`. `eventSelector=eventType("PROCESS_RESTART")` filters (305 of 864), and an unknown type there gets 400 "Constraints violated".
  - `mzSelector` on `/entities` is **ignored**, even with a real zone (38 of 38 hosts). `mzName("<zone>")` inside `entitySelector` filters (18 of 38); a nonexistent zone name there gets 400 "Constraints violated".
  - `entitySelector` on `/metrics` (the descriptor list) is **ignored**: 17 = 17. The endpoint has no documented entity filter at all.
  - `entitySelector` on `/logs/search` is **honoured** (see the logs entry).
  - Since the release after v0.3.1, dtmgd sends `get events --type` as `eventSelector=eventType("…")`, adds `get entities --mz` to `entitySelector` as `mzName("…")` (quote and tilde escaped with `~`; an `mzName(…)`/`mzId(…)` value passes through), and refuses `get metrics --entity` with exit 2, pointing at `query metrics --entity`. `cmd/filters_test.go` pins the parameters sent.
  - The MCP server still sends the ignored parameters. It also sends top-level `impactLevel` on `/problems`, and `riskLevel` and `entitySelector` on `/securityProblems`, which dtmgd covers with `problemSelector` / `securityProblemSelector` criteria (reported in dynatrace-oss/dynatrace-managed-mcp#329). dtmgd's `fields=+affectedEntities,+managementZones` on `/problems` is unchecked and harmless: both fields are in the default response.
- **When it applies**: Adding or debugging a filter flag. Before trusting any undocumented parameter, compare an impossible value with no filter on a live environment, choosing a query and time range that return results.
- **Date**: 2026-10-29 (checked live 2026-10-30)
- **Ref**: Phase 3 (2/2) MCP parity; filter fixes after v0.3.1

### A cluster context's host is the cluster's own address, not an environment context's node host
- **Context**: Phase 4 planned the Cluster API commands on top of the owner's existing environment contexts. Their hosts are node addresses on port 9999 (`https://<node>:9999`), where `/api/cluster/v2/...` and `/api/v1.0/onpremise/...` answer 404. The Cluster API answers on the cluster's public name, the address the Cluster Management Console uses.
- **Insight**: A cluster context (no `env-id`) carries its own host, and dtmgd never pairs contexts by host. Every cluster command returning 404 means a wrong host; `runClusterFetch` says so (`hintNoClusterAPI`). Its token must be a Cluster API token with `ServiceProviderAPI`, created in the CMC. An environment token gets **401** ("Token Authentication failed"), checked live on 2026-10-29. That is why the 401 hint says an environment token cannot work.
- **When it applies**: Setting up or debugging a cluster context; adding a cluster command
- **Date**: 2026-10-29
- **Ref**: Phase 4 Cluster API

### `/api/v1.0/onpremise/cluster` returns an array of nodes
- **Context**: `ClusterVersion()` decoded `/api/v1.0/onpremise/cluster` as `{"version": …}`. On a real Cluster API host that fails with "cannot unmarshal array", and the doctor tests faked the object shape, so nothing caught it. Environment contexts never reached it: their :9999 hosts answer 404 there, and the fallback to `/e/<id>/api/v1/config/clusterversion` hid the bug.
- **Insight**: The endpoint returns `[{"id", "buildVersion", "operationState", "clusterMemberAddress", "osInfo", …}]`, one entry per node. The cluster's version is the **lowest** node's `buildVersion` (`client.LowestClusterVersion`), because nodes differ mid-upgrade. Fakes use `nodeList(...)` in `cmd/doctor_test.go` or `cmd/testdata/api/cluster_nodes.json`. A cluster-scoped client never falls back to the environment endpoint.
- **When it applies**: Anything that reads the cluster version; writing fakes for the Cluster API
- **Date**: 2026-10-29
- **Ref**: Phase 4 Cluster API

### Cluster API: shapes, paging and licensing as observed live
- **Context**: The Phase 4 spike on 2026-10-29 read every endpoint dtmgd uses on the owner's prod cluster (1.348, classic licensing), printing shapes only. The stage cluster's public name was unreachable that day.
- **Insight**:
  - `/api/cluster/v2/environments` pages like the environment API: a later page is asked for with `nextPageKey` alone, and the last page has `nextPageKey: null`. `quotas` and `storage` are `null` unless `includeConsumptionInfo` / `includeStorageInfo` are set, and an unlimited limit is JSON `null`.
  - A missing environment gives 404 "HTTP 404 Not Found".
  - `clusterLicense` and `/environment/total` work only with classic licensing. The prod cluster reported `licenseType FULL`, `licenseStatus VALID`, and no `overageUsage`. What a DPS cluster returns was **not observed**: the license commands give the classic-licensing hint for any client error but 401/403/429. `--consumption`'s 422 on DPS comes from the OpenAPI spec.
  - `/cluster/configuration` is `{"clusterNodes": [{id, ipAddress, webUI, agent, datacenter, kubernetesRole}]}`, and `/cluster/metadata` is `{clusterName, clusterUuid}`. `/backup/config` matches `backupConfigDto`. There is no backup health anywhere.
  - `clusterLicense` returns the full `licenseKey`. `describe license` masks it inside Fetch, so every output format shows `****` plus 4 characters, and the `-vv` body dump redacts it too (`client.redactBody`). Add any new secret field there.
- **When it applies**: Changing a cluster command, or meeting a DPS-licensed cluster for the first time (check what it really returns, then tighten the hint)
- **Date**: 2026-10-29
- **Ref**: Phase 4 Cluster API

### `ALL_ENVIRONMENTS` excludes cluster contexts; context kind is the raw `env-id`
- **Context**: Once a config holds cluster contexts (no `env-id`), `--env ALL_ENVIRONMENTS` fanned out to them too, and they failed with "env-id is required". The owner's config already had two.
- **Insight**: `config.Context.IsCluster()` is `EnvID == ""` as written in the file, before `${VAR}` expansion, so `env-id: ${DT_ENV_ID}` is an environment context even while the variable is unset. `client.resolveContexts` gives `ALL_ENVIRONMENTS` environment contexts only. A cluster context named in a `;` list is an error. `runFetch` (`checkEnvironmentContexts`) and `runClusterFetch` (`checkClusterContext`) refuse the wrong kind with exit 2 before any request. `client.New` with an empty env-id builds a cluster-scoped client: its environment calls return `ErrClusterScoped`, and `DashboardURL()` is `""`, so no UI link becomes `{host}/e/`.
- **When it applies**: Adding a command (pick `runFetch` or `runClusterFetch`), or touching context selection
- **Date**: 2026-10-29
- **Ref**: Phase 4 Cluster API

### Dependabot's `/tools` group can break a tool by raising its pre-release dependency
- **Context**: The first grouped tools PR (#60) raised `go.yaml.in/yaml/v4` from `v4.0.0-rc.3` to `rc.6` along with five other modules. actionlint v1.7.12 stopped building ("undefined: yaml.ParserError"), so the `lint` job, which builds actionlint before running it, failed on `main`. #60 had been merged with its own `lint` check red.
- **Insight**: The `/tools` entry sets `allow: dependency-type: all`, so Dependabot raises the *indirect* dependencies of the tools too, past what a tool supports. The tools module's own `require` then wins under minimum version selection. A pre-release such as `v4.0.0-rc.N` may change its API between candidates. The fix:
  1. Find the version the tool requires: `go -C tools mod graph | grep <module>`.
  2. Pin it with `go -C tools get <module>@<that version> && go -C tools mod tidy`.
  3. Add an `ignore` for the module to the `/tools` Dependabot entry, with the reason. A newer tool raises the module itself once it supports the newer version.
- **When it applies**: Reviewing a Dependabot tools PR (don't merge one whose `lint` is red: that job is the only one that builds actionlint), or when `lint` fails while building a tool
- **Date**: 2026-10-30
- **Ref**: #60, #63
