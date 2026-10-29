# Phase 1 — Security and Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close dtmgd's security and safety gaps and ship them to Homebrew users as v0.0.29.

**Architecture:**
- Build with a patched Go and keep it patched with a scheduled drift check that opens an issue.
- Stop config writes from wiping an unparseable config.
- Close the open pentest notes.
- Suppress Snyk where its secret is unavailable.
- Protect `main` with a ruleset.
- Cut the release with the existing goreleaser flow.

**Tech Stack:** Go (Cobra, resty, go-keyring), GitHub Actions, goreleaser, `gh` CLI.

**Spec:** `docs/superpowers/specs/2026-10-29-dtmgd-catch-up-design.md` (Phase 1 section, plus "Owner decisions").

## Global Constraints

- Work on branch `fix/phase1-security-safety`, created from `docs/catch-up-design`, so the design and this plan travel in the same PR.
- Each commit message ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- PRs are opened by the owner in the GitHub UI or with `gh pr create`, which is authenticated as the owner. Never by a bot token.
- Get the owner's go-ahead **at the time** for:
  - pushing a branch
  - creating the ruleset
  - pushing the release tag
- Local test signal: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...`. The `pkg/skills` tests `TestInstallAndStatusAndUninstall` and `TestStatusAll` fail locally on machines that have dtmgd skills installed. That is a known issue; leave it alone.
- HTTP error tests use 400/403, never 5xx: resty retries 5xx, which slows tests.
- Keyring-touching tests rely on `keyring.MockInit()` in `cmd/config_test.go` `TestMain`. Don't add keyring-touching tests to other packages.
- Never print, log or commit token values. When inspecting `~/.config/dtmgd/config`, filter out lines matching `^\s*token:`.
- Toolchain: `toolchain go1.26.10`, the latest 1.26 patch on 2026-10-29. Keep `go 1.26.0`.
- Pin govulncheck to `golang.org/x/vuln/cmd/govulncheck@v1.9.0` everywhere it is installed.
- Required status checks, using today's exact names: `test (ubuntu-latest)`, `test (macos-latest)`, `test (windows-latest)`, `lint`, `golangci-lint`, `vulncheck`. A later phase that renames a job must update the ruleset in the same PR.

## Review Focus

1. **Config unreadable for a reason other than "missing"** (permission denied, path is a directory). `ctx set` and `config set-credentials` must fail and leave the file untouched, exactly as they do for a parse error. Owner: Task 3, test `TestSetContextRefusesUnreadableConfig`.
2. **Agent notice only on auto-detection.** The notice must not appear when the user passed `--agent`, nor when they passed `--no-agent`. Owner: Task 4, test `TestAgentNoticeOnlyWhenAutoDetected`.
3. **`skills install --cross-client` in a non-interactive shell** must still work without `--for`: cross-client doesn't depend on detection. Owner: Task 4, test `TestChooseAgentNonInteractive`.
4. **Drift check when go.dev is unreachable.** The script must exit 2 ("could not check") and the workflow must leave any open drift issue alone. It must not close it as resolved. Owner: Task 2, step "unreachable go.dev".
5. **CI actually builds with go1.26.10.** If `setup-go` ignored the `toolchain` line, `GOTOOLCHAIN: local` would keep CI on Go 1.26.0 with everything green-looking except `vulncheck`. Owner: Task 7, log check.

---

### Task 1: Patched toolchain and dependency refresh

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `.github/workflows/security.yml` (pin govulncheck)

**Interfaces:** none.

- [ ] **Step 1: Create the branch**

```bash
git switch docs/catch-up-design && git switch -c fix/phase1-security-safety
```

- [ ] **Step 2: Show the failure CI sees**

Run: `GOTOOLCHAIN=go1.26.0 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`
Expected: non-zero exit, with stdlib findings such as `GO-2026-6809 … Fixed in: crypto/x509@go1.26.10`.

- [ ] **Step 3: Pin the toolchain and refresh dependencies**

```bash
go mod edit -toolchain=go1.26.10
go get -u ./... && go mod tidy
```

`go.mod` must still say `go 1.26.0`. It must now contain `toolchain go1.26.10`, and `golang.org/x/net` must be ≥ v0.62.0.

- [ ] **Step 4: Verify with the CI Go version**

Run:

```bash
GOTOOLCHAIN=go1.26.10 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
GOTOOLCHAIN=go1.26.10 go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...
```

Expected: `No vulnerabilities found.` and all packages `ok`.

- [ ] **Step 5: Pin govulncheck in `security.yml`**

Change `go install golang.org/x/vuln/cmd/govulncheck@latest` to `@v1.9.0`.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum .github/workflows/security.yml
git commit -m "fix(deps): pin patched go1.26.10 toolchain and refresh dependencies"
```

