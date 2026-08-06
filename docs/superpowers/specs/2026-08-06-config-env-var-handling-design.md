# Config environment-variable handling

**Date:** 2026-08-06
**Status:** Approved, not implemented
**Follow-up to:** PR #27 (`f55592c`)

## Problem

`pkg/config/config.go:135` runs `os.ExpandEnv` over the entire config file text at load
time. The in-memory `Config` therefore holds expanded values that the file never
contained, and every symptom below follows from that one decision.

### Symptoms

Four distinct failures, all reproduced against the CLI:

1. **Credential leak on save.** `SaveTo` marshals the in-memory config back over the
   same path, replacing `token: ${DT_API_TOKEN}` with the live token. Any command that
   writes config — `use-context`, `set-context`, `set-credentials`, `delete-context`,
   `migrate-tokens` — writes a real API token into `.dtmgd.yaml`, a file `README.md:127`
   tells users to create in their project directory. The next `git add .` commits it.

2. **Silent config wipe.** With the variables unset, the same path writes empty strings
   for `host` and `env-id` and reports success. The documented onboarding flow
   (`config init` then `config set-credentials`) does exactly this.

3. **Literal `$` corruption.** `os.ExpandEnv` consumes any `$` sequence, not just
   documented `${VAR}` references:

   ```
   os.ExpandEnv("pa$$w0rd")   -> "paw0rd"
   os.ExpandEnv("cost is $5") -> "cost is "
   os.ExpandEnv("a$b-c")      -> "a-c"
   ```

   A proxy password containing `$` is corrupted when read, so proxy auth fails at
   runtime, and corrupted when written, so the file is damaged permanently.

4. **`config view` prints secrets.** `dtmgd config view` marshals the expanded config to
   stdout, printing the live token. It reaches terminal scrollback, CI logs, and the
   `--agent` JSON envelope consumed by AI tooling. No save is involved, so PR #27 does
   not address it.

### What PR #27 fixed, and what it cost

PR #27 records the path a config was expanded from and refuses to overwrite it. That
stops symptoms 1 and 2, which was the right call for a live credential leak.

The cost is that placeholder configs became read-only to every `dtmgd config` command.
`dtmgd config init` now produces a file that no `dtmgd` command can modify, while still
printing *"Environment variables can be used with `${VAR_NAME}` syntax."* Symptoms 3 and
4 are untouched.

This design removes the root cause and restores the commands.

## Approach

**The in-memory `Config` is a faithful image of the file. Expansion happens only at the
boundary where a value is about to be used.**

```
      +------------ raw zone -------------+   +---- resolved zone ----+

file -> LoadFrom -> Config -+-> SaveTo -> file
        (no expand)         |
                            +-> config view / ctx list      shows ${VAR}
                            |
                            +-> Resolve() --> ResolvedContext --> client.New --> network
                                    |
                                    +- error: unset ${VAR}
```

Nothing crosses right-to-left. A resolved value is never written back into a `Config`,
which makes the leak structurally impossible rather than guarded against.

### Alternatives considered

**Load twice — raw for writing, expanded for reading.** Keep `LoadFrom` as-is and add a
`LoadFromRaw` that skips expansion; config commands use raw, read commands use expanded.
Smallest diff and no consumer changes, but it solves only symptom 1. Lazy per-context
errors would require diffing the two trees, every caller must know which of the two
objects it holds, and symptoms 3 and 4 remain.

**Round-trip placeholders on save.** Walk the `yaml.Node` tree on save and restore
`${VAR}` wherever the in-memory value still matches an expansion. Proposed by the author
of PR #27. Fixes only symptom 1, is the fiddliest of the three, and every value in memory
is still a live secret.

Both were rejected because they treat symptoms. Expanding the whole document at load is
the shared cause, and only the chosen approach removes it.

## Decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Only `${VAR}` is expanded; bare `$VAR` is not | `README.md:353` documents only `${VAR}`. Restricting the syntax is what makes literal `$` safe. |
| D2 | Unset variables fail lazily, at the point of use | `pkg/client/multi.go:47` and `cmd/get_environments.go:50` iterate every context. Failing at load would break the documented three-context config for anyone holding credentials for only one. |
| D3 | Overwriting a placeholder is allowed, with a warning | The user named the exact context or credential and passed an explicit value. Refusing would risk a second dead end in the commands this change exists to un-break. The warning fires when a command replaces a field whose current *raw* value satisfies `HasPlaceholder`, including the keyring case where `SetToken` writes `token: ""` and the placeholder disappears without the token ever appearing in the file. |
| D4 | `migrate-tokens` skips placeholder tokens | Migrating `${DT_API_TOKEN}` would freeze the current value into the keyring, silently converting an indirection into a snapshot. |
| D5 | PR #27's `SaveTo` guard stays | It stops firing in normal operation once nothing expands at load, becoming a backstop for direct `pkg/config` library use. Removing it would be a regression in isolation. |
| D6 | YAML comments and formatting are out of scope | `yaml.Marshal` reflows the document on every save. Pre-existing, orthogonal, and fixing it means node-tree surgery across all mutation paths. |

