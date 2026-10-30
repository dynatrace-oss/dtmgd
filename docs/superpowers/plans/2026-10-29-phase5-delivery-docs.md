# Phase 5: Delivery and Documentation Practices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Signed, SBOM-carrying, test-gated releases; lean CI with least-privilege tokens; install scripts for hosts without Homebrew or Go; and docs where AGENTS.md is the contributor guide, the command reference is generated, and SKILL.md is short.

**Architecture:** Two PRs, then a release. PR 1 (Tasks 1–5) changes delivery: version reporting, goreleaser and workflows, install scripts. PR 2 (Tasks 6–9) changes docs only, except for the skill's embed directive. Freshness and link checks run as Go tests, so the required `test` checks enforce them on all three OSes without a new required check. Task 10 (Homebrew tap) waits on an org action. Task 11 is the release.

**Tech Stack:** Go 1.26.10, Cobra (+ `github.com/spf13/cobra/doc`, tests only), GoReleaser v2.19.0, cosign (sigstore/cosign-installer v4.1.2), syft (anchore/sbom-action v0.24.3), golangci-lint v2.15.0, actionlint v1.7.12 (tools module), POSIX sh, PowerShell.

**Spec:** `docs/superpowers/specs/2026-10-29-dtmgd-catch-up-design.md`, section "Phase 5 — Delivery and documentation practices", plus "Owner decisions (2026-10-29)".

## Global Constraints

- Releases stay tag-triggered: the owner pushes the tag or approves the agent pushing it. No release-please, no bot-opened PRs (spec, Owner decisions).
- Get the owner's go-ahead at the time of every push, PR, merge, tag, ruleset or secret change, and anything in another repo. Read-only `gh api` GETs are fine.
- The required check names must not change: `test (ubuntu-latest)`, `test (macos-latest)`, `test (windows-latest)`, `lint`, `golangci-lint`, `vulncheck`. These are the job ids `test`, `lint`, `golangci-lint` and `vulncheck`.
- Pin every action to a full commit SHA with a `# vX.Y.Z` comment. Never add `go install …@version` to a workflow (SonarCloud S8545): CI Go tools come from the checksum-locked `tools/` module, and other tools from pinned action inputs.
- Never preset `GOTOOLCHAIN` in a workflow (KB: setup-go ignores the toolchain line).
- Pinned versions: Go `1.26.10` (from go.mod via `go-version-file`), golangci-lint `v2.15.0`, GoReleaser `v2.19.0`, actionlint `v1.7.12`.
- No real host names, environment IDs or tokens in any file. Examples use `https://cluster.example.com`, `https://node.example.com:9999` and synthetic UUIDs.
- Tests never touch the default config path (`--config` with a temp file). HTTP error tests use 400/403, never 5xx.
- Local test signal: `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/... ./pkg/version/... ./skills/... .` (`./pkg/skills` fails locally by design, see KB).
- Lint gate before each commit (mirrors CI): `go vet ./...`, `test -z "$(gofmt -l .)"`, golangci-lint v2.15.0 exit code 0, `go mod tidy` and `go -C tools mod tidy` leave no diff. Chain the gate and the commit with `&&`, never `;`.
- Docs for whatever a task changes are updated in that task.
- Commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; PR bodies end with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.

## Review Focus

1. **`install.sh` under `sh` that is not bash:** dash on Ubuntu, bash in POSIX mode on macOS, BSD userland (`shasum` instead of `sha256sum`). A reasonable person expects the same result on both. Task 5's tests run under `sh` in the ubuntu and macOS `test` jobs, and `shellcheck` lints the script.
2. **A failed or tampered download never replaces an installed dtmgd.** Checksum mismatch, 404 and a truncated archive all exit non-zero and leave the install directory untouched. Task 5 has a test for each.
3. **A tag whose tests or govulncheck fail publishes nothing.** The `goreleaser` job `needs: test`. Task 3 checks this with actionlint and a grep; the reviewer confirms there is no path around it.
4. **Generated reference docs are byte-identical on Linux, macOS and Windows:** no CRLF, no OS path separators, no home-directory defaults. Task 6's freshness test runs in all three `test` jobs, and `.gitattributes` forces LF.
5. **An existing skill install (SKILL.md only) gains `references/` on `dtmgd skills install --force`** without losing the user's own files. `pkg/skills/force_test.go` already covers the installer with a `references/` tree; Task 7 adds an embed test proving the folder is actually embedded.

## PRs and review

- PR 1: branch `ci/phase5-delivery` from `main`, Tasks 1–5. PR 2: branch `docs/phase5-docs` from `main` after PR 1 merges, Tasks 6–9.
- Before each PR: a fresh reviewer on the most capable model reviews that branch, using the review package (`review-package PLAN BASE HEAD`). Tell it never to run destructive commands (no `go clean -modcache`, no `git clean`, no `rm -rf` outside its scratch dir) and never to read `~/.config/dtmgd/`. Re-grade its findings by effect. Fix the Important ones and the cheap minors test-first, then re-run the lint gate and the local test signal.
- After CI: check SonarCloud for the PR (`api/issues/search?componentKeys=dynatrace-oss_dtmgd&pullRequest=<n>&resolved=false` and `api/qualitygates/project_status?projectKey=dynatrace-oss_dtmgd&pullRequest=<n>`).

**Left out on purpose:** CodeQL, which the spec marks optional. gosec (through golangci-lint), govulncheck and the SonarCloud app already analyse the Go code, and another scanner adds CI time without adding coverage. The owner can ask for it.

---

### Task 1: `go install` builds report their module version

**Files:**
- Modify: `pkg/version/version.go`
- Test: `pkg/version/version_test.go` (new)

**Interfaces:**
- Produces: `fromBuildInfo(bi *debug.BuildInfo)` (unexported), which fills `Version`, `Commit` and `Date` only where they still hold their defaults (`"dev"`, `"none"`, `"unknown"`). An `init()` calls it with `debug.ReadBuildInfo()`. `cmd/version.go`, the `commands` catalog and the User-Agent keep reading the package variables unchanged.

- [ ] **Step 1: Write the failing tests**

Each test saves and restores the three variables with `t.Cleanup`, and sets them to the defaults first.

