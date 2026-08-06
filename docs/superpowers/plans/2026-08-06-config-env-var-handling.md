# Config Environment-Variable Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `dtmgd` from expanding `${VAR}` references at config-load time, so the in-memory config stays a faithful image of the file and expansion happens only where a value is used.

**Architecture:** `LoadFrom` drops `os.ExpandEnv`. A new `pkg/config/expand.go` provides `${VAR}`-only expansion primitives, and a new `pkg/config/resolve.go` provides a `ResolvedContext` type produced by `Context.Resolve()` at the client-construction boundary. Raw values flow to display and save paths; resolved values flow only to the network.

**Tech Stack:** Go 1.25+, `gopkg.in/yaml.v3`, `github.com/spf13/cobra`, `github.com/zalando/go-keyring`. Standard `testing` package, no test framework.

**Spec:** `docs/superpowers/specs/2026-08-06-config-env-var-handling-design.md`

## Global Constraints

- Go 1.25+ (`go.mod`). Run `make vet` and `gofmt` before every commit.
- Only `${VAR}` is expanded. Bare `$VAR` is **not** expanded (spec D1).
- `pkg/config` and `pkg/client` never print. All user-facing warnings live in `cmd/` via `pkg/output`.
- Unset variables fail at point of use, never at load (spec D2).
- PR #27's `SaveTo` guard and its `expandedFrom` field stay in place (spec D5).
- YAML comment and formatting preservation is out of scope (spec D6).
- Tests use `t.Setenv` (auto-restores) and `t.TempDir`. Never mutate the real environment or the user's real config.
- HTTP error tests use status 400 or 403, never 5xx — the resty client retries 5xx three times and adds multi-second waits.

## Deviations from the spec

Three refinements decided during planning. All are narrower or better-behaved than the spec text; none change the approved architecture.

1. **`BareRefs` reports only names that are currently-set environment variables.** The spec implies a plain regexp scan, but `\$([A-Za-z_][A-Za-z0-9_]*)` matches `$w0rd` inside `pa$$w0rd` — spuriously warning about the exact string the spec exists to preserve. Requiring the name to be a set variable removes the false positive.

2. **The bare-`$VAR` warning is emitted at load time in `cmd/`, not inside `Resolve`.** The spec places detection in `Resolve`, but `Resolve` is called from `pkg/client`, which has no output channel. Warning there would mean threading warnings up through client construction. Scanning once in `cmd.LoadConfig`/`cmd.loadConfigRaw` is simpler and fires exactly once per invocation.

3. **Task order inverts the spec's narrative.** Resolution is wired into the client boundary (Task 3) *before* `LoadFrom` stops expanding (Task 4). While load still expands, `Resolve` is a harmless no-op — there are no `${...}` sequences left to match. This keeps the CLI working at every commit; the reverse order leaves it broken in between.

## File Structure

**New files:**

| File | Responsibility |
|---|---|
| `pkg/config/expand.go` | `${VAR}` regexps and the three expansion primitives. No config types. |
| `pkg/config/expand_test.go` | Table tests for the primitives. |
| `pkg/config/resolve.go` | `ResolvedContext`, `Context.Resolve`, `UnresolvedVarsError`, and the two base-URL methods moved off `Context`. |
| `pkg/config/resolve_test.go` | `Resolve` behaviour plus the relocated base-URL tests. |
| `pkg/config/keyring_test.go` | `MigrateTokensToKeyring` skip accounting. |
| `cmd/config_test.go` | End-to-end config-command tests against a temp dir. |

**Modified files:**

| File | Change |
|---|---|
| `pkg/config/config.go` | `LoadFrom` drops expansion; `GetToken` resolves; base-URL methods removed. |
| `pkg/config/config_test.go` | Base-URL tests move out; PR #27's test rewritten. |
| `pkg/config/keyring.go` | `MigrateTokensToKeyring` skips placeholders, returns which. |
| `pkg/client/client.go` | `NewFromConfig` resolves before `New`. |
| `pkg/client/multi.go` | `MultiRequest` resolves per context into `EnvResult.Error`. |
| `cmd/get_environments.go` | Resolves per context into `item.Status`. |
| `cmd/config.go` | `setCredentials` extracted and testable; clobber and migrate warnings. |
| `cmd/ctx.go` | Clobber warning in `setContext`. |
| `cmd/root.go` | Bare-`$VAR` warning after load. |
| `README.md` | Document `${VAR}` as the only supported syntax. |

New files rather than growing `config.go`: it is 285 lines today and would reach roughly 400 with expansion and resolution folded in. Splitting keeps each file to one responsibility.

---

### Task 1: Expansion primitives

Self-contained. No existing code changes; nothing consumes these yet.

**Files:**
- Create: `pkg/config/expand.go`
- Test: `pkg/config/expand_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func HasPlaceholder(s string) bool`
  - `func BareRefs(s string) []string`
  - `func expand(s string) (expanded string, unset []string)` — unexported, package-internal.

- [ ] **Step 1: Write the failing test**

Create `pkg/config/expand_test.go`:

```go
package config

import (
	"reflect"
	"testing"
)

func TestExpand(t *testing.T) {
	t.Setenv("SET_VAR", "VALUE")
	t.Setenv("OTHER_VAR", "OTHER")
	t.Setenv("EMPTY_VAR", "")

	tests := []struct {
		name      string
		input     string
		want      string
		wantUnset []string
	}{
		{"no references", "plain value", "plain value", nil},
		{"single set", "${SET_VAR}", "VALUE", nil},
		{"embedded", "a-${SET_VAR}-b", "a-VALUE-b", nil},
		{"two distinct", "${SET_VAR}/${OTHER_VAR}", "VALUE/OTHER", nil},
		{"repeated same", "${SET_VAR}${SET_VAR}", "VALUEVALUE", nil},
		{"set but empty is not unset", "${EMPTY_VAR}", "", nil},
		{"single unset", "${MISSING_VAR}", "", []string{"MISSING_VAR"}},
		{"unset deduped", "${MISSING_VAR}${MISSING_VAR}", "", []string{"MISSING_VAR"}},
		{"unset ordered by first use", "${B_MISSING}${A_MISSING}", "", []string{"B_MISSING", "A_MISSING"}},
		{"mixed set and unset", "${SET_VAR}:${MISSING_VAR}", "VALUE:", []string{"MISSING_VAR"}},

		// Literal $ must survive untouched. These are the strings that
		// os.ExpandEnv corrupts today.
		{"double dollar password", "pa$$w0rd", "pa$$w0rd", nil},
		{"dollar digit", "cost is $5", "cost is $5", nil},
		{"bare ref not expanded", "a$b-c", "a$b-c", nil},
		{"bare uppercase ref not expanded", "$SET_VAR", "$SET_VAR", nil},
		{"unclosed brace", "${SET_VAR", "${SET_VAR", nil},
		{"lone dollar", "$", "$", nil},
		{"empty braces", "${}", "${}", nil},
		{"name starting with digit", "${1BAD}", "${1BAD}", nil},
		{"proxy url with dollars", "http://user:pa$$w0rd@proxy.corp:8080", "http://user:pa$$w0rd@proxy.corp:8080", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, unset := expand(tt.input)
			if got != tt.want {
				t.Errorf("expand(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !reflect.DeepEqual(unset, tt.wantUnset) {
				t.Errorf("expand(%q) unset = %v, want %v", tt.input, unset, tt.wantUnset)
			}
		})
	}
}

func TestHasPlaceholder(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"${DT_API_TOKEN}", true},
		{"prefix-${VAR}-suffix", true},
		{"plain", false},
		{"", false},
		{"pa$$w0rd", false},
		{"$BARE", false},
		{"${unclosed", false},
		{"${}", false},
	}
	for _, tt := range tests {
		if got := HasPlaceholder(tt.input); got != tt.want {
			t.Errorf("HasPlaceholder(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBareRefs(t *testing.T) {
	t.Setenv("DT_API_TOKEN", "secret")
	t.Setenv("SET_VAR", "VALUE")

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"bare ref to set var", "$DT_API_TOKEN", []string{"DT_API_TOKEN"}},
		{"braced form is not bare", "${DT_API_TOKEN}", nil},
		{"mixed forms", "${SET_VAR}/$DT_API_TOKEN", []string{"DT_API_TOKEN"}},
		{"unset name is not reported", "$NOT_A_REAL_VAR", nil},
		{"password is not reported", "pa$$w0rd", nil},
		{"deduped", "$SET_VAR $SET_VAR", []string{"SET_VAR"}},
		{"plain text", "no refs here", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BareRefs(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BareRefs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run 'TestExpand|TestHasPlaceholder|TestBareRefs' -v`