## Components

### New: `pkg/config/expand.go`

```go
// ${VAR} only. Every other $ is left alone.
var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Bare $VAR, which is no longer expanded (D1) but is worth warning about.
var bareRefRe = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// HasPlaceholder reports whether s contains a ${VAR} reference.
func HasPlaceholder(s string) bool

// BareRefs returns bare $VAR references in s, excluding ${VAR} forms.
func BareRefs(s string) []string

// expand replaces every ${VAR} in s with its environment value.
// unset lists variables referenced but not set, in first-appearance order, deduped.
func expand(s string) (expanded string, unset []string)
```

A regexp rather than `os.Expand` with a custom mapping function: `os.Expand` still
recognises bare `$VAR`, so it cannot express "`${VAR}` and nothing else."

An unset reference expands to the empty string and is reported in `unset`. Every caller
treats a non-empty `unset` as an error, so the partial expansion is never used — the
empty-string choice only keeps `expand` total and free of error handling.

### New: `ResolvedContext`

```go
type ResolvedContext struct {
    Host, EnvID, TokenRef       string
    HTTPProxyURL, HTTPSProxyURL string
}

// Resolve expands ${VAR} references. On failure the error is *UnresolvedVarsError.
func (c *Context) Resolve() (*ResolvedContext, error)

type UnresolvedVarsError struct{ Vars []string }
```

`Resolve` does not know the context's name, so the error carries only variable names and
callers — which do know the name — wrap it. `UnresolvedVarsError.Error()` returns the
bare form (`${EU_TOKEN} is not set`); callers prefix the context name in whichever shape
suits their output. Message formatting stays in the layer that has the information.

`APIBaseURL` and `ClusterAPIBaseURL` move from `Context` onto `ResolvedContext`. They
have no production callers today (only `pkg/config/config_test.go`; production code goes
through `client.New`, which builds its own URL), so the move is free and removes the one
place that could quietly produce `https://${DT_MANAGED_HOST}/e/.../api`.

### Type safety boundary

`Context.Host` and friends stay readable, because the display paths legitimately need raw
values. The compiler enforces the raw/resolved split only at client construction, which
is the boundary where secrets and network access live. Everywhere else the guarantee
rests on a small, reviewable set of call sites plus the tests below.

## Changed call sites

| Site | Change |
|---|---|
| `pkg/config/config.go:135` `LoadFrom` | drop `os.ExpandEnv` |
| `pkg/config/config.go:195` `GetToken` | expand the plaintext branch; keyring branch untouched |
| `pkg/config/config.go:55,69` `APIBaseURL`, `ClusterAPIBaseURL` | move onto `ResolvedContext` |
| `pkg/config/keyring.go:47` `MigrateTokensToKeyring` | skip placeholder tokens, return which were skipped |
| `pkg/client/client.go:38` `NewFromConfig` | `Resolve()` before `New(...)` |
| `pkg/client/multi.go:47` `MultiRequest` | `Resolve()` inside the goroutine, failure into `r.Error` |
| `cmd/get_environments.go:50` | resolve per context, failure into `item.Status` |
| `cmd/ctx.go:143,177` and `cmd/config.go:174` | clobber warning via `HasPlaceholder` |
| `cmd/config.go:217` `migrate-tokens` | report skipped tokens; do not save when nothing migrated |
| `cmd/ctx.go:115` `listContexts`, `cmd/config.go:29` `config view` | unchanged — raw display, now leak-free |
| `cmd/root.go:204` `loadConfigRaw` | unchanged — its name simply becomes accurate |

`MultiRequest` already has a per-context error slot (`EnvResult.Error`, `multi.go:41`)
and `get_environments.go` already has `item.Status`. Lazy per-context errors need no new
plumbing; a resolution failure is another per-context error alongside the existing
"token error" and "client error" cases.