---

### Task 2: Scheduled drift check that opens an issue

**Files:**
- Create: `scripts/drift-check.sh` (executable)
- Create: `.github/workflows/drift.yml`

**Interfaces:**
- Produces: `scripts/drift-check.sh`. It writes a Markdown report to stdout and exits with:

  | Exit | Meaning |
  |---|---|
  | 0 | no drift |
  | 1 | drift found |
  | 2 | a check could not run |

  It uses the env var `GOVULNCHECK`, which defaults to `govulncheck`.

- [ ] **Step 1: Write `scripts/drift-check.sh`**

`set -euo pipefail`. It runs three checks and gives each its own Markdown section:
- **Toolchain:**
  - `current` is `go mod edit -json | jq -r '.Toolchain // ("go" + .Go)'`.
  - `line` is the `goX.Y` prefix of `current`.
  - `latest` is the highest stable version starting with `line.` from `https://go.dev/dl/?mode=json&include=all`; sort with `sort -V`, don't trust the API's order.
  - Drift if `latest` differs from `current`.
  - If the fetch fails, the section says "could not check" and the script's final status is at least 2.
- **Linked modules behind:**
  - Linked set: `go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}{{end}}{{end}}' ./... | sort -u`.
  - Outdated set: `go list -m -u -f '{{if and .Update (not .Main)}}{{.Path}} {{.Version}} {{.Update.Version}}{{end}}' all`.
  - Report only outdated modules in the linked set. Graph-only modules are deliberately excluded; see the Snyk/Dependabot entries in `.claude/CLAUDE.md`.
- **Vulnerabilities:**
  - Run `"$GOVULNCHECK" ./...`.
  - Exit code 3 means drift, and the output goes in a fenced block.
  - Any other non-zero exit means "could not check".

The final exit code is the highest of the three checks' results.

- [ ] **Step 2: Verify the clean case**

Run: `GOVULNCHECK="go run golang.org/x/vuln/cmd/govulncheck@v1.9.0" bash scripts/drift-check.sh; echo "exit=$?"`
Expected: `exit=0`, and the report says the toolchain is current.

- [ ] **Step 3: Verify the drift case**

In a scratch copy of the repo, not the working tree:

```bash
d=$(mktemp -d) && git worktree add "$d" HEAD
(cd "$d" && go mod edit -toolchain=go1.26.9 && bash scripts/drift-check.sh; echo "exit=$?")
git worktree remove --force "$d"
```

Expected: `exit=1`, and the report names `go1.26.9` → `go1.26.10`.

- [ ] **Step 4: Verify the "unreachable go.dev" case**

Run: `HTTPS_PROXY=http://127.0.0.1:9 bash scripts/drift-check.sh; echo "exit=$?"`
Expected: `exit=2`, and the toolchain section says it could not check.

- [ ] **Step 5: Write `.github/workflows/drift.yml`**

- Triggers: `schedule: cron '0 6 * * 1'` and `workflow_dispatch`.
- `permissions: { contents: read, issues: write }`.
- Steps:
  1. checkout
  2. setup-go, same pinned SHA and `go-version-file: go.mod` as the other workflows
  3. `go install golang.org/x/vuln/cmd/govulncheck@v1.9.0`
  4. run the script into `report.md`, capturing its status without failing the step
  5. handle the issue with `gh`, using `GH_TOKEN: ${{ github.token }}`, as below:

```bash
label=drift
gh label create "$label" --color FBCA04 --description "Toolchain or dependency drift" 2>/dev/null || true
num=$(gh issue list --label "$label" --state open --json number --jq '.[0].number // empty')
case "$status" in
  0) [ -n "$num" ] && gh issue close "$num" --comment "Drift resolved as of $(date -u +%F)." || true ;;
  1) if [ -n "$num" ]; then gh issue edit "$num" --body-file report.md
     else gh issue create --title "Toolchain or dependency drift" --label "$label" --body-file report.md; fi ;;
  *) echo "::warning::Drift check could not complete; leaving issues unchanged"; cat report.md; exit 1 ;;
esac
```

