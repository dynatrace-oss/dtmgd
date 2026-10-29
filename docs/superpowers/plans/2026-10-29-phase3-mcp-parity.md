# Phase 3 — Code Health and MCP Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clear SonarCloud's red gate and the code-health debt it reports, then bring dtmgd to honest parameter-level parity with the Dynatrace Managed MCP server. Ships as v0.2.0.

**Architecture:**
- govulncheck moves into a separate, checksum-locked `tools/` module. This removes the S8545 "vulnerability" without anyone dismissing it by hand.
- Tests are declared as tests to SonarCloud's automatic analysis.
- Production functions are kept under Sonar's cognitive-complexity limit, enforced locally by golangci-lint's `gocognit`.
- Parity work extends the existing `fetchSpec`/`runFetch` commands with flags and validation, and adds `config import` for MCP configs plus UI links in the agent envelope.

**Tech Stack:** Go 1.26.10, Cobra, resty v2, `dtctl/sdk/agentmode`, golangci-lint v2.15 (`gocognit`), GitHub Actions, SonarCloud automatic analysis.

**Spec:** `docs/superpowers/specs/2026-10-29-dtmgd-catch-up-design.md` — Phase 3 section, Goals, Anti-goals, Constraints. Owner decisions on 2026-10-29: govulncheck goes in a locked tools module (option 2), and the cheap Sonar fixes are folded into Phase 3.

## Global Constraints

- **Two PRs, in this order.**
  - PR A (`chore/phase3-code-health`, Tasks 1–3) merges first, so `main`'s Sonar gate turns green early.
  - PR B (`feat/phase3-mcp-parity`, Tasks 4–12) is branched from `main` after PR A merges, and is released as **v0.2.0**.
  - Phase 4 then releases as v0.3.0 rather than the spec's v0.2.0, because Phase 3 adds new validation errors and so needs a minor bump first.
- **Outward actions.** Get the owner's go-ahead at the time of every push, PR, merge request and tag. PRs are opened with `gh`, as the owner.
- **Commit trailer.** Every commit ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. PR bodies end with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- **Tests.**
  - Local suite: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...`. Never `./pkg/skills`: it fails locally for a known, unrelated reason.
  - HTTP error tests use 400, 401, 403 or 404, never 5xx.
  - Command tests go through the harness (`cmd/harness_test.go`: `runCLI`, `runCLIWithEnv`, `newFakeAPI`, `withFakeEnvs`).
  - Goldens are regenerated with `-update` and every change is named in its commit.
- **Tokens and live clusters.**
  - Never read token values from `~/.config/dtmgd/config`; filter out `token:` lines.
  - Tests never use the default config path.
  - The only live-cluster call this plan allows is Task 8's read-only check against the owner's `stage` context, run through dtmgd with the owner's go-ahead.
- **Output shape stays Phase 2's.** json, yaml and agent output carry the API's own response shape, multi-environment results are a map keyed by context name, human notes go to stderr, and exit codes are 0–5.
- **Enum values**, all verified against `../docs/managed/_data/openapi/env2temp.json`. Input is matched case-insensitively and sent upper-cased, except SLO `--sort` and `--enabled`, which are sent as written in the table.

  | Flag | Allowed values |
  |---|---|
  | problem `--status` | `OPEN`, `CLOSED` |
  | problem `--impact` | `APPLICATION`, `ENVIRONMENT`, `INFRASTRUCTURE`, `SERVICES` |
  | security `--status` | `OPEN`, `RESOLVED` |
  | security `--risk` | `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `NONE` |
  | SLO `--timeframe` | `CURRENT`, `GTF` |
  | SLO `--enabled` | `true`, `false`, `all` |
  | SLO `--sort` | `name`, `-name` |

- **Never fake a filter.** dtmgd adds no `--entity` to `get security-problems`, because `/securityProblems` has no `entitySelector` parameter. The MCP server sends one that the API does not document. Entity scoping uses `--selector` criteria instead (`relatedHostIds(...)`, `affectedPgIds(...)`, `managementZoneIds(...)`).

