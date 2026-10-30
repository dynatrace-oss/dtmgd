# Test coverage, live integration tests and the architecture gaps: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three gaps found when dtmgd was checked against `cli-tool-architecture` and dtctl on 2026-10-30 (no colors, extra verbs not documented as intentional, no live integration tests), and raise test coverage where it protects real behaviour.

**Architecture:** Two PRs from one branch. PR 1 is tests only: a test sandbox for the home and config directories, command tests for code no test runs, branch gaps, and an opt-in live integration suite behind the `integration` build tag. PR 2 is user-visible: colors in human output (dtctl's rules) and the verb paragraph in the README.

**Tech Stack:** Go 1.26, Cobra, tablewriter v1.1.5, `adrg/xdg`, `golang.org/x/term`, `golang.org/x/sys/windows`.

**Spec:** no separate spec. The owner approved the three items in the session of 2026-10-30 ("ok, do all three. And maybe you can find more places to improve the test coverage"); the gap analysis in that session is the authority:
- Architecture guideline 5: default human output is "table with colors"; `--plain`, non-TTY stdout and `NO_COLOR` turn colors off, `FORCE_COLOR` turns them on.
- Architecture guideline 1: tool-specific verbs "should be documented as intentional extensions, not accidents".
- dtctl has `test/integration` against a real environment configured by a git-ignored env file.

## Global Constraints

- Tests never read or write the developer's real config, keyring or home directory, except the live integration suite, which reads (never writes) the config named by `DTMGD_IT_CONFIG`.
- The integration suite runs only read commands (`get`, `describe`, `query`, `doctor`, `commands`), never prints stdout or tokens on failure (exit code and the first stderr line only), and never runs in CI. Running it against the owner's clusters needs the owner's go-ahead at the time.
- No real host names, environment IDs or zone names in repository files; the env file holding context names is git-ignored.
- HTTP error cases in unit tests use 400/403, never 5xx/429.
- Colors only in table/wide output and stderr messages; never in json, yaml, csv, tsv or the agent envelope. Goldens must not change (tests write to non-terminals, so plain mode applies).
- Docs travel with the change; `make docs` after any help-text change.
- Ask the owner before every push, PR and live-cluster call.

## Review Focus

1. A colored table must keep its column alignment: tablewriter must measure display width, not bytes, so escape codes cannot shift columns.
2. `FORCE_COLOR` with `-o csv`/`-o json`, or in agent mode, must produce no escape codes.
3. Stderr redirected to a file while stdout is a terminal must not get escape codes in the file.
4. The test sandbox must be in place before any test runs: `adrg/xdg` reads the environment in its package `init`, so `TestMain` must set the variables and call `xdg.Reload()`, and must refuse to run when `DefaultConfigPath()` is still outside the sandbox.
5. The integration suite must treat a filter check with an empty unfiltered result as skipped (inconclusive), never as passed.

---

### Task 1: Test sandbox for home and config directories

**Files:**
- Create: `internal/testsandbox/sandbox.go` (test helper package, not imported by production code)
- Modify: `cmd/config_test.go` (TestMain), `pkg/config/keyring_test.go` (TestMain)
- Create: `pkg/skills/main_test.go` (TestMain)
- Test: `cmd/sandbox_test.go`, `pkg/config/sandbox_test.go`

**Interfaces:**
- Produces: `testsandbox.Enter() (cleanup func(), err error)`: sets `HOME`, `USERPROFILE`, `XDG_CONFIG_HOME`, `LOCALAPPDATA`, `APPDATA` to directories under one `os.MkdirTemp`, calls `xdg.Reload()`, and returns an error unless `filepath.Join(xdg.ConfigHome, "dtmgd", "config")` lies inside the temp root.

- [ ] **Step 1: Write the failing tests.** `TestDefaultConfigPathIsSandboxed` (in `cmd` and `pkg/config`): `config.DefaultConfigPath()` has the prefix `os.Getenv("XDG_CONFIG_HOME")`, and that directory is not the user's real config home (compare with the value captured before `Enter`, exported as `testsandbox.OriginalConfigHome`). In `pkg/skills`, the existing `TestInstallAndStatusAndUninstall` and `TestStatusAll` are the failing tests (they fail locally whenever a real dtmgd skill is installed).
- [ ] **Step 2: Run them.** `go test ./cmd/ ./pkg/config/ ./pkg/skills/ -run 'Sandboxed|TestInstallAndStatusAndUninstall|TestStatusAll'`. Expected: FAIL (helper missing; skills "expected not installed in fresh dir").
- [ ] **Step 3: Implement `testsandbox.Enter` and call it first in the three `TestMain`s** (before `keyring.MockInit()`); on error print it and `os.Exit(1)`; run `cleanup` after `m.Run()`.
- [ ] **Step 4: Run** `go test ./...` (the whole module, pkg/skills included). Expected: PASS locally for the first time.
- [ ] **Step 5: Update docs.** AGENTS.md "Build, test, lint" and "Testing": the local signal is `go test ./...`; describe the sandbox. `.claude/CLAUDE.md`: rewrite the "pkg/skills tests fail locally" entry as the fix, and add the sandbox to the "Never point a test at the default config path" entry.
- [ ] **Step 6: Commit** `test: sandbox HOME and the config directory in every test binary that touches them`.

### Task 2: Command tests for commands no test runs

**Files:**
- Create: `cmd/skills_cmd_test.go`, `cmd/config_init_test.go`, `cmd/misc_commands_test.go`
- Modify: `cmd/watch.go`, `cmd/watch_test.go`, `cmd/config.go` (completion hooks only)

**Interfaces:**
- Consumes: Task 1's sandbox (`--global` and `saveConfig`'s fallback can now be exercised).
- Produces: `App.watchStop <-chan struct{}`: when non-nil, `runWatch` stops when it closes instead of on SIGINT.