Warnings are emitted in `cmd/` via `pkg/output`. `pkg/config` stays silent and returns
data.

## Error handling

Unset variable, single-env:

```
✗ context "eu-prod" needs ${EU_TOKEN}, which is not set
```

Several unset in one context: `needs ${EU_HOST} and ${EU_TOKEN}, which are not set`.

Multi-env, one context failing does not sink the others:

```
✓ prod, staging
✗ eu-prod: ${EU_TOKEN} is not set
```

Clobber warning:

```
⚠ Replaced ${DT_API_TOKEN} in .dtmgd.yaml — that environment variable no longer affects this token.
✓ Credential "my-token" stored in OS keyring
```

`migrate-tokens` where every token is a placeholder. `cmd/config.go:217` currently prints
`No tokens to migrate` whenever `migrated == 0`, which is actively wrong here:

```
⚠ Skipped "prod-token": it resolves from ${DT_API_TOKEN}; migrating would freeze the current value.
ℹ No tokens to migrate
```

Bare `$VAR`, detected during `Resolve` since it is already walking these strings:

```
⚠ "$DT_API_TOKEN" looks like an environment variable reference. Use ${DT_API_TOKEN}.
```

Without this, dropping bare `$VAR` support fails silently — the literal string becomes
the token and the user sees an unexplained 401.

## Breaking changes

Both need release notes:

- **Bare `$VAR` no longer expands.** Undocumented, but real behaviour today. Mitigated by
  the warning above.
- **`config view` prints `${DT_API_TOKEN}` instead of the secret.** Breaks
  `dtmgd config view -o json | jq -r .Tokens[0].Token` as a token-extraction idiom. Users
  should read the environment variable directly.

`README.md:353` must be updated to document `${VAR}` as the only supported syntax, and to
state that config commands preserve placeholders.

Out of scope: `config view` still prints genuinely plaintext tokens, i.e. those actually
stored in the file rather than referenced. That is a separate concern.

## Testing

### The merged PR's test is expected to fail

`TestSaveToKeepsEnvPlaceholders` (added by PR #27) asserts that `SaveTo` refuses. Once
`LoadFrom` stops expanding, `expandedFrom` is never set, `SaveTo` succeeds, and
placeholders round-trip — the desired outcome, but the opposite of what the test asserts.

It is rewritten to assert round-trip preservation. A separate in-package test sets
`expandedFrom` directly to keep the D5 backstop covered.

### Unit — `pkg/config`

The load-bearing regression test: **load a placeholder config with every variable set,
and assert the in-memory value is still the literal `${DT_API_TOKEN}`.** That single
assertion is what makes the leak impossible to reintroduce.

Beyond that:

- `expand` table test, seeded with the probed strings: `pa$$w0rd`, `cost is $5`, `a$b-c`,
  `${SET`, `$`, plus `${}` and `${1BAD}`, multiple references, and a repeated reference.
- `Resolve`: success, one unset, several unset (asserting order and dedupe).
- `GetToken`: placeholder set, placeholder unset, keyring branch.
- `MigrateTokensToKeyring`: skip accounting across placeholder and plaintext tokens.
- Existing `APIBaseURL` and `ClusterAPIBaseURL` tests port to `ResolvedContext` unchanged.

### Command-level — `cmd`

`CLAUDE.md` notes that `cmd` coverage is limited because `RunE` needs a live API. That
does not apply here: the config commands are entirely local. The documented onboarding
flow (`init` → `set-credentials` → `use-context`) is driven against a temp dir and
asserted end to end.

Constraint: `IsKeyringAvailable()` (`pkg/config/keyring.go:35`) probes the real OS
keyring, so `set-credentials` takes different branches on headless CI than on a developer
Mac. Rather than introduce a keyring seam, which is scope creep, the test asserts that
the `host` and `env-id` placeholders survive — true on both branches.

### Acceptance criteria

The four symptoms, as scripted regression checks:

1. `use-context` on a placeholder config with variables set: succeeds, and the file still
   contains `${DT_API_TOKEN}` and no token value.
2. `config init` then `set-credentials` with variables unset: succeeds, and `host` and
   `env-id` are still `${DT_MANAGED_HOST}` and `${DT_ENV_ID}`.
3. A config with `http-proxy: http://user:pa$$w0rd@proxy.corp:8080` survives a
   `use-context` byte-for-byte, and the client receives `pa$$w0rd`.
4. `config view` with all variables set prints `${DT_API_TOKEN}`, not the token.