## Review Focus

1. **A config with a duplicate context name stops loading.** The error must name the file, the duplicated name, and what to do, because no dtmgd command can repair the file. Owner: Task 7, `TestDuplicateContextNameIsRejected`.
2. **Lower-case enum input** (`--status open`, `--risk critical`) is accepted and sent upper-cased, not rejected. Owner: Task 4, `TestEnumFlagsAreCaseInsensitive`.
3. **`config import` of an MCP config whose tokens are `${VAR}` references** keeps the reference in the dtmgd config. It never expands the reference and never writes the secret value to the keyring or the file. Owner: Task 9, `TestImportKeepsTokenReferences`.
4. **`get slos --show-global=false` and `--demo=false`** are actually sent; the MCP server drops falsy values. An unset flag sends nothing. Owner: Task 6, `TestSlosBooleanFlagsSentOnlyWhenSet`.
5. **UI links in a multi-environment call** are per environment or absent, never one environment's link for all of them. Owner: Task 10, `TestAgentLinksPerEnvironment`.

---

## PR A — code health (Tasks 1–3)

### Task 1: govulncheck from a checksum-locked tools module

**Files:**
- Create: `tools/go.mod`, `tools/go.sum`, `tools/README.md` (3 lines: what it is and how to run it)
- Modify: `.github/workflows/security.yml`, `.github/workflows/drift.yml`, `.github/workflows/lint.yml`, `.github/workflows/snyk.yml`, `.github/dependabot.yml`, `scripts/drift-check.sh` (header comment only), `Makefile` (add a `vulncheck` target)

**Interfaces:**
- `tools/go.mod`:
  - `module github.com/dynatrace-oss/dtmgd/tools`
  - `go 1.26.10`
  - `tool golang.org/x/vuln/cmd/govulncheck`, with the `golang.org/x/vuln v1.9.0` requirement.
  - Create it with `go -C tools mod init …` and `go -C tools get -tool golang.org/x/vuln/cmd/govulncheck@v1.9.0`.
- How workflows run govulncheck: build the binary from the locked module, then run it. Don't use `go tool …`, so govulncheck's exit code 3 reaches the caller.

  ```bash
  go -C tools build -o "$RUNNER_TEMP/govulncheck" golang.org/x/vuln/cmd/govulncheck
  "$RUNNER_TEMP/govulncheck" ./...
  ```

  `drift.yml` passes `GOVULNCHECK="$RUNNER_TEMP/govulncheck"` to the script.
- `Makefile` target `vulncheck`: the same two commands, with `$(TMPDIR)` or `./bin` as the output directory.
- `dependabot.yml`: add a `gomod` entry with `directory: /tools`, weekly, so govulncheck stays current.
- `snyk.yml` args: add `--exclude=tools`, because the tool's dependencies are not part of dtmgd.
- `lint.yml`: also check that `go -C tools mod tidy` leaves no diff in `tools/go.mod` or `tools/go.sum`.

- [ ] **Step 1: Check the failure SonarCloud sees.**

Run:

```bash
grep -n "go install golang.org/x/vuln" .github/workflows/*.yml
```

Expected: 2 matches (`security.yml:22`, `drift.yml:29`).

- [ ] **Step 2: Create the tools module and switch every workflow and the Makefile to the build-then-run form.**

- [ ] **Step 3: Verify locally.**

Run:

```bash
go -C tools build -o /tmp/claude-502/-workspace-dtmgd/879c970e-7206-4485-81c4-1f96759bf4c3/scratchpad/govulncheck golang.org/x/vuln/cmd/govulncheck \
  && /tmp/claude-502/-workspace-dtmgd/879c970e-7206-4485-81c4-1f96759bf4c3/scratchpad/govulncheck ./...
grep -n "go install golang.org/x/vuln" .github/workflows/*.yml
go list ./... | grep -c /tools
go mod tidy && git diff --exit-code go.mod go.sum
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml
```

