package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// writeImportSource writes an MCP server config to a temp file named name,
// so its extension picks the format, and returns the path.
func writeImportSource(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// newConfigPath returns a temp config path that does not exist yet, so an
// import into it also exercises creating the config.
func newConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config")
}

// loadImported loads the config an import wrote, failing the test if it
// cannot: a config that no longer loads is itself a failed import.
func loadImported(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("imported config does not load: %v", err)
	}
	return cfg
}

func mustContext(t *testing.T, cfg *config.Config, name string) config.Context {
	t.Helper()
	nc, err := cfg.GetContext(name)
	if err != nil {
		t.Fatalf("context %q missing: %v", name, err)
	}
	return nc.Context
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const mcpTwoEnvironments = `# Production environment
- dynatraceUrl: https://prod.example.com/
  apiEndpointUrl: https://prod.example.com:9999/
  environmentId: abc-123/
  alias: imp-prod
  apiToken: dt0c01.IMPORTLITERAL
  httpsProxyUrl: http://proxy.corp:8080
# Staging environment
- dynatraceUrl: https://stage.example.com/
  apiEndpointUrl: https://stage.example.com:9999
  environmentId: xyz-789
  alias: imp-stage
  apiToken: ${DT_IMP_STAGE_TOKEN}
  httpProxyUrl: http://proxy.corp:3128
`

// TestImportMCPConfigFile pins the field mapping: each MCP environment
// becomes a context named after its alias, with the trailing slash trimmed
// from host and env-id, a token named <alias>-token, and its proxies. A
// literal token goes to the keyring, not the file; dynatraceUrl, which dtmgd
// has no setting for, is named once on stderr.
func TestImportMCPConfigFile(t *testing.T) {
	t.Cleanup(func() { _ = keyring.Delete("dtmgd", "imp-prod-token") })
	cfgPath := newConfigPath(t)
	src := writeImportSource(t, "dt-config.yaml", mcpTwoEnvironments)

	r := runCLI(t, cfgPath, "config", "import", src)
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	cfg := loadImported(t, cfgPath)
	if len(cfg.Contexts) != 2 {
		t.Fatalf("got %d contexts, want 2: %+v", len(cfg.Contexts), cfg.Contexts)
	}
	prod := mustContext(t, cfg, "imp-prod")
	want := config.Context{Host: "https://prod.example.com:9999", EnvID: "abc-123", TokenRef: "imp-prod-token", HTTPSProxyURL: "http://proxy.corp:8080"}
	if prod != want {
		t.Errorf("imp-prod = %+v, want %+v", prod, want)
	}
	stage := mustContext(t, cfg, "imp-stage")
	want = config.Context{Host: "https://stage.example.com:9999", EnvID: "xyz-789", TokenRef: "imp-stage-token", HTTPProxyURL: "http://proxy.corp:3128"}
	if stage != want {
		t.Errorf("imp-stage = %+v, want %+v", stage, want)
	}
	if cfg.CurrentContext != "imp-prod" {
		t.Errorf("current-context = %q, want the first imported context", cfg.CurrentContext)
	}

	if got, err := keyring.Get("dtmgd", "imp-prod-token"); err != nil || got != "dt0c01.IMPORTLITERAL" {
		t.Errorf("keyring imp-prod-token = %q, %v; want the literal token", got, err)
	}
	if strings.Contains(readFile(t, cfgPath), "IMPORTLITERAL") {
		t.Error("the literal token was written into the config file")
	}
	if n := strings.Count(r.Stderr, "dynatraceUrl"); n != 1 {
		t.Errorf("stderr names dynatraceUrl %d times, want once: %q", n, r.Stderr)
	}

	// The extension decides the format: the same data as JSON imports too,
	// and an extension the MCP server does not read is a usage error.
	jsonSrc := writeImportSource(t, "dt-config.json",
		`[{"apiEndpointUrl":"https://json.example.com/","environmentId":"j-1","alias":"imp-json","apiToken":"${DT_IMP_JSON_TOKEN}"}]`)
	if r := runCLI(t, cfgPath, "config", "import", jsonSrc); r.Code != 0 {
		t.Fatalf("JSON import: code %d, stderr %q", r.Code, r.Stderr)
	}
	if got := mustContext(t, loadImported(t, cfgPath), "imp-json"); got.Host != "https://json.example.com" || got.EnvID != "j-1" {
		t.Errorf("imp-json = %+v", got)
	}
	txtSrc := writeImportSource(t, "dt-config.txt", mcpTwoEnvironments)
	if r := runCLI(t, cfgPath, "config", "import", txtSrc); r.Code != 2 {
		t.Errorf(".txt source: code %d, want 2; stderr %q", r.Code, r.Stderr)
	}
}

// TestImportKeepsTokenReferences is Review Focus 3: an apiToken that is a
// ${VAR} reference stays a reference in the dtmgd config. It is never
// expanded, so the secret it names reaches neither the file nor the keyring,
// and dtmgd resolves it when the token is used.
func TestImportKeepsTokenReferences(t *testing.T) {
	cfgPath := newConfigPath(t)
	src := writeImportSource(t, "dt-config.yaml", `- apiEndpointUrl: https://my-api.company.com/
  environmentId: abc-123
  alias: prod
  apiToken: ${DT_PROD_TOKEN}
`)

	r := runCLIWithEnv(t, map[string]string{"DT_PROD_TOKEN": "secret"}, cfgPath, "config", "import", src)
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	written := readFile(t, cfgPath)
	if strings.Contains(written, "secret") {
		t.Errorf("the referenced secret was written into the config:\n%s", written)
	}
	if strings.Contains(r.Stdout+r.Stderr, "secret") {
		t.Errorf("the referenced secret was printed: stdout %q, stderr %q", r.Stdout, r.Stderr)
	}
	cfg := loadImported(t, cfgPath)
	if got, ok := configToken(cfg, "prod-token"); !ok || got != "${DT_PROD_TOKEN}" {
		t.Errorf("token prod-token = %q (present %v), want the reference ${DT_PROD_TOKEN}", got, ok)
	}
	if got := mustContext(t, cfg, "prod").TokenRef; got != "prod-token" {
		t.Errorf("token-ref = %q, want prod-token", got)
	}
	if _, err := keyring.Get("dtmgd", "prod-token"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("keyring holds prod-token (err %v); a reference must never reach the keyring", err)
	}
	// The reference is live: dtmgd expands it at the point of use.
	if got, err := cfg.GetToken("prod-token"); err != nil || got != "secret" {
		t.Errorf("GetToken = %q, %v; want the variable's value", got, err)
	}
}

const existingConfig = `apiVersion: dtmgd.io/v1
kind: Config
current-context: keep
contexts:
  - name: keep
    context:
      host: https://keep.example.com
      env-id: k1
      token-ref: keep-token
tokens:
  - name: keep-token
    token: ${DT_KEEP_TOKEN}
`

// TestImportRejectsDuplicateAliases pins that a source the MCP server itself
// would reject is a usage error raised before anything is written: the
// config file stays byte-identical and nothing reaches the keyring. A
// duplicate alias uses the MCP server's own message.
func TestImportRejectsDuplicateAliases(t *testing.T) {
	cases := []struct {
		name, source, want string
	}{
		{"duplicate alias", `
- {apiEndpointUrl: https://a.example.com, environmentId: e1, alias: dup, apiToken: dt0c01.DUPONE}
- {apiEndpointUrl: https://b.example.com, environmentId: e2, alias: dup, apiToken: dt0c01.DUPTWO}
`, `Duplicate alias found: "dup". Aliases must be unique across environments.`},
		{"missing alias", `
- {apiEndpointUrl: https://a.example.com, environmentId: e1, apiToken: dt0c01.DUPONE}
`, `"alias"`},
		{"missing apiEndpointUrl", `
- {environmentId: e1, alias: dup, apiToken: dt0c01.DUPONE}
`, `"apiEndpointUrl"`},
		{"missing environmentId", `
- {apiEndpointUrl: https://a.example.com, alias: dup, apiToken: dt0c01.DUPONE}
`, `"environmentId"`},
		{"semicolon in alias", `
- {apiEndpointUrl: https://a.example.com, environmentId: e1, alias: "a;b", apiToken: dt0c01.DUPONE}
`, `cannot contain semicolons`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := withTempConfig(t, existingConfig)
			src := writeImportSource(t, "dt-config.yaml", tc.source)

			r := runCLI(t, cfgPath, "config", "import", src)
			if r.Code != 2 {
				t.Fatalf("code %d, want 2; stderr %q", r.Code, r.Stderr)
			}
			if !strings.Contains(r.Stderr, tc.want) {
				t.Errorf("stderr %q does not contain %q", r.Stderr, tc.want)
			}
			if got := readFile(t, cfgPath); got != existingConfig {
				t.Errorf("config changed:\n%s", got)
			}
			for _, name := range []string{"dup-token", "a;b-token"} {
				if _, err := keyring.Get("dtmgd", name); !errors.Is(err, keyring.ErrNotFound) {
					t.Errorf("keyring holds %s (err %v)", name, err)
				}
			}
		})
	}
}

