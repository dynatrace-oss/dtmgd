// Package testsandbox points the home and config directories of a test binary
// at a temporary directory, so no test can read or write a developer's real
// dtmgd config or installed skills, even one that forgets --config.
//
// Call Enter first thing in TestMain. adrg/xdg resolves its directories in
// its package init, before TestMain runs, so Enter sets the environment and
// then reloads them.
package testsandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

// Root is the sandbox's temporary directory, set by Enter.
var Root string

// Original holds the values the sandbox replaced, as they were before Enter,
// for a test that must compare with the real default path (never open it).
var Original = map[string]string{}

// sandboxed are the variables Enter points into the sandbox.
var sandboxed = []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "LOCALAPPDATA", "APPDATA"}

// localConfigName is pkg/config.LocalConfigName; importing pkg/config here
// would be an import cycle for pkg/config's own tests.
const localConfigName = ".dtmgd.yaml"

// Enter creates the sandbox: HOME and USERPROFILE point at Root/home, and
// XDG_CONFIG_HOME, LOCALAPPDATA and APPDATA at Root/config; DTMGD_CONFIG and
// DTMGD_CONTEXT are unset. It fails, and leaves nothing behind, when a
// .dtmgd.yaml lies in the working directory or above it, or unless the
// default config directory then lies inside Root. cleanup removes the
// sandbox.
func Enter() (cleanup func(), err error) {
	if local := findLocalConfig(); local != "" {
		return nil, fmt.Errorf("testsandbox: %s exists; dtmgd finds it by walking up from the tests' directory, so a test without --config would read and rewrite it. Move it away to run the tests", local)
	}
	for _, k := range sandboxed {
		Original[k] = os.Getenv(k)
	}
	for _, k := range []string{"DTMGD_CONFIG", "DTMGD_CONTEXT"} {
		if err := os.Unsetenv(k); err != nil {
			return nil, fmt.Errorf("testsandbox: %w", err)
		}
	}
	root, err := os.MkdirTemp("", "dtmgd-test-home-")
	if err != nil {
		return nil, fmt.Errorf("testsandbox: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(root) }
	home := filepath.Join(root, "home")
	cfg := filepath.Join(root, "config")
	for _, d := range []string{home, cfg} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			cleanup()
			return nil, fmt.Errorf("testsandbox: %w", err)
		}
	}
	for k, v := range map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CONFIG_HOME": cfg,
		"LOCALAPPDATA":    cfg,
		"APPDATA":         cfg,
	} {
		if err := os.Setenv(k, v); err != nil {
			cleanup()
			return nil, fmt.Errorf("testsandbox: %w", err)
		}
	}
	xdg.Reload()
	if !strings.HasPrefix(xdg.ConfigHome, root+string(filepath.Separator)) {
		cleanup()
		return nil, fmt.Errorf("testsandbox: config home %q is outside the sandbox %q", xdg.ConfigHome, root)
	}
	Root = root
	return cleanup, nil
}

// findLocalConfig returns the first .dtmgd.yaml in the working directory or
// above it, as pkg/config.FindLocalConfig would find it, or "".
func findLocalConfig() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(dir, localConfigName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Main runs a test binary inside the sandbox: TestMain calls
// os.Exit(testsandbox.Main(m.Run)) after its own setup.
func Main(run func() int) int {
	cleanup, err := Enter()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()
	return run()
}
