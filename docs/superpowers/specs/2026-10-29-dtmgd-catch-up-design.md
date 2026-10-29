# dtmgd catch-up: review findings and improvement design

Date: 2026-10-29
Status: draft for owner approval

## Intent

dtmgd was left alone for about four months because there was no AI budget for it. Nothing is broken badly enough to block anyone, and nothing is scheduled. The owner now has a limited budget and wants the project brought up to standard on four axes:

1. **Its own health:** code, tests, CI, releases, docs.
2. **The shared Dynatrace CLI architecture** in `../cli-tool-architecture/`. The owner's direction is explicit: *follow the architecture*. That includes covering the Cluster Management API.
3. **dtctl (`../dtctl/`)** as the model CLI for UX and engineering practice.
4. **Feature parity with the Dynatrace Managed MCP server** (`../dynatrace-managed-mcp/`).

Because the budget is limited, the work is ordered so that **each phase ends with a releasable `main` and a tagged release**. The owner can stop after any phase and be better off than today.

Who uses dtmgd (a guess, not confirmed): mostly AI agents driving it through the bundled skill, plus the owner at a terminal. That is why agent-facing correctness (envelope, limits, exit codes) ranks high.

## What the review found (summary)

Four read-only reviews ran on 2026-10-29. The headlines are below; evidence is at file:line.

