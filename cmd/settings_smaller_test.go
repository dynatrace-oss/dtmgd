package cmd

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

const notApplicableBody = `{"error":{"code":403,"message":"Setting not applicable: 'Host anomaly detection settings are not applicable for PaaS/standalone deployments.'"}}`

func TestNotApplicable403HasItsOwnHint(t *testing.T) {
	routes := map[string]route{effectivePath: {Status: 403, Body: notApplicableBody}}
	_, cfg := settingsFake(t, routes)
	args := []string{"get", "settings", "--effective", "--scope", "HOST-0000000000000001", "--schema", "builtin:x"}
	r := runCLI(t, cfg, args...)
	if r.Code != 5 || !strings.Contains(r.Stderr, "does not apply to this scope") ||
		strings.Contains(r.Stderr, "Permission denied") || strings.Contains(r.Stderr, "This command needs the token scope(s)") {
		t.Errorf("%s", r)
	}
	r = runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, args...)
	if !strings.Contains(r.Stdout, "does not apply to this scope") || strings.Contains(r.Stdout, "Permission denied") {
		t.Errorf("agent mode: %s", r)
	}
}

func TestClusterNotApplicable403SkipsTheScopeHint(t *testing.T) {
	_, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterEffective: {Status: 403, Body: notApplicableBody}})
	r := runCLI(t, cfg, "get", "settings", "--effective", "--scope", "HOST-0000000000000001", "--schema", "builtin:x")
	if r.Code != 5 || !strings.Contains(r.Stderr, "does not apply to this scope") ||
		strings.Contains(r.Stderr, hintClusterSettingsScopes) || strings.Contains(r.Stderr, "Permission denied") {
		t.Errorf("%s", r)
	}
}

func TestTablesUnescapeSummaries(t *testing.T) {
	body := `{"items":[{"objectId":"o1","schemaId":"builtin:x","scope":"environment","summary":"dtmgd\\-it\\-team \\- made\\.","value":{}}],"totalCount":1}`
	_, cfg := settingsFake(t, map[string]route{objectsPath: {Body: body}})
	r := runCLI(t, cfg, "get", "settings", "--schema", "builtin:x")
	if !strings.Contains(r.Stdout, "dtmgd-it-team - made.") || strings.Contains(r.Stdout, `\-`) {
		t.Errorf("table: %s", r)
	}
	r = runCLI(t, cfg, "get", "settings", "--schema", "builtin:x", "-o", "json")
	if !strings.Contains(r.Stdout, `dtmgd\\-it\\-team`) {
		t.Errorf("json: %s", r)
	}
}

func TestDescribeSchemaShowsMaturityKeyAndReferences(t *testing.T) {
	api := newFakeAPI(t, map[string]route{schemasPath + "/builtin:example.references": {File: "settings_schema_references.json"}})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	r := runCLI(t, cfg, "describe", "settings-schema", "builtin:example.references")
	for _, want := range []string{"Maturity:     EARLY_ADOPTER", "Key property: name", "setting → builtin:alerting.profile", "list of setting → builtin:management-zones"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("stdout lacks %q", want)
		}
	}
	assertGolden(t, "describe_settings_schema_references", r.String())
}

func TestNotApplicable403OnAWriteSaysWhatTheUserSees(t *testing.T) {
	_, cfg := applyFake(t, "readwrite-all", map[string]route{
		updateWindowsDef:   {Body: `{"multiObject":true}`},
		updateWindowsRoute: {File: "settings_update_windows.json"},
		postObjects:        {Status: 403, Body: notApplicableBody},
	})
	r := runCLI(t, cfg, "apply", "-f", "testdata/apply/window_new.yaml")
	if r.Code == 0 || strings.Contains(r.Stderr, "This command needs the token scope(s)") || strings.Contains(r.Stderr, "Permission denied") {
		t.Errorf("%s", r)
	}
	if !strings.Contains(r.String(), "Setting not applicable") {
		t.Errorf("the cluster's reason is missing: %s", r)
	}
}

// A multi-environment failure is reported per environment, then the hint
// the failures share: no scopes line when every environment's 403 is "not
// applicable", the scopes line when one is a real permission problem.
func TestNotApplicableAcrossEnvironments(t *testing.T) {
	const ordinary = `{"error":{"code":403,"message":"Token is missing required scope"}}`
	r := multiEnvEffective(t, nil, 403, notApplicableBody, notApplicableBody)
	if r.Code != 5 || !strings.Contains(r.Stderr, "Setting not applicable") || strings.Contains(r.Stderr, "This command needs the token scope(s)") {
		t.Errorf("both not applicable: %s", r)
	}
	r = multiEnvEffective(t, nil, 403, notApplicableBody, ordinary)
	if r.Code != 5 || !strings.Contains(r.Stderr, "Setting not applicable") || !strings.Contains(r.Stderr, "This command needs the token scope(s)") {
		t.Errorf("mixed: %s", r)
	}

	na := client.EnvResult{Error: client.APIError(403, notApplicableBody)}
	other := client.EnvResult{Error: client.APIError(403, ordinary)}
	both := &client.PartialFailureError{Failed: []client.EnvResult{na, na}, Total: 2}
	mixed := &client.PartialFailureError{Failed: []client.EnvResult{na, other}, Total: 2}
	if isPermissionDenied(both) || !strings.Contains(hintFor(both), "does not apply to this scope") {
		t.Errorf("both not applicable: denied=%v hint=%q", isPermissionDenied(both), hintFor(both))
	}
	if !isPermissionDenied(mixed) || hintFor(mixed) != "" {
		t.Errorf("mixed: denied=%v hint=%q", isPermissionDenied(mixed), hintFor(mixed))
	}
}
