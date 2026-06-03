package skills

import (
	"os"
	"path/filepath"
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
