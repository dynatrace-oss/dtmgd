# Contributing to dtmgd

Thanks for helping. dtmgd is a small, read-only CLI for Dynatrace Managed;
changes that keep it that way are the easiest to accept. Please follow the
[code of conduct](CODE_OF_CONDUCT.md).

## Reporting bugs and asking for features

Open an issue with the bug report or feature request template. Remove tokens,
host names, environment IDs and license keys from anything you paste.
**Security vulnerabilities go through [SECURITY.md](SECURITY.md)**, not a
public issue.

## Development setup

You need:

- Go 1.26.10 or later (the version in `go.mod`)
- `make`
- [golangci-lint](https://golangci-lint.run/docs/welcome/install/) v2.15.0, for `make lint`
- optionally `shellcheck`, which `make lint` uses for `install.sh` when it is installed

```bash
git clone https://github.com/dynatrace-oss/dtmgd.git
cd dtmgd
make build        # ./dtmgd
make install      # go install into $GOPATH/bin (or $HOME/go/bin)
make test         # go test -race ./...
make integration  # live, read-only checks against your cluster (see below)
make lint         # go vet, gofmt check, golangci-lint, actionlint, shellcheck
make fmt          # gofmt -w .
make docs         # regenerate docs/reference from the commands
make vulncheck    # govulncheck
make clean        # remove ./dtmgd and bin/
```

### Live integration tests

`make test` runs the CLI against fake APIs, which cannot tell whether a
cluster honours a parameter. `make integration` runs `test/integration`
(build tag `integration`) against a real Dynatrace Managed cluster: it checks
that the list commands answer, that `--limit` keeps the next-page key, that
`--type`, `--mz`, `--status` and `query logs --entity` really filter (an
impossible value must match nothing or be rejected), the `--env` fan-out,
and, with a cluster context, the Cluster API commands and that
`describe license` masks the key.

1. Copy `.integrationtests.env.example` to `.integrationtests.env` (git
   ignores it) and name your config file and contexts. It holds no tokens:
   dtmgd reads those from your config or the OS keyring as usual.
2. Run `make integration`. Tests whose settings are missing skip, and so do
   filter checks whose unfiltered result is empty, since they would prove
   nothing.

The suite only runs `get`, `describe`, `query`, `doctor` and `commands`, and
never changes the config. A failure shows the exit code and the first line of
stderr with URLs, host names and IP addresses removed, never response data.
It never runs in CI, which has no cluster; CI only checks that it compiles
(`go vet -tags integration ./test/...`).

## Making a change

[AGENTS.md](AGENTS.md) describes the architecture, how to add a command, the
conventions and the testing rules. The short version:

- Every command goes through `runFetch` (environment API) or
  `runClusterFetch` (Cluster Management API), and is tested end to end with
  the harness in `cmd/harness_test.go`.
- Write the test first and watch it fail.
- Tests never read your real config (`--config` with a temp file), and HTTP
  error cases use 400 or 403.
- Run `make docs` when a command, flag or help text changes: the generated
  reference in `docs/reference/` is checked by the tests.
- Update the README, `docs/` and `skills/dtmgd/` for what you changed.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) prefixes:
`feat`, `fix`, `docs`, `test`, `refactor`, `ci`, `build`, `chore`, with an
optional scope (`fix(install): …`). Release notes are built from them;
`docs`, `test` and `ci` commits (with or without a scope), `build(deps)` and
`chore(deps)` are left out.

## Pull requests

- Keep a PR to one change, and fill in the template.
- These checks must pass: `test` on ubuntu, macOS and Windows, `lint`,
  `golangci-lint`, `vulncheck` and `goreleaser-check`. Also look at the
  SonarCloud result.
- A changed golden file is a changed behaviour: say why in the PR.
- Breaking changes go into the release notes with the new way to do the old
  thing.

## Releases

Maintainers release by pushing a `vX.Y.Z` tag; [AGENTS.md](AGENTS.md#ci-and-releases)
has the steps.