// TestImportSkipsExistingUnlessOverwrite pins that an alias naming an
// existing context is skipped with a warning, and replaced with --overwrite.
// The replaced token is a reference, so the old keyring value under that
// name is removed: left there, it would be read ahead of the reference.
func TestImportSkipsExistingUnlessOverwrite(t *testing.T) {
	cfgPath := withTempConfig(t, `apiVersion: dtmgd.io/v1
kind: Config
current-context: imp-x
contexts:
  - name: imp-x
    context:
      host: https://old.example.com
      env-id: old
      token-ref: imp-x-token
tokens:
  - name: imp-x-token
    token: ""
`)
	storeKeyringTokens(t, "imp-x-token")
	src := writeImportSource(t, "dt-config.yaml", `
- {apiEndpointUrl: https://new.example.com/, environmentId: new, alias: imp-x, apiToken: "${DT_IMP_X_TOKEN}"}
- {apiEndpointUrl: https://y.example.com/, environmentId: y1, alias: imp-y, apiToken: "${DT_IMP_Y_TOKEN}"}
`)

	r := runCLI(t, cfgPath, "config", "import", src)
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if !strings.Contains(r.Stderr, `"imp-x"`) || !strings.Contains(r.Stderr, "--overwrite") {
		t.Errorf("no skip warning naming imp-x and --overwrite: %q", r.Stderr)
	}
	cfg := loadImported(t, cfgPath)
	if got := mustContext(t, cfg, "imp-x").Host; got != "https://old.example.com" {
		t.Errorf("imp-x host = %q, want it kept without --overwrite", got)
	}
	mustContext(t, cfg, "imp-y")
	if got, err := keyring.Get("dtmgd", "imp-x-token"); err != nil || got != "dt0c01.TEST" {
		t.Errorf("keyring imp-x-token = %q, %v; want it untouched without --overwrite", got, err)
	}

	r = runCLI(t, cfgPath, "config", "import", src, "--overwrite")
	if r.Code != 0 {
		t.Fatalf("--overwrite: code %d, stderr %q", r.Code, r.Stderr)
	}
	cfg = loadImported(t, cfgPath)
	if got := mustContext(t, cfg, "imp-x"); got.Host != "https://new.example.com" || got.EnvID != "new" {
		t.Errorf("imp-x = %+v, want it replaced", got)
	}
	if len(cfg.Contexts) != 2 {
		t.Errorf("got %d contexts, want 2 (replaced, not duplicated)", len(cfg.Contexts))
	}
	if got, _ := configToken(cfg, "imp-x-token"); got != "${DT_IMP_X_TOKEN}" {
		t.Errorf("token imp-x-token = %q, want the reference", got)
	}
	if _, err := keyring.Get("dtmgd", "imp-x-token"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("stale keyring value kept for imp-x-token (err %v); it would shadow the reference", err)
	}
}

