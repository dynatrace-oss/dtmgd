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
make lint         # go vet, gofmt check, golangci-lint, actionlint, shellcheck
make fmt          # gofmt -w .
make docs         # regenerate docs/reference from the commands
make vulncheck    # govulncheck
make clean        # remove ./dtmgd and bin/
```

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
