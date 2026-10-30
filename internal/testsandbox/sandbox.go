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

// Enter creates the sandbox: HOME and USERPROFILE point at Root/home, and
// XDG_CONFIG_HOME, LOCALAPPDATA and APPDATA at Root/config. It fails, and
// leaves nothing behind, unless the default config directory then lies inside
// Root. cleanup removes the sandbox.
func Enter() (cleanup func(), err error) {
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
