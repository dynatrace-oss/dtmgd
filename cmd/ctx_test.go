package cmd

import (
	"io"
	"os"
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

	stderr := captureStderr(t, func() {
		if err := setContext("prod", "https://new.example.com", "xyz99999", "", ""); err != nil {
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
	withTempConfig(t, mixed)

	stderr := captureStderr(t, func() {
		if err := setContext("prod", "", "", "newref", ""); err != nil {
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
	withTempConfig(t, placeholderConfig)

	stderr := captureStderr(t, func() {
		if err := setContext("brandnew", "https://b.example.com", "b1", "", ""); err != nil {
			t.Fatalf("setContext() error = %v", err)
		}
	})

	if strings.Contains(stderr, "Replaced") {
		t.Errorf("warned when creating a new context:\n%s", stderr)
	}
}
