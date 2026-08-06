package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempConfig points the cmd package at a temp config file for one test and
// restores the previous value afterwards.
func withTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".dtmgd.yaml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	prev := cfgFile
	cfgFile = path
	t.Cleanup(func() { cfgFile = prev })
	return path
}

const placeholderConfig = `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: ${DT_MANAGED_HOST}
      env-id: ${DT_ENV_ID}
      token-ref: prod-token
  - name: staging
    context:
      host: https://staging.example.com
      env-id: def67890
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ${DT_API_TOKEN}
`

// TestUseContextPreservesPlaceholders is acceptance criterion 1: the exact
// scenario that wrote a live token into .dtmgd.yaml before this change.
func TestUseContextPreservesPlaceholders(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)

	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")
	t.Setenv("DT_API_TOKEN", "dt0c01.LIVESECRET")

	if err := useContext("staging"); err != nil {
		t.Fatalf("useContext() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	if strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("the live token was written into the config:\n%s", got)
	}
	if strings.Contains(got, "https://managed.example.com") {
		t.Errorf("the expanded host was written into the config:\n%s", got)
	}
	for _, want := range []string{"${DT_MANAGED_HOST}", "${DT_ENV_ID}", "${DT_API_TOKEN}"} {
		if !strings.Contains(got, want) {
			t.Errorf("placeholder %s missing after save:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "current-context: staging") {
		t.Errorf("the context switch was not persisted:\n%s", got)
	}
}

// TestSetCredentialsPreservesOtherPlaceholders is acceptance criterion 2: the
// documented onboarding flow, which used to empty host and env-id.
//
// It deliberately does not assert on how the token itself was stored:
// IsKeyringAvailable probes the real OS keyring, so that branch differs
// between headless CI and a developer Mac. Host and env-id are unaffected by
// the keyring either way.
func TestSetCredentialsPreservesOtherPlaceholders(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)

	// Variables deliberately unset — the wipe case.
	t.Setenv("DT_MANAGED_HOST", "")
	os.Unsetenv("DT_MANAGED_HOST")
	t.Setenv("DT_ENV_ID", "")
	os.Unsetenv("DT_ENV_ID")

	if err := setCredentials("prod-token", "dt0c01.REAL"); err != nil {
		t.Fatalf("setCredentials() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	for _, want := range []string{"${DT_MANAGED_HOST}", "${DT_ENV_ID}"} {
		if !strings.Contains(got, want) {
			t.Errorf("placeholder %s was wiped:\n%s", want, got)
		}
	}
	if strings.Contains(got, `host: ""`) {
		t.Errorf("host was emptied:\n%s", got)
	}
}

// TestUseContextPreservesLiteralDollars is acceptance criterion 3.
func TestUseContextPreservesLiteralDollars(t *testing.T) {
	const cfg = `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: https://managed.example.com
      env-id: abc12345
      token-ref: prod-token
      http-proxy: http://user:pa$$w0rd@proxy.corp:8080
  - name: staging
    context:
      host: https://staging.example.com
      env-id: def67890
      token-ref: prod-token
tokens:
  - name: prod-token
    token: ""
`
	path := withTempConfig(t, cfg)

	if err := useContext("staging"); err != nil {
		t.Fatalf("useContext() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "pa$$w0rd") {
		t.Errorf("the literal dollars were corrupted:\n%s", after)
	}
}

// TestSetContextOnPlaceholderConfigSucceeds confirms the read-only dead end
// left by PR #27 is gone.
func TestSetContextOnPlaceholderConfigSucceeds(t *testing.T) {
	path := withTempConfig(t, placeholderConfig)

	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")
	t.Setenv("DT_API_TOKEN", "dt0c01.LIVESECRET")

	if err := setContext("newenv", "https://new.example.com", "xyz99999", "prod-token", ""); err != nil {
		t.Fatalf("setContext() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)

	if !strings.Contains(got, "newenv") {
		t.Errorf("the new context was not written:\n%s", got)
	}
	if !strings.Contains(got, "${DT_API_TOKEN}") {
		t.Errorf("the existing placeholder was lost:\n%s", got)
	}
	if strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("the live token was written into the config:\n%s", got)
	}
}
