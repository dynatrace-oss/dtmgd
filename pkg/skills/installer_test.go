package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSupportedAgents(t *testing.T) {
	got := SupportedAgents()
	want := []string{"claude", "copilot", "cursor", "kiro", "junie", "opencode"}
	if len(got) != len(want) {
		t.Fatalf("expected %d agents, got %d: %v", len(want), len(got), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("agent[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestFindAgent(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"claude", true},
		{"copilot", true},
		{"cross-client", true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, ok := FindAgent(tt.name)
			if ok != tt.found {
				t.Errorf("FindAgent(%q) found = %v, want %v", tt.name, ok, tt.found)
			}
			if ok && agent.Name != tt.name {
				t.Errorf("FindAgent(%q) = %q", tt.name, agent.Name)
			}
		})
	}
}

func TestDetectAgent(t *testing.T) {
	// Clear all known agent env vars.
	for _, a := range agents {
		if a.EnvVar != "" {
			t.Setenv(a.EnvVar, "")
		}
	}

	if _, ok := DetectAgent(); ok {
		t.Error("DetectAgent() should return false when no env var is set")
	}

	t.Setenv("CLAUDECODE", "1")
	agent, ok := DetectAgent()
	if !ok {
		t.Fatal("DetectAgent() should return true when CLAUDECODE is set")
	}
	if agent.Name != "claude" {
		t.Errorf("DetectAgent() = %q, want %q", agent.Name, "claude")
	}
}

func TestInstallAndStatusAndUninstall(t *testing.T) {
	baseDir := t.TempDir()
	agent, _ := FindAgent("claude")

	// Initially not installed
	if s := Status(agent, baseDir); s.Installed {
		t.Fatal("expected not installed in fresh dir")
	}

	// Install
	res, err := Install(agent, baseDir, false, false)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if res.Replaced {
		t.Error("Replaced should be false for fresh install")
	}

	// SKILL.md exists
	skillFile := filepath.Join(res.Path, "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("SKILL.md not found at %s: %v", skillFile, err)
	}

	// Status reports installed
	s := Status(agent, baseDir)
	if !s.Installed {
		t.Fatal("expected installed after Install")
	}
	if s.Path != res.Path {
		t.Errorf("Status path = %q, want %q", s.Path, res.Path)
	}

	// Re-install without force should error
	if _, err := Install(agent, baseDir, false, false); err == nil {
		t.Error("expected error when reinstalling without force")
	}

	// Re-install with force should succeed and report Replaced
	res2, err := Install(agent, baseDir, false, true)
	if err != nil {
		t.Fatalf("force Install error: %v", err)
	}
	if !res2.Replaced {
		t.Error("Replaced should be true on force overwrite")
	}

	// Uninstall
	removed, err := Uninstall(agent, baseDir)
	if err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}
	if len(removed) != 1 || removed[0] != res.Path {
		t.Errorf("Uninstall removed = %v, want [%s]", removed, res.Path)
	}
	if s := Status(agent, baseDir); s.Installed {
		t.Error("expected not installed after Uninstall")
	}
}

func TestStatusAll(t *testing.T) {
	baseDir := t.TempDir()
	results := StatusAll(baseDir)
	// 1 cross-client + len(agents) regular agents
	if len(results) != len(agents)+1 {
		t.Errorf("StatusAll returned %d results, want %d", len(results), len(agents)+1)
	}
	for _, r := range results {
		if r.Installed {
			t.Errorf("agent %q should not be installed in fresh dir", r.Agent.Name)
		}
	}
}

// fakeHome points the home directory at a temp dir for one test, so the
// global location Uninstall removes is never the developer's real one.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	return home
}

// mkSkillDir creates dir with a file in it.
func mkSkillDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// uninstallFixture creates, under a fresh base and home, the skill locations
// of paths that a TestUninstallLocations case says exist. It returns the base,
// the global location, and what Uninstall(installed, base) should report
// removing: the project location, then the global one if installed has one.
func uninstallFixture(t *testing.T, paths, installed Agent, project, global bool) (base, globalDir string, want []string) {
	t.Helper()
	base, home := t.TempDir(), fakeHome(t)
	projectDir := filepath.Join(base, paths.ProjectPath)
	globalDir = filepath.Join(home, paths.GlobalPath)
	if project {
		mkSkillDir(t, projectDir)
		want = append(want, projectDir)
	}
	if global {
		mkSkillDir(t, globalDir)
		if installed.GlobalPath != "" {
			want = append(want, globalDir)
		}
	}
	return base, globalDir, want
}

