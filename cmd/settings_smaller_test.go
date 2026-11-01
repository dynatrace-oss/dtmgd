package cmd

import (
	"strings"
	"testing"
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