Expected:
- `No vulnerabilities found.`
- no `go install` match
- `0` (the root module's packages don't include `tools`)
- the root tidy check passes
- actionlint is clean

- [ ] **Step 4: Commit.** Message: `ci: run govulncheck from a checksum-locked tools module`

---

### Task 2: SonarCloud classifies tests as tests; clear the cheap code smells

**Files:**
- Create: `.sonarcloud.properties`. SonarCloud's automatic analysis reads this file; it ignores `sonar-project.properties`.
- Modify:
  - `pkg/config/config.go` (`writeFileAtomic`: rename `real` to `resolved`)
  - `pkg/client/client.go` (`quietLogger` methods: add a comment inside each empty body)
  - `cmd/scopes.go` (add scope constants)
  - every `withScopes(...)` call site (use the constants)
  - `cmd/query_metrics.go` (add `const entityDimensionPrefix = "dt.entity."`)
  - `cmd/describe_problem.go` and `cmd/describe_problem_test.go` (replace the nested anonymous structs with named types)

**Interfaces:**
- `.sonarcloud.properties`:
  ```
  sonar.sources=.
  sonar.exclusions=**/*_test.go,**/testdata/**,tools/**
  sonar.tests=.
  sonar.test.inclusions=**/*_test.go
  ```
- `cmd/scopes.go` constants: `scopeProblemsRead = "problems.read"`, `scopeEntitiesRead`, `scopeEventsRead`, `scopeMetricsRead`, `scopeLogsRead`, `scopeSLORead`, `scopeSecurityProblemsRead`, `scopeDataExport = "DataExport"`.
- `cmd/describe_problem.go` named types: `problemEntityRef`, `problemImpact`, `problemEvidence`, … Name them after their fields. JSON tags must not change.

- [ ] **Step 1: Make the edits.** All are behaviour-preserving.

- [ ] **Step 2: Verify.**

