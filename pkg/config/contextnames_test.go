package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeContexts writes a config with one context per name and returns its path.
func writeContexts(t *testing.T, names ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("apiVersion: dtmgd.io/v1\nkind: Config\ncontexts:\n")
	for _, n := range names {
		b.WriteString("  - name: \"" + n + "\"\n    context:\n      host: https://example.invalid\n      env-id: env1\n")
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDuplicateContextNameIsRejected pins that a config naming one context
// twice does not load, and that the error says which file, which name, how
// often, and that the fix is editing the file (no dtmgd command can repair it).
func TestDuplicateContextNameIsRejected(t *testing.T) {
	path := writeContexts(t, "prod", "stage", "prod")
	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("LoadFrom accepted a duplicate context name")
	}
	for _, want := range []string{path, `"prod"`, "2 times", "editing the file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestSemicolonInContextNameIsRejected pins that a context name containing
// ';' does not load: --env "a;b" splits on it, so such a context could never
// be selected.
func TestSemicolonInContextNameIsRejected(t *testing.T) {
	path := writeContexts(t, "a;b")
	_, err := LoadFrom(path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), `"a;b" contains ';'`) {
		t.Errorf("LoadFrom = %v, want an error naming the file and the name", err)
	}
}

// TestDistinctContextNamesLoad pins that names differing only in case are
// distinct (matching is exact), so they still load.
func TestDistinctContextNamesLoad(t *testing.T) {
	cfg, err := LoadFrom(writeContexts(t, "prod", "Prod"))
	if err != nil || len(cfg.Contexts) != 2 {
		t.Errorf("LoadFrom = %v, %v", cfg, err)
	}
}