- [ ] **Step 6: Commit**

```bash
git add scripts/drift-check.sh .github/workflows/drift.yml
git commit -m "ci: weekly toolchain and dependency drift check that opens an issue"
```

---

### Task 3: Config writes never wipe an existing config

**Files:**
- Modify: `pkg/config/config.go` (`LoadFrom`, around lines 103-115)
- Modify: `cmd/root.go` (add `loadConfigForUpdate` next to `loadConfigRaw`, around line 200)
- Modify: `cmd/ctx.go:185-189` (`setContext`), `cmd/config.go:228-232` (`setCredentials`)
- Test: `pkg/config/config_test.go`, `cmd/ctx_test.go`, `cmd/config_test.go`

**Interfaces:**
- Produces:
  - `type NotFoundError struct{ Path string }` in `pkg/config`. Its `Error()` is the current message, unchanged: `config file not found at <path>. Run 'dtmgd config set-context' to create one`. `Unwrap() error` returns `fs.ErrNotExist`.
  - `func loadConfigForUpdate() (*config.Config, error)` in `cmd`. It returns `config.NewConfig(), nil` only when `errors.Is(err, fs.ErrNotExist)`, and the loaded config or the error otherwise.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/config/config_test.go
func TestLoadFromMissingIsNotExist(t *testing.T) {
	_, err := LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, fs.ErrNotExist) { t.Fatalf("want fs.ErrNotExist, got %v", err) }
	if !strings.Contains(err.Error(), "config file not found at") { t.Errorf("message changed: %v", err) }
}

// cmd/ctx_test.go
func TestSetContextRefusesToOverwriteUnparseableConfig(t *testing.T) {
	bad := "contexts:\n  - name: prod\n    context:\n      host: https://h\n    - name: broken\n"
	path := withTempConfig(t, bad)
	err := setContext("new", "https://new.example.com", "env1", "tok", "")
	if err == nil || !strings.Contains(err.Error(), "failed to parse config file") { t.Fatalf("want parse error, got %v", err) }
	after, _ := os.ReadFile(path)
	if string(after) != bad { t.Errorf("config was modified:\n%s", after) }
}

func TestSetContextRefusesUnreadableConfig(t *testing.T) { // Review Focus 1
	if runtime.GOOS == "windows" || os.Geteuid() == 0 { t.Skip("permission bits not enforced") }
	path := withTempConfig(t, placeholderConfig)
	if err := os.Chmod(path, 0o000); err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if err := setContext("new", "https://new.example.com", "env1", "tok", ""); err == nil { t.Fatal("want error for unreadable config") }
	_ = os.Chmod(path, 0o600)
	after, _ := os.ReadFile(path)
	if string(after) != placeholderConfig { t.Errorf("config was modified") }
}

func TestSetContextCreatesMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.yaml")
	prev := cfgFile; cfgFile = path; t.Cleanup(func() { cfgFile = prev })
	if err := setContext("prod", "https://h.example.com", "env1", "tok", ""); err != nil { t.Fatal(err) }
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "name: prod") { t.Errorf("context not written:\n%s", after) }
}

// cmd/config_test.go
func TestSetCredentialsRefusesToOverwriteUnparseableConfig(t *testing.T) {
	bad := "tokens:\n  - name: a\n    token: x\n  - name\n"
	path := withTempConfig(t, bad)
	if err := setCredentials("prod-token", "dt0c01.TEST"); err == nil { t.Fatal("want parse error") }
	after, _ := os.ReadFile(path)
	if string(after) != bad { t.Errorf("config was modified:\n%s", after) }
}
```

Check `bad` really fails to parse: if `LoadFrom` accepts either string, replace it with one it rejects, keeping the assertion unchanged.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/config/ ./cmd/ -run 'TestLoadFromMissingIsNotExist|Refuses|CreatesMissingConfig' -v`
Expected:
- `TestLoadFromMissingIsNotExist` FAIL (want fs.ErrNotExist).
- Both `RefusesToOverwrite` tests FAIL ("config was modified" or nil error).
- `TestSetContextCreatesMissingConfig` PASS. It pins today's correct behaviour.

- [ ] **Step 3: Implement**

