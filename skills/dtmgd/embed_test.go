package dtmgdskill

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// referenceFiles lists skills/dtmgd/references on disk.
func referenceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("references")
	if err != nil {
		t.Fatalf("reading references/: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		t.Fatal("references/ is empty")
	}
	return names
}

func TestContentEmbedsEveryReference(t *testing.T) {
	for _, name := range referenceFiles(t) {
		if _, err := fs.ReadFile(Content, "references/"+name); err != nil {
			t.Errorf("references/%s is not embedded: %v", name, err)
		}
	}
}

func TestEveryReferenceIsLinkedFromSkill(t *testing.T) {
	skill, err := fs.ReadFile(Content, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range referenceFiles(t) {
		if !strings.Contains(string(skill), "references/"+name) {
			t.Errorf("SKILL.md does not point to references/%s", name)
		}
	}
}

func TestSkillHasNoDemoContent(t *testing.T) {
	err := fs.WalkDir(Content, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(Content, path)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(data)), "bookstore") {
			t.Errorf("%s mentions the BookStore demo", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