Run:

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...
git status --short cmd/testdata
```

Expected: build, vet and gofmt clean; tests `ok`; goldens unchanged.

- [ ] **Step 3: Commit.** Message: `chore: declare tests to SonarCloud and clear its cheap code smells`

---

### Task 3: Keep production functions under the cognitive-complexity limit

**Files:**
- Modify: `.golangci.yml`, enabling `gocognit` with `min-complexity: 15` and excluding `_test.go` files.
- Refactor, behaviour-preserving:

  | Function | Complexity now |
  |---|---|
  | `pkg/output/table.go` `PrintList` | 28 |
  | `pkg/client/multi.go` `MultiRequest` | 24 |
  | `cmd/root.go` `rewriteNegativeArgs` | 24 |
  | `cmd/get_problems.go` `newGetProblemsCmd` | 24 |
  | `pkg/client/client.go` `SetVerbosity` | 22 |
  | `cmd/describe_problem.go` `printProblemDetail` | 20 |
  | `cmd/runner.go` `renderMulti` | 18 |
  | `cmd/query_metrics.go` (the two flagged functions) | 17, 16 |
  | `cmd/get_problems.go` `problemRows` | 16 |

  Also refactor anything else `gocognit` reports above 15 in non-test code.

**Interfaces:**
- Extracted helpers are unexported and live next to their caller. Name them after what they compute, for example `problemSelectorParams(opts) map[string]string`, `rootCauseLabel(*rootCauseEntity) string`, `tableColumns(t reflect.Type, wide bool, columns []string) ([]string, []int)`, `envFailures(results) ([]EnvResult, …)`.
- No exported signature changes.

- [ ] **Step 1: Show the failure.**

Run:

```bash
/tmp/claude-502/-workspace-dtmgd/879c970e-7206-4485-81c4-1f96759bf4c3/scratchpad/golangci-lint run
```

Expected: `gocognit` issues for the functions listed above. Rebuild the binary with `GOBIN=… go install …golangci-lint@v2.15.0` if it is missing.

- [ ] **Step 2: Refactor each function until it is ≤ 15.** Keep the tests and goldens untouched.

- [ ] **Step 3: Verify.**

Run:

```bash
golangci-lint run
go test -race ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...
git status --short cmd/testdata
```

Expected: `0 issues.`; tests `ok`; goldens unchanged.

- [ ] **Step 4: Commit, then push PR A with the owner's go-ahead.**
  - Commit message: `refactor: keep production functions under cognitive complexity 15 (gocognit)`.
  - PR title: `Phase 3 (1/2): govulncheck from a locked tools module; SonarCloud code health`.
  - After CI runs, check the SonarCloud PR issues API:

    ```bash
    curl -s 'https://sonarcloud.io/api/issues/search?componentKeys=dynatrace-oss_dtmgd&pullRequest=<n>&resolved=false'
    ```

    Expected: no `githubactions:S8545`, and no `go:S3776` in `_test.go` files.
  - If S8545 still appears, report it to the owner, who can mark it a false positive. Don't change the approach again.

---

## PR B — MCP parity (Tasks 4–12)

### Task 4: Validate enum flags and `--limit`

**Files:**
- Create: `cmd/validate.go`, `cmd/validate_test.go`
- Modify: `cmd/get_problems.go`, `cmd/get_security.go`, `cmd/get_slos.go`, and every command with `--limit`

**Interfaces:**
- Produces:
  - `func enumFlag(flag, value string, allowed []string, upper bool) (string, error)`.
    - `""` returns `""` with no error.
    - A match ignores case and returns the canonical form: upper-cased when `upper`, otherwise the allowed spelling.
    - A miss returns `&UsageError{Err: fmt.Errorf("invalid --%s %q (want one of: %s)", flag, value, strings.Join(allowed, ", "))}`.
  - `func limitFlag(n int) error`: a negative value returns `&UsageError{Err: errors.New("--limit must be 0 (no limit) or positive")}`.
- Help text:
  - security `--status` lists `OPEN, RESOLVED`. Drop `MUTED`: it is a separate `muted(...)` selector criterion, not a status.
  - `--risk` adds `NONE`.
  - `--impact` adds `ENVIRONMENT`.

- [ ] **Step 1: Write the failing tests.**

| Test | Asserts |
|---|---|
| `TestEnumFlagsAreCaseInsensitive` | `get problems --status open` sends `problemSelector` containing `status("OPEN")`; `get security-problems --risk critical` sends `riskLevel("CRITICAL")`. Check with `fakeAPI.Queries`. |
| `TestInvalidEnumIsUsageError` | `get problems --impact SERVICE` exits 2, stderr contains `want one of: APPLICATION, ENVIRONMENT, INFRASTRUCTURE, SERVICES`, and the API gets no hits. |
| `TestNegativeLimitIsUsageError` | `get problems --limit -1 -A` exits 2. |

- [ ] **Step 2: Run them and confirm they fail.**

Run:

```bash
go test ./cmd/ -run 'EnumFlags|InvalidEnum|NegativeLimit' -v
```

Expected: FAIL (lower case is sent as is, `SERVICE` is accepted, `-1` bypasses the default).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the tests again and confirm they pass.**

- [ ] **Step 5: Commit.** Message: `feat(cli): validate enum flags and --limit against the API's values`

---

### Task 5: `get security-problems --from/--to/--sort`

**Files:** Modify `cmd/get_security.go` and its test.

**Interfaces:**
- `--from`: sends `from`. When the flag is unset nothing is sent, and the API default `now-30d` applies.
- `--to`: sends `to`.
- `--sort`: free text, because the API accepts many fields. It replaces the default `-riskAssessment.riskScore`, which stays when the flag is unset.
- `Long` help adds entity-scoping examples through `--selector`:
  - `relatedHostIds("HOST-…")`
  - `affectedPgIds("PROCESS_GROUP-…")`
  - `managementZoneIds("…")`

  The help also states that the API has no `entitySelector` for security problems.

