# AGENTS.md — working on dtmgd

This is the guide for anyone, human or AI agent, changing dtmgd. For *using*
the CLI, read [skills/dtmgd/SKILL.md](skills/dtmgd/SKILL.md) and the
[command reference](docs/reference/dtmgd.md). Lessons learned while working on
the code are kept in the knowledge base in
[.claude/CLAUDE.md](.claude/CLAUDE.md); read the entries that match your task.

## What dtmgd is

A `kubectl`-style CLI (`dtmgd <verb> <resource> [flags]`) for **Dynatrace
Managed** (self-hosted) clusters, written in Go with Cobra. It is
**read-only**: it never creates, changes or deletes anything on a cluster, and
that holds for the Cluster Management API too. Primary users are AI agents
driving it through the bundled skill, so output, limits and exit codes are
part of its contract.

## Build, test, lint

```bash
make build        # ./dtmgd (CGO_ENABLED=0)
make test         # go test -race ./...
make integration  # live read-only suite against a real cluster (test/integration)
make lint         # go vet, gofmt check, golangci-lint, actionlint, shellcheck (checks only)
make fmt          # gofmt -w .
make docs         # regenerate docs/reference after changing a command
make vulncheck    # govulncheck from the tools/ module
```

`go test ./...` gives the same result on your machine as in CI: the test
binaries that touch the home or config directory run in a sandbox (see
Testing).

## Architecture

```
main.go         os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))
cmd/            Cobra commands, one file per command (get_*.go, describe_*.go, query_*.go)
  app.go        App (one per invocation), Run, newRootCmd, the global flags
  runner.go     runFetch and fetchSpec: fetch → limit → render, for every environment command
  cluster.go    runClusterFetch, clusterCommand: the same for Cluster Management API commands
  contextkind.go  refuses a command run against the wrong kind of context (exit 2)
  errors.go     error reporting, hints (withHint), exit codes
  scopes.go     withScopes: the token scopes each command declares
  commands.go   the `dtmgd commands` catalog
pkg/client/     HTTP client (resty): GetV2, GetV1, GetCluster, GetV2Paged, GetClusterPaged,
                MultiRequest (parallel --env fan-out), exit codes, error diagnosis
pkg/config/     config file, contexts, ${VAR} references, keyring
pkg/output/     printers: table (struct tags), csv and tsv (the same tags), json, yaml, agent envelope
pkg/skills/     `dtmgd skills install`; skills/dtmgd/ holds the embedded skill
pkg/version/    version, commit, date: ldflags, else the module's build info
tools/          checksum-locked CI tools (govulncheck, actionlint), a separate module
internal/testsandbox/  TestMain helper: temp HOME and config directory for test binaries
test/integration/      live read-only suite against a real cluster (build tag integration)
```

Endpoints:

| Target | URL |
|---|---|
| Environment API, environment contexts | `{host}/e/{env-id}/api/v2/` (some commands `/api/v1/`) |
| Cluster Management API, cluster contexts (no `env-id`) | `{host}/api/cluster/v2/` and `{host}/api/v1.0/onpremise/` |

A context **without `env-id`** is a cluster context (`config.Context.IsCluster`,
judged on the raw file value, before `${VAR}` expansion). `client.New` then
builds a cluster-scoped client whose environment calls return
`client.ErrClusterScoped`. A cluster context's host is the cluster's own
address (the one the Cluster Management Console uses), not a node's `:9999`
address. `--env ALL_ENVIRONMENTS` selects environment contexts only.

Agent detection and the envelope come from dtctl's `sdk/agentmode`
(`github.com/dynatrace-oss/dtctl/sdk`). That is the only code shared with
dtctl, and it stays that way.

## Adding a command

1. Create `cmd/get_foos.go` with a constructor `newGetFoosCmd(a *App) *cobra.Command`.
   Bind flags to locals, never to package-level variables, and register the
   command in its verb's `AddCommand`.
2. In `RunE`, call **`a.runFetch(fetchSpec{…})`** for an environment command,
   or **`a.runClusterFetch(fetchSpec{…})`** for a Cluster Management API
   command, and wrap the constructor's result in `clusterCommand(cmd)`. Don't
   branch on single vs multi-environment, and don't render output yourself:
   the runner handles `--limit`, truncation notes, empty results, `--env`
   fan-out, partial failures and every output format.
   - `Fetch` returns `fetched{Body, Total, HasMore, …}` in the API's own shape.
     For paged lists use `pagedFetch(path, params, listKey, pageCap, maxPages)`
     (or `clusterPagedFetch` / `clusterObjectFetch`).
   - `Rows` turns `Body` into `[]T` of a struct with `table:"HEADER"` tags
     (`table:"HEADER,wide"` for `-o wide` only); `Human` renders a summary
     instead. With neither, a table shows the body as YAML.
   - Set `List: true` for lists, so agents get the default 50-item limit.
3. Declare an environment command's token scopes with
   `withScopes(cmd, "foos.read")`. A cluster command needs nothing more:
   `clusterCommand` already declares `ServiceProviderAPI`, and `withScopes`
   replaces the list rather than adding to it. A test keeps the README scope
   list (between `<!-- scopes:start -->` and `<!-- scopes:end -->`) in step
   with what commands declare.
4. Add `--watch` only where re-running makes sense: `addWatchFlags` and
   `a.watchOrRun`.
5. Test it end to end (see Testing), then run `make docs`: the reference in
   `docs/reference/` is generated, and a test fails while it is stale.

## Conventions

- **Pagination:** a later page is requested with `nextPageKey` alone; resending
  the filters gives HTTP 400. `GetV2Paged` stops once `Limit` items are in hand
  and keeps the last `nextPageKey`, so truncation is never silent.
