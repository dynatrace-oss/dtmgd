package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadFromDoesNotEchoFileContent is PRISM-13823 note 2. Because --config
// accepts any readable path, a yaml type error that quotes the offending value
// turns the flag into a short read primitive: `dtmgd --config /etc/hostname`
// printed the machine's hostname to stderr, and into agent-mode JSON.
func TestLoadFromDoesNotEchoFileContent(t *testing.T) {
	dir := t.TempDir()

	// A single scalar where a mapping is expected — the shape /etc/hostname
	// has, and the shape that produces a yaml.TypeError.
	path := filepath.Join(dir, "hostname")
	const secretish = "DT-JWKYTT3"
	if err := os.WriteFile(path, []byte(secretish+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("LoadFrom() succeeded on a non-config file")
	}

	if strings.Contains(err.Error(), secretish) {
		t.Errorf("error echoed the file's content: %v", err)
	}
	// The path still has to be named, or the user cannot tell which file the
	// complaint is about.
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error does not name the offending file: %v", err)
	}
}

// TestLoadFromKeepsSyntaxDiagnostics guards against over-correcting. A syntax
// error reports a line number and a structural problem without quoting values,
// and it is the error users most often need to act on — replacing it with a
// generic message would trade a non-finding for a real usability loss.
func TestLoadFromKeepsSyntaxDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(path, []byte("contexts:\n  - name: prod\n   bad indent here\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("LoadFrom() succeeded on malformed yaml")
	}

	if !strings.Contains(err.Error(), "line") {
		t.Errorf("syntax error lost its line number: %v", err)
	}
}

// TestLoadFromMissingFileUnchanged confirms the not-found path still gives its
// actionable message rather than being swallowed by the new parse handling.
func TestLoadFromMissingFileUnchanged(t *testing.T) {
	_, err := LoadFrom(filepath.Join(t.TempDir(), "absent"))
	if err == nil {
		t.Fatal("LoadFrom() succeeded on a missing file")
	}
	if !strings.Contains(err.Error(), "set-context") {
		t.Errorf("missing-file error lost its guidance: %v", err)
	}
}
