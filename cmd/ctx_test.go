package cmd

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// was written. pkg/output writes warnings there, so this is how the warning
// text becomes assertable.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestSetContextWarnsOnClobberedPlaceholder covers warnClobberedContextRefs'
// loop body, which the other setContext tests miss because they create a NEW
// context and the function returns early when there is nothing to clobber.
func TestSetContextWarnsOnClobberedPlaceholder(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)
	a := newTestApp(path)

	stderr := captureStderr(t, func() {
		if err := a.setContext("prod", "https://new.example.com", "xyz99999", "", ""); err != nil {
			t.Fatalf("setContext() error = %v", err)
		}
	})

	for _, want := range []string{"${DT_MANAGED_HOST}", "${DT_ENV_ID}"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning did not name %s:\n%s", want, stderr)
		}
	}
	// token-ref was not passed, so replacing it must not be announced.
	if strings.Contains(stderr, "token-ref") {
		t.Errorf("warned about token-ref, which was not being replaced:\n%s", stderr)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "https://new.example.com") {
		t.Errorf("the new host was not persisted:\n%s", after)
	}
}

// TestSetContextWarningOmitsSurroundingValue pins the property that only the
// matched ${VAR} references are echoed. A value that mixes a literal secret
// with a reference must not have the literal half printed to the terminal.
func TestSetContextWarningOmitsSurroundingValue(t *testing.T) {
	const mixed = `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: https://managed.example.com
      env-id: abc12345
      token-ref: dt0c01.LITERALSECRET${REF_SUFFIX}
tokens:
  - name: dt0c01.LITERALSECRET
    token: ""
`
	a := newTestApp(withTempConfig(t, mixed))

	stderr := captureStderr(t, func() {
		if err := a.setContext("prod", "", "", "newref", ""); err != nil {
			t.Fatalf("setContext() error = %v", err)
		}
	})

	if !strings.Contains(stderr, "${REF_SUFFIX}") {
		t.Errorf("warning did not name the replaced reference:\n%s", stderr)
	}
	if strings.Contains(stderr, "dt0c01.LITERALSECRET") {
		t.Errorf("the literal half of the value was echoed to the user:\n%s", stderr)
	}
}

// TestSetContextNewContextIsSilent confirms the early return: a context that
// does not exist yet has nothing to clobber, so nothing is warned about.
func TestSetContextNewContextIsSilent(t *testing.T) {
	a := newTestApp(withTempConfig(t, placeholderConfig))

	stderr := captureStderr(t, func() {
		if err := a.setContext("brandnew", "https://b.example.com", "b1", "", ""); err != nil {
			t.Fatalf("setContext() error = %v", err)
		}
	})

	if strings.Contains(stderr, "Replaced") {
		t.Errorf("warned when creating a new context:\n%s", stderr)
	}
}

// TestSetContextRefusesToOverwriteUnparseableConfig is the regression test for
// the config wipe: setContext used to start a fresh config on any load error,
// so one YAML typo plus `ctx set` silently replaced every context and token.
func TestSetContextRefusesToOverwriteUnparseableConfig(t *testing.T) {
	bad := "contexts:\n  - name: prod\n    context:\n      host: https://h\n    - name: broken\n"
	path := withTempConfig(t, bad)
	a := newTestApp(path)

	err := a.setContext("new", "https://new.example.com", "env1", "tok", "")
	if err == nil || !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("want parse error, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != bad {
		t.Errorf("config was modified:\n%s", after)
	}
}

// TestSetContextRefusesUnreadableConfig covers load failures other than a
// parse error: they must abort just the same. The file is write-only (0200)
// so that the load fails while a save would still succeed — with 0000 the
// save fails too and hides the wipe.
func TestSetContextRefusesUnreadableConfig(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced here")
	}
	path := withTempConfig(t, placeholderConfig)
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	a := newTestApp(path)

	if err := a.setContext("new", "https://new.example.com", "env1", "tok", ""); err == nil {
		t.Fatal("want an error for an unreadable config")
	}
	_ = os.Chmod(path, 0o600)
	after, _ := os.ReadFile(path)
	if string(after) != placeholderConfig {
		t.Errorf("config was modified:\n%s", after)
	}
}

// TestSetContextCreatesMissingConfig pins the one case where starting from an
// empty config is right: there is no file yet.
func TestSetContextCreatesMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.yaml")
	a := newTestApp(path)

	if err := a.setContext("prod", "https://h.example.com", "env1", "tok", ""); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "name: prod") {
		t.Errorf("context not written:\n%s", after)
	}
}

// TestSetContextRejectsSemicolonInName pins that ctx set and config
// set-context refuse a name containing ';' (which --env splits on) with a
// usage error, and write nothing.
func TestSetContextRejectsSemicolonInName(t *testing.T) {
	for _, args := range [][]string{
		{"ctx", "set", "a;b", "--host", "https://example.invalid", "--env-id", "env1"},
		{"config", "set-context", "a;b", "--host", "https://example.invalid", "--env-id", "env1"},
	} {
		cfgPath := filepath.Join(t.TempDir(), "config")
		r := runCLI(t, cfgPath, args...)
		if r.Code != 2 || !strings.Contains(r.Stderr, `context name "a;b" contains ';'`) {
			t.Errorf("%v: want exit 2 and the ';' message, got %s", args, r)
		}
		if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
			t.Errorf("%v: config was written (%v)", args, err)
		}
	}
}