Expected: FAIL — compile error, `undefined: expand`, `undefined: HasPlaceholder`, `undefined: BareRefs`.

- [ ] **Step 3: Write the implementation**

Create `pkg/config/expand.go`:

```go
package config

import (
	"os"
	"regexp"
)

// placeholderRe matches a ${VAR} reference. This is deliberately not
// os.Expand: os.Expand also recognises bare $VAR, so it cannot express
// "${VAR} and nothing else". Restricting to the braced form is what keeps
// literal dollar signs — proxy passwords, prices — intact.
var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// bareRefRe matches a bare $VAR reference, which is no longer expanded.
var bareRefRe = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// HasPlaceholder reports whether s contains a ${VAR} reference.
func HasPlaceholder(s string) bool {
	return placeholderRe.MatchString(s)
}

// BareRefs returns bare $VAR references in s that name a currently-set
// environment variable, deduped and in first-appearance order.
//
// The set-variable requirement is what keeps this quiet: a plain regexp scan
// matches "$w0rd" inside "pa$$w0rd", and reporting that would warn about the
// very strings this package exists to preserve. A bare name that resolves to
// a real variable is almost certainly a reference the user expected to work.
func BareRefs(s string) []string {
	// Remove braced forms first so ${VAR} is never reported as bare.
	stripped := placeholderRe.ReplaceAllString(s, "")

	var refs []string
	seen := make(map[string]bool)
	for _, m := range bareRefRe.FindAllStringSubmatch(stripped, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		if _, ok := os.LookupEnv(name); !ok {
			continue
		}
		seen[name] = true
		refs = append(refs, name)
	}
	return refs
}

// expand replaces every ${VAR} in s with its environment value.
//
// unset lists variables that were referenced but not set, deduped and in
// first-appearance order. A set-but-empty variable is not unset — the user
// chose that value. Unset references expand to the empty string; every caller
// treats a non-empty unset slice as an error, so the partial result is never
// used. Returning it keeps this function total and free of error handling.
func expand(s string) (string, []string) {
	var unset []string
	seen := make(map[string]bool)

	expanded := placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1]
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		if !seen[name] {
			seen[name] = true
			unset = append(unset, name)
		}
		return ""
	})

	return expanded, unset
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ -run 'TestExpand|TestHasPlaceholder|TestBareRefs' -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Verify nothing else broke**

Run: `go build ./... && make vet && go test ./pkg/... ./cmd/...`
Expected: build clean, vet clean, all pre-existing tests still pass. `pkg/skills` may fail — see "Known pre-existing failure" at the end of this plan.

- [ ] **Step 6: Commit**

```bash
gofmt -w pkg/config/expand.go pkg/config/expand_test.go
git add pkg/config/expand.go pkg/config/expand_test.go
git commit -m "feat(config): add \${VAR}-only expansion primitives

Adds HasPlaceholder, BareRefs and expand. Nothing consumes them yet.

Unlike os.ExpandEnv, these only recognise the braced \${VAR} form, so
literal dollar signs in values survive untouched."
```

---

### Task 2: ResolvedContext and Context.Resolve

**Files:**
- Create: `pkg/config/resolve.go`
- Create: `pkg/config/resolve_test.go`
- Modify: `pkg/config/config.go:53-78` (remove `APIBaseURL` and `ClusterAPIBaseURL` from `Context`)
- Modify: `pkg/config/config_test.go:34-108` (move the base-URL tests to `resolve_test.go`)

**Interfaces:**
- Consumes: `expand` from Task 1.
- Produces:
  - `type ResolvedContext struct { Host, EnvID, TokenRef, HTTPProxyURL, HTTPSProxyURL string }`
  - `func (c *Context) Resolve() (*ResolvedContext, error)`
  - `func (r *ResolvedContext) APIBaseURL() string`
  - `func (r *ResolvedContext) ClusterAPIBaseURL() string`
  - `type UnresolvedVarsError struct { Vars []string }`
  - `func (e *UnresolvedVarsError) Error() string` — `"${EU_TOKEN} is not set"`
  - `func (e *UnresolvedVarsError) InContext(name string) error` — `context "eu-prod" needs ${EU_TOKEN}, which is not set`

- [ ] **Step 1: Write the failing test**

Create `pkg/config/resolve_test.go`:

```go
package config

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveSuccess(t *testing.T) {
	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")

	ctx := &Context{
		Host:         "${DT_MANAGED_HOST}",
		EnvID:        "${DT_ENV_ID}",
		TokenRef:     "prod-token",
		HTTPProxyURL: "http://user:pa$$w0rd@proxy.corp:8080",
	}

	got, err := ctx.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Host != "https://managed.example.com" {
		t.Errorf("Host = %q, want the expanded value", got.Host)
	}
	if got.EnvID != "abc12345" {
		t.Errorf("EnvID = %q, want the expanded value", got.EnvID)
	}
	if got.TokenRef != "prod-token" {
		t.Errorf("TokenRef = %q, want it unchanged", got.TokenRef)
	}
	// A literal $ must survive resolution.
	if got.HTTPProxyURL != "http://user:pa$$w0rd@proxy.corp:8080" {
		t.Errorf("HTTPProxyURL = %q, want the dollars preserved", got.HTTPProxyURL)
	}

	// Resolving must not mutate the receiver.
	if ctx.Host != "${DT_MANAGED_HOST}" {
		t.Errorf("Resolve mutated the source context: Host = %q", ctx.Host)
	}
}

