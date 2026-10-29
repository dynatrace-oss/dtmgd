package cmd

import (
	"strings"
	"testing"
)

// TestInvalidOutputFormatIsRejected pins that an unknown -o value fails before
// any request is made, instead of silently falling back to a table.
func TestInvalidOutputFormatIsRejected(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "xml")
	if r.Code == 0 {
		t.Errorf("code = 0, want non-zero")
	}
	if !strings.Contains(r.Stderr, `unknown output format "xml"`) {
		t.Errorf("stderr = %q", r.Stderr)
	}
	if n := api.Hits(problemsPath); n != 0 {
		t.Errorf("API hits = %d, want 0", n)
	}
}

// TestYmlAliasPrintsAPIShapedYAML pins that -o yml is the same as -o yaml.
func TestYmlAliasPrintsAPIShapedYAML(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	yml := runCLI(t, cfg, "get", "problems", "-o", "yml")
	yaml := runCLI(t, cfg, "get", "problems", "-o", "yaml")
	if yml.Stdout != yaml.Stdout || yml.Code != yaml.Code {
		t.Errorf("-o yml differs from -o yaml:\n--- yml\n%s\n--- yaml\n%s", yml.Stdout, yaml.Stdout)
	}
}