```go
func TestFromBuildInfoUsesTheModuleVersionOfGoInstall(t *testing.T) {
	// go install github.com/dynatrace-oss/dtmgd@v0.3.1: a module version, no VCS stamp.
	fromBuildInfo(&debug.BuildInfo{Main: debug.Module{Path: "github.com/dynatrace-oss/dtmgd", Version: "v0.3.1"}})
	// want Version "0.3.1", Commit "none", Date "unknown"
}

func TestFromBuildInfoUsesTheVCSStampOfALocalBuild(t *testing.T) {
	fromBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v0.3.1-0.20261029225836-ad0dc9391341+dirty"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ad0dc93913418699172fdc2631342fc45b60e62d"},
			{Key: "vcs.time", Value: "2026-10-29T22:58:36Z"},
		},
	})
	// want Version "0.3.1-0.20261029225836-ad0dc9391341+dirty", Commit "ad0dc93", Date "2026-10-29T22:58:36Z"
}

func TestFromBuildInfoKeepsDevWithoutAModuleVersion(t *testing.T) {
	// go test and go run report "(devel)"; some builds report "".
	for _, v := range []string{"(devel)", ""} { /* fromBuildInfo → Version stays "dev" */ }
}

func TestFromBuildInfoNeverOverridesLdflags(t *testing.T) {
	// GoReleaser sets all three through -ldflags; build info must not win.
	// preset Version "0.3.1", Commit "abc1234", Date "2026-10-30T00:00:00Z",
	// then fromBuildInfo with Version "v9.9.9" and other vcs values → all three unchanged
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./pkg/version/`
Expected: FAIL to compile, `undefined: fromBuildInfo`.

- [ ] **Step 3: Implement `fromBuildInfo` and the `init` in `pkg/version/version.go`**

Strip a leading `v` from `Main.Version`. Ignore `"(devel)"` and `""`. Shorten `vcs.revision` to 7 characters. Keep the existing comment about ldflags, and add one line saying that `go install` builds fall back to build info.

- [ ] **Step 4: Run the tests to see them pass, plus the local signal**

Run: `go test ./pkg/version/ && go test ./cmd/...`
Expected: PASS. Test binaries report `(devel)`, so `version.Version` stays `dev` and no golden changes.

- [ ] **Step 5: Check a real build**

Run: `go build -o "$SCRATCH/dtmgd" . && "$SCRATCH/dtmgd" version --no-agent`
Expected: `dtmgd <pseudo-version or tag without v> (commit: <7 hex>, built: <RFC 3339>)`, not `dtmgd dev`.

- [ ] **Step 6: Lint gate and commit**

```bash
git add pkg/version && git commit -m "fix(version): report the module version of go install builds"
```

---

### Task 2: Lint workflows with actionlint; `make lint` runs golangci-lint without `gofmt -w`

**Files:**
- Modify: `tools/go.mod`, `tools/go.sum`, `Makefile`, `.github/workflows/lint.yml`, and whichever workflow actionlint flags
- Test: verification commands (configuration, no Go code)

**Interfaces:**
- Produces: `make lint`, which runs `go vet`, a `gofmt -l` check that fails without rewriting, `golangci-lint run` and `./bin/actionlint`; and a `Lint workflows` step in the `lint` job. Tasks 3 and 4 verify their workflow edits with `./bin/actionlint`.

- [ ] **Step 1: Add actionlint to the tools module**

Run: `go -C tools get -tool github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 && go -C tools mod tidy && grep '^go ' tools/go.mod`
Expected: `go 1.26.10`. If the `go` line rises above the root's, stop and record a ruling: bump the root first, as the KB tools entry says.

- [ ] **Step 2: Get shellcheck locally**

actionlint runs shellcheck on `run:` blocks when shellcheck is on PATH, and the ubuntu runners have it.

Download shellcheck `v0.11.0` (koalaman/shellcheck release, linux x86_64) into `$SCRATCH/bin` and put that directory on PATH for this phase.

- [ ] **Step 3: Run actionlint on the current workflows**

Run: `go -C tools build -o "$PWD/bin/actionlint" github.com/rhysd/actionlint/cmd/actionlint && ./bin/actionlint; echo "exit=$?"`
Expected: the findings it reports (likely shellcheck SC2086 on unquoted `$GITHUB_OUTPUT` in `build.yml`), exit 1.

If it reports nothing, prove that it can fail: copy `test.yml` to `.github/workflows/zz-broken.yml` with `needs: nope`, run it (exit 1), and delete the copy.

- [ ] **Step 4: Fix each finding with the smallest change**

`build.yml` is rewritten in Task 4, so fixing its quoting now costs a line or two.

- [ ] **Step 5: Write the failing check for `make lint`**

Create `zz_unformatted.go` at the repo root containing `package main\nfunc  zz() {}`.

Run: `make lint; echo "exit=$?"; cat zz_unformatted.go`
Expected: today `make lint` runs `gofmt -w`, so the file comes back reformatted (the double space is gone), and the target exits 0.

- [ ] **Step 6: Rewrite the Makefile's `lint` target**

```make
# Keep in step with .github/workflows/sast.yml.
GOLANGCI_LINT_VERSION := v2.15.0

lint: vet
	@UNFORMATTED=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED" ]; then echo "Unformatted files (run make fmt):"; echo "$$UNFORMATTED"; exit 1; fi
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: install $(GOLANGCI_LINT_VERSION) (https://golangci-lint.run/docs/welcome/install/)"; exit 1; }
	golangci-lint run
	go -C tools build -o "$(CURDIR)/bin/actionlint" github.com/rhysd/actionlint/cmd/actionlint
	./bin/actionlint
```

`fmt` stays as the explicit `gofmt -w .` target. Add `lint` and `fmt` to `.PHONY` if missing.

- [ ] **Step 7: Run the check to see it pass**

Re-create `zz_unformatted.go` with the double space, with the scratchpad golangci-lint on PATH.

Run: `make lint; echo "exit=$?"; grep -c 'func  zz' zz_unformatted.go`
Expected: `Unformatted files (run make fmt):` naming `zz_unformatted.go`, exit 2, and the grep prints `1` (the file is unchanged). Then `rm zz_unformatted.go && make lint` exits 0.

- [ ] **Step 8: Add the CI step**

In `.github/workflows/lint.yml`, add this step to the `lint` job after `Run go vet`:

```yaml
      # actionlint is pinned and checksum-locked in tools/go.mod and go.sum.
      # The runner has shellcheck, so run: blocks are checked too.
      - name: Lint workflows
        run: |
          go -C tools build -o "$RUNNER_TEMP/actionlint" github.com/rhysd/actionlint/cmd/actionlint
          "$RUNNER_TEMP/actionlint"
```

- [ ] **Step 9: Lint gate and commit**

```bash
git add tools Makefile .github/workflows && git commit -m "ci: lint workflows with actionlint; make lint runs golangci-lint and no longer rewrites files"
```

---

### Task 3: Release pipeline: signing, SBOMs, test gate, `goreleaser check`

**Files:**
- Modify: `.goreleaser.yaml`, `.github/workflows/release.yml`, `.github/workflows/lint.yml`, `Makefile`, `README.md` (Installation), `SECURITY.md`
- Test: `goreleaser check`, a snapshot release and actionlint

**Interfaces:**
- Consumes: `./bin/actionlint` (Task 2).
- Produces: release assets `checksums.txt.sigstore.json` (cosign bundle over `checksums.txt`) and `<archive>.sbom.json` (SPDX JSON) for each archive. Task 5's install scripts and Task 11's smoke test rely on these names. A non-required job `goreleaser-check` is added to `lint.yml`.

- [ ] **Step 1: Run the failing check**

Run: `goreleaser check; echo "exit=$?"`
Expected: `DEPRECATED: homebrew_casks.hooks.post.install`, `check failed`, non-zero exit.