- `LoadFrom` returns `&NotFoundError{Path: path}` when the file doesn't exist.
- Add `loadConfigForUpdate()` in `cmd/root.go`.
- Replace the `cfg, err := loadConfigRaw(); if err != nil { cfg = config.NewConfig() }` blocks in `setContext` and `setCredentials` with `cfg, err := loadConfigForUpdate(); if err != nil { return err }`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/config/... ./cmd/...`
Expected: `ok` for both packages.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/config.go pkg/config/config_test.go cmd/root.go cmd/ctx.go cmd/config.go cmd/ctx_test.go cmd/config_test.go
git commit -m "fix(config): never replace an existing config that fails to load"
```

---

### Task 4: Pentest note 5 — announce auto-detected agent mode; require `--for` when non-interactive

**Files:**
- Modify: `pkg/output/agent.go` (`DetectAgent`)
- Modify: `cmd/root.go` (add a `PersistentPreRun` on `rootCmd`)
- Modify: `cmd/skills.go` (`chooseAgent` around line 287, and its callers)
- Test: `pkg/output/output_test.go`, `cmd/root_test.go`, `cmd/skills_test.go` (create)

**Interfaces:**
- Produces:
  - `func DetectedAgentVar() string` in `pkg/output`: the first set variable from `AgentEnvVars`, or `""`. `DetectAgent()` becomes `DetectedAgentVar() != ""`.
  - `func agentNotice() string` in `cmd`: returns the notice text, or `""` when no notice is due. The pre-run hook prints a non-empty result once via `output.PrintInfo`.
  - `chooseAgent(forFlag string, crossClient, interactive bool) (skills.Agent, error)`.
- Notice text (exact): `Agent mode is on because <VAR> is set. Use --no-agent for human-readable output.`
- Error text (exact) when non-interactive with no `--for`:
  - Agent detected: `--for is required when not running in a terminal (detected <name>; use --for <name>)`
  - Nothing detected: the existing "no AI agent detected" error.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/output/output_test.go
func TestDetectedAgentVar(t *testing.T) {
	for _, v := range AgentEnvVars { t.Setenv(v, "") }
	if got := DetectedAgentVar(); got != "" { t.Fatalf("want empty, got %q", got) }
	t.Setenv("KIRO", "1")
	if got := DetectedAgentVar(); got != "KIRO" { t.Fatalf("want KIRO, got %q", got) }
}

// cmd/root_test.go  (Review Focus 2)
func TestAgentNoticeOnlyWhenAutoDetected(t *testing.T) {
	for _, v := range output.AgentEnvVars { t.Setenv(v, "") }
	t.Setenv("CLAUDECODE", "1")
	pa, pn := agentFlag, noAgentFlag
	t.Cleanup(func() { agentFlag, noAgentFlag = pa, pn })

	agentFlag, noAgentFlag = false, false
	if got := agentNotice(); !strings.Contains(got, "CLAUDECODE is set") { t.Errorf("auto-detected: got %q", got) }
	agentFlag = true
	if got := agentNotice(); got != "" { t.Errorf("--agent given: want no notice, got %q", got) }
	agentFlag, noAgentFlag = false, true
	if got := agentNotice(); got != "" { t.Errorf("--no-agent given: want no notice, got %q", got) }
}

// cmd/skills_test.go  (Review Focus 3)
func TestChooseAgentNonInteractive(t *testing.T) {
	for _, v := range output.AgentEnvVars { t.Setenv(v, "") }
	t.Setenv("CLAUDECODE", "1")
	if _, err := chooseAgent("", false, false); err == nil || !strings.Contains(err.Error(), "--for claude") {
		t.Errorf("want --for required naming claude, got %v", err)
	}
	if a, err := chooseAgent("", false, true); err != nil || a.Name != "claude" { t.Errorf("interactive: got %v, %v", a, err) }
	if _, err := chooseAgent("", true, false); err != nil { t.Errorf("--cross-client must not need --for: %v", err) }
	if a, err := chooseAgent("cursor", false, false); err != nil || a.Name != "cursor" { t.Errorf("explicit --for: got %v, %v", a, err) }
}
```

Check the agent name `skills.FindAgent` uses for Claude Code. If it isn't `claude`, use the real one in the test and in the error text.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/output/ ./cmd/ -run 'DetectedAgentVar|AgentNotice|ChooseAgentNonInteractive' -v`
Expected: build failure (undefined `DetectedAgentVar`, `agentNotice`, wrong `chooseAgent` arity).