- [ ] **Step 1: skills tests** (`t.Chdir(t.TempDir())`, `runCLI` with an empty temp config):
  - `skills install --for claude` → exit 0, stdout contains `Installed Claude Code skill`, `.claude/skills/dtmgd/SKILL.md` exists.
  - again without `--force` → exit 1, stderr mentions `--force`; with `--force` → stdout `Updated`.
  - `skills status --for claude` → a row with `true` and `project`; `skills status` → one row per agent plus `cross-client`.
  - `skills install --list` → rows for `claude`, `copilot`, `cursor`, `junie`, `kiro`, `opencode`, `cross-client`.
  - `--cross-client --for claude` → exit 1 "cannot be used together"; `--for nope` → exit 1 listing supported agents.
  - no `--for` with `CLAUDECODE=1` (stdin is not a terminal under `go test`) → exit 1 "--for is required".
  - `skills install --for claude --global` → `SKILL.md` under the sandbox `HOME/.claude/skills/dtmgd/`.
  - `skills uninstall --for claude` → `Removed:`; again → `No Claude Code skill files found to remove.`
  - agent mode (`CLAUDECODE=1`, `--for claude`) → envelope `ok: true`, `result.action == "installed"`, `result.scope == "project"`.
- [ ] **Step 2: config init, migrate-tokens, current context:**
  - `config init` in a temp cwd → `.dtmgd.yaml` with `current-context: my-cluster` and `${DT_API_TOKEN}`; again → exit 1 "already exists"; `--force --context foo` → `current-context: foo`.
  - `config migrate-tokens` with a plaintext token → exit 0, "Migrated 1 token(s)", the file no longer contains the token, `config.GetToken` returns it from the mock keyring; with only a `${VAR}` token → warning "Skipped" and "No tokens to migrate"; keyring unavailable (`keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)` with `t.Cleanup(keyring.MockInit)`) → exit 1 "keyring not available".
  - `config current-context` and `ctx current` print the file's `current-context`.
  - `saveConfig` without `--config`: in a temp cwd holding `.dtmgd.yaml`, with `DTMGD_CONFIG` empty, `config use-context b` rewrites the local file (runCLI with `cfgPath ""`).