- [ ] **Step 2: Edit `.goreleaser.yaml`**

- `before.hooks`: delete `- go mod tidy`. The lint job already fails on an untidy module, and a release must not change the tree it builds.
- `homebrew_casks[0].hooks`: replace `post.install` with the steps DSL. Homebrew merged `postflight_steps` on 2026-08-05 (Homebrew/brew#23366), and every 7.0.x release has it.

```yaml
    hooks:
      post:
        install_steps: |
          on_macos do
            run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{ .StagedPath }}/dtmgd"]
          end
```

- After `checksum:`, add dtctl's blocks unchanged apart from the comment:

```yaml
signs:
  # cosign v3 writes one Sigstore bundle; checksums.txt is signed into
  # checksums.txt.sigstore.json. Verification is in README (Verifying a download).
  - cmd: cosign
    artifacts: checksum
    signature: "${artifact}.sigstore.json"
    args: ["sign-blob", "--yes", "--bundle=${signature}", "${artifact}"]

sboms:
  - artifacts: archive
    cmd: syft
    args: ["${artifact}", "--output", "spdx-json=${document}"]
```

- `changelog.filters.exclude`: add `'^build\(deps\)'` and `'^chore\(deps\)'`.

- [ ] **Step 3: Run the check to see it pass**

Run: `goreleaser check`
Expected: `1 configuration file(s) validated`, exit 0.

- [ ] **Step 4: Run a snapshot release**

Run: `HOMEBREW_TAP_GITHUB_TOKEN=unused goreleaser release --snapshot --clean --skip=publish,sign,sbom && tar tzf dist/dtmgd_*_linux_amd64.tar.gz && grep -n 'postflight' dist/homebrew/Casks/dtmgd.rb`

Expected:
- The archive lists `dtmgd`, `LICENSE`, `README.md`, `SKILL.md` and `completions/dtmgd.{bash,zsh,fish}`.
- The cask has `postflight_steps do` and no bare `postflight do`.

If goreleaser writes no cask under `--skip=publish`, record a ruling and rely on Step 3 plus the release in Task 11.

Then run `rm -rf dist completions`.

- [ ] **Step 5: Rewrite `.github/workflows/release.yml`**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions: {}

jobs:
  # The tagged commit passed CI on main, but nothing stops a tag on another
  # commit; a release is never built from code that fails its tests or has a
  # reachable vulnerability.
  test:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
      - name: Test
        run: go test -race ./...
      # govulncheck is pinned and checksum-locked in tools/go.mod and go.sum.
      - name: Vulnerability check
        run: |
          go -C tools build -o "$RUNNER_TEMP/govulncheck" golang.org/x/vuln/cmd/govulncheck
          "$RUNNER_TEMP/govulncheck" ./...

  goreleaser:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write # cosign keyless signing
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
        with:
          fetch-depth: 0
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
      - name: Install cosign
        uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
      - name: Install syft
        uses: anchore/sbom-action/download-syft@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7
        with:
          version: v2.19.0 # keep in step with lint.yml and build.yml
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

- [ ] **Step 6: Add the `goreleaser-check` job to `lint.yml`**

```yaml
  # Not filtered on .goreleaser.yaml: a new GoReleaser can deprecate a
  # property this repo uses, and check exits non-zero on a deprecation.
  goreleaser-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7
        with:
          version: v2.19.0 # keep in step with release.yml
          args: check
```

Also add a Makefile target `release-check: ; goreleaser check` next to `release-dry`.

- [ ] **Step 7: Validate the workflows**

Run: `./bin/actionlint && grep -n 'needs: test' .github/workflows/release.yml`
Expected: actionlint exits 0, and the grep finds the `goreleaser` job's `needs`.

- [ ] **Step 8: Document verification**

- README, Installation: add a "Verifying a download" subsection. It shows `sha256sum --ignore-missing -c checksums.txt` (macOS: `shasum -a 256 --ignore-missing -c checksums.txt`), then:

```bash
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/dynatrace-oss/dtmgd/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

  Say that releases from v0.3.1 onwards are signed and carry an SPDX SBOM per archive (`<archive>.sbom.json`).
- `SECURITY.md`, "Using dtmgd securely": add an item, "Verify what you download", pointing at that README subsection.

- [ ] **Step 9: Lint gate and commit**

```bash
git add .goreleaser.yaml .github/workflows Makefile README.md SECURITY.md && git commit -m "ci(release): sign checksums with cosign, attach SBOMs, gate on tests; check the goreleaser config in CI"
```

---

### Task 4: CI consolidation and repository hygiene

**Files:**
- Delete: `.github/workflows/sonar.yml`, `sonar-project.properties`
- Modify: `.github/workflows/{build,test,lint,sast,security,snyk}.yml`, `.github/dependabot.yml`, `scripts/drift-check.sh`, `.gitignore`
- Test: actionlint, greps, a drift-check run, `git check-ignore`

**Interfaces:**
- Consumes: `./bin/actionlint` (Task 2) and the `v2.19.0` pins (Task 3).
- Produces: a drift-report section `## Pinned CI tools`.

- [ ] **Step 1: Delete the Sonar scanner workflow and its scanner-only config**

Run: `git rm .github/workflows/sonar.yml sonar-project.properties`

The SonarCloud app reads `.sonarcloud.properties`, which stays (KB). Tests then run once per OS: three runs instead of four.

- [ ] **Step 2: Turn `build.yml` into one snapshot build on `main`**

Six targets per PR go away.

```yaml
name: Build

# Cross-compiles every release target with the release's own GoReleaser config,
# on main only. PRs get goreleaser-check (lint.yml) and the tests on three OSes.
on:
  push:
    branches: [main]
  workflow_dispatch: {}

permissions:
  contents: read

jobs:
  snapshot:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
        with:
          fetch-depth: 0
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7
        with:
          version: v2.19.0 # keep in step with release.yml
          args: build --snapshot --clean
```

Run: `goreleaser build --snapshot --clean && ls dist/*/dtmgd* | wc -l && rm -rf dist completions`
Expected: `6` binaries, built without `HOMEBREW_TAP_GITHUB_TOKEN` set. If `build` needs it, add the same `env:` as `release.yml` and record a ruling.

- [ ] **Step 3: Least-privilege tokens**

Add `permissions:\n  contents: read` at the top level of `test.yml`, `lint.yml`, `sast.yml`, `security.yml` and `snyk.yml`. `drift.yml` and `release.yml` already declare theirs.

Run: `grep -L '^permissions:' .github/workflows/*.yml`
Expected: no output.

- [ ] **Step 4: Pin golangci-lint**

In `sast.yml`, replace `version: latest` with `version: v2.15.0 # keep in step with GOLANGCI_LINT_VERSION in the Makefile`.

Run: `grep -rn 'version: latest\|~> v2' .github/workflows`
Expected: no output.

- [ ] **Step 5: Group Dependabot updates**

Give each of the three entries a `groups:` block:
- gomod `/`: group `go-modules` with `update-types: [minor, patch]`, so a major bump still gets its own PR.
- gomod `/tools`: group `tools` with `patterns: ["*"]`.
- github-actions: group `actions` with `patterns: ["*"]`.

Keep the existing `allow` and the comments.

- [ ] **Step 6: Check that the required job ids are intact**

Run: `grep -n '^  test:\|^  lint:\|^  golangci-lint:\|^  vulncheck:' .github/workflows/*.yml && ./bin/actionlint`
Expected: four matches (test.yml, lint.yml, sast.yml, security.yml), and actionlint exits 0.

- [ ] **Step 7: Make the drift check watch the pinned tools**

Dependabot cannot see a `version:` input, so the drift check must.

In `scripts/drift-check.sh`, add a `## Pinned CI tools` section before the govulncheck section. For each of these, compare every pinned version found with the latest GitHub release (`https://api.github.com/repos/<repo>/releases/latest`, `.tag_name`, with `Authorization: Bearer $GITHUB_TOKEN` when it is set):
- golangci-lint: the `version:` in `sast.yml` and `GOLANGCI_LINT_VERSION` in the Makefile, against `golangci/golangci-lint`.
- GoReleaser: the `version:` lines carrying the `keep in step with` comment in `release.yml`, `lint.yml` and `build.yml`, against `goreleaser/goreleaser`.

Pins that disagree with each other, or one behind the latest release, `raise 1`. An unreachable API, `raise 2`. Pass `GITHUB_TOKEN: ${{ github.token }}` to the `Check for drift` step in `drift.yml`.

- [ ] **Step 8: See it flag a stale pin, then pass**

Run: `sed -i 's/version: v2.15.0/version: v2.14.0/' .github/workflows/sast.yml && GOVULNCHECK=true bash scripts/drift-check.sh; echo "exit=$?"; git checkout .github/workflows/sast.yml`
Expected: the `## Pinned CI tools` section flags golangci-lint (pins disagree, behind `v2.15.0`), exit ≥ 1.

Run it again unmodified: the section says every pin is current. The exit can still be 1 if go.dev or the modules show drift; read the section, not only the exit code.

- [ ] **Step 9: Fix `.gitignore`**

Run before the change: `git check-ignore -q .dtmgd.yaml; echo $?; git check-ignore -q pkg/x/go/y.go; echo $?`
Expected: `1` (not ignored: a plaintext token could be committed), then `0` (any nested `go/` directory is silently ignored).

Edit: add `.dtmgd.yaml` under "Config files (might contain tokens)", and change `go/` to `/go/`.

Re-run. Expected: `0`, then `1`.

- [ ] **Step 10: Lint gate and commit**

```bash
git add -A .github scripts .gitignore && git commit -m "ci: drop the sonar scanner job, build release targets on main only, least-privilege tokens, pin golangci-lint, group Dependabot"
```

---

### Task 5: `install.sh` and `install.ps1`

**Files:**
- Create: `install.sh`, `install.ps1`, `install_test.go` (package `main`, repo root)
- Modify: `README.md` (Installation), `.github/workflows/lint.yml` (shellcheck), `Makefile` (`lint` runs shellcheck when present)

**Interfaces:**
- Consumes: the release asset names `dtmgd_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) and `checksums.txt`, as `.goreleaser.yaml` writes them.
- Produces: the scripts' environment interface, documented in README and in each script's header:
  - `DTMGD_VERSION`: `0.3.1` or `v0.3.1`. Default: the latest release, from `https://api.github.com/repos/dynatrace-oss/dtmgd/releases/latest`, authenticated with `GITHUB_TOKEN` when set.
  - `DTMGD_INSTALL_DIR`: default `~/.local/bin` on Linux, `/usr/local/bin` on macOS, `%LOCALAPPDATA%\dtmgd` on Windows.
  - `DTMGD_DOWNLOAD_BASE`: default `https://github.com/dynatrace-oss/dtmgd/releases/download`. A mirror laid out as `<base>/v<version>/<asset>`, for jump hosts that reach an internal mirror but not GitHub.
  - `DTMGD_NO_MODIFY_PATH=1`: install.ps1 only; leave the user PATH alone.

Behaviour of both scripts:
1. Download the archive and `checksums.txt` into a temp dir.
2. Verify the archive's SHA-256 against its line in `checksums.txt` (sh: `sha256sum`, else `shasum -a 256`, else fail; ps1: `Get-FileHash`).
3. Extract in the temp dir, and only then move `dtmgd`/`dtmgd.exe` into the install directory, replacing an old one. sh uses `sudo mv` only when the directory is not writable.
4. On macOS, remove the quarantine attribute.
5. Run `dtmgd version`, and print a PATH hint (sh) or add the directory to the user PATH (ps1).

Any failure prints `Error: …` to stderr, exits non-zero, and leaves the install directory as it was.

- [ ] **Step 1: Write the failing tests in `install_test.go`**

Helpers:
- `fakeRelease(t, version string, tamper func(name string, data []byte) []byte) string` builds a stand-in binary: a temp module `module fakedtmgd` whose `main` prints `dtmgd <version>` when called with `version`, built with `CGO_ENABLED=0` for `runtime.GOOS`/`runtime.GOARCH`.
  - It packs the binary as `dtmgd` (tar.gz) or `dtmgd.exe` (zip, on Windows) with a `LICENSE` beside it, under the real asset name.
  - It writes `checksums.txt`, serves both from an `httptest.Server` at `/v<version>/…`, and returns the server URL.
  - `tamper` may rewrite a file's bytes after the checksum is computed.
- `runInstallSh(t, base, version, dir string) (stdout, stderr string, code int)` runs `sh install.sh` with `DTMGD_DOWNLOAD_BASE`, `DTMGD_VERSION`, `DTMGD_INSTALL_DIR`, and `HOME` set to a temp dir.

Tests (each `t.Skip` on Windows):

```go
func TestInstallShInstallsAVerifiedBinary(t *testing.T) {
	// fakeRelease(t, "0.0.0-test", nil); run → code 0; <dir>/dtmgd exists and is executable;
	// running "<dir>/dtmgd version" prints "dtmgd 0.0.0-test"; stdout names the install path
}
func TestInstallShRefusesAChecksumMismatch(t *testing.T) {
	// tamper flips a byte of the archive → code != 0; stderr contains "checksum"; <dir>/dtmgd absent
}
func TestInstallShFailsWhenTheAssetIsMissing(t *testing.T) {
	// version "0.0.0-missing" (the server returns 404) → code != 0; stderr starts "Error:"; nothing installed
}
func TestInstallShKeepsTheOldBinaryWhenTheArchiveIsTruncated(t *testing.T) {
	// pre-create <dir>/dtmgd with "old"; tamper truncates the archive AND checksums.txt matches the truncated bytes
	// → code != 0 (extraction fails); <dir>/dtmgd still contains "old"
}
func TestInstallShReplacesAnExistingBinary(t *testing.T) {
	// pre-create <dir>/dtmgd with "old" → code 0; "<dir>/dtmgd version" prints the new version
}
func TestInstallPs1InstallsAVerifiedBinary(t *testing.T) {
	// skip unless runtime.GOOS == "windows" and pwsh is on PATH;
	// pwsh -NoProfile -File install.ps1 with DTMGD_NO_MODIFY_PATH=1 and the same env;
	// <dir>\dtmgd.exe prints the version
}
func TestInstallPs1RefusesAChecksumMismatch(t *testing.T) { /* Windows + pwsh only; no dtmgd.exe installed */ }
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -run 'TestInstall' -v .`
Expected: FAIL. `install.sh` does not exist (`sh: install.sh: No such file`); the ps1 tests SKIP on Linux.

- [ ] **Step 3: Write `install.sh`**

POSIX `sh`, `set -eu`, a `trap` that removes the temp dir. Follow dtctl's `install.sh` for OS/arch detection and the PATH hint, but add the checksum step and the environment interface above. Header usage line: `curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.sh | sh`. The closing hint suggests `dtmgd ctx set` and `dtmgd doctor`, not dtctl's auth login.

- [ ] **Step 4: Run the tests to see them pass, then shellcheck**

Run: `go test -run 'TestInstall' -v . && shellcheck install.sh`
Expected: the five sh tests PASS (ps1 tests SKIP), and shellcheck prints nothing.

- [ ] **Step 5: Write `install.ps1`**

Follow dtctl's `install.ps1` for architecture detection, `Resolve-LongPath` and the PATH update, with these differences:
- Verify the zip with `Get-FileHash -Algorithm SHA256` against `checksums.txt`.
- Expand into a temp dir and copy only `dtmgd.exe` into the install dir.
- Honour `DTMGD_VERSION`, `DTMGD_INSTALL_DIR`, `DTMGD_DOWNLOAD_BASE` and `DTMGD_NO_MODIFY_PATH`.
- Usage: `irm https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.ps1 | iex`.

The ps1 tests run only in the Windows `test` job. If `pwsh` is available locally, also run `pwsh -NoProfile -Command '$null = [scriptblock]::Create((Get-Content -Raw install.ps1))'` for a parse check.

- [ ] **Step 6: Add shellcheck to lint**

- `lint.yml`, `lint` job: add a step `Check install.sh` that runs `shellcheck install.sh` (preinstalled on ubuntu runners).
- Makefile `lint`: add `@if command -v shellcheck >/dev/null; then shellcheck install.sh; else echo "shellcheck not found; skipping install.sh"; fi`.

- [ ] **Step 7: Document it**

In the README's Installation section, add "Option: install script (Linux, macOS, Windows)" after Homebrew. It shows both one-liners, says the script verifies the archive against `checksums.txt`, and lists the four variables. Note that `go install …@latest` now reports its version.

- [ ] **Step 8: Run the local signal and the lint gate, then commit**

```bash
git add install.sh install.ps1 install_test.go README.md Makefile .github/workflows/lint.yml && git commit -m "feat: install.sh and install.ps1 with checksum verification"
```

**PR 1 checkpoint:**
1. Run the whole-branch review (see "PRs and review").
2. Fix the findings that are worth fixing.
3. Push `ci/phase5-delivery` with the owner's go-ahead.
4. Open the PR with `gh` as the owner, titled "Phase 5 (1/2): signed releases, CI consolidation, install scripts".
5. After merge, suggest that the owner adds `goreleaser-check` to the `main` ruleset's required checks (a ruleset change, the owner's call).

