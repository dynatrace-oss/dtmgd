package cmd

import (
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func testRoot() *cobra.Command {
	return newRootCmd(&App{Out: io.Discard, Err: io.Discard})
}

// TestEveryAPICommandDeclaresScopes pins that a command that calls the API
// says which token scopes it needs; the 403 hint and the README list are
// built from these declarations.
func TestEveryAPICommandDeclaresScopes(t *testing.T) {
	for _, verb := range testRoot().Commands() {
		if verb.Name() != "get" && verb.Name() != "describe" && verb.Name() != "query" {
			continue
		}
		for _, c := range verb.Commands() {
			if c.Runnable() && len(requiredScopes(c)) == 0 {
				t.Errorf("%s declares no token scopes", c.CommandPath())
			}
		}
	}
}

var readmeScope = regexp.MustCompile("^- .*\\(`([A-Za-z.]+)`\\)")

// TestReadmeScopeListMatchesCommands pins that the README's scope list is
// exactly the scopes the commands declare, so it cannot drift again (three
// conflicting lists existed before).
func TestReadmeScopeListMatchesCommands(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start, end := strings.Index(s, "<!-- scopes:start -->"), strings.Index(s, "<!-- scopes:end -->")
	if start < 0 || end < start {
		t.Fatal("README has no <!-- scopes:start --> ... <!-- scopes:end --> section")
	}
	var listed []string
	for _, line := range strings.Split(s[start:end], "\n") {
		if m := readmeScope.FindStringSubmatch(line); m != nil {
			listed = append(listed, m[1])
		}
	}
	sort.Strings(listed)
	want := allScopes(testRoot())
	if strings.Join(listed, ",") != strings.Join(want, ",") {
		t.Errorf("README lists %v, commands declare %v", listed, want)
	}
}

// Test403HintNamesScopes pins that a permission error names the scope the
// failed command needs.
func Test403HintNamesScopes(t *testing.T) {
	api := newFakeAPI(t, map[string]route{"GET /e/env1/api/v2/slo": {Status: 403, File: "error_403.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "slos")
	if !strings.Contains(r.Stderr, "slo.read") || strings.Contains(r.Stderr, "ReadConfig") {
		t.Errorf("stderr = %q", r.Stderr)
	}
}

// TestSetContextHelpListsScopes pins that set-context's help shows the
// declared scopes rather than a hand-kept list.
func TestSetContextHelpListsScopes(t *testing.T) {
	root := testRoot()
	c, _, err := root.Find([]string{"config", "set-context"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range allScopes(root) {
		if !strings.Contains(c.Long, s) {
			t.Errorf("set-context help does not list %s", s)
		}
	}
	if strings.Contains(c.Long, "ReadConfig") || strings.Contains(c.Long, scopesPlaceholder) {
		t.Errorf("stale or unfilled scope list:\n%s", c.Long)
	}
}
