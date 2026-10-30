package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// referenceDir is the generated command reference, relative to this package.
const referenceDir = "../docs/reference"

// genReferenceDocs writes the command reference, one Markdown page per
// command, into dir. It lives in a test so cobra/doc never reaches the
// binary; make docs runs it through TestReferenceDocsAreCurrent -update.
func genReferenceDocs(dir string) error {
	root := newRootCmd(&App{Out: io.Discard, Err: io.Discard})
	var prepare func(*cobra.Command)
	prepare = func(c *cobra.Command) {
		c.DisableAutoGenTag = true // the tag carries the generation date
		c.Long = fenceIndented(c.Long)
		for _, sub := range c.Commands() {
			prepare(sub)
		}
	}
	prepare(root)
	return doc.GenMarkdownTree(root, dir)
}

// fenceIndented puts each run of lines indented by two or more spaces (the
// examples and aligned lists of the help texts) in a fenced code block, so
// Markdown keeps their line breaks instead of joining them into a paragraph.
func fenceIndented(long string) string {
	var out, blanks []string // blanks: blank lines seen inside a block, not yet placed
	in := false
	for _, line := range strings.Split(long, "\n") {
		switch {
		case strings.HasPrefix(line, "  "):
			if !in {
				out, in = append(out, "```"), true
			}
			out, blanks = append(append(out, blanks...), line), nil
		case in && strings.TrimSpace(line) == "":
			blanks = append(blanks, line)
		default:
			if in {
				out, in = append(out, "```"), false
			}
			out, blanks = append(append(out, blanks...), line), nil
		}
	}
	if in {
		out = append(out, "```")
	}
	return strings.Join(append(out, blanks...), "\n")
}

// diffDirs names every file that differs between the directories want and
// got, sorted: "missing: x" (only in want), "extra: y" (only in got) and
// "changed: z".
func diffDirs(want, got string) ([]string, error) {
	read := func(dir string) (map[string][]byte, error) {
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		files := map[string][]byte{}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			files[e.Name()] = data
		}
		return files, nil
	}
	w, err := read(want)
	if err != nil {
		return nil, err
	}
	g, err := read(got)
	if err != nil {
		return nil, err
	}
	var diffs []string
	for name, data := range w {
		switch other, ok := g[name]; {
		case !ok:
			diffs = append(diffs, "missing: "+name)
		case !bytes.Equal(data, other):
			diffs = append(diffs, "changed: "+name)
		}
	}
	for name := range g {
		if _, ok := w[name]; !ok {
			diffs = append(diffs, "extra: "+name)
		}
	}
	sort.Strings(diffs)
	return diffs, nil
}

func TestFenceIndentedFencesEachIndentedRun(t *testing.T) {
	long := "Lists things.\n\nExamples:\n  dtmgd get x\n  dtmgd get x -o json\n\nMore text.\n  one more"
	want := "Lists things.\n\nExamples:\n```\n  dtmgd get x\n  dtmgd get x -o json\n```\n\nMore text.\n```\n  one more\n```"
	if got := fenceIndented(long); got != want {
		t.Errorf("fenceIndented =\n%s\nwant\n%s", got, want)
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiffDirsNamesMissingExtraAndChangedFiles(t *testing.T) {
	want, got := t.TempDir(), t.TempDir()
	writeFiles(t, want, map[string]string{"a.md": "a", "b.md": "1", "same.md": "s"})
	writeFiles(t, got, map[string]string{"b.md": "2", "c.md": "c", "same.md": "s"})
	diffs, err := diffDirs(want, got)
	if err != nil {
		t.Fatal(err)
	}
	if w := []string{"changed: b.md", "extra: c.md", "missing: a.md"}; !reflect.DeepEqual(diffs, w) {
		t.Errorf("diffDirs = %q, want %q", diffs, w)
	}
}

// TestReferenceDocsAreCurrent fails when docs/reference no longer matches the
// commands. make docs (-update) regenerates it.
func TestReferenceDocsAreCurrent(t *testing.T) {
	tmp := t.TempDir()
	if err := genReferenceDocs(tmp); err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.RemoveAll(referenceDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(referenceDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := genReferenceDocs(referenceDir); err != nil {
			t.Fatal(err)
		}
		return
	}
	// What the commands generate is what docs/reference must hold.
	diffs, err := diffDirs(tmp, referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) > 0 {
		t.Errorf("docs/reference is stale (%s): run make docs", strings.Join(diffs, ", "))
	}
}

func TestReferenceDocsHaveNoMachineSpecificText(t *testing.T) {
	tmp := t.TempDir()
	if err := genReferenceDocs(tmp); err != nil {
		t.Fatal(err)
	}
	banned := []string{"\r\n", "%SCOPES%"}
	if home, err := os.UserHomeDir(); err == nil && len(home) >= 5 {
		banned = append(banned, home)
	}
	if td := os.TempDir(); len(td) >= 5 {
		banned = append(banned, td)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range banned {
			if strings.Contains(string(data), b) {
				t.Errorf("%s contains %q", e.Name(), b)
			}
		}
	}
}