**Security and safety (urgent)**
- **Unpatched Go in CI and releases.** Every workflow uses `go-version-file: go.mod`. `go.mod` says `go 1.26.0` with no `toolchain` line, because Dependabot PR #40 (99a4038) dropped `toolchain go1.25.12`. CI and goreleaser therefore build with Go 1.26.0. govulncheck reports reachable stdlib vulnerabilities, for example GO-2026-6809 in `crypto/x509`, fixed in 1.26.10. That is why the `Security` workflow has failed on every run since 2026-08-14.
- **No protection on `main`.** There is no branch protection and no ruleset (API returns 404). Dependabot PRs merged while `Security` was red.
- **Security fixes are unreleased.** The last release, v0.0.28 (2026-07-06), is what Homebrew serves. It predates the pentest credential-exposure fix (#33), the proxy fix (#34), and #27, #29 and #35. 33 commits are unreleased.
- **A config typo wipes the config.** `setContext` (`cmd/ctx.go:184`) and `setCredentials` (`cmd/config.go:229-232`) start a fresh config on *any* load error. A YAML typo plus `ctx set` silently wipes every context and token.

**Correctness**
- **Table output prints raw Go values.** The default table printer prints raw Go `map[...]` / `&{...}` for non-struct data (`pkg/output/table.go:58-90`). This hits `describe entity|entity-type|entity-relations|event|metric`, `config view`, and every multi-environment (`--env "a;b"`) call.
- **Wrong exit codes.** Multi-environment failures exit 0, because `UnwrapSingle` drops `firstErr` (`pkg/client/multi.go:163-179`). Everything else exits 1 regardless of cause (`cmd/root.go:159`).
- **`--limit` is unreliable.** It is ignored in JSON/agent output for problems, SLOs, events, metrics and security problems (`get slos --limit 5 -o json` returned 60). `get problems --limit N` with N > 10 fetches every page. Truncation by `--max-pages` is silent, because `nextPageKey` is deleted (`pkg/client/pagination.go:54`).
- **Multi-environment client bypasses settings.** The client built for multi-environment calls never applies `-v` (`multi.go:90-97`). `get environments` never applies the context proxy (`cmd/root.go:336`, `cmd/get_environments.go:61`).
- **`query log-counts` has demo logic.** It hard-codes BookStore names (`cmd/query_log_counts.go:228-255`). It reports 0 for groups outside the API's top-100 window, swallows lookup errors (`:102`), ignores `--env`, and corrupts `-o yaml`.
- **Unvalidated output formats.** `-o` is not validated (`-o xml` gives a table), and `yml` is accepted only by the printer. An empty result with `-o json` writes nothing to stdout.
- **Slow failure on dead hosts.** Retries cover every transport error, including DNS/TLS/proxy-refused (`pkg/client/client.go:127-133`), so a dead host takes about 4 minutes to fail.
- **Root cause is duplication.** These bugs come from duplication in `cmd/`: 17 `isMultiEnv` branches, 19 client constructions, 60 package-level flag variables, and no shared fetch → limit → render path. `cmd` coverage is 31.6%.

**Architecture conformance** (`cli-tool-architecture/README.md`, `DT-CLI-SDK-PLAN.md`)
- **Conforms:** naming, repo, verb–noun grammar, config path and schema, own auth and keyring, completion, GoReleaser + Homebrew.
- **Deviates:**
  - The agent envelope always uses error code `"error"`, has no `suggestions`, and its `context` holds only `resource` (`pkg/output/agent.go`).
  - No `--plain`.
  - No `dtmgd commands` catalog.
  - No cluster-scoped context: `env-id` is mandatory (`pkg/client/client.go:72-74`).
  - **No Cluster Management API coverage at all.** The only cluster call is a version probe. The architecture says dtmgd covers "nodes, environments, license, backups" (README.md:330-337, decision log).
- **The architecture repo itself is stale** (last commit 2026-05-12) and contradicts itself on code sharing. This design resolves that as described under Constraints.

**dtctl patterns worth adopting**
- Typed exit codes 0–5.
- `sdk/agentmode` (stdlib-only).
- "Did you mean" for resources and flags.
- A `cmd` test harness with golden files.
- release-please, cosign signing, SBOM, `goreleaser check`.
- `install.sh`/`install.ps1`.
- `doctor`.
- `DTMGD_CONFIG`/`DTMGD_CONTEXT`.
- Credential deletion.
- AGENTS.md as the canonical agent guide.
- A thin SKILL.md with a `references/` folder.
- Standard OSS files.

**MCP parity**
- **Gaps:**
  - All 18 MCP tools have a dtmgd counterpart, but "full feature parity" is overstated.
  - `get security-problems` lacks entity selector, `from`, `to`, `sort`.
  - `get slos` lacks `timeFrame`, `from`, `to`, `sort`, `showGlobalSlos`, `demo`.
  - `describe slo` doesn't set `timeFrame=GTF` when dates are given.
  - No enum validation, and help shows `--impact SERVICE` where the API value is `SERVICES`.
  - Duplicate context names silently overwrite each other in fan-out.
  - Logs aren't sorted newest-first.
  - The docs list an MCP tool, `aggregate_logs`, that never existed.
  - No import of the MCP `dt-config.yaml`.
- **Where dtmgd is ahead:** security-problem detail fields, SLO evaluation paging, display-ID lookup, parallel multi-environment queries.

**In good shape — leave alone unless a phase needs to touch it**
- Build, vet, golangci-lint and race tests are clean.
- `pkg/config`, `pkg/client` and `pkg/output` have 85–87% coverage.
- The config security design: `${VAR}` resolved at point of use, masked tokens, `--token-stdin`, trimmed API error bodies, parent-directory config warning.
- Keyring mocked in `TestMain`.
- Actions pinned to SHAs.
- Multi-environment fan-out, which dtctl lacks.

## Goals

1. Close the security and safety gaps and ship them to Homebrew users.
2. Make output, limits and exit codes correct and consistent across every command and format. Make them testable, so they stay correct.
3. Conform to the shared architecture: agent envelope, `--plain`, `commands` catalog, cluster-scoped contexts, and Cluster Management API coverage of environments, nodes, license and backups.
4. Honest MCP parity: close the parameter gaps, and describe the remaining differences truthfully.
5. Adopt dtctl's delivery and documentation practices where they fit a small, read-only tool.

## Non-goals

- **Any write operation.** dtmgd stays read-only, including against the Cluster API: no creating/deleting environments, no maintenance mode, no token management. *(My call — the architecture's write verbs are allowed, not required, and cluster writes are high blast-radius.)*
- **Cluster API areas the architecture does not list:** ActiveGates, synthetic locations, users/groups, IAM, tokens, remote access, SSL certificates. These are possible later additions, not part of this work.
- **Multi-cluster fan-out for cluster commands.** `--env` fan-out stays for environment-scoped commands only.
- **Adopting dtctl's SDK beyond `agentmode`:** no `httpclient`, `session` or `urls`. The rest is SaaS-only, or would undo dtmgd's error-body trimming.
- **Out of scope:** SaaS / 3rd-gen features, DQL, merging dtmgd into dtctl.
- **Not copied from dtctl:** plugins, safety levels, dry-run, serve mode, stability tiers, toon/parquet/chart output, OAuth, nix/devcontainer, and dtctl's command-factory machinery.
- **TLS CA-file / skip-verify options.** Neither tool has them; this is not a parity gap.

## Anti-goals (it would be a failure even if it technically works)

- **Weakening the security posture.** Re-exposing API error bodies, resolving `${VAR}` at load time, or making tokens visible.
- **A big-bang rewrite** that leaves `main` unreleasable mid-way. Every phase lands in mergeable PRs and ends with a release.
- **Silent behaviour changes.** Every breaking change goes into the release notes with the new way to do the old thing.
- **Docs that again claim more than the tool does.** In particular, the parity table must match reality.
- **CI that is green without testing anything.** The `sonar` job today is exactly that.

## Constraints

- **Stack:** keep the current Go stack: Cobra, resty, tablewriter, keyring, GoReleaser. No Viper is required.
- **Architecture sources.** Where the architecture docs contradict each other, follow `cli-tool-architecture/README.md` (its dtmgd section and decision log) plus the SDK plan's Tier-1 `agentmode`. The account-management ADR says dtmgd "cannot share auth, config, or API client code with dtctl". That is consistent with sharing `agentmode` only.
- **Cluster-scoped context.** A context without `env-id` is cluster-scoped (README.md:339-356; Open Question #4 resolved as "absence of env-id").
- **Existing testing rules from `.claude/CLAUDE.md` still apply:**
  - Use 400/403 for HTTP error tests, never 5xx.
  - Keyring-touching tests need `keyring.MockInit()` in `TestMain`.
  - The `pkg/skills` local failures are known and not part of this work. Use `go test ./pkg/config/... ./pkg/client/... ./cmd/... ./pkg/output/...` for a local signal.
- **Snyk resolves the full module graph.** A new module dependency, including the dtctl SDK, can bring graph-only findings. Triage them using the knowledge-base note, not by bumping blindly.
- **Phase gate.** Each phase ends with tests, lint and govulncheck green under the CI Go version. Docs for whatever changed are updated in the same PRs.
- **Outward-facing actions need the owner's go-ahead at the time they happen.** These include repository settings (rulesets), pushing tags/releases, closing issues, and anything in other repos.

## The work, in phases

Phases are in priority order. Each lists what must be true at the end; how to get there is the builder's call unless stated.

### Phase 1 — Security and safety → release v0.0.29

1. **Patched toolchain.**
   - Add a `toolchain` directive at the latest patched Go (≥ 1.26.10 at time of writing), so CI and goreleaser build with it.
   - Bump `golang.org/x/net` past its open vulnerabilities, and refresh the other indirect dependencies (`go get -u ./... && go mod tidy`).
   - Add a scheduled workflow so this doesn't silently recur. It opens or updates a GitHub **issue** (not a PR, see "Owner decisions") when the Go toolchain or any direct or indirect dependency is behind or has a known vulnerability.
   - Exit criterion: the `Security` workflow is green.
2. **Fix the config wipe.** `setContext` and `setCredentials` fall back to a new config only when the file does not exist. A parse error aborts with the parse error and changes nothing. Add regression tests.
3. **Add `SECURITY.md`** (reporting route; the pentest makes this overdue).
4. **Close the two still-open pentest notes** from PRISM-13823 (vault copy: `../vault/jira-products/PRISM-13823/`).
   - Notes 2, 4 and 6–11 were fixed by #33–#35 or are covered by Phase 2 item 10.
   - Note 3 is already documented in the README.
   - **Note 1:** print a stderr notice when `--config` points outside the standard locations (`$XDG_CONFIG_HOME/dtmgd/`, a local `.dtmgd.yaml`).
   - **Note 5:** print a one-line stderr notice when agent mode was *auto-detected* (not when forced with `--agent`), naming the variable and `--no-agent`. `skills install` requires `--for` when stdin is not a terminal.
   - Closing the ticket is the owner's.
5. **Suppress Snyk where it can't run; don't delete it.** Skip the job when the token secret isn't available to the run (Dependabot and fork PRs), so it stops failing those PRs. It keeps running on `main` pushes and on schedule, where it passes today. It becomes a required check once the owner has a token for Dependabot.
6. **Ruleset on `main`** requiring Test, Lint, SAST and Security (Snyk not required for now). Only the owner (admin) may bypass it *(my call)*. This is a repository settings change, so confirm with the owner before applying.
7. **Cut v0.0.29** with the existing goreleaser flow.
   - Before tagging, check that the personal access token goreleaser uses for the tap is still valid (unused since 2026-07-06), and that the generated cask keeps the tap's current `postflight_steps` form (the tap hand-edited it on 2026-09-15).
   - The exit criterion is that the tap PR is *opened*. Merging it is up to the tap's code owner, outside this project.

### Phase 2 — Correctness foundation → release v0.1.0 (contains breaking changes)

1. **Command test harness.** Run a full command against `httptest`, with a temp config and captured stdout/stderr/exit code, plus golden files for rendered output. `RunE` code becomes testable; the knowledge-base advice to avoid it is retired.
2. **One shared pipeline for list/describe commands:** resolve targets (single or multi-environment) → fetch → decode → apply limit → render.
   - Writer and client factory are injected.
   - One client factory serves both the single-environment and multi-environment paths, so proxy, verbosity and retries can't drift apart again.
   - Migrate every existing command onto it; this also removes the package-level flag state.
   - The design of the pipeline is the builder's.
3. **Output correctness.**
   - Non-struct data renders readably in table mode (never Go syntax).
   - `-o` is validated against a known set, with `yml` as an alias.
   - **Output shape** *(my call, following the architecture's "`-o json` = raw API JSON")*:
     - `-o json`/`-o yaml` and the envelope's `result` keep the API's own response shape, as today (for example `{"problems":[…],"totalCount":N}`). `--limit` truncates the list inside it.
     - An empty result prints that object with an empty list, never nothing.
     - Multi-environment output stays a map keyed by context name, with each failed environment's error inline. Failures also go to the envelope's `warnings` and produce a non-zero exit.
     - This keeps today's documented shape and avoids an unnecessary break for the skill and scripts.
   - `--plain` disables glyphs (✓⚠✗💡) and the ANSI screen clear. It switches on automatically when stdout is not a TTY or `NO_COLOR` is set.
   - `--watch` is offered only on commands that implement it, and never writes ANSI codes in JSON/agent mode.
   - `describe` help says what it actually does.
4. **Limits and truncation.**
   - `--limit` is honoured in every format.
   - Truncation is visible: `has_more`/`total` in the envelope, and a stderr note in human mode.
   - In agent mode the default limit is 50 when `--limit` isn't given *(my call, mirrors dtctl)*.
   - Human table mode keeps today's fetch-all default.
5. **Exit codes per dtctl:**

   | Code | Meaning |
   |---|---|
   | 0 | success |
   | 1 | general error |
   | 2 | usage error |
   | 3 | auth (401) |
   | 4 | not found (404) |
   | 5 | permission denied (403) |

   For a multi-environment call, if any environment fails, the exit code is non-zero and partial results are still printed. Picking the code for mixed failures is the builder's call; document it.
6. **Agent envelope.**
   - Import `github.com/dynatrace-oss/dtctl/sdk/agentmode` *(my call: the architecture names it as the shared Tier-1 package)*. That brings classified error codes, `suggestions`, `context.verb/total/has_more/warnings`, and broader agent detection.
   - Keep dtmgd's error-body trimming.
   - If the Snyk graph-only noise from the import is unacceptable, copy the two files instead and note why.
7. **Errors.**
   - "Did you mean" for unknown resources and flags.
   - `--no-agent` honoured even when argument parsing fails.
   - Required token scopes are declared once per command. The 403 hint, `doctor` and the README scope list are all generated from that one source; three conflicting lists exist today.
8. **Network behaviour.**
   - Don't retry DNS/TLS/proxy-refused errors.
   - Bound the total time to fail.
   - Keep resty's own log lines off stderr unless `-v`.
9. **`query log-counts`.**
   - Remove all BookStore-specific logic.
   - Never report 0 for a group that merely fell outside the API's top-N window. If the API can't avoid the window, say so in the output.
   - Surface entity-lookup errors, honour `--env`, and emit valid YAML.
10. **Security display-ID lookup (`S-NNN`)** (pentest note 4). The documented `securityProblemSelector` has no display-ID criterion, so the lookup stays client-side *(my call)*, but:
    - It stops paging at the first match.
    - It is announced at `-v`.
    - It surfaces errors instead of falling through.
    - It isn't repeated needlessly per environment.
    - If a live cluster shows a server-side filter that works, use it.
11. **Housekeeping.**
    - Remove dead code and the unused `logrus` dependency.
    - Save the config atomically, keeping 0600 permissions on existing files.
    - Surface keyring errors other than not-found, instead of reporting "token not found".
12. **New commands from dtctl and the architecture:**
    - `dtmgd commands [--brief] -o json`, a catalog built by walking the Cobra tree.
    - `dtmgd doctor`, which checks each configured context: reachability, auth, cluster version, and a warning below the MCP's minimum (1.328.0). It falls back the way MCP does when the token can't read the cluster version.
    - `DTMGD_CONFIG` and `DTMGD_CONTEXT` environment variables.
    - A way to delete stored credentials from the keyring (`ctx delete` cleans up, plus an explicit credential delete).
    - `get environments` keeps working in this phase, but prints a deprecation note pointing to `doctor`.

### Phase 3 — MCP parity → release

1. **`get security-problems`:** entity selector, `--from`, `--to`, `--sort`. Keep the current risk-score sort as the default.
2. **`get slos`:** `--timeframe`, `--from`, `--to`, `--sort`, `--show-global`, `--demo`.
3. **`describe slo`:** sets `timeFrame=GTF` when `--from`/`--to` are given without a timeframe.
4. **Validate enum flags against the API's values** (problem status/impact, security status/risk, and so on). Fix `SERVICE` → `SERVICES` in help and docs.
5. **Reject duplicate context names** when loading config.
6. **Defaults.**
   - `query logs` sorts newest-first by default, with a `--sort` override.
   - `query metrics` keeps returning a time series by default *(my call: more useful at a terminal)*. It gets a `--resolution` flag if it doesn't already have one, and the difference is documented in the parity table.
   - No hard result caps beyond the agent-mode default from Phase 2 *(my call)*.
7. **`dtmgd config import`** reads an MCP `dt-config.yaml` (or `DT_ENVIRONMENT_CONFIGS`) and creates the equivalent contexts. Tokens go to the keyring where available. This is a one-time import, not a second config format at runtime *(my call)*.
8. **Parity documentation rewritten honestly.**
   - "Covers every MCP tool", with a table of parameter and default differences.
   - `aggregate_logs` dropped, `query log-counts` marked dtmgd-only.
   - `get_environments_info` maps to `doctor`.
9. **Optional, last:** Dynatrace UI deep links in output, as the MCP server now provides.

Also for the owner, outside this repo: the review found that the MCP server sends `impactLevel`/`riskLevel` as top-level query parameters, which the v2 API doesn't document. MCP filtering may silently not work. Not verified against a live cluster.

### Phase 4 — Cluster Management API → release v0.2.0 (breaking: `get environments`)

Read-only coverage of the four areas the architecture names. The endpoints below come from the Managed docs repo's OpenAPI specs (`../docs/managed/_data/openapi/cluster.json` for `/api/cluster/v2` and `clusterv1.json` for `/api/v1.0/onpremise`).

| Area | Endpoints | Proposed commands (naming is the builder's, within get = list / describe = one) |
|---|---|---|
| Environments | `GET /api/cluster/v2/environments` (pageSize, nextPageKey, sort, filter e.g. `state(ENABLED)`, includeConsumptionInfo, includeStorageInfo), `GET /environments/{id}` | `get environments`, `describe environment <id>` |
| Nodes and cluster | `GET /api/v1.0/onpremise/cluster` (known servers), `/cluster/configuration`, `/cluster/metadata` | `get nodes`, `describe cluster` |
| License | `GET /api/cluster/v2/clusterLicense`, `/clusterLicense/environment/total` (both EARLY_ADOPTER and classic-licensing only) | `describe license`, per-environment usage via a flag or `get license-usage` |
| Backups | `GET /api/v1.0/onpremise/backup/config` | `describe backup`: configuration only (enabled, schedule, storage path, and so on). `/backup/config/status` reports the status of a config *change request*, not backup health, so it is not used. The API exposes no last-backup time or health; the command help says so. |

1. **Cluster-scoped contexts.**
   - `ctx set` without `--env-id` creates one. It uses a Cluster API token (`ServiceProviderAPI` scope).
   - Environment-scoped commands refuse a cluster context, and cluster commands refuse an environment context. Each error says what to use instead, naming the configured contexts of the other kind. Don't match on host (see the next point).
   - **A cluster context carries its own host.** Unauthenticated probes on 2026-10-29 showed this on both of the owner's clusters:
     - The environment contexts use node hosts on port 9999, where `/api/v1.0/onpremise/...` and `/api/cluster/v2/...` return 404.
     - The Cluster API answers (401 without a token) on the public names `https://demo.dynatrace-managed.com` and `https://latest.managed-dev.dynalabs.io`.
     - So today's `ClusterVersion()` probe of `{host}/api/v1.0/onpremise/cluster` always 404s on these clusters and falls back to the v1 environment endpoint.
   - `--env` fan-out does not apply to cluster commands.
2. **One client for both API surfaces.** Cluster calls go through the same client factory, proxy, retry and error-trimming as environment calls.
3. **`get environments` becomes the cluster listing**, per the architecture. Run against an environment context, it errors with a hint to use `doctor` or a cluster context. The release notes call this out.
4. **`doctor` understands cluster contexts.**
5. **Commands catalog, skill, README and generated docs** cover the new commands.
6. **Unknowns to verify before building:**
   - The backup endpoints are in the OpenAPI spec but not in the public docs, so their required scope and stability are unconfirmed.
   - `clusterLicense` works only on classic licensing. On a DPS-licensed cluster, the command must say so clearly instead of failing obscurely.

   - **How to settle them.** The owner is adding cluster-scoped contexts with Cluster API tokens (`ServiceProviderAPI`) for both clusters. Phase 4 therefore starts with a short verification spike against them, using read-only GETs only. The spike also finds out each cluster's licensing model. The cluster core source is optional backup.
   - If a backup endpoint turns out to be unusable, ship the other three areas and record the gap.

### Phase 5 — Delivery and documentation practices

1. **Releases.**
   - Releases stay tag-triggered: goreleaser runs on tag push, and the owner pushes the tag or approves the agent doing it.
   - No release-please: its PRs would be bot-opened and wouldn't trigger CI (see "Owner decisions").
   - `go install` builds report their module version from the build info instead of "dev".
   - Changelog without `build(deps)` noise.
   - goreleaser adds cosign keyless signing and SBOMs, and drops the `go mod tidy` hook.
   - `goreleaser check` runs in CI, and `release.yml` runs the tests before releasing.
2. **Homebrew tap updates move off the personal fork + PAT** (`ivan-gudak/homebrew-tap`) to an org-owned token, as dtctl does. This needs owner/org action.
3. **`install.sh` and `install.ps1`**, for jump hosts without Homebrew or Go.
4. **CI consolidation.**
   - Delete `sonar.yml`; analysis comes from the SonarCloud app.
   - Run tests once per OS, not four times.
   - Stop cross-compiling six targets on every PR.
   - Add `permissions:` blocks to every workflow.
   - Pin golangci-lint and govulncheck versions.
   - Group Dependabot updates.
   - Indirect dependencies are covered by the scheduled check from Phase 1, which opens an issue, not a PR.
   - `make lint` runs golangci-lint and no longer runs `gofmt -w` first.
   - CodeQL is optional.
5. **Agent and contributor docs.**
   - `AGENTS.md` becomes the canonical guide: architecture, how to add a resource, the testing rules.
   - `.claude/CLAUDE.md` points to it and keeps the knowledge base. Correct the stale entries: the skills installer line number, the "snyk gates" claim, and the "don't test RunE" coverage advice.
   - `.github/copilot-instructions.md` shrinks to a pointer.
   - Add `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and issue/PR templates.
6. **User docs.**
   - README restructured, with reference docs generated from Cobra into `docs/` and a CI check that they're fresh.
   - SKILL.md slimmed (dtctl's is about 180 lines) with a `references/` folder and no BookStore content.
7. **`.gitignore`:** add `.dtmgd.yaml` (it can hold a plaintext token), and narrow the broad `go/` pattern.

## Owner decisions (2026-10-29)

- **Follow the architecture**, including Cluster Management API coverage of environments, nodes, license and backups.
- **Snyk.** The owner has no Snyk token, so `snyk.yml` stays but is suppressed where the secret isn't available, and it is not a required check. Runs on `main` pass today, so a `SNYK_API_TOKEN` secret does exist at repository level; someone else set it.
- **No bot-opened PRs.** The agent pushes branches. PRs are opened either by the owner in the GitHub UI or by the agent with `gh`, which is authenticated as the owner. Both count as human-authored, so CI runs. Consequences:
  - no release-please
  - scheduled checks open issues, not PRs
  - no new GitHub App or bot token is needed
- **Live verification.** `~/.config/dtmgd/config` has two environment contexts: `prod` (demo.dynatrace-managed.com) and `stage` (latest.managed-dev.dynalabs.io). The owner will add cluster-scoped contexts with Cluster API tokens.
  - Verification uses read-only calls only.
  - Never print or log token values. When inspecting the config, filter out the `token:` lines.

## What is left to the builder

- Exact command and flag names, within the verb–noun grammar and the tables above.
- The design of the shared command pipeline, the test harness and the golden-file format.
- How Phase 2 is split into PRs, provided `main` stays releasable.
- The exit code for mixed multi-environment failures.
- The YAML library choice: `gopkg.in/yaml.v3` is archived upstream.
- The order of items within a phase.

## Follow-ups outside this repo (owner's call, not part of the plan)

- **A PR to `cli-tool-architecture`:**
  - Update the dtmgd section: prototype status, repo URL, `get environments` semantics.
  - Record Open Question #4 as resolved.
  - Add an exit-code convention and an envelope schema.
  - Remove the code-sharing contradiction between ANALYSIS.md and the account-management ADR.
- **Report the MCP `impactLevel`/`riskLevel` parameter issue** to the MCP repo.
- **Triage stale issues** #15, #16 and #26.