- [ ] **Step 1: Write the failing test.** `TestSecurityProblemsTimeAndSort` runs with `--from now-7d --to now --sort +firstSeenTimestamp` and asserts that the query sends `from=now-7d`, `to=now` and `sort=+firstSeenTimestamp`. Without the flags, the query sends `sort=-riskAssessment.riskScore` and no `from`.
- [ ] **Step 2: Run it and confirm it fails** (`unknown flag: --from`, exit 2).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run it again and confirm it passes.**
- [ ] **Step 5: Commit.** Message: `feat(security): --from, --to and --sort on get security-problems`

---

### Task 6: `get slos` timeframe, sort, global and demo flags

**Files:** Modify `cmd/get_slos.go` and its test.

**Interfaces:**
- `--timeframe`: one of `CURRENT` or `GTF`.
- `--from` and `--to`: when either is set and `--timeframe` is not, send `timeFrame=GTF`, the same rule `describe slo` uses.
- `--sort`: `name` or `-name`.
- `--show-global` and `--demo`: bools. Send `showGlobalSlos` or `demo` **only when the flag was given**, using `cmd.Flags().Changed`, and send the value as written. So `--show-global=false` sends `showGlobalSlos=false`.
- `--enabled`: validated by `enumFlag` against `true`, `false` and `all`.

- [ ] **Step 1: Write the failing tests.**
  - `TestSlosTimeframeFlags`: `--from now-7d` sends `timeFrame=GTF` and `from=now-7d`, and `--timeframe CURRENT --from now-7d` keeps `CURRENT`.
  - `TestSlosBooleanFlagsSentOnlyWhenSet` (Review Focus 4): `--show-global=false --demo=false` sends both as `false`, and without the flags neither key is sent.
- [ ] **Step 2: Run them and confirm they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them again and confirm they pass.**
- [ ] **Step 5: Commit.** Message: `feat(slo): timeframe, sort, show-global and demo on get slos`

---

### Task 7: Reject duplicate context names and `;` in names

**Files:** Modify `pkg/config/config.go` (`LoadFrom`), `cmd/ctx.go` and `cmd/config.go` (`setContext`), and their tests.

**Interfaces:**
- `LoadFrom` returns an error for two contexts with the same name (exact match), and for a name containing `;`, which is unreachable through `--env "a;b"`.
- Error text: ``config file %s: context %q is defined %d times; rename or remove the extras by editing the file`` and ``config file %s: context name %q contains ';', which --env uses to separate names; rename it by editing the file``.
- `ctx set` and `config set-context` reject a new name containing `;` with a `UsageError`.

- [ ] **Step 1: Write the failing tests.**
  - `TestDuplicateContextNameIsRejected` (pkg/config, Review Focus 1): the error contains the path, `"prod"`, `2 times` and `editing the file`.
  - `TestSemicolonInContextNameIsRejected` covers both `LoadFrom` and `ctx set "a;b" …`, which exits 2.
- [ ] **Step 2: Run them and confirm they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them again and confirm they pass.**
- [ ] **Step 5: Commit.** Message: `fix(config): reject duplicate context names and ';' in names`

---

### Task 8: `query logs` defaults; verify `--entity`

**Files:** Modify `cmd/query_logs.go`, `README.md` and `skills/dtmgd/SKILL.md` (only if Step 3 changes `--entity`), and the test.

**Interfaces:**
- `query logs` always sends `sort`: the flag's value, or `-timestamp` when it is unset. This is the API's and the MCP server's default, made explicit.
- `--entity` follows the decision in Step 3.

- [ ] **Step 1: Write the failing test.** `TestQueryLogsSortsNewestFirst`: the query sends `sort=-timestamp` when `--sort` is not given.

- [ ] **Step 2: Implement it, then run it and confirm it passes.**

- [ ] **Step 3: Verify `--entity` against the live stage environment. Ask the owner first.**

Run these two read-only queries through dtmgd. Both are 1 hour, limit 1, aggregate counts only:

```bash
dtmgd query logs -c stage --query 'status="ERROR"' --from now-1h --limit 1 -o json --no-agent | jq '.totalCount'
dtmgd query logs -c stage --query 'status="ERROR"' --from now-1h --limit 1 --entity 'type(HOST),entityId("HOST-NONEXISTENT0000")' -o json --no-agent | jq '.totalCount'
```