- [ ] **Step 3: completion and version:** each of `bash zsh fish powershell` → exit 0 and stdout contains `dtmgd`; `completion tcsh` → exit 2. `version` → stdout starts with `dtmgd ` and contains `commit:`.
- [ ] **Step 4: context-name completion:** `__complete ctx delete ""` lists the config's context names. Add `ValidArgsFunction: a.contextCompletionFn` to `config use-context` and `config delete-context` (they take a context name but have no completion); test `__complete config use-context ""` (RED first).
- [ ] **Step 5: watch loop.** Tests (construct the App directly, `watchStop` closed after the third call of `fn`): returns nil; `fn` ran at least 3 times; stderr has the `Every 10ms — press Ctrl+C to stop` header per tick; an error from the first tick is reported and the loop continues; in plain mode stdout has no `\033[H\033[2J`; with `FORCE_COLOR=1` and table output it has one per tick. Also pin that the newline printed on stop goes to `a.Err`, not the process stderr (fix `fmt.Fprintln(os.Stderr)` in `runWatch`).
- [ ] **Step 6:** `go test ./cmd/` PASS; commit `test: cover skills, config init, migrate-tokens, completion, version and the watch loop`.

### Task 3: Branch gaps in rendering and notes

**Files:** Test: `cmd/branch_gaps_test.go` (or the matching existing `_test.go` files); production code only where a test exposes a bug.

- [ ] **Step 1: Write tests for the uncovered branches,** each named for the behaviour:
  - `truncationNote`: a result without `Rows` and a known total → `Showing part of N <resource>`; total unknown (`-1`) → `More <resource> are available`.
  - `query logs`: no records → stderr `No log records found.`; a record without status → `[ts] content` (no `[]`); `nextSliceKey` present → the `Showing … Use --limit or narrower filters.` note; a body with `totalCount` above the records shown → HasMore.
  - `query log-counts` human output: the branches of `printLogCounts` (read it first; pin each branch it has: empty result, unknown entity names, truncation).
  - `describe security-problem` human summary: component display name preferred over package name; a description line over 220 characters is cut to 217 + `…`; affected count falls back to the component's `numberOfAffected`; management-zone names line.
- [ ] **Step 2:** run them; any that fail expose a bug: fix it test-first and ledger a ruling.
- [ ] **Step 3:** `go test ./...` PASS; commit `test: pin the rendering branches no test reached`.

### Task 4: Opt-in live integration suite

**Files:**
- Create: `test/integration/harness_test.go`, `test/integration/environment_test.go`, `test/integration/cluster_test.go` (all `//go:build integration`)
- Create: `.integrationtests.env.example`
- Modify: `.gitignore` (`.integrationtests.env`), `Makefile` (`integration` target; `lint` also vets with `-tags integration`), `.golangci.yml` (`run.build-tags: [integration]`), `.github/workflows/lint.yml` (vet with the tag), `CONTRIBUTING.md`, `AGENTS.md`, `.claude/CLAUDE.md`

**Interfaces:**
- Consumes: `cmd.Run(args []string, stdout, stderr io.Writer) int`.
- Produces (test-only): `run(t, args ...string) result{Out []byte; Err string; Code int}`, which prepends `--config $DTMGD_IT_CONFIG --no-agent --plain` and calls `t.Fatalf` if `args[0]` is not one of `get describe query doctor commands`; `mustJSON(t, r) map[string]any`, which on a decode failure reports only the exit code and the first stderr line.

Environment (all read from the process environment; `make integration` sources `.integrationtests.env` first):
`DTMGD_IT_CONFIG` (required; else every test skips), `DTMGD_IT_ENV` (environment context; env tests skip without it), `DTMGD_IT_CLUSTER` (cluster context; cluster tests skip without it), `DTMGD_IT_MZ` (optional existing zone name), `DTMGD_IT_LOG_QUERY` (default `error`), `DTMGD_IT_FROM` (default `now-24h`).