---

### Task 6: Command reference generated from Cobra, with a freshness test

**Files:**
- Create: `cmd/docs_test.go`, `docs/reference/*.md` (generated)
- Modify: `go.mod`/`go.sum` (`go mod tidy` adds `cpuguy83/go-md2man/v2` and `russross/blackfriday/v2`, which are already in Snyk's module graph through cobra), `Makefile` (`docs` target), `.gitattributes`

**Interfaces:**
- Consumes: `newRootCmd(a *App) *cobra.Command` and the `update` flag in `cmd/harness_test.go`.
- Produces: `docs/reference/dtmgd.md` and one `dtmgd_<path>.md` per visible command. Task 9's README and docs pages link to them. `make docs` regenerates.

- [ ] **Step 1: Write the failing tests in `cmd/docs_test.go`**

```go
// genReferenceDocs writes the reference into dir: newRootCmd(&App{Out: io.Discard, Err: io.Discard}),
// DisableAutoGenTag on every command (no dates), doc.GenMarkdownTree.
func genReferenceDocs(dir string) error

// diffDirs names every file that differs between want and got: "missing: x.md", "extra: y.md", "changed: z.md".
func diffDirs(want, got string) ([]string, error)

func TestDiffDirsNamesMissingExtraAndChangedFiles(t *testing.T) {
	// want has a.md, b.md("1"); got has b.md("2"), c.md → ["changed: b.md", "extra: c.md", "missing: a.md"] (sorted)
}

func TestReferenceDocsAreCurrent(t *testing.T) {
	// genReferenceDocs(t.TempDir()); with -update, replace ../docs/reference with it (removing stale files);
	// otherwise diffDirs("../docs/reference", tmp) must be empty, and the failure says:
	// "docs/reference is stale (<diffs>): run make docs"
}

func TestReferenceDocsHaveNoMachineSpecificText(t *testing.T) {
	// genReferenceDocs output contains none of: os.UserHomeDir(), os.TempDir(), "\r\n", "%SCOPES%"
	// (skip a home or temp value shorter than 5 characters, such as "/", which any text contains)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./cmd/ -run 'TestDiffDirs|TestReferenceDocs'`
Expected: FAIL to compile (`undefined: genReferenceDocs`). Once the functions exist, `TestReferenceDocsAreCurrent` fails on `missing: dtmgd.md …`, because `docs/reference` does not exist yet.

- [ ] **Step 3: Implement `genReferenceDocs` and `diffDirs` in `cmd/docs_test.go`**

They are test code, so cobra/doc never reaches the binary. Run `go mod tidy`.

- [ ] **Step 4: Generate the docs and see the tests pass**

Add to the Makefile: `docs: ; go test ./cmd/ -run '^TestReferenceDocsAreCurrent$$' -update`. Add `docs/reference/** text eol=lf` to `.gitattributes` with a comment that the Windows runner checks out with autocrlf.

Run: `make docs && go test ./cmd/ -run 'TestDiffDirs|TestReferenceDocs' && go test ./cmd/...`
Expected: PASS, with `docs/reference/` holding `dtmgd.md`, `dtmgd_get_problems.md`, `dtmgd_describe_license.md` and the rest.

- [ ] **Step 5: Spot-check the output**

Run: `head -30 docs/reference/dtmgd_get_nodes.md && grep -rln 'Auto generated' docs/reference | wc -l`
Expected: the Long text and examples of `get nodes`, and `0`.

- [ ] **Step 6: Lint gate and commit**

```bash
git add cmd/docs_test.go docs/reference go.mod go.sum Makefile .gitattributes && git commit -m "docs: generate the command reference from Cobra; tests fail when it is stale"
```

---

### Task 7: SKILL.md slimmed, with a `references/` folder

**Files:**
- Create: `skills/dtmgd/references/{output,contexts,cluster-api,recipes,mcp-parity}.md`, `skills/dtmgd/embed_test.go`
- Modify: `skills/dtmgd/SKILL.md`, `skills/dtmgd/embed.go`, `.goreleaser.yaml` (archive files)

**Interfaces:**
- Produces: `dtmgdskill.Content` now embeds `SKILL.md` and `references/`. The installer already copies the whole embedded tree. `skills/dtmgd/references/mcp-parity.md` becomes the only copy of the MCP parity table; Task 9's README links to it.

- [ ] **Step 1: Write the failing tests in `skills/dtmgd/embed_test.go`**

```go
func TestContentEmbedsEveryReference(t *testing.T) {
	// every file in os.ReadDir("references") is readable from Content as "references/<name>";
	// fails while the directive embeds SKILL.md alone (and while references/ does not exist: require ≥ 1 file)
}
func TestEveryReferenceIsLinkedFromSkill(t *testing.T) {
	// SKILL.md contains "references/<name>" for each file in references/ (no orphans)
}
func TestSkillHasNoDemoContent(t *testing.T) {
	// no embedded file contains "bookstore" (case-insensitive)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./skills/...`
Expected: FAIL. `TestContentEmbedsEveryReference` finds no `references/` directory.

- [ ] **Step 3: Split SKILL.md**

Move sections verbatim, adjusting heading levels and adding a one-line intro per file:
- `references/output.md`: Output Formats, AI Agent Envelope Mode (details), Pagination, Watch Mode, Column Filtering, Time Format Reference.
- `references/contexts.md`: Context Management, Multi-Environment Queries.
- `references/cluster-api.md`: Cluster Management API.
- `references/recipes.md`: Problems, Entities, Events, Metrics, Logs, SLOs, Security Problems.
- `references/mcp-parity.md`: the parity section. Before deleting README's copy, check that it matches; if the two differ, keep the line that matches the code and record a ruling.

SKILL.md keeps the frontmatter (description unchanged, so triggering is unchanged), contexts, recommended initialization, the agent envelope essentials, global flags, exit codes, the typical investigation workflow and gotchas. It ends with a "References" list that says when to read each file. Target: 200 lines or fewer.

- [ ] **Step 4: Embed the folder and ship it in archives**

- `embed.go`: change the directive to `//go:embed SKILL.md references`, and update the package comment.
- `.goreleaser.yaml` archives `files`: add `- src: skills/dtmgd/references/*`, `dst: references`, `strip_parent: true`. SKILL.md stays at the archive root, so its relative links still work and the archive path of SKILL.md does not move.

- [ ] **Step 5: Run the tests to see them pass, then check the snapshot archive**

Run: `go test ./skills/... && wc -l skills/dtmgd/SKILL.md && HOMEBREW_TAP_GITHUB_TOKEN=unused goreleaser release --snapshot --clean --skip=publish,sign,sbom && tar tzf dist/dtmgd_*_linux_amd64.tar.gz | grep -E '^(SKILL.md|references/)' && rm -rf dist completions`
Expected: PASS, at most 200 lines, and the archive lists `SKILL.md` and the five `references/*.md`.

- [ ] **Step 6: Check the installed result in a temp project**

Run: `go build -o "$SCRATCH/dtmgd" . && (cd "$(mktemp -d -p "$SCRATCH")" && "$SCRATCH/dtmgd" skills install --for claude --no-agent && find .claude/skills/dtmgd -type f | sort)`
Expected: `SKILL.md` and the five references under `.claude/skills/dtmgd/`.

- [ ] **Step 7: Lint gate and commit**

```bash
git add skills .goreleaser.yaml && git commit -m "docs(skill): slim SKILL.md and move detail into references/"
```

---

### Task 8: Agent and contributor docs

**Files:**
- Create: `AGENTS.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `.github/ISSUE_TEMPLATE/{bug_report.yml,feature_request.yml,config.yml}`, `.github/pull_request_template.md`
- Modify: `.claude/CLAUDE.md`, `.github/copilot-instructions.md`, `README.md` (remove Architecture, API Endpoints and Building)
- Test: greps here; Task 9's link test then covers every file

**Interfaces:**
- Produces: `AGENTS.md`, the canonical guide that Task 9's README links to.

- [ ] **Step 1: Write `AGENTS.md`, aiming for about 250 lines**

Sources: `.github/copilot-instructions.md`, README's Architecture and API Endpoints, and the testing rules in the KB. Check every claim against the code as it stands. Sections:
1. **What dtmgd is:** read-only, Dynatrace Managed only, verb–noun.
2. **Build, test, lint:** make targets, the local test signal and why `./pkg/skills` is excluded, `make docs`.
3. **Architecture:** the package map. Endpoints: environment API `{host}/e/{env-id}/api/v2|v1`; Cluster API `{host}/api/cluster/v2/…` and `{host}/api/v1.0/onpremise/…`. Environment contexts vs cluster contexts (`IsCluster`, `ErrClusterScoped`).
4. **Adding a command:**
   - Environment commands use `runFetch(fetchSpec{…})`. Cluster commands use `runClusterFetch` plus `clusterCommand`.
   - Declare scopes with `withScopes` and update the README scope list.
   - Test with the harness and goldens.
   - Run `make docs`.
5. **Conventions:** table struct tags, pagination (`pagedFetch`, `nextPageKey` alone on later pages), multi-environment fan-out, agent envelope, errors (`withHint`, UsageError → exit 2), negative problem IDs.
6. **Testing rules:** 400/403 only, keyring `MockInit` in `TestMain`, never the default config path, constructors rather than package-level closures (coverage), and Sonar S3776 on tests.
7. **CI and releases:**
   - The required checks.
   - Tools from `tools/`; golangci-lint and GoReleaser pinned and watched by `drift.yml`.
   - Release steps: annotated tag; `release.yml` tests, signs and publishes; prepend notes; tap; verify with cosign.
8. **Rules for agents:**
   - Never print or commit token values; filter `token:` lines when showing a config.
   - Live calls are read-only.
   - Synthetic hosts and IDs in examples.
   - The owner approves every push, PR, merge and tag.
9. **Knowledge base:** lives in `.claude/CLAUDE.md`.

- [ ] **Step 2: Point the agent files at AGENTS.md**

- `.claude/CLAUDE.md`: replace everything above `## Knowledge Base` with a title, `@../AGENTS.md` (the import resolves relative to the file) and one line saying that the knowledge base below records lessons learned. Then correct these KB entries:
  - The pkg/skills entry: replace `pkg/skills/installer.go:327` with "`Status()` in `pkg/skills/installer.go`" (no line number to go stale).
  - The sonar entry: rewrite it as "SonarCloud findings come only from the app". The `sonar` job and `sonar-project.properties` were removed in Phase 5, and what gates a merge is unchanged.
  - The tools entry: add actionlint as a second `tool`, and say that golangci-lint and GoReleaser are action inputs pinned by version and watched by `scripts/drift-check.sh`.
  - The snyk and RunE claims the spec calls stale are already correct; verify, and leave them.
- `.github/copilot-instructions.md`: 10 lines or fewer: one sentence on what dtmgd is, and "Read AGENTS.md; skills/dtmgd/SKILL.md covers using the CLI."

- [ ] **Step 3: Contributor files**

- `CONTRIBUTING.md`, about 100 lines:
  - Prerequisites: Go 1.26.10, golangci-lint v2.15.0, make; shellcheck is optional.
  - Setup, and `make build|test|lint|docs`.
  - Conventional commit prefixes (`feat`, `fix`, `docs`, `ci`, `build`, `test`, `refactor`, `chore`). `build(deps)`/`chore(deps)` are left out of release notes.
  - PR expectations: required checks, regenerated docs, goldens reviewed.
  - Coding and testing standards, linking to AGENTS.md.
  - Security issues: see SECURITY.md.
  - Building from source (moved from README's Building section).
- `CODE_OF_CONDUCT.md`: dtctl's file unchanged (Contributor Covenant 2.0, enforcement contact opensource@dynatrace.com).
- `.github/ISSUE_TEMPLATE/bug_report.yml`:
  - Fields: what happened, steps to reproduce, `dtmgd version` output, OS, Dynatrace Managed version (`dtmgd doctor`).
  - A markdown notice: "Remove tokens, host names, environment IDs and license keys from anything you paste; `-vv` output masks the token and license key but not host names."
- `feature_request.yml`: problem, proposed command, alternatives.
- `config.yml`: `blank_issues_enabled: true`, with contact links to `https://github.com/dynatrace-oss/dtmgd/security/advisories/new` (private vulnerability reporting is enabled) and to the docs.
- `.github/pull_request_template.md`: summary, related issue, checklist (`make test`, `make lint`, `make docs` if commands changed, README scope list if scopes changed).

- [ ] **Step 4: Remove the moved sections from README**

Remove Architecture, API Endpoints and Building from README. Task 9 adds a Contributing link section.

- [ ] **Step 5: Verify**

Run: `head -5 .claude/CLAUDE.md; grep -c 'installer.go:327' .claude/CLAUDE.md; wc -l .github/copilot-instructions.md AGENTS.md CONTRIBUTING.md; grep -rnE 'https?://[a-z0-9.-]+(:9999)?' AGENTS.md CONTRIBUTING.md | grep -vE 'example\.com|github\.com|dynatrace\.com|golangci-lint\.run|contributor-covenant\.org|token\.actions'`
Expected:
- `@../AGENTS.md` near the top of CLAUDE.md.
- `0` for the stale line number.
- copilot-instructions at 10 lines or fewer.
- No real host names in the URL grep.

- [ ] **Step 6: Commit**

```bash
git add AGENTS.md CONTRIBUTING.md CODE_OF_CONDUCT.md .github README.md && git add -f .claude/CLAUDE.md && git commit -m "docs: AGENTS.md as the contributor guide; CONTRIBUTING, code of conduct, issue and PR templates"
```

`git add .claude/CLAUDE.md` without `-f` exits non-zero because `.claude/` is ignored, even though the file is tracked.

---

### Task 9: README restructured, topic pages in `docs/`, a link test

**Files:**
- Create: `docs/README.md`, `docs/configuration.md`, `docs/output.md`, `docs/agent-mode.md`, `docs_links_test.go` (package `main`, repo root)
- Modify: `README.md`

**Interfaces:**
- Consumes: `docs/reference/` (Task 6), `skills/dtmgd/references/mcp-parity.md` (Task 7), `AGENTS.md` and `CONTRIBUTING.md` (Task 8).
- Produces: `checkMarkdownLinks(root string, files []string) []string`, which returns each broken relative link as `file: target`.

- [ ] **Step 1: Write the failing tests in `docs_links_test.go`**

```go
// checkMarkdownLinks reports relative links in files (paths relative to root) whose target file does not
// exist, or whose #anchor names no heading in the target (GitHub slugs: lowercase, drop punctuation except
// '-' and '_', spaces → '-', repeated headings get -1, -2). Links inside fenced code blocks and
// http(s)/mailto links are ignored.
func checkMarkdownLinks(root string, files []string) []string

func TestCheckMarkdownLinksFindsBrokenFilesAndAnchors(t *testing.T) {
	// temp tree: a.md links b.md (exists), c.md (missing), b.md#real-heading (ok), b.md#nope (bad),
	// #local-heading (ok), https://x (ignored), and one inside ``` (ignored)
	// → exactly ["a.md: c.md", "a.md: b.md#nope"]
}

func TestRepositoryMarkdownLinksResolve(t *testing.T) {
	// files: `git ls-files --cached --others --exclude-standard -- '*.md'` minus docs/superpowers/**;
	// skip when git is unavailable; checkMarkdownLinks(".", files) must be empty
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test -run 'MarkdownLinks' .`
Expected: FAIL to compile (`undefined: checkMarkdownLinks`). After Step 3, the repository test may already pass on the current tree; the unit test is the RED that proves the checker catches breakage.

- [ ] **Step 3: Implement `checkMarkdownLinks`**

Run: `go test -run 'MarkdownLinks' -v .`
Expected: PASS for both. If the repository test fails, the failures are links broken by Tasks 6–8 or earlier; fix them.

- [ ] **Step 4: Restructure README to about 250 lines**

Move sections verbatim, fixing heading levels and links. Add no claim the tool doesn't back.

- **README.md keeps:**
  - Title and intro.
  - Installation: Homebrew, install script, go install, release binaries with "Verifying a download", shell completion.
  - Quick Start, including "Supplying the token in scripts and CI".
  - Authentication and Required API Token Scopes. The `<!-- scopes:start/end -->` markers stay, so `TestReadmeScopeListMatchesCommands` still passes.
  - A Commands overview: one table of verbs and resources, cluster commands marked, linking to `docs/reference/dtmgd.md` and `dtmgd commands`.
  - A short "Contexts" paragraph linking to `docs/configuration.md`.
  - A short "AI agents" paragraph linking to `docs/agent-mode.md`.
  - "Dynatrace Managed MCP server parity", one paragraph linking to `skills/dtmgd/references/mcp-parity.md`.
  - Documentation, Contributing, Security and License links.
- **`docs/configuration.md`:** Configuration commands and MCP config import, Cluster Contexts, Multi-Environment Queries, Configuration File Format, Environment variables in the config, Which config file is used.
- **`docs/output.md`:** Output, Global Flags (pointing at `reference/dtmgd.md` for the authoritative list), Exit Codes, Pagination, Watch Mode, Time Formats, Environment variables.
- **`docs/agent-mode.md`:** AI Agent Mode, and Install the dtmgd Skill.
- **`docs/README.md`:** an index of the three pages, `reference/`, and the skill's references.
- **Dropped from README, now in the generated reference:** the per-verb command example blocks (Get, Describe, Query, Doctor, Commands). Before deleting each, check that every example it shows is in that command's help; move any that is not into the command's `Long` and run `make docs`.

- [ ] **Step 5: Run the tests to see them pass**

Run: `go test -run 'MarkdownLinks' . && go test ./cmd/ -run 'TestReadmeScopeList|TestReferenceDocs' && wc -l README.md`
Expected: PASS, and README at about 250 lines.

- [ ] **Step 6: Run the local signal and the lint gate, then commit**

```bash
git add README.md docs docs_links_test.go cmd && git commit -m "docs: restructure README into topic pages; test that every relative link resolves"
```

**PR 2 checkpoint:**
1. Run the whole-branch review. Ask the reviewer to compare every moved claim in README, docs, SKILL.md and AGENTS.md with the code.
2. Fix the findings that are worth fixing.
3. Push `docs/phase5-docs` with the owner's go-ahead.
4. Open the PR with `gh` as the owner, titled "Phase 5 (2/2): AGENTS.md, generated reference, slimmer README and skill".

---

### Task 10: Homebrew tap through the org's GitHub App (blocked on an org action)

dtctl updates the tap with a GitHub App token: secrets `HOMEBREW_TAP_APP_ID` and `HOMEBREW_TAP_APP_PRIVATE_KEY`, scoped to `dynatrace-oss/homebrew-tap`, pushing directly to `main`. On 2026-10-29, dtmgd could see neither: no organization secrets, and repository secrets limited to `HOMEBREW_TAP_GITHUB_TOKEN`, `SNYK_*` and `SONAR_TOKEN`.

**Files:**
- Modify: `.github/workflows/release.yml`, `.goreleaser.yaml`, `.claude/CLAUDE.md` (the 403 entry), `AGENTS.md` (release steps)

- [ ] **Step 1: The owner asks the org admins**

The request: make the dtctl tap App's secrets available to `dynatrace-oss/dtmgd`, either by adding dtmgd to the organization secrets' repository access or by creating them as repository secrets. The App installation already covers `homebrew-tap`.

- [ ] **Step 2: Check (read-only)**

Run: `gh api repos/dynatrace-oss/dtmgd/actions/organization-secrets --jq '.secrets[].name'; gh api repos/dynatrace-oss/dtmgd/actions/secrets --jq '.secrets[].name'`
Expected: both `HOMEBREW_TAP_APP_ID` and `HOMEBREW_TAP_APP_PRIVATE_KEY` appear.

If they do not, stop here. Record `Task 10: deferred — org secrets not available`, keep the fork flow, and continue to Task 11.

- [ ] **Step 3: Switch the release to the App**

- In `release.yml`, add to the `goreleaser` job, before GoReleaser:

```yaml
      - name: Generate Homebrew tap token
        id: app-token
        uses: actions/create-github-app-token@64ab729a6294000f5aa4a4bc0b73d1f32bc45510 # v3.3.0
        with:
          app-id: ${{ secrets.HOMEBREW_TAP_APP_ID }}
          private-key: ${{ secrets.HOMEBREW_TAP_APP_PRIVATE_KEY }}
          owner: dynatrace-oss
          repositories: homebrew-tap
```

  Then set `HOMEBREW_TAP_GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}`.
- In `.goreleaser.yaml`, set `homebrew_casks[0].repository` to `owner: dynatrace-oss`, `name: homebrew-tap` and the token, and delete `branch:` and `pull_request:`, as dtctl does.

- [ ] **Step 4: Verify**

Run: `goreleaser check && ./bin/actionlint`
Expected: both exit 0.

- [ ] **Step 5: Update the docs, then commit**

Replace the KB entry "goreleaser's tap PR gets 403; open it by hand" with the App flow, and update AGENTS.md's release steps. This lands in whichever Phase 5 PR is open, or in its own PR.

After the first release that publishes through the App, the owner may:
- delete the `HOMEBREW_TAP_GITHUB_TOKEN` secret;
- close the stale tap PRs #10–#15;
- delete the `ivan-gudak/homebrew-tap` branches.

---

### Task 11: Release (each outward step needs the owner's go-ahead)

- [ ] **Step 1: Tag the merge commit of PR 2 and push the tag**

Suggest `v0.3.1`: no command changes, and a fix to version reporting. The owner picks the number.

```bash
git tag -a v0.3.1 -m "v0.3.1" <merge-sha>
git push origin v0.3.1
```

- [ ] **Step 2: Watch `release.yml`**

Expected:
- `test` passes, then `goreleaser` runs.
- 14 assets: 6 archives, 6 `.sbom.json`, `checksums.txt` and `checksums.txt.sigstore.json`.
- The tap step fails with the known 403 unless Task 10 landed.
- On a 408 upload timeout, follow the KB entry.

- [ ] **Step 3: Verify the release**

In a fresh `$SCRATCH/v0.3.1/`:
1. Download `checksums.txt`, the bundle, and `dtmgd_0.3.1_linux_amd64.tar.gz` with its `.sbom.json`.
2. Download cosign v3.1.4 for linux amd64 from `sigstore/cosign` and check it against that release's checksum file.

Run the README's `cosign verify-blob` command.
Expected: `Verified OK`.

Run: `sha256sum --ignore-missing -c checksums.txt`
Expected: `OK`.

Run: `jq -r .spdxVersion *.sbom.json`
Expected: `SPDX-2.3` (or the version syft writes).

- [ ] **Step 4: Smoke-test the install paths**

- **Install script:** `DTMGD_INSTALL_DIR="$SCRATCH/v0.3.1/bin" sh install.sh`, using the file on `main`. Expected: it prints `dtmgd 0.3.1 (commit: <sha7>, built: …)`.
- **go install:** `GOBIN="$SCRATCH/v0.3.1/gobin" go install github.com/dynatrace-oss/dtmgd@v0.3.1 && "$SCRATCH/v0.3.1/gobin/dtmgd" version`. Expected: `dtmgd 0.3.1 (commit: none, built: unknown)`.
- Then the archive smoke test with a temp `--config` and `--no-agent`, as in earlier releases.

- [ ] **Step 5: Publish notes and open the tap PR**

1. Prepend the notes with `gh release edit v0.3.1 --notes-file <notes>`. Cover signing and SBOMs, install scripts, `go install` version reporting, the CI changes, the docs moves (README sections now in `docs/`), and the deleted `sonar` job.
2. Unless Task 10 landed, open the tap PR by hand: `gh pr create --repo dynatrace-oss/homebrew-tap --head ivan-gudak:dtmgd-0.3.1 --base main`.

- [ ] **Step 6: Update the records**

Update `memory/catch-up-program-status.md`: Phase 5 shipped. Open threads: Task 10 if deferred; the stage-cluster live check; the live checks of the undocumented parameters.