Decision rule:
- If the second `totalCount` is lower (normally 0), the API honours `entitySelector`. Keep `--entity` and document that it works.
- If both counts are equal, the API ignores it. Make `--entity` print `output.PrintWarning("--entity is ignored by /logs/search on this cluster; filter with dt.entity.* fields in --query instead")`, mark the flag deprecated in its help, and update the docs.

Record the counts and the decision in the commit message.

- [ ] **Step 4: Commit.** Message: `feat(logs): query logs sorts newest-first explicitly; --entity <kept|deprecated>`

---

### Task 9: `dtmgd config import` for MCP server configs

**Files:** Create `cmd/config_import.go` and `cmd/config_import_test.go`. Modify `cmd/config.go` (register the command) and `README.md`.

**Interfaces:**
- Usage: `dtmgd config import <file> | --from-env [--overwrite] [--dry-run]`.
  - `<file>` is the MCP `DT_CONFIG_FILE` format, a YAML or JSON array; the extension decides which.
  - `--from-env` reads the `DT_ENVIRONMENT_CONFIGS` JSON.
- Field mapping:

  | MCP field | dtmgd setting |
  |---|---|
  | `alias` | context name |
  | `apiEndpointUrl` | `host` (trailing `/` trimmed) |
  | `environmentId` | `env-id` (trailing `/` trimmed) |
  | `apiToken` | token named `<alias>-token`, set as `token-ref` |
  | `httpProxyUrl` | `http-proxy` |
  | `httpsProxyUrl` | `https-proxy` |

  Other fields (`dynatraceUrl`, …) are ignored with one stderr note listing them.
- Tokens:
  - A literal `apiToken` is stored with `cfg.SetToken`: in the keyring when one is available, otherwise as plaintext with the usual warning.
  - An `apiToken` that is or contains a `${VAR}` reference is written **as the reference** into the config's `tokens` entry. It never goes to the keyring and is never expanded.
- Validation, all `UsageError`s before anything is written:
  - a missing `alias`, `apiEndpointUrl` or `environmentId`
  - a duplicate `alias` within the source, using the MCP server's message: ``Duplicate alias found: "<alias>". Aliases must be unique across environments.``
  - an `alias` containing `;`
- An alias matching an existing context is skipped with a warning, unless `--overwrite` is given.
- `--dry-run` prints the contexts it would create, with tokens shown as `<keyring>`, `<plaintext>` or `<reference ${VAR}>`, and writes nothing.
- Saves through `a.saveConfig`, which is atomic and 0600.

- [ ] **Step 1: Write the failing tests.**

| Test | Asserts |
|---|---|
| `TestImportMCPConfigFile` | A YAML file with 2 environments creates 2 contexts with the right host, env-id, token-ref and proxy. |
| `TestImportKeepsTokenReferences` (Review Focus 3) | `apiToken: ${DT_PROD_TOKEN}` ends up as `token: ${DT_PROD_TOKEN}` in the config, `keyring.Get` returns `ErrNotFound`, and the test sets `DT_PROD_TOKEN=secret` and asserts `secret` appears nowhere in the file. |
| `TestImportRejectsDuplicateAliases` | Exit 2 with the MCP message, and the config is unchanged. |
| `TestImportSkipsExistingUnlessOverwrite` | An existing context is skipped without `--overwrite` and replaced with it. |
| `TestImportFromEnv` | `--from-env` with `DT_ENVIRONMENT_CONFIGS='[…]'` creates the contexts. |
| `TestImportDryRunWritesNothing` | `--dry-run` leaves the config unchanged. |

- [ ] **Step 2: Run them and confirm they fail** (`unknown subcommand "import"`).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them again and confirm they pass.**
- [ ] **Step 5: Commit.** Message: `feat(config): import contexts from a Dynatrace Managed MCP server config`

---