- [ ] **Step 1: Harness and offline self-test.** `TestHarnessRefusesWriteVerbs` (runs with no env at all): `run` with `config` as the verb fails the subtest (use a fake `testing.TB` recorder). It must pass with `go test -tags integration ./test/integration/ -run Harness`.
- [ ] **Step 2: Environment tests** (`DTMGD_IT_ENV`):
  - `doctor --context $ENV -o json` → exit 0, the row's status is `ok` or `warn`.
  - each list (`get problems`, `get entities --type HOST`, `get events`, `get metrics`, `get slos`, `get security-problems`, `get entity-types`) with `-o json --limit 5` → exit 0, its list key is an array of at most 5.
  - `get metrics --limit 1 -o json` → exactly 1 item and a non-empty `nextPageKey` when `totalCount` > 1.
  - event type filter: unfiltered `get events --from $FROM` count N (skip if 0); `--type` set to the first event's `eventType` → every returned event has that type and the count ≤ N; `--type ZZ_NOT_A_TYPE` → non-zero exit and `400` in stderr.
  - zone filter: `get entities --type HOST --mz zz-no-such-zone-<random>` → non-zero exit; with `DTMGD_IT_MZ`, the count ≤ the unfiltered count.
  - logs entity filter: `query logs --query $Q --from $FROM -o json` count N (skip if 0); with `--entity 'entityId("HOST-0000000000000000")'` → 0 records.
  - `get problems --status OPEN -o json` → every problem's status is `OPEN`.
  - `query metrics --metric builtin:host.cpu.usage --from now-1h -o json` → exit 0 (skip the data assertion when the result is empty).
- [ ] **Step 3: Cluster tests** (`DTMGD_IT_CLUSTER`): `get environments`, `get nodes`, `describe cluster`, `describe backup` → exit 0 with valid JSON; `describe license -o json` → `licenseKey` starts with `****` (assert without printing the value; the failure message says only "licenseKey is not masked").
- [ ] **Step 4: Wiring.** `.integrationtests.env.example` (names only, commented: "tokens stay in your dtmgd config or keyring"); `.gitignore`; `make integration`: `set -a; [ -f .integrationtests.env ] && . ./.integrationtests.env; set +a; go test -tags integration -count=1 -v ./test/integration/...`; `make lint` and `lint.yml` run `go vet -tags integration ./test/...`; `.golangci.yml` builds with the tag.
- [ ] **Step 5:** `go vet -tags integration ./test/...` and the lint gate clean; `go test -tags integration ./test/integration/` with no env → all skip except the harness self-test, exit 0.
- [ ] **Step 6: Docs.** CONTRIBUTING.md and AGENTS.md "Live integration tests" (what it checks, how to configure, that it is read-only and never in CI); KB entry; the "Undocumented filter parameters" entry points at the suite for re-checking.
- [ ] **Step 7:** commit `test: opt-in live integration suite (read-only, never in CI)`.
- [ ] **Step 8 (owner go-ahead first):** run `make integration` against prod (and the prod cluster context); report pass/skip counts only.

### Task 5: Colors in human output

**Files:**
- Create: `pkg/output/color.go`, `pkg/output/console_windows.go`, `pkg/output/console_other.go`, `pkg/output/color_test.go`
- Modify: `pkg/output/output.go` (prefixes), `pkg/output/table.go` (bold headers, `status` tag option), `cmd/app.go` (decide colors), `cmd/doctor.go`, `cmd/get_problems.go`, `cmd/get_slos.go` (tag option), `docs/output.md`, `docs/agent-mode.md` if it describes `--plain`, README global flags, `--plain` help text, `make docs`

**Interfaces:**
- Produces: `output.SetColor(stderr bool)`; `output.Colorize(code, s string) string`; `output.EnableVT(w io.Writer) bool` (Windows: `SetConsoleMode` with `ENABLE_VIRTUAL_TERMINAL_PROCESSING` on an `*os.File` console; true for anything else); `TablePrinter.Color bool`; table tag option `status` (`table:"STATUS,status"`).