// TestImportFromEnv pins --from-env, which reads the MCP server's
// DT_ENVIRONMENT_CONFIGS JSON, and that exactly one source must be named.
func TestImportFromEnv(t *testing.T) {
	cfgPath := newConfigPath(t)
	env := map[string]string{"DT_ENVIRONMENT_CONFIGS": `[{"apiEndpointUrl":"https://api.example.com/","environmentId":"abc-123","alias":"imp-env","apiToken":"${DT_IMP_ENV_TOKEN}"},` +
		`{"apiEndpointUrl":"https://api2.example.com","environmentId":"def-456","alias":"imp-env-2","apiToken":"${DT_IMP_ENV2_TOKEN}"}]`}

	r := runCLIWithEnv(t, env, cfgPath, "config", "import", "--from-env")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	cfg := loadImported(t, cfgPath)
	if got := mustContext(t, cfg, "imp-env"); got.Host != "https://api.example.com" || got.EnvID != "abc-123" || got.TokenRef != "imp-env-token" {
		t.Errorf("imp-env = %+v", got)
	}
	if got := mustContext(t, cfg, "imp-env-2"); got.Host != "https://api2.example.com" || got.EnvID != "def-456" {
		t.Errorf("imp-env-2 = %+v", got)
	}

	src := writeImportSource(t, "dt-config.yaml", mcpTwoEnvironments)
	if r := runCLIWithEnv(t, env, cfgPath, "config", "import", src, "--from-env"); r.Code != 2 {
		t.Errorf("file and --from-env: code %d, want 2", r.Code)
	}
	if r := runCLI(t, cfgPath, "config", "import"); r.Code != 2 {
		t.Errorf("no source: code %d, want 2", r.Code)
	}
	if r := runCLIWithEnv(t, map[string]string{"DT_ENVIRONMENT_CONFIGS": ""}, cfgPath, "config", "import", "--from-env"); r.Code != 2 {
		t.Errorf("--from-env with DT_ENVIRONMENT_CONFIGS unset: code %d, want 2", r.Code)
	}
}