### Task 10: Dynatrace UI links

**Files:** Create `cmd/links.go` and `cmd/links_test.go`. Modify `cmd/runner.go` (attach links), `fetchSpec` (add a `Link` field), and the commands listed below.

**Interfaces:**
- `fetchSpec.Link func(base string, body interface{}) string`, where `base` is `client.DashboardURL()`, i.e. `{host}/e/{env-id}`.
- Patterns, taken from the MCP server:

  | Command | Link |
  |---|---|
  | `get problems` | `/ui/problems` |
  | `describe problem` | `/#problems/problemdetails;pid=<problemId>` |
  | `describe entity` | `/ui/entity/<entityId>` |
  | `get security-problems` | `/ui/security/vulnerabilities` |
  | `describe security-problem` | `/ui/security/vulnerabilities/<securityProblemId>` |
  | `get slos` | `/ui/slo` |
  | `query logs` | `/ui/log-monitoring` |
  | `get metrics` and `query metrics` | `/ui/data-explorer` |

- Single environment:
  - Agent mode puts `context.links = {"ui": <url>}`.
  - In human mode, `describe` commands print `Open in Dynatrace: <url>` to stderr.
  - json and yaml are unchanged.
- Multi-environment (Review Focus 5): `context.links = {"<env>": <url>, …}`, one entry per environment that answered.
- `fetched` gains the base URL: `fetched.UIBase string`, filled in `runFetch`'s fetch wrapper from `c.DashboardURL()`.

- [ ] **Step 1: Write the failing tests.**
  - `TestAgentLinksSingleEnv`: `describe problem P -A` gives `context.links.ui` = `http://FAKE/e/env1/#problems/problemdetails;pid=P` once the harness has normalised the URL.
  - `TestAgentLinksPerEnvironment`: `get problems --env "a;b" -A` gives `context.links` with keys `a` and `b`.
  - `TestDescribeHumanPrintsLink`: stderr contains `Open in Dynatrace:`, and stdout is unchanged.
- [ ] **Step 2: Run them and confirm they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them again and confirm they pass.**
- [ ] **Step 5: Commit.** Message: `feat(cli): Dynatrace UI links in agent output and describe commands`

---

### Task 11: Phase 2 leftovers

**Files:** As named per item. One commit per item, each test-first.

| # | Change | Test |
|---|---|---|
| a | `config view` masks the password in `http-proxy` and `https-proxy` the way it masks tokens: `url.Parse` then `Redacted()` | `TestConfigViewMasksProxyPasswords` |
| b | `doctor` declares scopes `DataExport` and `metrics.read`. `TestReadmeScopeListMatchesCommands` must stay green: the README already lists both. | `TestDoctorDeclaresScopes` |
| c | `DTMGD_CONTEXT` naming a missing context: ``DTMGD_CONTEXT names context %q, which is not in <path>`` | `TestDTMGDContextMissingContextNamesVariable` |
| d | Agent and json `query log-counts`: when counts are unknown, `context.warnings` carries the top-N note | `TestLogCountsAgentWarnsAboutUnknownCounts` |
| e | Truncated `query logs` in agent mode gets the "Showing n of m" suggestion. `fetched` gains `Count int` (-1 = unknown), set by `listFetched` and the logs fetch, and `responseContext` uses it when `Rows` is nil | `TestQueryLogsAgentTruncationSuggestion` |
| f | `--env ALL_ENVIRONMENTS` and any `;` spec always render the map form, even with one result. This is a **breaking change**; list it in PR B. | `TestAllEnvironmentsSingleContextIsMap` |
| g | Unknown top-level command: `root.DisableSuggestions = true`, plus a `closest` suggestion over the root's commands, so `error.message` holds no "Did you mean this?" text | `TestUnknownTopLevelCommandSuggestion` |
| h | `writeFileAtomic`: `f.Sync()` before rename. When `CreateTemp` fails with a permission error, fall back to an in-place `os.WriteFile(path, data, 0o600)`. `deleteCredentials` saves the config before deleting from the keyring | `TestSaveToFallsBackWhenDirNotWritable` (skip on windows and when running as root), `TestDeleteCredentialsSavesBeforeKeyring` |
| i | Delete dead code: `client.UnwrapSingle` and its tests, the stale comment at `pkg/client/multi.go:70-76`, `client.NewFromConfig`, `output.NewPrinter`. Keep `DashboardURL`, which Task 10 uses. | build, vet, lint |