Rules (the architecture's guideline 5, dtctl's colors):
- stdout color = not plain (`--plain`, `NO_COLOR`, non-terminal stdout without `FORCE_COLOR` all mean plain) and `EnableVT(stdout)`; only the table and wide printers use it.
- stderr color = not plain and (stderr is a terminal or `FORCE_COLOR` is set) and `EnableVT(stderr)`.
- Agent mode: no color anywhere.
- Glyphs: `✗` bold red, `⚠` bold yellow, `✓` bold green; `💡` unchanged. Plain mode keeps the words (`Error:`, `Warning:`, `Hint:`) uncolored.
- Table: headers bold; a `status` column colors `ok`, `SUCCESS`, `HEALTHY`, `RESOLVED`, `CLOSED` green, `warn`, `WARNING` yellow, `fail`, `FAILURE`, `OPEN` red (case-insensitive), anything else uncolored. Applied to doctor STATUS, `get problems` STATUS and `get slos` STATUS.

- [ ] **Step 1: Failing tests** in `pkg/output/color_test.go`: with color on, `PrintHumanError("x")` writes `"\x1b[1;31m✗\x1b[0m x\n"`; plain wins over color; table with color on: headers wrapped in bold, `OPEN` red, `UNKNOWN` plain; **stripping escape codes from the colored table gives exactly the uncolored table** (Review Focus 1); csv printer with color on emits no `\x1b`.
- [ ] **Step 2: Failing command tests** (`runCLIWithEnv`): `FORCE_COLOR=1 get problems` → stdout has `\x1b[1m` and the red `OPEN`; `FORCE_COLOR=1 -o csv`, `-o json` and agent mode (`CLAUDECODE=1`) → no `\x1b` in stdout (Review Focus 2); `FORCE_COLOR=1` with an error → stderr has the red `✗`; `NO_COLOR=1 FORCE_COLOR=1` → no `\x1b` anywhere; without `FORCE_COLOR` (non-terminal writers) → goldens unchanged.
- [ ] **Step 3: Implement** to the rules above; port dtctl's `enableVTOnHandle` (Apache-2.0, same organisation) for Windows.
- [ ] **Step 4:** `go test ./...` PASS, `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` clean, goldens unchanged (`git status cmd/testdata` clean).
- [ ] **Step 5: Docs:** docs/output.md (colors, when they are off, `NO_COLOR`/`FORCE_COLOR`), the `--plain` flag help ("no colors, symbols or screen control"), README global flags; `make docs`.
- [ ] **Step 6:** commit `feat: colors in human output, off with --plain, NO_COLOR or a pipe`.

### Task 6: Document the extra verbs as intentional

**Files:** Modify: `README.md` (Commands section), `cmd/docs_links_test.go` (or a new `cmd/verbs_doc_test.go`)

- [ ] **Step 1: Failing test** `TestEveryTopLevelCommandIsInTheREADME`: every non-hidden top-level command of `newRootCmd` except `help` appears as `` `name` `` in the README's Commands table, and the README says the non-standard verbs are intentional (assert the sentence's key phrase `intentional extensions`).
- [ ] **Step 2:** run it: FAIL on the phrase.
- [ ] **Step 3:** add the paragraph under the table: `get` and `describe` are the portfolio's standard verbs; `query`, `doctor`, `commands`, `skills`, `config`/`ctx`, `version` and `completion` are intentional extensions, the same ones dtctl has; `query` reads time series and logs, which are not resources; `create`, `edit`, `delete` and `apply` are absent because dtmgd is read-only.
- [ ] **Step 4:** PASS; commit `docs: say which verbs are intentional extensions of the portfolio grammar`.

### Finish

- Fresh reviewer (most capable model) over the whole branch, then one fix pass test-first.
- PR 1 = Tasks 1–4, PR 2 = Tasks 5–6 (branch `feat/colors-and-verbs` cut after PR 1's commits). Ask the owner before each push and PR; check CI and the SonarCloud gate on each.
- No release tag until `HOMEBREW_TAP_APP_ID` and `HOMEBREW_TAP_APP_PRIVATE_KEY` exist in dtmgd.
