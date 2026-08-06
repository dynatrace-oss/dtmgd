package skills

import (
	"os"
	"path/filepath"
	"testing"

	dtmgdskill "github.com/dynatrace-oss/dtmgd/skills/dtmgd"
)

// TestInstallForcePreservesUserFiles is PRISM-13823 note 6.
//
// Install used to call os.RemoveAll(skillDir) whenever --force was passed and
// a SKILL.md already existed. The whole directory went, not just the file the
// command manages, and the only output — printed after the deletion — said
// "Updated". Anything the user kept next to SKILL.md was unrecoverable.
func TestInstallForcePreservesUserFiles(t *testing.T) {
	base := t.TempDir()
	agent, ok := FindAgent("claude")
	if !ok {
		t.Fatal("claude agent not found in the registry")
	}

	// First install creates the directory and SKILL.md.
	first, err := Install(agent, base, false, false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(first.Preserved) != 0 {
		t.Errorf("fresh install reported preserved files: %v", first.Preserved)
	}

	// The user adds their own material beside it, and edits SKILL.md.
	notes := filepath.Join(first.Path, "my-notes.md")
	const notesBody = "cluster quirks I do not want to lose"
	if err := os.WriteFile(notes, []byte(notesBody), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(first.Path, "reference")
	if err := os.MkdirAll(nested, 0750); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(nested, "queries.md")
	if err := os.WriteFile(extra, []byte("saved selectors"), 0600); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(first.Path, skillFileName)
	if err := os.WriteFile(skillFile, []byte("locally edited"), 0600); err != nil {
		t.Fatal(err)
	}

	// Re-install with --force.
	second, err := Install(agent, base, false, true)
	if err != nil {
		t.Fatalf("Install(force) error = %v", err)
	}
	if !second.Replaced {
		t.Error("Replaced = false, want true on a forced re-install")
	}

	// The user's files survive.
	got, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("--force deleted the user's file: %v", err)
	}
	if string(got) != notesBody {
		t.Errorf("user file content changed: %q", got)
	}
	if _, err := os.ReadFile(extra); err != nil {
		t.Errorf("--force deleted a nested user file: %v", err)
	}

	// SKILL.md is genuinely refreshed — that is what --force is for.
	refreshed, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(refreshed) == "locally edited" {
		t.Error("--force did not overwrite SKILL.md")
	}

	// And the user is told what was kept.
	if len(second.Preserved) != 2 {
		t.Errorf("Preserved = %v, want the two user files", second.Preserved)
	}
}

// TestUnmanagedFilesOnMissingDir covers the first-install case: nothing to
// inspect is not an error.
func TestUnmanagedFilesOnMissingDir(t *testing.T) {
	got, err := unmanagedFiles(filepath.Join(t.TempDir(), "absent"), dtmgdskill.Content)
	if err != nil {
		t.Fatalf("unmanagedFiles() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unmanagedFiles() = %v, want empty", got)
	}
}
