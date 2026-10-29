package cmd

import (
	"bytes"
	"os"
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

// TestConfigViewMasksProxyPasswords pins that config view masks a password in
// a context's http-proxy or https-proxy as it masks tokens, keeps the rest of
// the URL, leaves a ${VAR} proxy alone, and shows it all with --show-tokens.
func TestConfigViewMasksProxyPasswords(t *testing.T) {
	path := withTempConfig(t, `apiVersion: dtmgd.io/v1
kind: Config
current-context: p
contexts:
  - name: p
    context:
      host: https://example.invalid
      env-id: env1
      token-ref: p-token
      http-proxy: http://user:pr0xyPASS@proxy.example:3128
      https-proxy: https://admin:t0pSECRET@secure.example:8443
  - name: q
    context:
      host: https://example.invalid
      env-id: env2
      https-proxy: ${HTTPS_PROXY}
`)
	r := runCLI(t, path, "config", "view")
	if r.Code != 0 {
		t.Fatalf("config view: %s", r)
	}
	for _, secret := range []string{"pr0xyPASS", "t0pSECRET"} {
		if strings.Contains(r.Stdout, secret) {
			t.Errorf("config view printed the proxy password %q:\n%s", secret, r.Stdout)
		}
	}
	for _, want := range []string{"http://user:xxxxx@proxy.example:3128", "https://admin:xxxxx@secure.example:8443", "${HTTPS_PROXY}"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("config view lacks %q:\n%s", want, r.Stdout)
		}
	}
	if r := runCLI(t, path, "config", "view", "--show-tokens"); !strings.Contains(r.Stdout, "pr0xyPASS") || !strings.Contains(r.Stdout, "t0pSECRET") {
		t.Errorf("--show-tokens did not reveal the proxy passwords:\n%s", r.Stdout)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "pr0xyPASS") {
		t.Error("config view changed the file")
	}
}

// TestConfigViewMasksUnparseableProxyPasswords pins that proxy masking fails
// closed: a proxy URL that url.Parse rejects or misreads (an invalid port or
// escape, no scheme, a ${VAR} host) still never shows its password.
func TestConfigViewMasksUnparseableProxyPasswords(t *testing.T) {
	for _, proxy := range []string{
		"http://user:pa/ssA1@proxy:3128",
		"http://u:50%offA1@p:1",
		"user:secretA1@proxy:3128",
		"http://svc:literalA1@${PROXY_HOST}:3128",
	} {
		path := withTempConfig(t, "apiVersion: dtmgd.io/v1\nkind: Config\ncontexts:\n  - name: p\n    context:\n      host: https://example.invalid\n      env-id: env1\n      https-proxy: '"+proxy+"'\n")
		r := runCLI(t, path, "config", "view")
		if r.Code != 0 || strings.Contains(r.Stdout, "A1") {
			t.Errorf("%s: code %d, config view shows the password:\n%s", proxy, r.Code, r.Stdout)
		}
	}
}