// assertGone checks that every dir in dirs no longer exists.
func assertGone(t *testing.T, label string, dirs []string) {
	t.Helper()
	for _, dir := range dirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s: %s still exists (%v)", label, dir, err)
		}
	}
}

// TestUninstallLocations pins what Uninstall removes and reports: the
// project location, then the global one, each only if it exists; an agent
// without a global path has only the project location.
func TestUninstallLocations(t *testing.T) {
	agent := Agent{Name: "test", ProjectPath: filepath.Join(".test", "skills", "dtmgd"), GlobalPath: filepath.Join(".test-global", "skills", "dtmgd")}
	projectOnly := Agent{Name: "test", ProjectPath: agent.ProjectPath}

	cases := []struct {
		name            string
		agent           Agent
		project, global bool // which locations exist beforehand
	}{
		{"both", agent, true, true},
		{"project only", agent, true, false},
		{"global only", agent, false, true},
		{"neither", agent, false, false},
		{"no global path", projectOnly, true, true},
	}
	for _, tc := range cases {
		base, globalDir, want := uninstallFixture(t, agent, tc.agent, tc.project, tc.global)
		removed, err := Uninstall(tc.agent, base)
		if err != nil || !reflect.DeepEqual(removed, want) {
			t.Errorf("%s: Uninstall = %q, %v; want %q, nil", tc.name, removed, err, want)
		}
		assertGone(t, tc.name, want)
		if tc.agent.GlobalPath == "" && tc.global {
			if _, err := os.Stat(globalDir); err != nil {
				t.Errorf("%s: %s was removed without a global path: %v", tc.name, globalDir, err)
			}
		}
	}
}

// TestUninstallErrors pins Uninstall's failures: without a home directory the
// project location is still removed and the error says so; a location that
// cannot be removed is reported, and the next one still tried, with the
// messages joined by "; ".
func TestUninstallErrors(t *testing.T) {
	agent := Agent{Name: "test", ProjectPath: filepath.Join(".test", "skills", "dtmgd"), GlobalPath: filepath.Join(".test-global", "skills", "dtmgd")}

	t.Run("no home directory", func(t *testing.T) {
		base := t.TempDir()
		projectDir := filepath.Join(base, agent.ProjectPath)
		mkSkillDir(t, projectDir)
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		_, homeErr := os.UserHomeDir()
		if homeErr == nil {
			t.Skip("no way to make os.UserHomeDir fail here")
		}
		removed, err := Uninstall(agent, base)
		want := "failed to determine home directory: " + homeErr.Error()
		if err == nil || err.Error() != want || !reflect.DeepEqual(removed, []string{projectDir}) {
			t.Errorf("Uninstall = %q, %v; want [%s], %s", removed, err, projectDir, want)
		}
	})

	t.Run("cannot remove", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("needs POSIX permissions enforced")
		}
		base, home := t.TempDir(), fakeHome(t)
		projectDir := filepath.Join(base, agent.ProjectPath)
		globalDir := filepath.Join(home, agent.GlobalPath)
		for _, dir := range []string{projectDir, globalDir} {
			mkSkillDir(t, dir)
			parent := filepath.Dir(dir)
			if err := os.Chmod(parent, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(parent, 0o750) })
		}
		removed, err := Uninstall(agent, base)
		if removed != nil || err == nil {
			t.Fatalf("Uninstall = %q, %v; want nil and an error", removed, err)
		}
		parts := strings.Split(err.Error(), "; ")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "failed to remove "+projectDir+": ") ||
			!strings.HasPrefix(parts[1], "failed to remove "+globalDir+": ") {
			t.Errorf("err = %v, want one failure per location, project first", err)
		}
	})
}
