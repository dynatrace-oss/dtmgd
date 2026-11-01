package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestWritesReadTheSchemaDefinitionOnce pins that a create, real or dry
// run, reads its schema definition once: the planner's read serves the
// secret guard too.
func TestWritesReadTheSchemaDefinitionOnce(t *testing.T) {
	value := valueFile(t, "type: WEBHOOK\nwebHookNotification:\n  url: https://example.invalid/x\n  headers:\n    - name: X\n      secretValue: real\n")
	cases := map[string][]string{
		"create":         {"create", "settings", "-f", value, "--schema", notifSchema, "--scope", "environment"},
		"create dry run": {"create", "settings", "-f", value, "--schema", notifSchema, "--scope", "environment", "--dry-run"},
		"apply":          {"apply", "-f", notifFile(t, "real", "")},
		"apply dry run":  {"apply", "-f", notifFile(t, "real", ""), "--dry-run"},
	}
	for name, args := range cases {
		api, cfg := applyFake(t, "readwrite-all", notifRoutes())
		r := runCLI(t, cfg, args...)
		if r.Code != 0 || api.Hits(notifDef) != 1 {
			t.Errorf("%s: definition read %d times, want 1: %s", name, api.Hits(notifDef), r)
		}
	}
}

// TestCollectListingsRejectsABodyWithoutItems pins that a per-schema answer
// without an items list fails the fallback instead of reading as empty.
func TestCollectListingsRejectsABodyWithoutItems(t *testing.T) {
	_, _, _, _, err := collectListings([]fetched{{Body: map[string]interface{}{"totalCount": 1.0}}}, []error{nil})
	if err == nil {
		t.Error("a listing without items was accepted")
	}
}

// TestDeleteNotApplicableDefinitionKeepsItsHint pins that a 403 "Setting
// not applicable" while reading a definition does not blame settings.read.
func TestDeleteNotApplicableDefinitionKeepsItsHint(t *testing.T) {
	key := "GET " + envSchemas + "/builtin:problem.notifications"
	api, cfg := deleteFake(t, "readwrite-all", "", map[string]route{key: {Status: 403, Body: notApplicableBody}})
	r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes")
	if r.Code == 0 || api.Hits(deletePrefix+"obj-alerting-1") != 0 || strings.Contains(r.Stderr, hintReadSchemas) || !strings.Contains(r.Stderr, "does not apply to this scope") {
		t.Errorf("%s", r)
	}
}

// multiEnvEffective runs an effective listing over contexts a and b, whose
// fakes answer bodyA and bodyB with status.
func multiEnvEffective(t *testing.T, env map[string]string, status int, bodyA, bodyB string) cliResult {
	t.Helper()
	a := newFakeAPI(t, map[string]route{effectivePath: {Status: status, Body: bodyA}})
	b := newFakeAPI(t, map[string]route{effectivePath: {Status: status, Body: bodyB}})
	cfg := withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b})
	return runCLIWithEnv(t, env, cfg, "get", "settings", "--effective", "--scope", "HOST-0000000000000001", "--schema", "builtin:x", "--env", "a;b")
}

// TestMultiEnvironmentFailureShowsTheCommonHint pins that a person sees the
// hint every failed environment shares, and the scopes line for a missing
// permission, once, after the per-environment errors.
func TestMultiEnvironmentFailureShowsTheCommonHint(t *testing.T) {
	denied := `{"error":{"code":403,"message":"Token is missing required scope"}}`
	r := multiEnvEffective(t, nil, 403, denied, denied)
	if r.Code != 5 || strings.Count(r.Stderr, "Permission denied") != 1 || strings.Count(r.Stderr, "This command needs the token scope(s): settings.read.") != 1 {
		t.Errorf("two denied environments: %s", r)
	}
	r = multiEnvEffective(t, nil, 403, notApplicableBody, notApplicableBody)
	if !strings.Contains(r.Stderr, "does not apply to this scope") || strings.Contains(r.Stderr, "This command needs the token scope(s)") {
		t.Errorf("two not-applicable environments: %s", r)
	}
}

// TestMultiEnvironmentFailureAgentGetsTheCommonHint pins the same hint in
// the agent envelope's suggestions.
func TestMultiEnvironmentFailureAgentGetsTheCommonHint(t *testing.T) {
	denied := `{"error":{"code":403,"message":"Token is missing required scope"}}`
	r := multiEnvEffective(t, map[string]string{"CLAUDECODE": "1"}, 403, denied, denied)
	var env struct {
		Error struct {
			Suggestions []string `json:"suggestions"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil || !strings.Contains(strings.Join(env.Error.Suggestions, " "), "Permission denied") {
		t.Errorf("%v: %s", err, r)
	}
}

// TestMultiEnvironmentOwnHintsAreNotRepeated pins that environments whose
// errors carry their own hint get no generic hint at the end.
func TestMultiEnvironmentOwnHintsAreNotRepeated(t *testing.T) {
	a := newFakeAPI(t, map[string]route{})
	b := newFakeAPI(t, map[string]route{})
	cfg := withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b})
	r := runCLI(t, cfg, "get", "settings", "obj-missing", "--env", "a;b")
	if r.Code == 0 || !strings.Contains(r.Stderr, hintObjectNotFound) || strings.Contains(r.Stderr, "Resource not found") {
		t.Errorf("%s", r)
	}
}

// TestWriteFailure403ShowsAHint pins a hint for a write the cluster refused
// with 403: the scopes it needs, or the not-applicable reading.
func TestWriteFailure403ShowsAHint(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"denied":         {`{"error":{"code":403,"message":"Token is missing required scope"}}`, "settings.write"},
		"not applicable": {notApplicableBody, "does not apply to this scope"},
	}
	for name, tc := range cases {
		_, cfg := applyFake(t, "readwrite-all", map[string]route{
			updateWindowsDef:   {Body: `{"multiObject":true}`},
			updateWindowsRoute: {File: "settings_update_windows.json"},
			postObjects:        {Status: 403, Body: tc.body},
		})
		r := runCLI(t, cfg, "apply", "-f", "testdata/apply/window_new.yaml")
		if r.Code == 0 || !strings.Contains(r.Stderr, tc.want) {
			t.Errorf("%s: %s", name, r)
		}
	}
}
