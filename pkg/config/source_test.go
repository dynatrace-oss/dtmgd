package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLocalConfig drops a minimal .dtmgd.yaml into dir.
func writeLocalConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, LocalConfigName)
	if err := os.WriteFile(path, []byte("apiVersion: dtmgd.io/v1\nkind: Config\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestResolveSourceExplicit: --config wins over everything, and is never
// treated as invisible — the user typed the path.
func TestResolveSourceExplicit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeLocalConfig(t, dir)

	src := ResolveSource("/etc/dtmgd/elsewhere.yaml")

	if src.Kind != SourceExplicit {
		t.Errorf("Kind = %v, want SourceExplicit", src.Kind)
	}
	if src.Path != "/etc/dtmgd/elsewhere.yaml" {
		t.Errorf("Path = %q, want the explicit path", src.Path)
	}
	if src.Invisible() {
		t.Error("an explicitly named path was reported as invisible")
	}
}

// TestResolveSourceLocalCWD: the documented project-local workflow. The file
// is in the directory the user is standing in, so it does not warrant a
// warning on every command.
func TestResolveSourceLocalCWD(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	want := writeLocalConfig(t, dir)

	src := ResolveSource("")

	if src.Kind != SourceLocalCWD {
		t.Fatalf("Kind = %v, want SourceLocalCWD", src.Kind)
	}
	if !sameFile(src.Path, want) {
		t.Errorf("Path = %q, want %q", src.Path, want)
	}
	if src.Invisible() {
		t.Error("a config in the working directory was reported as invisible")
	}
}

// TestResolveSourceLocalAncestor is PRISM-13820: a .dtmgd.yaml planted in a
// parent directory is picked up by the upward search, and nothing in the
// user's working directory reveals that it exists.
func TestResolveSourceLocalAncestor(t *testing.T) {
	parent := t.TempDir()
	want := writeLocalConfig(t, parent)

	child := filepath.Join(parent, "project", "nested")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)

	src := ResolveSource("")

	if src.Kind != SourceLocalAncestor {
		t.Fatalf("Kind = %v, want SourceLocalAncestor", src.Kind)
	}
	if !sameFile(src.Path, want) {
		t.Errorf("Path = %q, want the planted file %q", src.Path, want)
	}
	if !src.Invisible() {
		t.Error("a config from a parent directory was not reported as invisible")
	}
}

// TestResolveSourceGlobal: with no local config anywhere above, the global
// path is used and nothing is reported.
func TestResolveSourceGlobal(t *testing.T) {
	t.Chdir(t.TempDir())

	// t.TempDir sits under the OS temp directory, which is world-writable and
	// may itself hold a .dtmgd.yaml — including one a previous exploit demo
	// left behind. That is exactly the attack this finding describes, so the
	// honest response is to skip rather than to pretend the search was clean.
	if local := FindLocalConfig(); local != "" {
		t.Skipf("a local config exists above the temp dir: %s", local)
	}

	src := ResolveSource("")

	if src.Kind != SourceGlobal {
		t.Errorf("Kind = %v, want SourceGlobal", src.Kind)
	}
	if src.Path != DefaultConfigPath() {
		t.Errorf("Path = %q, want %q", src.Path, DefaultConfigPath())
	}
	if src.Invisible() {
		t.Error("the global config was reported as invisible")
	}
}