func TestResolveUnsetSingle(t *testing.T) {
	ctx := &Context{Host: "${MISSING_HOST}", EnvID: "abc12345"}

	_, err := ctx.Resolve()
	if err == nil {
		t.Fatal("Resolve() succeeded with an unset variable")
	}

	var uerr *UnresolvedVarsError
	if !errors.As(err, &uerr) {
		t.Fatalf("error type = %T, want *UnresolvedVarsError", err)
	}
	if !reflect.DeepEqual(uerr.Vars, []string{"MISSING_HOST"}) {
		t.Errorf("Vars = %v, want [MISSING_HOST]", uerr.Vars)
	}
	if got, want := uerr.Error(), "${MISSING_HOST} is not set"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := uerr.InContext("eu-prod").Error(),
		`context "eu-prod" needs ${MISSING_HOST}, which is not set`; got != want {
		t.Errorf("InContext() = %q, want %q", got, want)
	}
}

func TestResolveUnsetMultiple(t *testing.T) {
	ctx := &Context{Host: "${MISSING_HOST}", EnvID: "${MISSING_ENV}", TokenRef: "${MISSING_HOST}"}

	_, err := ctx.Resolve()
	if err == nil {
		t.Fatal("Resolve() succeeded with unset variables")
	}

	var uerr *UnresolvedVarsError
	if !errors.As(err, &uerr) {
		t.Fatalf("error type = %T, want *UnresolvedVarsError", err)
	}
	// First-appearance order, deduped across fields.
	if !reflect.DeepEqual(uerr.Vars, []string{"MISSING_HOST", "MISSING_ENV"}) {
		t.Errorf("Vars = %v, want [MISSING_HOST MISSING_ENV]", uerr.Vars)
	}
	if got, want := uerr.Error(), "${MISSING_HOST} and ${MISSING_ENV} are not set"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := uerr.InContext("eu-prod").Error(),
		`context "eu-prod" needs ${MISSING_HOST} and ${MISSING_ENV}, which are not set`; got != want {
		t.Errorf("InContext() = %q, want %q", got, want)
	}
}

func TestResolveNoPlaceholders(t *testing.T) {
	// The no-op case: a context that was already expanded, or never used
	// placeholders, must resolve unchanged.
	ctx := &Context{Host: "https://managed.company.com", EnvID: "env-prod", TokenRef: "prod-token"}
	got, err := ctx.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Host != ctx.Host || got.EnvID != ctx.EnvID || got.TokenRef != ctx.TokenRef {
		t.Errorf("Resolve() altered a placeholder-free context: %+v", got)
	}
}