- [ ] **Steps:** for each item, write the test, confirm it fails, implement, confirm it passes, commit with `fix: <item summary>`. For (i), the check is build, vet and lint clean.

---

### Task 12: Honest parity docs, knowledge base, PR B, release v0.2.0

**Files:** `README.md`, `skills/dtmgd/SKILL.md`, `.claude/CLAUDE.md`

**Interfaces:** none.

- [ ] **Step 1: Rewrite the MCP parity section** (README ~616–640, SKILL.md ~497–521) as a table with columns **MCP tool | dtmgd command | Differences**. Cover every one of the 18 tools. Differences include:
  - default result counts: MCP returns one capped page; dtmgd fetches all pages in human mode and 50 in agent mode
  - `query metrics` resolution: MCP sends `Inf`, dtmgd uses the API default
  - security problems: no `entitySelector` (MCP's is undocumented), so dtmgd uses `--selector`
  - dtmgd-only commands and flags: `log-counts`, `--selector`, `--watch`, `S-NNN`, multi-environment output, `commands`, `doctor`, `config import`
  - MCP-only behaviour, such as HTTP mode

  Replace "full feature parity" and "same feature set" (README:5, SKILL.md:3 and :9) with "covers every MCP tool; differences are listed below".

  Also fix the Phase 2 doc inaccuracies:
  - "Go 1.26+" becomes "Go 1.26.10+".
  - `config`/`ctx` honour a single `--env` name and don't fan out.
  - SECURITY.md restores "check a hand-written config with `ls -l`" for files dtmgd only reads.

- [ ] **Step 2: Knowledge base.** Add entries for:
  - the tools module: why it exists and how to bump it
  - `.sonarcloud.properties` versus `sonar-project.properties`
  - `/securityProblems` has no `entitySelector`
  - the Task 8 `--entity` finding
  - the MCP server sends undocumented top-level `impactLevel`, `riskLevel`, `entitySelector` and `eventType`

- [ ] **Step 3: Full verification.**

Run:

```bash
gofmt -l .
go vet ./...
golangci-lint run
go mod tidy && git diff --exit-code go.mod go.sum
go -C tools mod tidy && git diff --exit-code tools/
go test -race ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...
./govulncheck ./...   # the binary built from tools/
grep -n -i "full feature parity\|same feature set" README.md skills/dtmgd/SKILL.md
```

Expected: all clean, govulncheck reports `No vulnerabilities found.`, and the final grep prints nothing.

- [ ] **Step 4: Final review, then PR B.**
  - Run a final whole-branch review with a fresh reviewer on the most capable model.
  - Fix its findings in one pass.
  - Push and open PR B with the owner's go-ahead. Title: `Phase 3 (2/2): MCP parameter parity, config import, UI links`.
  - The PR body lists breaking changes:
    - new validation errors for enums and a negative `--limit`
    - `ALL_ENVIRONMENTS` with one context now prints a map
    - `query logs` sends `sort=-timestamp` explicitly
    - `--entity` on `query logs`, if Step 3 of Task 8 deprecated it
    - duplicate context names or `;` in a name make the config fail to load
    - `config` and `skills` typos were already handled in v0.1.0
  - Also give the owner the issue text for the MCP repository: its undocumented top-level parameters.

- [ ] **Step 5: After the merge, tag v0.2.0 (owner go-ahead).**
  - Follow the v0.1.0 release steps.
  - Open the Homebrew tap PR by hand from `ivan-gudak:dtmgd-0.2.0`.
  - Put the breaking-changes list at the top of the release notes.
  - Smoke-test the released binary.