// TestImportDryRunWritesNothing pins --dry-run: it shows the contexts it
// would create, with each token's destination rather than its value, and
// writes neither the config nor the keyring.
func TestImportDryRunWritesNothing(t *testing.T) {
	cfgPath := withTempConfig(t, existingConfig)
	src := writeImportSource(t, "dt-config.yaml", mcpTwoEnvironments)

	r := runCLIWithEnv(t, map[string]string{"DT_IMP_STAGE_TOKEN": "stagesecret"}, cfgPath, "config", "import", src, "--dry-run")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if got := readFile(t, cfgPath); got != existingConfig {
		t.Errorf("--dry-run changed the config:\n%s", got)
	}
	if _, err := keyring.Get("dtmgd", "imp-prod-token"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("--dry-run stored a token in the keyring (err %v)", err)
	}
	for _, want := range []string{"imp-prod", "imp-stage", "<keyring>", "<reference ${DT_IMP_STAGE_TOKEN}>"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("dry-run output lacks %q:\n%s", want, r.Stdout)
		}
	}
	for _, secret := range []string{"IMPORTLITERAL", "stagesecret"} {
		if strings.Contains(r.Stdout+r.Stderr, secret) {
			t.Errorf("dry-run printed a token value %q", secret)
		}
	}

	// Nor does it create a config that does not exist yet.
	missing := newConfigPath(t)
	if r := runCLI(t, missing, "config", "import", src, "--dry-run"); r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--dry-run created %s (stat err %v)", missing, err)
	}
}

// TestImportSavesBeforeKeyring pins that import changes the keyring only once
// the config is saved: a save that fails (a read-only config) must leave the
// stale keyring token that a reference would replace where it was.
func TestImportSavesBeforeKeyring(t *testing.T) {
	cfgPath := withTempConfig(t, `apiVersion: dtmgd.io/v1
kind: Config
current-context: imp-x
contexts:
  - name: imp-x
    context:
      host: https://old.example.com
      env-id: old
      token-ref: imp-x-token
tokens:
  - name: imp-x-token
    token: ""
`)
	storeKeyringTokens(t, "imp-x-token")
	src := writeImportSource(t, "dt-config.yaml", `
- {apiEndpointUrl: https://new.example.com/, environmentId: new, alias: imp-x, apiToken: "${DT_IMP_X_TOKEN}"}
`)
	readOnlyConfig(t, cfgPath)

	if r := runCLI(t, cfgPath, "config", "import", src, "--overwrite"); r.Code == 0 {
		t.Fatalf("import succeeded although the config cannot be saved: %s", r)
	}
	if got, err := keyring.Get("dtmgd", "imp-x-token"); err != nil || got != "dt0c01.TEST" {
		t.Errorf("keyring imp-x-token = %q, %v; want it untouched when the save fails", got, err)
	}
}

// TestImportDryRunMasksProxyPasswords pins that --dry-run, in table and json
// output, never shows a proxy password from the MCP config.
func TestImportDryRunMasksProxyPasswords(t *testing.T) {
	src := writeImportSource(t, "dt-config.yaml", `
- {apiEndpointUrl: https://p.example.com/, environmentId: p1, alias: imp-p, httpsProxyUrl: "http://svc:Pr0xyPw@proxy.example:3128", httpProxyUrl: "user:Pr0xyPw@plain:8080"}
`)
	for _, args := range [][]string{{"-o", "wide"}, {"-o", "json"}} {
		r := runCLI(t, newConfigPath(t), append([]string{"config", "import", src, "--dry-run"}, args...)...)
		if r.Code != 0 || strings.Contains(r.Stdout+r.Stderr, "Pr0xyPw") {
			t.Errorf("%v: code %d, dry run shows the proxy password:\n%s%s", args, r.Code, r.Stdout, r.Stderr)
		}
	}
}