// --- ResolvedContext.APIBaseURL (moved from config_test.go) ---

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		ctx  ResolvedContext
		want string
	}{
		{"normal", ResolvedContext{Host: "https://managed.company.com", EnvID: "abc123"}, "https://managed.company.com/e/abc123/api"},
		{"trailing slash", ResolvedContext{Host: "https://managed.company.com/", EnvID: "abc123"}, "https://managed.company.com/e/abc123/api"},
		{"multiple trailing slashes", ResolvedContext{Host: "https://managed.company.com///", EnvID: "abc123"}, "https://managed.company.com/e/abc123/api"},
		{"empty host", ResolvedContext{Host: "", EnvID: "abc123"}, ""},
		{"empty env id", ResolvedContext{Host: "https://managed.company.com", EnvID: ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.APIBaseURL(); got != tt.want {
				t.Errorf("APIBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClusterAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		ctx  ResolvedContext
		want string
	}{
		{"normal", ResolvedContext{Host: "https://managed.company.com"}, "https://managed.company.com/api"},
		{"trailing slash", ResolvedContext{Host: "https://managed.company.com/"}, "https://managed.company.com/api"},
		{"empty host", ResolvedContext{Host: ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.ClusterAPIBaseURL(); got != tt.want {
				t.Errorf("ClusterAPIBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Delete the old base-URL tests from `config_test.go`**

Remove the block from the `// --- Context.APIBaseURL ---` comment through the end of `TestClusterAPIBaseURL` (currently `pkg/config/config_test.go:34-108`). The replacements above now live in `resolve_test.go`. Leave `makeTestConfig` and everything below `TestClusterAPIBaseURL` alone.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run 'TestResolve|TestAPIBaseURL|TestClusterAPIBaseURL' -v`
Expected: FAIL — compile error, `undefined: ResolvedContext`, `undefined: UnresolvedVarsError`, and `ctx.Resolve undefined`.

- [ ] **Step 4: Write the implementation**

Create `pkg/config/resolve.go`:

```go
package config

import (
	"fmt"
	"strings"
)

// ResolvedContext is a Context whose ${VAR} references have been expanded.
//
// It is a separate type from Context on purpose. Context mirrors the file and
// is what display and save paths use; ResolvedContext is what reaches the
// network. Keeping them distinct means a resolved value can never be written
// back into a Config and saved to disk.
type ResolvedContext struct {
	Host          string
	EnvID         string
	TokenRef      string
	HTTPProxyURL  string
	HTTPSProxyURL string
}

// UnresolvedVarsError reports environment variables that a context references
// but which are not set.
type UnresolvedVarsError struct {
	Vars []string
}

// Error returns the bare form, e.g. "${EU_TOKEN} is not set".
func (e *UnresolvedVarsError) Error() string {
	return fmt.Sprintf("%s %s not set", e.varList(), e.verb())
}

// InContext returns the error phrased for a named context, e.g.
// `context "eu-prod" needs ${EU_TOKEN}, which is not set`.
func (e *UnresolvedVarsError) InContext(name string) error {
	return &contextVarsError{name: name, err: e}
}

// contextVarsError names the context an UnresolvedVarsError came from.
//
// It is a type rather than fmt.Errorf with %w because wrapping would append
// the inner message, producing "...which is not set: ${EU_TOKEN} is not set".
// Unwrap keeps errors.As working through it.
type contextVarsError struct {
	name string
	err  *UnresolvedVarsError
}

func (c *contextVarsError) Error() string {
	return fmt.Sprintf("context %q needs %s, which %s not set", c.name, c.err.varList(), c.err.verb())
}

func (c *contextVarsError) Unwrap() error { return c.err }

func (e *UnresolvedVarsError) verb() string {
	if len(e.Vars) == 1 {
		return "is"
	}
	return "are"
}

// varList renders the variables as "${A}", "${A} and ${B}", or "${A}, ${B} and ${C}".
func (e *UnresolvedVarsError) varList() string {
	quoted := make([]string, len(e.Vars))
	for i, v := range e.Vars {
		quoted[i] = "${" + v + "}"
	}
	switch len(quoted) {
	case 0:
		return ""
	case 1:
		return quoted[0]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}

// Resolve expands ${VAR} references in the context's fields.
//
// It does not mutate the receiver. If any referenced variable is unset the
// error is a *UnresolvedVarsError naming all of them, so the user can fix
// every one in a single pass rather than discovering them one at a time.
func (c *Context) Resolve() (*ResolvedContext, error) {
	var unset []string
	seen := make(map[string]bool)

	resolveField := func(s string) string {
		value, missing := expand(s)
		for _, name := range missing {
			if !seen[name] {
				seen[name] = true
				unset = append(unset, name)
			}
		}
		return value
	}

	r := &ResolvedContext{
		Host:          resolveField(c.Host),
		EnvID:         resolveField(c.EnvID),
		TokenRef:      resolveField(c.TokenRef),
		HTTPProxyURL:  resolveField(c.HTTPProxyURL),
		HTTPSProxyURL: resolveField(c.HTTPSProxyURL),
	}

	if len(unset) > 0 {
		return nil, &UnresolvedVarsError{Vars: unset}
	}
	return r, nil
}

// APIBaseURL returns the environment API base URL for this context.
// Format: {host}/e/{env-id}/api
func (r *ResolvedContext) APIBaseURL() string {
	if r.Host == "" || r.EnvID == "" {
		return ""
	}
	return fmt.Sprintf("%s/e/%s/api", strings.TrimRight(r.Host, "/"), r.EnvID)
}

// ClusterAPIBaseURL returns the cluster-level API base URL.
// Format: {host}/api
func (r *ResolvedContext) ClusterAPIBaseURL() string {
	if r.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s/api", strings.TrimRight(r.Host, "/"))
}
```

Note: `Resolve` evaluates the struct fields in source order, which is what makes
the `unset` ordering deterministic and matches `TestResolveUnsetMultiple`.

- [ ] **Step 5: Remove the old methods from `config.go`**

Delete `Context.APIBaseURL` and `Context.ClusterAPIBaseURL` (currently `pkg/config/config.go:53-78`), including their doc comments. They had no production callers — everything real goes through `client.New`, which builds its own URL — so nothing outside the tests you already moved will break.

If `fmt` becomes unused in `config.go`, leave it: `LoadFrom` and the other methods still use it.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./pkg/config/ -v`
Expected: PASS, including the relocated base-URL tests.

- [ ] **Step 7: Verify the whole build**

Run: `go build ./... && make vet && go test ./pkg/... ./cmd/...`
Expected: build clean, vet clean.

- [ ] **Step 8: Commit**

```bash
gofmt -w pkg/config/resolve.go pkg/config/resolve_test.go pkg/config/config.go pkg/config/config_test.go
git add pkg/config/resolve.go pkg/config/resolve_test.go pkg/config/config.go pkg/config/config_test.go
git commit -m "feat(config): add ResolvedContext and Context.Resolve

Introduces the resolved-zone type and moves APIBaseURL and
ClusterAPIBaseURL onto it. Those had no production callers, so the move
is free and removes the one place that could produce a URL containing an
unexpanded \${VAR}."
```

---

### Task 3: Resolve at the client-construction boundary

Wires resolution into every path that reaches the network. `LoadFrom` still expands at this point, so `Resolve` finds no `${...}` and is a no-op — the CLI behaves identically before and after this task. That is deliberate: it lets the boundary land under test without a broken intermediate commit.

**Files:**
- Modify: `pkg/client/client.go:28-43` (`NewFromConfig`)
- Modify: `pkg/client/multi.go:36-58` (`MultiRequest` goroutine body)
- Modify: `cmd/get_environments.go:33-56`

**Interfaces:**
- Consumes: `Context.Resolve`, `ResolvedContext`, `UnresolvedVarsError.InContext` from Task 2.
- Produces: no new exported API. `client.New` keeps its `(host, envID, token string)` signature.

- [ ] **Step 1: Update `NewFromConfig`**

In `pkg/client/client.go`, replace the body of `NewFromConfig` (lines 28-43):

```go
// NewFromConfig creates a Client from the current config context.
func NewFromConfig(cfg *config.Config) (*Client, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, err
	}
	resolved, err := ctx.Resolve()
	if err != nil {
		var uerr *config.UnresolvedVarsError
		if errors.As(err, &uerr) {
			return nil, uerr.InContext(cfg.CurrentContext)
		}
		return nil, err
	}
	token, err := cfg.GetToken(resolved.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get API token: %w", err)
	}
	c, err := New(resolved.Host, resolved.EnvID, token)
	if err != nil {
		return nil, err
	}
	c.SetProxy(resolved.HTTPProxyURL, resolved.HTTPSProxyURL)
	return c, nil
}
```

Add `"errors"` to the import block in `pkg/client/client.go`.

- [ ] **Step 2: Update `MultiRequest`**

In `pkg/client/multi.go`, inside the goroutine, insert resolution before the token lookup (replacing lines 38-53):

```go
			// EnvResult already carries Name, and the printer renders it as
			// "eu-prod: ...", so the bare error form reads correctly here —
			// no InContext wrapping needed.
			resolved, resolveErr := nc.Context.Resolve()
			if resolveErr != nil {
				r.Error = resolveErr
				results[idx] = r
				return
			}

			token, tokenErr := cfg.GetToken(resolved.TokenRef)
			if tokenErr != nil {
				r.Error = fmt.Errorf("token error: %w", tokenErr)
				results[idx] = r
				return
			}

			c, clientErr := New(resolved.Host, resolved.EnvID, token)
			if clientErr != nil {
				r.Error = fmt.Errorf("client error: %w", clientErr)
				results[idx] = r
				return
			}
			c.SetProxy(resolved.HTTPProxyURL, resolved.HTTPSProxyURL)
```

No import changes are needed in `pkg/client/multi.go`.

- [ ] **Step 3: Update `get_environments.go`**

In `cmd/get_environments.go`, inside the `for _, nc := range cfg.Contexts` loop, resolve before the token lookup. Replace lines 34-50:

```go
		for _, nc := range cfg.Contexts {
			item := EnvListItem{
				Name:  nc.Name,
				Host:  nc.Context.Host,
				EnvID: nc.Context.EnvID,
			}

			resolved, resolveErr := nc.Context.Resolve()
			if resolveErr != nil {
				item.Version = "—"
				item.Status = resolveErr.Error()
				items = append(items, item)
				continue
			}
			// Show what the context actually points at once resolved.
			item.Host = resolved.Host
			item.EnvID = resolved.EnvID

			token, tokenErr := cfg.GetToken(resolved.TokenRef)
			if tokenErr != nil {
				item.Version = "—"
				item.Status = fmt.Sprintf("no token: %v", tokenErr)
				items = append(items, item)
				continue
			}

			c, clientErr := NewClientWithHostEnv(resolved.Host, resolved.EnvID, token)
```

`get environments` is a connectivity check, so showing the resolved host is the useful
behaviour — unlike `ctx list` and `config view`, which show what is in the file.

- [ ] **Step 4: Verify the build and full test suite**

Run: `go build ./... && make vet && go test ./pkg/... ./cmd/...`
Expected: build clean, vet clean, all tests pass. Behaviour is unchanged because
`LoadFrom` still expands.

- [ ] **Step 5: Verify the CLI still works end to end**

```bash
go build -o /tmp/dtmgd-t3 .
mkdir -p /tmp/t3 && cd /tmp/t3
cat > .dtmgd.yaml <<'EOF'
apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: ${DT_MANAGED_HOST}
      env-id: ${DT_ENV_ID}
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ${DT_API_TOKEN}
EOF
DT_MANAGED_HOST=https://managed.example.com DT_ENV_ID=abc12345 \
  DT_API_TOKEN=dt0c01.TEST /tmp/dtmgd-t3 config current-context
```

Expected: prints `prod`. No resolution error — load still expands, so `Resolve` is a no-op.

- [ ] **Step 6: Commit**

```bash
gofmt -w pkg/client/client.go pkg/client/multi.go cmd/get_environments.go
git add pkg/client/client.go pkg/client/multi.go cmd/get_environments.go
git commit -m "refactor(client): resolve contexts at the construction boundary

Every path that builds a client now goes through Context.Resolve. This is
a no-op while LoadFrom still expands, and becomes the load-bearing step
once it stops."
```

---

### Task 4: Stop expanding at load

The core change. After this task the leak is fixed.

**Files:**
- Modify: `pkg/config/config.go:122-149` (`LoadFrom`)
- Modify: `pkg/config/config.go:193-212` (`GetToken`)
- Modify: `pkg/config/config_test.go:439-487` (rewrite PR #27's test)

**Interfaces:**
- Consumes: `expand` (Task 1), `UnresolvedVarsError` (Task 2).
- Produces: no signature changes. `LoadFrom` and `GetToken` keep their current signatures; only behaviour changes.

- [ ] **Step 1: Write the failing regression test**

Add to `pkg/config/config_test.go`:

```go
// TestLoadFromDoesNotExpand is the regression test for the credential leak.
// If this fails, expanded secrets are back in the in-memory config and every
// save path can write them to disk.
func TestLoadFromDoesNotExpand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalConfigName)
	original := `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: ${DT_MANAGED_HOST}
      env-id: ${DT_ENV_ID}
      token-ref: prod
tokens:
  - name: prod
    token: ${DT_API_TOKEN}
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")
	t.Setenv("DT_API_TOKEN", "dt0c01.SECRET.VALUE")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.Tokens[0].Token; got != "${DT_API_TOKEN}" {
		t.Errorf("token = %q, want the literal placeholder", got)
	}
	if got := cfg.Contexts[0].Context.Host; got != "${DT_MANAGED_HOST}" {
		t.Errorf("host = %q, want the literal placeholder", got)
	}
	if got := cfg.Contexts[0].Context.EnvID; got != "${DT_ENV_ID}" {
		t.Errorf("env-id = %q, want the literal placeholder", got)
	}
}

// TestSaveToPreservesPlaceholders replaces the refusal test added by PR #27.
// Saving is now allowed because there is nothing expanded to leak.
func TestSaveToPreservesPlaceholders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalConfigName)
	original := `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: ${DT_MANAGED_HOST}
      env-id: ${DT_ENV_ID}
      token-ref: prod
  - name: staging
    context:
      host: https://staging.example.com
      env-id: def67890
      token-ref: prod
tokens:
  - name: prod
    token: ${DT_API_TOKEN}
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")
	t.Setenv("DT_API_TOKEN", "dt0c01.SECRET.VALUE")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}

	// The mutation a real command would make.
	cfg.CurrentContext = "staging"
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo() error = %v, want nil", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	if strings.Contains(got, "dt0c01.SECRET.VALUE") {
		t.Errorf("the expanded token was written into the config:\n%s", got)
	}
	if strings.Contains(got, "https://managed.example.com") {
		t.Errorf("the expanded host was written into the config:\n%s", got)
	}
	for _, want := range []string{"${DT_MANAGED_HOST}", "${DT_ENV_ID}", "${DT_API_TOKEN}"} {
		if !strings.Contains(got, want) {
			t.Errorf("placeholder %s missing after save:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "current-context: staging") {
		t.Errorf("the mutation was not persisted:\n%s", got)
	}
}

// TestSaveToGuardStillRefuses covers the PR #27 backstop directly. Nothing
// sets expandedFrom any more, so it is set by hand here to keep the guard
// under test for direct pkg/config library use.
func TestSaveToGuardStillRefuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalConfigName)

	cfg := makeTestConfig()
	cfg.expandedFrom = path

	if err := cfg.SaveTo(path); err == nil {
		t.Error("SaveTo() succeeded, want refusal when expandedFrom is set")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("SaveTo() wrote the file despite refusing")
	}
}

func TestGetTokenExpandsPlaceholder(t *testing.T) {
	t.Setenv("DT_API_TOKEN", "dt0c01.SECRET.VALUE")

	cfg := &Config{Tokens: []NamedToken{{Name: "prod", Token: "${DT_API_TOKEN}"}}}

	got, err := cfg.GetToken("prod")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "dt0c01.SECRET.VALUE" {
		t.Errorf("GetToken() = %q, want the expanded value", got)
	}
}

func TestGetTokenUnsetPlaceholder(t *testing.T) {
	cfg := &Config{Tokens: []NamedToken{{Name: "prod", Token: "${MISSING_TOKEN_VAR}"}}}

	_, err := cfg.GetToken("prod")
	if err == nil {
		t.Fatal("GetToken() succeeded with an unset variable")
	}

	var uerr *UnresolvedVarsError
	if !errors.As(err, &uerr) {
		t.Fatalf("error type = %T, want *UnresolvedVarsError", err)
	}
	if !reflect.DeepEqual(uerr.Vars, []string{"MISSING_TOKEN_VAR"}) {
		t.Errorf("Vars = %v, want [MISSING_TOKEN_VAR]", uerr.Vars)
	}
}

func TestGetTokenLiteralDollarPreserved(t *testing.T) {
	cfg := &Config{Tokens: []NamedToken{{Name: "prod", Token: "pa$$w0rd"}}}

	got, err := cfg.GetToken("prod")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "pa$$w0rd" {
		t.Errorf("GetToken() = %q, want the dollars preserved", got)
	}
}
```

Add `"errors"` and `"reflect"` to the import block in `pkg/config/config_test.go`.

- [ ] **Step 2: Delete PR #27's `TestSaveToKeepsEnvPlaceholders`**

Remove the whole function (currently `pkg/config/config_test.go:439-487`). It asserts
`SaveTo` refuses, which is exactly the behaviour this task removes.
`TestSaveToPreservesPlaceholders` and `TestSaveToGuardStillRefuses` replace it.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run 'TestLoadFromDoesNotExpand|TestSaveToPreservesPlaceholders|TestGetToken' -v`
Expected: FAIL. `TestLoadFromDoesNotExpand` reports `token = "dt0c01.SECRET.VALUE", want the literal placeholder`; the `SaveTo` test fails on the refusal error; `TestGetTokenUnsetPlaceholder` fails because no error is returned.

- [ ] **Step 4: Remove expansion from `LoadFrom`**

In `pkg/config/config.go`, replace lines 135-148:

```go
	// Deliberately no os.ExpandEnv here. The in-memory Config is a faithful
	// image of the file, so a save can never write an expanded secret back.
	// ${VAR} references are expanded at the point of use — see Context.Resolve
	// and Config.GetToken.
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	// Apply defaults for fields that may be absent in minimal configs.
	if cfg.APIVersion == "" {
		cfg.APIVersion = "dtmgd.io/v1"
	}
	if cfg.Kind == "" {
		cfg.Kind = "Config"
	}
	return &cfg, nil
```

This removes the `expanded := []byte(os.ExpandEnv(string(data)))` line and the
`if !bytes.Equal(data, expanded)` block that set `expandedFrom`.

Remove `"bytes"` from the import block — it becomes unused and the build will fail
otherwise. Keep the `expandedFrom` field on `Config` and the guard in `SaveTo`: nothing
sets the field now, but it stays as a backstop for direct library use (spec D5), and
`TestSaveToGuardStillRefuses` covers it.

- [ ] **Step 5: Make `GetToken` expand the plaintext branch**

In `pkg/config/config.go`, replace the loop body in `GetToken`:

```go
	for _, nt := range c.Tokens {
		if nt.Name == tokenRef {
			if nt.Token == "" {
				return "", fmt.Errorf("token %q not found in keyring (may need to re-add credentials)", tokenRef)
			}
			// The stored value may be a ${VAR} reference. Expand it here, at
			// the point of use, rather than at load.
			token, unset := expand(nt.Token)
			if len(unset) > 0 {
				return "", &UnresolvedVarsError{Vars: unset}
			}
			return token, nil
		}
	}
```

The keyring branch above is unchanged — the keyring never holds placeholders, and
Task 5 makes sure `migrate-tokens` cannot put one there.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./pkg/config/ -v`
Expected: PASS, all tests.

- [ ] **Step 7: Verify the fix against the real CLI**

```bash
go build -o /tmp/dtmgd-t4 .
rm -rf /tmp/t4 && mkdir -p /tmp/t4 && cd /tmp/t4
cat > .dtmgd.yaml <<'EOF'
apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: ${DT_MANAGED_HOST}
      env-id: ${DT_ENV_ID}
      token-ref: prod-token
  - name: staging
    context:
      host: https://staging.example.com
      env-id: def67890
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ${DT_API_TOKEN}
EOF

# Acceptance criterion 1: use-context succeeds and leaks nothing.
DT_MANAGED_HOST=https://managed.example.com DT_ENV_ID=abc12345 \
  DT_API_TOKEN=dt0c01.LIVESECRET /tmp/dtmgd-t4 config use-context staging
grep -q 'dt0c01.LIVESECRET' .dtmgd.yaml && echo "FAIL: token leaked" || echo "PASS: no token in file"
grep -q '${DT_API_TOKEN}' .dtmgd.yaml && echo "PASS: placeholder preserved" || echo "FAIL: placeholder lost"
grep -q 'current-context: staging' .dtmgd.yaml && echo "PASS: mutation persisted" || echo "FAIL: not saved"

# Acceptance criterion 4: config view does not print the secret.
DT_API_TOKEN=dt0c01.LIVESECRET /tmp/dtmgd-t4 config view | grep -q 'dt0c01.LIVESECRET' \
  && echo "FAIL: config view leaked the token" || echo "PASS: config view shows the placeholder"
```

Expected: four `PASS` lines.

- [ ] **Step 8: Verify the whole build**

Run: `go build ./... && make vet && go test ./pkg/... ./cmd/...`
Expected: build clean, vet clean.

- [ ] **Step 9: Commit**

```bash
gofmt -w pkg/config/config.go pkg/config/config_test.go
git add pkg/config/config.go pkg/config/config_test.go
git commit -m "fix(config): stop expanding environment variables at load

LoadFrom no longer runs os.ExpandEnv over the file. The in-memory Config
now mirrors the file exactly, so no save path can write an expanded
secret back, config view no longer prints live tokens, and literal dollar
signs in values survive.

GetToken expands \${VAR} at the point of use instead.

Replaces the refusal test from #27 with a round-trip test; the SaveTo
guard stays as a library-level backstop."
```

---

### Task 5: migrate-tokens skips placeholders

**Files:**
- Modify: `pkg/config/keyring.go:46-62` (`MigrateTokensToKeyring`)
- Create: `pkg/config/keyring_test.go`
- Modify: `cmd/config.go:201-227` (`configMigrateTokensCmd`)

**Interfaces:**
- Consumes: `HasPlaceholder` (Task 1).
- Produces: `func MigrateTokensToKeyring(cfg *Config) (migrated int, skipped []string, err error)` — signature change from `(int, error)`. The only caller is `cmd/config.go:213`.

- [ ] **Step 1: Write the failing test**

Create `pkg/config/keyring_test.go`:

```go
package config

import (
	"reflect"
	"testing"
)

// TestMigrateTokensSkipsPlaceholders verifies the accounting without touching
// the OS keyring: with every token either empty or a placeholder, SetToken is
// never reached, so the test is safe on headless CI.
func TestMigrateTokensSkipsPlaceholders(t *testing.T) {
	cfg := &Config{
		Tokens: []NamedToken{
			{Name: "prod-token", Token: "${DT_API_TOKEN}"},
			{Name: "keyring-token", Token: ""},
			{Name: "eu-token", Token: "${EU_TOKEN}"},
		},
	}

	migrated, skipped, err := MigrateTokensToKeyring(cfg)
	if err != nil {
		t.Fatalf("MigrateTokensToKeyring() error = %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0", migrated)
	}
	if !reflect.DeepEqual(skipped, []string{"prod-token", "eu-token"}) {
		t.Errorf("skipped = %v, want [prod-token eu-token]", skipped)
	}
	// Placeholders must survive so the indirection is not silently dropped.
	if cfg.Tokens[0].Token != "${DT_API_TOKEN}" {
		t.Errorf("placeholder token was modified: %q", cfg.Tokens[0].Token)
	}
	if cfg.Tokens[2].Token != "${EU_TOKEN}" {
		t.Errorf("placeholder token was modified: %q", cfg.Tokens[2].Token)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestMigrateTokensSkipsPlaceholders -v`
Expected: FAIL — compile error, `assignment mismatch: 3 variables but MigrateTokensToKeyring returns 2 values`.

- [ ] **Step 3: Update `MigrateTokensToKeyring`**

In `pkg/config/keyring.go`, replace the function:

```go
// MigrateTokensToKeyring moves plaintext tokens from config to the OS keyring.
// Returns the number migrated and the names of tokens skipped because they are
// ${VAR} references.
//
// Placeholder tokens are skipped rather than migrated: storing the current
// expansion in the keyring would silently convert a live indirection into a
// frozen snapshot, and the user would not find out until the variable changed.
func MigrateTokensToKeyring(cfg *Config) (int, []string, error) {
	ts := NewTokenStore()
	migrated := 0
	var skipped []string

	for i, nt := range cfg.Tokens {
		if nt.Token == "" {
			continue
		}
		if HasPlaceholder(nt.Token) {
			skipped = append(skipped, nt.Name)
			continue
		}
		if err := ts.SetToken(nt.Name, nt.Token); err != nil {
			return migrated, skipped, fmt.Errorf("failed to migrate token %q: %w", nt.Name, err)
		}
		cfg.Tokens[i].Token = ""
		migrated++
	}
	return migrated, skipped, nil
}
```

- [ ] **Step 4: Update the command**

In `cmd/config.go`, replace the `RunE` body of `configMigrateTokensCmd` (lines 205-226):

```go
	RunE: func(cmd *cobra.Command, args []string) error {
		if !config.IsKeyringAvailable() {
			return fmt.Errorf("keyring not available. Tokens will remain in config file")
		}
		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}
		migrated, skipped, err := config.MigrateTokensToKeyring(cfg)
		if err != nil {
			return err
		}
		for _, name := range skipped {
			output.PrintWarning("Skipped %q: it resolves from an environment variable; migrating would freeze the current value", name)
		}
		if migrated == 0 {
			output.PrintInfo("No tokens to migrate")
			return nil
		}
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config after migration: %w", err)
		}
		output.PrintSuccess("Migrated %d token(s) to %s", migrated, config.KeyringBackend())
		return nil
	},
```

The skip warnings are printed before the `migrated == 0` check, so a config where every
token is a placeholder explains itself instead of just reporting "No tokens to migrate".

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/config/ -run TestMigrateTokens -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 6: Verify the whole build**

Run: `make vet && go test ./pkg/... ./cmd/...`
Expected: vet clean, tests pass.

- [ ] **Step 7: Commit**

```bash
gofmt -w pkg/config/keyring.go pkg/config/keyring_test.go cmd/config.go
git add pkg/config/keyring.go pkg/config/keyring_test.go cmd/config.go
git commit -m "fix(config): skip placeholder tokens in migrate-tokens

Migrating \${DT_API_TOKEN} would store its current expansion in the
keyring, converting a live indirection into a frozen snapshot. Skip them
with a warning, and report the skip when nothing was migrated instead of
the misleading 'No tokens to migrate'."
```

---

### Task 6: Clobber and bare-reference warnings

**Files:**
- Modify: `cmd/ctx.go:149-181` (`setContext`)
- Modify: `cmd/config.go:157-188` (extract `setCredentials`)
- Modify: `cmd/root.go:174-209` (`LoadConfig` and `loadConfigRaw`)

**Interfaces:**
- Consumes: `HasPlaceholder`, `BareRefs` (Task 1).
- Produces:
  - `func setCredentials(name, token string) error` in `cmd` — extracted from the `RunE` so Task 7 can test it.
  - `func warnBareRefs(cfg *config.Config)` in `cmd`.

- [ ] **Step 1: Add the bare-reference warning to `cmd/root.go`**

Append to `cmd/root.go`:

```go
// warnBareRefs warns about bare $VAR references, which are no longer expanded.
//
// Without this the failure is silent and confusing: the literal string
// "$DT_API_TOKEN" becomes the token and the user sees an unexplained 401.
// BareRefs only reports names that are actually set, so incidental dollar
// signs in passwords do not trigger it.
func warnBareRefs(cfg *config.Config) {
	seen := make(map[string]bool)
	warn := func(s string) {
		for _, name := range config.BareRefs(s) {
			if seen[name] {
				continue
			}
			seen[name] = true
			output.PrintWarning("%q looks like an environment variable reference. Use ${%s}.", "$"+name, name)
		}
	}
	for _, nc := range cfg.Contexts {
		warn(nc.Context.Host)
		warn(nc.Context.EnvID)
		warn(nc.Context.TokenRef)
		warn(nc.Context.HTTPProxyURL)
		warn(nc.Context.HTTPSProxyURL)
	}
	for _, nt := range cfg.Tokens {
		warn(nt.Token)
	}
}
```

Add `"github.com/dynatrace-oss/dtmgd/pkg/output"` to the import block in `cmd/root.go`
if it is not already there.

Call it from both loaders. In `LoadConfig`, after the `if err != nil { return nil, err }`
block and before the `--context` override:

```go
	warnBareRefs(cfg)
```

In `loadConfigRaw`, replace the body:

```go
func loadConfigRaw() (*config.Config, error) {
	var cfg *config.Config
	var err error
	if cfgFile != "" {
		cfg, err = config.LoadFrom(cfgFile)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil, err
	}
	warnBareRefs(cfg)
	return cfg, nil
}
```

- [ ] **Step 2: Add the clobber warning to `setContext`**

In `cmd/ctx.go`, insert before the `cfg.SetContext(...)` call in `setContext`:

```go
	// Warn before overwriting a ${VAR} reference: the user gets no other
	// signal that the indirection is gone.
	if existing, lookupErr := cfg.GetContext(name); lookupErr == nil {
		type field struct {
			newValue string
			oldValue string
			label    string
		}
		for _, f := range []field{
			{host, existing.Context.Host, "host"},
			{envID, existing.Context.EnvID, "env-id"},
			{tokenRef, existing.Context.TokenRef, "token-ref"},
		} {
			if f.newValue != "" && config.HasPlaceholder(f.oldValue) {
				output.PrintWarning("Replaced %s in the %s field — that environment variable no longer affects this context.", f.oldValue, f.label)
			}
		}
	}
```

- [ ] **Step 3: Extract and warn in `setCredentials`**

In `cmd/config.go`, replace the `configSetCredentialsCmd` `RunE` with a call to a new
package function, and add the function below the command definitions:

```go
	RunE: func(cmd *cobra.Command, args []string) error {
		token, _ := cmd.Flags().GetString("token")
		if token == "" {
			return fmt.Errorf("--token is required")
		}
		return setCredentials(args[0], token)
	},
```

```go
// setCredentials stores an API token under the given name.
func setCredentials(name, token string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		cfg = config.NewConfig()
	}

	// Warn before the placeholder disappears. This matters most in the keyring
	// case, where SetToken writes token: "" and the ${VAR} reference vanishes
	// without the new token ever appearing in the file.
	for _, nt := range cfg.Tokens {
		if nt.Name == name && config.HasPlaceholder(nt.Token) {
			output.PrintWarning("Replaced %s in the config — that environment variable no longer affects this token.", nt.Token)
			break
		}
	}

	if err := cfg.SetToken(name, token); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	if config.IsKeyringAvailable() {
		output.PrintSuccess("Credential %q stored securely in %s", name, config.KeyringBackend())
	} else {
		output.PrintWarning("Credential %q saved (plaintext — keyring not available)", name)
	}
	return nil
}
```

- [ ] **Step 4: Verify the build and tests**

Run: `go build ./... && make vet && go test ./pkg/... ./cmd/...`
Expected: build clean, vet clean, tests pass.

- [ ] **Step 5: Verify the warnings fire**

```bash
go build -o /tmp/dtmgd-t6 .
rm -rf /tmp/t6 && mkdir -p /tmp/t6 && cd /tmp/t6
/tmp/dtmgd-t6 config init
/tmp/dtmgd-t6 config set-credentials my-token --token dt0c01.REAL
```

Expected: a warning naming `${DT_API_TOKEN}`, then the credential-saved line.

```bash
rm -rf /tmp/t6b && mkdir -p /tmp/t6b && cd /tmp/t6b
cat > .dtmgd.yaml <<'EOF'
apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: $DT_MANAGED_HOST
      env-id: abc12345
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ""
EOF
DT_MANAGED_HOST=https://managed.example.com /tmp/dtmgd-t6 config current-context
```

Expected: `"$DT_MANAGED_HOST" looks like an environment variable reference. Use ${DT_MANAGED_HOST}.` followed by `prod`.

- [ ] **Step 6: Commit**

```bash
gofmt -w cmd/root.go cmd/ctx.go cmd/config.go
git add cmd/root.go cmd/ctx.go cmd/config.go
git commit -m "feat(cmd): warn when placeholders are replaced or written bare

Overwriting a \${VAR} reference is allowed but now announced — in the
keyring case it is the only signal the user gets, since the token never
appears in the file.

Also warns about bare \$VAR, which no longer expands; without it the
literal string becomes the token and the user sees a bare 401."
```

---

### Task 7: End-to-end command tests and documentation

**Files:**
- Create: `cmd/config_test.go`
- Modify: `README.md:353`

**Interfaces:**
- Consumes: `setContext`, `useContext`, `setCredentials` from `cmd`; the `cfgFile` package variable from `cmd/root.go:16`.
- Produces: nothing.

- [ ] **Step 1: Write the failing test**

Create `cmd/config_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempConfig points the cmd package at a temp config file for one test and
// restores the previous value afterwards.
func withTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".dtmgd.yaml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	prev := cfgFile
	cfgFile = path
	t.Cleanup(func() { cfgFile = prev })
	return path
}

const placeholderConfig = `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: ${DT_MANAGED_HOST}
      env-id: ${DT_ENV_ID}
      token-ref: prod-token
  - name: staging
    context:
      host: https://staging.example.com
      env-id: def67890
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ${DT_API_TOKEN}
`

// TestUseContextPreservesPlaceholders is acceptance criterion 1: the exact
// scenario that wrote a live token into .dtmgd.yaml before this change.
func TestUseContextPreservesPlaceholders(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)

	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")
	t.Setenv("DT_API_TOKEN", "dt0c01.LIVESECRET")

	if err := useContext("staging"); err != nil {
		t.Fatalf("useContext() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	if strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("the live token was written into the config:\n%s", got)
	}
	if strings.Contains(got, "https://managed.example.com") {
		t.Errorf("the expanded host was written into the config:\n%s", got)
	}
	for _, want := range []string{"${DT_MANAGED_HOST}", "${DT_ENV_ID}", "${DT_API_TOKEN}"} {
		if !strings.Contains(got, want) {
			t.Errorf("placeholder %s missing after save:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "current-context: staging") {
		t.Errorf("the context switch was not persisted:\n%s", got)
	}
}

// TestSetCredentialsPreservesOtherPlaceholders is acceptance criterion 2: the
// documented onboarding flow, which used to empty host and env-id.
//
// It deliberately does not assert on how the token itself was stored:
// IsKeyringAvailable probes the real OS keyring, so that branch differs
// between headless CI and a developer Mac. Host and env-id are unaffected by
// the keyring either way.
func TestSetCredentialsPreservesOtherPlaceholders(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)

	// Variables deliberately unset — the wipe case.
	t.Setenv("DT_MANAGED_HOST", "")
	os.Unsetenv("DT_MANAGED_HOST")
	t.Setenv("DT_ENV_ID", "")
	os.Unsetenv("DT_ENV_ID")

	if err := setCredentials("prod-token", "dt0c01.REAL"); err != nil {
		t.Fatalf("setCredentials() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	for _, want := range []string{"${DT_MANAGED_HOST}", "${DT_ENV_ID}"} {
		if !strings.Contains(got, want) {
			t.Errorf("placeholder %s was wiped:\n%s", want, got)
		}
	}
	if strings.Contains(got, `host: ""`) {
		t.Errorf("host was emptied:\n%s", got)
	}
}

// TestUseContextPreservesLiteralDollars is acceptance criterion 3.
func TestUseContextPreservesLiteralDollars(t *testing.T) {
	const cfg = `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: https://managed.example.com
      env-id: abc12345
      token-ref: prod-token
      http-proxy: http://user:pa$$w0rd@proxy.corp:8080
  - name: staging
    context:
      host: https://staging.example.com
      env-id: def67890
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ""
`
	path := withTempConfig(t, cfg)

	if err := useContext("staging"); err != nil {
		t.Fatalf("useContext() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "pa$$w0rd") {
		t.Errorf("the literal dollars were corrupted:\n%s", after)
	}
}

// TestSetContextOnPlaceholderConfigSucceeds confirms the read-only dead end
// left by PR #27 is gone.
func TestSetContextOnPlaceholderConfigSucceeds(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)

	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")
	t.Setenv("DT_API_TOKEN", "dt0c01.LIVESECRET")

	if err := setContext("newenv", "https://new.example.com", "xyz99999", "prod-token", ""); err != nil {
		t.Fatalf("setContext() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	if !strings.Contains(got, "newenv") {
		t.Errorf("the new context was not written:\n%s", got)
	}
	if !strings.Contains(got, "${DT_API_TOKEN}") {
		t.Errorf("the existing placeholder was lost:\n%s", got)
	}
	if strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("the live token was written into the config:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./cmd/ -run 'TestUseContext|TestSetCredentials|TestSetContextOn' -v`
Expected: PASS. These exercise Tasks 4 and 6, which are already implemented, so they
should pass immediately — they are acceptance tests, not TDD drivers. If any fail, the
bug is in the earlier task, not the test.

- [ ] **Step 3: Update the README**

In `README.md`, replace line 353:

```markdown
Environment variables are expanded in the config file: `${DT_MANAGED_HOST}`.
```

with:

```markdown
### Environment variables in the config

Values can reference environment variables with `${VAR_NAME}`:

```yaml
host: ${DT_MANAGED_HOST}
env-id: ${DT_ENV_ID}
token: ${DT_API_TOKEN}
```

Only the braced `${VAR_NAME}` form is expanded. A bare `$VAR_NAME` is treated as
literal text, so dollar signs in values — a proxy password, for example — are left
alone.

References are resolved when a value is used, not when the file is read. Config
commands (`use-context`, `set-context`, `set-credentials`, `delete-context`) preserve
them, so `${DT_API_TOKEN}` stays in the file rather than being replaced by the token
it currently resolves to. `dtmgd config view` shows the reference for the same reason.

If a referenced variable is unset, the command that needs it fails and names it. Other
contexts are unaffected, so a multi-context config still works when you hold
credentials for only one of them.
```

- [ ] **Step 4: Verify the whole suite**

Run: `go build ./... && make vet && go test ./pkg/... ./cmd/...`
Expected: build clean, vet clean, all tests pass.

- [ ] **Step 5: Commit**

```bash
gofmt -w cmd/config_test.go
git add cmd/config_test.go README.md
git commit -m "test(cmd): cover config commands on placeholder configs

Adds the four acceptance scenarios end to end: the token leak, the config
wipe, literal-dollar corruption, and the read-only dead end.

Documents \${VAR} as the only supported syntax."
```

---

## Known pre-existing failure

`go test ./pkg/skills/` fails on `TestInstallAndStatusAndUninstall` and `TestStatusAll`
with "expected not installed in fresh dir". This reproduces on `main` at `f55592c`,
before any change in this plan — the tests read the real home directory rather than an
isolated one, so they fail wherever `dtmgd` skills are already installed. Unrelated to
this work; do not try to fix it here.

Use `go test ./pkg/config/... ./pkg/client/... ./cmd/...` to get a clean signal.

## Verification checklist

Before opening the PR, confirm all four acceptance criteria from the spec:

1. `use-context` on a placeholder config with variables set: succeeds; the file still
   contains `${DT_API_TOKEN}` and no token value. — `TestUseContextPreservesPlaceholders`
2. `config init` then `set-credentials` with variables unset: succeeds; `host` and
   `env-id` remain placeholders. — `TestSetCredentialsPreservesOtherPlaceholders`
3. A config with `http-proxy: http://user:pa$$w0rd@proxy.corp:8080` survives a
   `use-context`, and the client receives `pa$$w0rd`. —
   `TestUseContextPreservesLiteralDollars` plus `TestResolveSuccess`
4. `config view` with all variables set prints `${DT_API_TOKEN}`, not the token. —
   Task 4 Step 7

Then: `go build ./... && make vet && gofmt -l . && go test ./pkg/config/... ./pkg/client/... ./cmd/...`
