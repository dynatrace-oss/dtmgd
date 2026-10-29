package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// TestDTMGDConfigSelectsTheConfigFile pins DTMGD_CONFIG: used when --config
// is not given, with --config winning. It checks path resolution only, so a
// regression can never fall back to the developer's real config.
func TestDTMGDConfigSelectsTheConfigFile(t *testing.T) {
	t.Setenv("DTMGD_CONFIG", "/tmp/from-env.yaml")
	a := &App{}
	if src := config.ResolveSource(a.configPath()); src.Path != "/tmp/from-env.yaml" || src.Kind != config.SourceExplicit {
		t.Errorf("source = %+v, want the DTMGD_CONFIG path, explicit", src)
	}
	a.Opts.ConfigPath = "/tmp/from-flag.yaml"
	if got := a.configPath(); got != "/tmp/from-flag.yaml" {
		t.Errorf("configPath = %q, want the --config value", got)
	}
}

// TestDTMGDContextSelectsTheContext pins DTMGD_CONTEXT: it overrides the
// file's current context, and -c overrides it.
func TestDTMGDContextSelectsTheContext(t *testing.T) {
	small := map[string]route{problemsPath: {File: "problems_small.json"}}
	apiA, apiB := newFakeAPI(t, small), newFakeAPI(t, small)
	cfg := withFakeEnvs(t, fakeEnv{"a", apiA}, fakeEnv{"b", apiB})

	runCLIWithEnv(t, map[string]string{"DTMGD_CONTEXT": "b"}, cfg, "get", "problems", "-o", "json")
	if apiB.Hits(problemsPath) != 1 || apiA.Hits(problemsPath) != 0 {
		t.Errorf("DTMGD_CONTEXT=b: hits a=%d b=%d", apiA.Hits(problemsPath), apiB.Hits(problemsPath))
	}
	runCLIWithEnv(t, map[string]string{"DTMGD_CONTEXT": "b"}, cfg, "get", "problems", "-o", "json", "-c", "a")
	if apiA.Hits(problemsPath) != 1 {
		t.Errorf("-c a with DTMGD_CONTEXT=b: hits a=%d", apiA.Hits(problemsPath))
	}
}

const credsConfig = `apiVersion: dtmgd.io/v1
kind: Config
current-context: a
contexts:
  - name: a
    context: {host: https://a.example.com, env-id: e1, token-ref: shared}
  - name: b
    context: {host: https://b.example.com, env-id: e2, token-ref: shared}
  - name: c
    context: {host: https://c.example.com, env-id: e3, token-ref: own}
tokens:
  - name: shared
    token: ""
  - name: own
    token: ""
`

func storeKeyringTokens(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := keyring.Set("dtmgd", n, "dt0c01.TEST"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = keyring.Delete("dtmgd", n) })
	}
}

// TestDeleteCredentials pins config delete-credentials: the token leaves
// both the keyring and the config, and contexts still pointing at it are
// named.
func TestDeleteCredentials(t *testing.T) {
	path := withTempConfig(t, credsConfig)
	storeKeyringTokens(t, "shared", "own")

	r := runCLI(t, path, "config", "delete-credentials", "shared")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if _, err := keyring.Get("dtmgd", "shared"); err != keyring.ErrNotFound {
		t.Errorf("keyring still holds shared (err %v)", err)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "name: shared") {
		t.Errorf("config still lists the token:\n%s", after)
	}
	if !strings.Contains(r.Stderr, `"a"`) || !strings.Contains(r.Stderr, `"b"`) {
		t.Errorf("stderr does not name the contexts still using it: %q", r.Stderr)
	}
	if r := runCLI(t, path, "config", "delete-credentials", "nope"); r.Code == 0 {
		t.Error("deleting an unknown token succeeded")
	}
}

// TestDeleteContextWithCredentials pins --delete-credentials: a token only
// the deleted context used goes with it; a shared one stays.
func TestDeleteContextWithCredentials(t *testing.T) {
	path := withTempConfig(t, credsConfig)
	storeKeyringTokens(t, "shared", "own")

	if r := runCLI(t, path, "config", "delete-context", "c", "--delete-credentials"); r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if _, err := keyring.Get("dtmgd", "own"); err != keyring.ErrNotFound {
		t.Errorf("unshared token kept (err %v)", err)
	}
	if r := runCLI(t, path, "ctx", "delete", "a", "--delete-credentials"); r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if _, err := keyring.Get("dtmgd", "shared"); err != nil {
		t.Errorf("shared token deleted although b still uses it (err %v)", err)
	}
}

// TestDeleteContextWithMissingCredential pins that --delete-credentials does
// not block deleting a context whose token is already gone from both the
// keyring and the config: the deletion used to fail with "token not found"
// and leave the context in place.
func TestDeleteContextWithMissingCredential(t *testing.T) {
	path := withTempConfig(t, `apiVersion: dtmgd.io/v1
kind: Config
current-context: a
contexts:
  - name: a
    context: {host: https://a.example.com, env-id: e1, token-ref: shared}
  - name: b
    context: {host: https://b.example.com, env-id: e2, token-ref: gone}
tokens:
  - name: shared
    token: ""
`)
	storeKeyringTokens(t, "shared")

	r := runCLI(t, path, "ctx", "delete", "b", "--delete-credentials")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if !strings.Contains(r.Stderr, `"gone" was already gone`) {
		t.Errorf("stderr does not say the credential was already gone: %q", r.Stderr)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "name: b") {
		t.Errorf("context b is still in the config:\n%s", after)
	}
}

// TestDTMGDContextMissingContextNamesVariable pins that a DTMGD_CONTEXT naming
// a context the config lacks fails with an error that names the variable and
// the file, since nothing on the command line mentions that context, and that
// --context still overrides it.
func TestDTMGDContextMissingContextNamesVariable(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	env := map[string]string{"DTMGD_CONTEXT": "nope"}

	r := runCLIWithEnv(t, env, cfg, "get", "problems", "-o", "json")
	want := `DTMGD_CONTEXT names context "nope", which is not in ` + cfg
	if r.Code == 0 || !strings.Contains(r.Stderr, want) {
		t.Errorf("want a failure containing %q, got %s", want, r)
	}
	if r := runCLIWithEnv(t, env, cfg, "get", "problems", "-o", "json", "-c", "a"); r.Code != 0 {
		t.Errorf("-c a should override DTMGD_CONTEXT: %s", r)
	}
}