- [ ] **Step 3: Implement**

- Add `DetectedAgentVar` and `agentNotice`. In `rootCmd`, `PersistentPreRun` prints the notice when it's non-empty.
- Give `chooseAgent` the new parameter. Callers pass `term.IsTerminal(int(os.Stdin.Fd()))` from `golang.org/x/term`, which is already a dependency.
- Update the install and uninstall `Long` help: in a non-interactive shell, `--for` is required.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/output/... ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add pkg/output cmd/root.go cmd/root_test.go cmd/skills.go cmd/skills_test.go
git commit -m "fix(agent): announce auto-detected agent mode; require --for when not interactive (pentest note 5)"
```

---

### Task 5: Pentest note 1 — notice for `--config` outside the standard locations

> **Owner confirmation pending.** This task reverses a documented decision (`cmd/root.go:218-233`: an explicit `--config` is not warned on, so that warnings keep their weight). Do this task only if the owner keeps it. If the owner drops it, skip it and record "by design" against note 1 in `SECURITY.md` (Task 6).

**Files:**
- Modify: `pkg/config/source.go` (add `IsStandardLocation`)
- Modify: `cmd/root.go` (`reportConfigSource`, and the comment above it)
- Test: `pkg/config/source_test.go`, `cmd/configsource_test.go`

**Interfaces:**
- Produces: `func IsStandardLocation(path string) bool` in `pkg/config`. It returns true when `path` resolves to:
  - a file in `filepath.Dir(DefaultConfigPath())`, or
  - a file named `LocalConfigName`.
- Warning text (exact): `Using config <path> from outside the standard locations (<dir of DefaultConfigPath()> or a local .dtmgd.yaml).`

- [ ] **Step 1: Write the failing tests**

- `TestIsStandardLocation` is table-driven:
  - `DefaultConfigPath()` → true
  - `filepath.Join(filepath.Dir(DefaultConfigPath()), "other")` → true
  - `filepath.Join(t.TempDir(), ".dtmgd.yaml")` → true
  - `filepath.Join(t.TempDir(), "x.yaml")` → false
- `TestReportConfigSourceWarnsOnNonStandardExplicitPath` uses `captureStderr`:
  - `reportConfigSource(config.Source{Path: <tmp>/x.yaml, Kind: config.SourceExplicit})` → stderr contains `from outside the standard locations`.
  - The same call with a `.dtmgd.yaml` path → empty stderr.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/config/ ./cmd/ -run 'IsStandardLocation|NonStandardExplicit' -v`
Expected: build failure (undefined `IsStandardLocation`).

- [ ] **Step 3: Implement**

Add the function and the `SourceExplicit` branch. Rewrite the comment so it states the new rule and why.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/config/... ./cmd/...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/source.go pkg/config/source_test.go cmd/root.go cmd/configsource_test.go
git commit -m "fix(config): warn when --config points outside the standard locations (pentest note 1)"
```

---

### Task 6: `SECURITY.md` and Snyk suppression

**Files:**
- Create: `SECURITY.md`
- Modify: `.github/workflows/snyk.yml`

**Interfaces:** none.

- [ ] **Step 1: Write `SECURITY.md`**

Adapt `../dtctl/SECURITY.md`: replace dtctl with dtmgd, use the same `opensource@dynatrace.com` route, and keep the disclosure timeline. Replace its "Security Best Practices" with dtmgd's real ones:
- tokens in the OS keyring
- on headless systems, tokens stored in plaintext at 0600; prefer `${VAR}` references
- `--token-stdin`
- the parent-directory config warning
- the auto-detected agent-mode notice and `--no-agent`

Add a "Supported versions" line: only the latest release gets fixes.

- [ ] **Step 2: Suppress Snyk where the secret is missing**

In `snyk.yml`:
- Move `SNYK_TOKEN: ${{ secrets.SNYK_API_TOKEN }}` to job-level `env`.
- Add `if: env.SNYK_TOKEN != ''` to the Snyk step.
- Add a step with `if: env.SNYK_TOKEN == ''` that runs `echo "::notice::Snyk skipped — SNYK_API_TOKEN is not available to this run (Dependabot and fork PRs)."`.
- Update the header comment: Snyk is not a required check until the owner has a token for Dependabot.

Don't delete the workflow.

- [ ] **Step 3: Verify the YAML**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add SECURITY.md .github/workflows/snyk.yml
git commit -m "docs: add SECURITY.md; ci: skip Snyk when its token is unavailable"
```

