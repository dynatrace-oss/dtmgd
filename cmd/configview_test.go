package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

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
	a := newTestApp(withTempConfig(t, plaintextTokenConfig))
	var out bytes.Buffer
	a.Out = &out
	cmd := newConfigViewCmd(a)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config view error = %v", err)
	}
	got := out.String()

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
	a := newTestApp(withTempConfig(t, plaintextTokenConfig))
	var out bytes.Buffer
	a.Out = &out
	cmd := newConfigViewCmd(a)

	if err := cmd.Flags().Set("show-tokens", "true"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config view error = %v", err)
	}
	got := out.String()

	if !strings.Contains(got, "dt0c01.LIVESECRET") {
		t.Errorf("--show-tokens did not reveal the value:\n%s", got)
	}
}
