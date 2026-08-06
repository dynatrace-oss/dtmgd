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