---

### Task 7: PR and CI verification

**Files:** none.

- [ ] **Step 1: Full local check**

Run:

```bash
gofmt -l . && go vet ./... && golangci-lint run && go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...
```

Expected: no gofmt output, vet clean, `0 issues.`, all `ok`.

- [ ] **Step 2: Push and open the PR (owner go-ahead first)**

```bash
git push -u origin fix/phase1-security-safety
gh pr create --base main --title "Phase 1: security and safety fixes" --body-file <body>
```

The body:
- links the spec and this plan
- lists the fixes
- states that v0.0.29 follows after merge
- ends with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`

- [ ] **Step 3: Confirm CI really uses go1.26.10 (Review Focus 5)**

Run:

```bash
run=$(gh run list --branch fix/phase1-security-safety --workflow security.yml --limit 1 --json databaseId --jq '.[0].databaseId')
gh run view "$run" --log | grep -m1 -E 'go1\.26\.[0-9]+'
```

Expected: `go1.26.10`. If it shows `go1.26.0`, setup-go is ignoring the `toolchain` line. In that case, put `go-version: '1.26.10'` in every workflow and record the decision in the PR.

- [ ] **Step 4: All checks green**

Run: `gh pr checks --watch`
Expected:
- `vulncheck`, `lint`, `golangci-lint` and `test (*)` pass.
- `snyk` passes, or is skipped with the notice.
- `sonar` failing is known and non-blocking.

- [ ] **Step 5: Owner merges the PR**

---

### Task 8: Ruleset on `main` (after merge, owner go-ahead first)

**Files:** none (repository setting).

- [ ] **Step 1: Create the ruleset**

```bash
gh api -X POST repos/dynatrace-oss/dtmgd/rulesets --input - <<'JSON'
{
  "name": "main",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] } },
  "bypass_actors": [ { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" } ],
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "pull_request", "parameters": { "required_approving_review_count": 0, "dismiss_stale_reviews_on_push": false, "require_code_owner_review": false, "require_last_push_approval": false, "required_review_thread_resolution": false } },
    { "type": "required_status_checks", "parameters": { "strict_required_status_checks_policy": false, "required_status_checks": [
      { "context": "test (ubuntu-latest)", "integration_id": 15368 },
      { "context": "test (macos-latest)", "integration_id": 15368 },
      { "context": "test (windows-latest)", "integration_id": 15368 },
      { "context": "lint", "integration_id": 15368 },
      { "context": "golangci-lint", "integration_id": 15368 },
      { "context": "vulncheck", "integration_id": 15368 } ] } }
  ]
}
JSON
```

`actor_id 5` is the repository admin role, so only the owner bypasses. `integration_id 15368` is GitHub Actions.

- [ ] **Step 2: Verify**

Run: `gh api repos/dynatrace-oss/dtmgd/rules/branches/main --jq '.[].type'`
Expected: `deletion`, `non_fast_forward`, `pull_request` and `required_status_checks` are listed.

---

### Task 9: Release v0.0.29 (owner go-ahead first)

**Files:** none.

- [ ] **Step 1: Owner confirms the tap token**

Ask the owner to confirm that the `HOMEBREW_TAP_GITHUB_TOKEN` repository secret has not expired; it was last used on 2026-07-06. If it has expired, the GitHub release still publishes, only the cask step fails, and the job can be re-run after the secret is refreshed.

- [ ] **Step 2: Tag and push**

```bash
git switch main && git pull --ff-only && git tag -a v0.0.29 -m "v0.0.29" && git push origin v0.0.29
```

- [ ] **Step 3: Watch the release**

Run: `gh run watch $(gh run list --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')`
Expected: success. `gh release view v0.0.29` lists the archives and `checksums.txt`.

- [ ] **Step 4: Confirm the tap PR is open**

Run: `gh pr list --repo dynatrace-oss/homebrew-tap --search "dtmgd v0.0.29"`
Expected: one open PR.

Add a comment to it saying that the generated cask reverts the tap's 2026-09-15 `postflight_steps` migration back to `postflight`, the same as dtctl v0.45.0. Merging is up to the tap's code owner.

- [ ] **Step 5: Smoke-test the released binary**

Download the archive for the current OS from the release and run `./dtmgd version`.
Expected: `v0.0.29`.
