package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// withNoExplicitConfig clears --config for one test so the discovery path runs.
func withNoExplicitConfig(t *testing.T) {
	t.Helper()
	prev := cfgFile
	cfgFile = ""
	t.Cleanup(func() { cfgFile = prev })
}

// withVerbosity sets the -v level for one test.
func withVerbosity(t *testing.T, level int) {
	t.Helper()
	prev := verbosity
	verbosity = level
	t.Cleanup(func() { verbosity = prev })
}

// TestReportConfigSourceWarnsOnAncestor is the disclosure half of PRISM-13820.
// Before this, a .dtmgd.yaml planted in a shared parent directory redirected
// every dtmgd call — host, env-id and token — with nothing printed at all.
func TestReportConfigSourceWarnsOnAncestor(t *testing.T) {
	withVerbosity(t, 0)
	planted := filepath.Join(t.TempDir(), ".dtmgd.yaml")

	stderr := captureStderr(t, func() {
		reportConfigSource(config.Source{Path: planted, Kind: config.SourceLocalAncestor})
	})

	if !strings.Contains(stderr, planted) {
		t.Errorf("warning did not name the file in use:\n%s", stderr)
	}
	if !strings.Contains(stderr, "parent directory") {
		t.Errorf("warning did not say where the file came from:\n%s", stderr)
	}
	// The user needs to know which config was bypassed to judge the surprise.
	if !strings.Contains(stderr, config.DefaultConfigPath()) {
		t.Errorf("warning did not name the config that was skipped:\n%s", stderr)
	}
}

// TestReportConfigSourceQuietForVisibleSources pins the other half of the
// design: warning on the documented project-local file, or on a path the user
// typed, would be noise on every command and would train people to ignore the
// one warning that matters.
func TestReportConfigSourceQuietForVisibleSources(t *testing.T) {
	withVerbosity(t, 0)

	for _, kind := range []config.SourceKind{
		config.SourceGlobal,
		config.SourceExplicit,
		config.SourceLocalCWD,
	} {
		stderr := captureStderr(t, func() {
			reportConfigSource(config.Source{Path: "/some/config", Kind: kind})
		})
		if stderr != "" {
			t.Errorf("kind %v printed %q, want silence at default verbosity", kind, stderr)
		}
	}
}

// TestReportConfigSourceVerboseNamesFile covers the audit path from the
// finding's recommendation 4: -v should say which file was loaded.
func TestReportConfigSourceVerboseNamesFile(t *testing.T) {
	withVerbosity(t, 1)

	stderr := captureStderr(t, func() {
		reportConfigSource(config.Source{Path: "/some/config", Kind: config.SourceGlobal})
	})

	if !strings.Contains(stderr, "/some/config") {
		t.Errorf("-v did not name the config file:\n%s", stderr)
	}
}

// TestLoadFromSourceWarnsEndToEnd wires discovery, reporting and loading
// together: standing in a subdirectory of a planted config must both load that
// file and say so.
func TestLoadFromSourceWarnsEndToEnd(t *testing.T) {
	withNoExplicitConfig(t)
	withVerbosity(t, 0)

	parent := t.TempDir()
	planted := filepath.Join(parent, ".dtmgd.yaml")
	if err := os.WriteFile(planted, []byte(`apiVersion: dtmgd.io/v1
kind: Config
current-context: exfil
contexts:
  - name: exfil
    context:
      host: http://127.0.0.1:8080
      env-id: abc12345
      token-ref: leaked
tokens:
  - name: leaked
    token: ""
`), 0600); err != nil {
		t.Fatal(err)
	}

	child := filepath.Join(parent, "project")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)

	var cfg *config.Config
	var err error
	stderr := captureStderr(t, func() { cfg, err = loadFromSource() })
	if err != nil {
		t.Fatalf("loadFromSource() error = %v", err)
	}

	if cfg.CurrentContext != "exfil" {
		t.Errorf("CurrentContext = %q, want the planted config to have loaded", cfg.CurrentContext)
	}
	if !strings.Contains(stderr, planted) {
		t.Errorf("loading a planted config was silent:\n%s", stderr)
	}
}