- **Output contract:** stdout is data only. Outside agent mode, notes,
  warnings and errors go to stderr; in agent mode errors are an `ok: false`
  envelope on stdout and warnings go into `context.warnings`. json, yaml and
  the envelope's `result` are the API's own response.
- **Errors and exit codes:** `0` success, `1` other error, `2` usage error,
  `3` authentication failed (401), `4` not found (404), `5` permission denied
  (403). Usage errors are `UsageError`. Add a next step to an error with
  `withHint`; agent output turns hints into `suggestions`.
- **Secrets:** API error bodies are trimmed, tokens are never printed, and the
  `-vv` dump redacts the token and `licenseKey` (`client.redactBody`; add any
  new secret field there). `${VAR}` references in the config are resolved at
  the point of use, never when loading.
- **Negative problem IDs** (`-6546711275898328738_…V2`): `rewriteNegativeArgs`
  inserts `--` for them. Never use `SetInterspersed(false)`: it breaks the
  persistent flags after a positional argument.

## Testing

- **Command tests run the real CLI.** `runCLI(t, cfgPath, args...)` (and
  `runCLIWithEnv`) in `cmd/harness_test.go` run `Run` against an httptest
  `newFakeAPI` with a route table; `withFakeEnvs` / `withFakeContexts` write a
  temp config pointing at it. Compare output with `assertGolden` and
  regenerate with `go test ./cmd/ -run <Test> -update`. A changed golden is a
  changed behaviour: read the diff.
- **Never use the default config path.** Pass `--config` with a temp file
  (`runCLI` does). A developer's global config (`~/.config/dtmgd/config` on
  Linux, `~/Library/Application Support/dtmgd/config` on macOS,
  `%LOCALAPPDATA%\dtmgd\config` on Windows) may hold live tokens. As a
  safety net, `TestMain` in `cmd`, `pkg/config` and `pkg/skills` runs the
  tests through `testsandbox.Main` (`internal/testsandbox`), which points the
  home and config directories at a temporary directory and refuses to run if
  the default config path is still outside it. Add the same `TestMain` to any
  new package whose tests reach the config or the home directory.
- **HTTP error cases use 400 or 403,** never 5xx or 429: the client retries
  those, which adds seconds of waiting.
- **Keyring:** `cmd` and `pkg/config` call `keyring.MockInit()` in `TestMain`.
  A test that reaches `SetToken`, `GetToken` or `MigrateTokensToKeyring` in any
  other package needs the same, or it overwrites real credentials.
- **Live integration suite.** `test/integration` (build tag `integration`,
  `make integration`) runs the real CLI against a real cluster, configured by
  the git-ignored `.integrationtests.env`. Add a check there when a behaviour
  depends on what the cluster does with a parameter, not on dtmgd's code.
  Its `run` helper refuses anything but read verbs; report failures with
  `brief(r)`, never stdout. It never runs in CI; `lint` vets it with the tag.
- **Coverage:** CI fails below 60% (`go tool cover -func` total). That total
  only counts code in named functions, so keep command code in constructors,
  not in closures assigned to package-level variables.
- **Complexity:** golangci-lint's `gocognit` (limit 15) matches SonarCloud's
  S3776 for non-test code. SonarCloud also checks test functions, so split
  long tests into helpers.

## CI and releases

- **Required checks** on `main`: `test` on ubuntu, macOS and Windows, `lint`,
  `golangci-lint`, `vulncheck` and `goreleaser-check`. `snyk` and SonarCloud
  report but don't gate; check SonarCloud on every PR anyway. A ruleset on
  `v*` tags lets only admins create, move or delete release tags.
- **Tools:** govulncheck and actionlint are `tool` entries in `tools/go.mod`
  (checksum-locked), built with `go -C tools build`. Never add
  `go install …@version` to a workflow. golangci-lint and GoReleaser are pinned
  by version in the workflows, and the Makefile's `GOLANGCI_LINT_VERSION` names
  the golangci-lint version `make lint` expects; `scripts/drift-check.sh`
  reports when a pin falls behind. Every action is pinned to a commit SHA.
- **Releases are tag-triggered** and the maintainer pushes the tag (no
  release-please). `release.yml` runs the tests and govulncheck, then
  GoReleaser builds, signs `checksums.txt` with cosign (keyless), attaches an
  SPDX SBOM per archive, publishes the release, and commits the Homebrew cask
  straight to `dynatrace-oss/homebrew-tap`.
  Steps:
  1. Annotated tag on the merge commit (`git tag -a vX.Y.Z -m vX.Y.Z <sha>`), push it.
  2. Wait for `release.yml`, then check that the tap's `main` has the new
     cask (see the knowledge base if the cask step fails).
  3. Add release notes at the top with `gh release edit vX.Y.Z --notes-file …`.
  4. Verify: `cosign verify-blob` and `sha256sum -c` as in the README's
     "Verifying a download", then smoke-test the binary with a temp `--config`.
- **Docs travel with the change:** README, `docs/`, SKILL.md and its
  `references/`, and `make docs` for the generated reference.

## Rules for agents

- **Never print, log or commit a token.** When you show a config file, filter
  out its `token:` lines. Use tokens only through dtmgd itself.
- **Calls to a real cluster are read-only GETs**, and only when the maintainer
  asks for them. Filter their output (`jq`) so license keys, emails, IPs and
  storage paths stay out of the transcript.
- **No real host names or environment IDs in repository files.** New examples
  use `example.com` hosts (`https://cluster.example.com`) and synthetic IDs;
  older ones use `managed.company.com`, so don't add more of those.
- **Ask the maintainer before every push, PR, merge, tag, repository-settings
  change, and anything in another repository.**
- Don't run destructive commands (`git clean`, `git reset --hard`,
  `go clean -modcache`) unless asked.
