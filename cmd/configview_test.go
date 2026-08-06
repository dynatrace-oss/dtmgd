package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. The printers resolve os.Stdout when they are constructed, which
// happens inside RunE, so swapping it here is enough to capture command output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out)
}

const plaintextTokenConfig = `apiVersion: dtmgd.io/v1
kind: Config
current-context: prod
contexts:
  - name: prod
    context:
      host: https://managed.example.com
      env-id: abc12345
      token-ref: prod-token
tokens:
  - name: prod-token
    token: dt0c01.LIVESECRET
`

// TestWithMaskedTokens covers the three cases config view has to tell apart.
func TestWithMaskedTokens(t *testing.T) {
	cfg := &config.Config{
		Tokens: []config.NamedToken{
			{Name: "plaintext", Token: "dt0c01.LIVESECRET"},
			{Name: "keyring", Token: ""},
			{Name: "placeholder", Token: "${DT_API_TOKEN}"},
		},
	}

	masked := withMaskedTokens(cfg)

	if masked.Tokens[0].Token != tokenMask {
		t.Errorf("plaintext token = %q, want it masked", masked.Tokens[0].Token)
	}
	if masked.Tokens[1].Token != "" {
		t.Errorf("keyring-backed token = %q, want it left empty", masked.Tokens[1].Token)
	}
	// A ${VAR} reference names a variable; it is not the secret, and hiding it
	// would obscure configuration without protecting anything.
	if masked.Tokens[2].Token != "${DT_API_TOKEN}" {
		t.Errorf("placeholder = %q, want it shown verbatim", masked.Tokens[2].Token)
	}
}

// TestWithMaskedTokensDoesNotMutateSource is the property that keeps masking
// from becoming a data-loss bug: config view shares its Config with the rest
// of the command, and an in-place mask would be one saveConfig away from
// writing the mask string over the real credential.
func TestWithMaskedTokensDoesNotMutateSource(t *testing.T) {
	cfg := &config.Config{
		Tokens: []config.NamedToken{{Name: "plaintext", Token: "dt0c01.LIVESECRET"}},
	}

	_ = withMaskedTokens(cfg)

	if cfg.Tokens[0].Token != "dt0c01.LIVESECRET" {
		t.Errorf("source config was mutated: token = %q", cfg.Tokens[0].Token)
	}
}

// TestConfigViewMasksByDefault drives the command, so it also covers the flag
// lookup and the default value of --show-tokens. This is the finding itself:
// the plaintext token must not reach stdout unasked, because that is what CI
// step logs capture.
func TestConfigViewMasksByDefault(t *testing.T) {
	withTempConfig(t, plaintextTokenConfig)
	resetShowTokens(t)

	var err error
	got := captureStdout(t, func() { err = configViewCmd.RunE(configViewCmd, nil) })
	if err != nil {
		t.Fatalf("config view error = %v", err)
	}

	if strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("config view printed the plaintext token:\n%s", got)
	}
	if !strings.Contains(got, "show-tokens") {
		t.Errorf("output does not tell the user how to reveal the value:\n%s", got)
	}
}

// TestConfigViewShowTokensReveals confirms the opt-out actually opts out —
// otherwise masking would be a functional regression for anyone who needs the
// value for support or debugging.
func TestConfigViewShowTokensReveals(t *testing.T) {
	withTempConfig(t, plaintextTokenConfig)
	resetShowTokens(t)

	if err := configViewCmd.Flags().Set("show-tokens", "true"); err != nil {
		t.Fatal(err)
	}

	var err error
	got := captureStdout(t, func() { err = configViewCmd.RunE(configViewCmd, nil) })
	if err != nil {
		t.Fatalf("config view error = %v", err)
	}

	if !strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("--show-tokens did not reveal the value:\n%s", got)
	}
}

// resetShowTokens restores the flag before and after a test. Cobra flag values
// live on the package-level command, so without this the two tests above would
// depend on execution order.
func resetShowTokens(t *testing.T) {
	t.Helper()
	set := func() {
		if err := configViewCmd.Flags().Set("show-tokens", "false"); err != nil {
			t.Fatal(err)
		}
	}
	set()
	t.Cleanup(set)
}
