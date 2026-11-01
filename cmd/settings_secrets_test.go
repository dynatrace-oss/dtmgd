package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"gopkg.in/yaml.v3"
)

const (
	notifSchema = "builtin:problem.notifications"
	notifDef    = "GET /e/env1/api/v2/settings/schemas/" + notifSchema
	notifList   = objectsPath + " schemaIds=" + notifSchema
	notifObj    = objPrefix + "obj-n1"
	maskValue   = "***3f9ff26c722fbb93***"
	secretPath  = "webHookNotification.headers[0].secretValue"
)

const notificationShapeJSON = `{"multiObject":true,"properties":{"type":{"type":"text"},"webHookNotification":{"type":{"$ref":"#/types/WebHook"}}},
 "types":{"WebHook":{"properties":{"url":{"type":"text"},"secretUrl":{"type":"secret"},
          "headers":{"type":"set","items":{"type":{"$ref":"#/types/Header"}}}}},
          "Header":{"properties":{"name":{"type":"text"},"secretValue":{"type":"secret"}}}}}`

func notificationShape() settingsSchemaShape {
	var s settingsSchemaShape
	if err := json.Unmarshal([]byte(notificationShapeJSON), &s); err != nil {
		panic(err)
	}
	return s
}

func webhookValue(secretURL, headerSecret string) map[string]interface{} {
	return map[string]interface{}{"type": "WEBHOOK", "webHookNotification": map[string]interface{}{
		"url": "https://example.invalid/x", "secretUrl": secretURL,
		"headers": []interface{}{map[string]interface{}{"name": "X", "secretValue": headerSecret}}}}
}

func TestMaskSecretsReplacesEverySecretAndCopies(t *testing.T) {
	v := webhookValue("s3cret", "real")
	got := maskSecrets(notificationShape(), v)
	wh := got["webHookNotification"].(map[string]interface{})
	h := wh["headers"].([]interface{})[0].(map[string]interface{})
	if wh["secretUrl"] != secretPlaceholder || h["secretValue"] != secretPlaceholder || wh["url"] != "https://example.invalid/x" || h["name"] != "X" {
		t.Errorf("masked %v", got)
	}
	in := v["webHookNotification"].(map[string]interface{})
	if in["secretUrl"] != "s3cret" || in["headers"].([]interface{})[0].(map[string]interface{})["secretValue"] != "real" {
		t.Errorf("input changed: %v", v)
	}
	if !notificationShape().hasSecrets() || (settingsSchemaShape{}).hasSecrets() {
		t.Error("hasSecrets wrong")
	}
	nilSecret := webhookValue("s", "x")
	nilSecret["webHookNotification"].(map[string]interface{})["secretUrl"] = nil
	if maskSecrets(notificationShape(), nilSecret)["webHookNotification"].(map[string]interface{})["secretUrl"] != nil {
		t.Error("a nil secret must stay nil")
	}
}

func TestMaskedSecretPaths(t *testing.T) {
	v := webhookValue("s3cret", maskValue)
	v["type"] = maskValue // not a secret property
	if got := maskedSecretPaths(notificationShape(), v); !reflect.DeepEqual(got, []string{secretPath}) {
		t.Errorf("paths %v", got)
	}
}

func TestMaskSecretsWithoutSecretsIsAnEqualCopy(t *testing.T) {
	v := map[string]interface{}{"name": "x", "list": []interface{}{"a", map[string]interface{}{"k": 1}}}
	if got := maskSecrets(settingsSchemaShape{}, v); !reflect.DeepEqual(got, v) {
		t.Errorf("got %v", got)
	}
}

func notifRoutes() map[string]route {
	return map[string]route{
		notifDef:    {Body: notificationShapeJSON},
		notifList:   {Body: `{"items":[],"totalCount":0}`},
		postObjects: {Body: `[{"code":200,"objectId":"new-1"}]`},
	}
}

func notifFile(t *testing.T, header string, objectID string) string {
	t.Helper()
	content := "schemaId: " + notifSchema + "\nscope: environment\n"
	if objectID != "" {
		content = "objectId: " + objectID + "\n" + content
	}
	content += "value:\n  type: WEBHOOK\n  webHookNotification:\n    url: https://example.invalid/x\n    headers:\n      - name: X\n        secretValue: " + header + "\n"
	return valueFile(t, content)
}

func TestCreateRefusesAMaskedSecret(t *testing.T) {
	file := valueFile(t, "type: WEBHOOK\nwebHookNotification:\n  url: https://example.invalid/x\n  headers:\n    - name: X\n      secretValue: '"+maskValue+"'\n")
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		api, cfg := applyFake(t, "readwrite-all", notifRoutes())
		args := append([]string{"create", "settings", "-f", file, "--schema", notifSchema, "--scope", "environment"}, extra...)
		r := runCLI(t, cfg, args...)
		if r.Code != 2 || !strings.Contains(r.Stderr, secretPath) || !strings.Contains(r.Stderr, "Exports mask secrets") || api.Hits(postObjects) != 0 {
			t.Errorf("%v: %s", extra, r)
		}
	}
}

func TestApplyRefusesAMaskedSecretInACreate(t *testing.T) {
	for name, objectID := range map[string]string{"no objectId": "", "unknown objectId": "obj-gone"} {
		t.Run(name, func(t *testing.T) {
			routes := notifRoutes()
			routes[objPrefix+"obj-gone"] = route{Status: 404, Body: `{"error":{"code":404,"message":"not found"}}`}
			api, cfg := applyFake(t, "readwrite-all", routes)
			r, _ := applyJSON(t, cfg, notifFile(t, "'"+maskValue+"'", objectID))
			if r.Code != 2 || !strings.Contains(r.Stderr, secretPath) || api.Hits(postObjects) != 0 {
				t.Errorf("%s", r)
			}
		})
	}
}

func notifObjectRoute(secret string) route {
	return route{Body: `{"objectId":"obj-n1","schemaId":"` + notifSchema + `","scope":"environment","updateToken":"tok","value":{"type":"WEBHOOK","webHookNotification":{"url":"https://example.invalid/old","headers":[{"name":"X","secretValue":"` + secret + `"}]}}}`}
}

func TestApplyMaskedSecretUpdatePasses(t *testing.T) {
	routes := notifRoutes()
	routes[notifObj] = notifObjectRoute(maskValue)
	routes[putPrefix+"obj-n1"] = route{Body: `{"code":200,"objectId":"obj-n1"}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	r, rep := applyJSON(t, cfg, notifFile(t, "'"+maskValue+"'", "obj-n1"))
	if r.Code != 0 || actions(rep) != "updated" || !strings.Contains(putBodyText(t, api), maskValue) {
		t.Errorf("%s", r)
	}
}

func TestEmptySecretCreatePasses(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", notifRoutes())
	r, rep := applyJSON(t, cfg, notifFile(t, `""`, ""))
	if r.Code != 0 || actions(rep) != "created" || api.Hits(postObjects) != 1 {
		t.Errorf("%s", r)
	}
}

func dryRunRoutes() map[string]route {
	routes := notifRoutes()
	routes[notifObj] = notifObjectRoute(maskValue)
	routes[putPrefix+"obj-n1"] = route{Body: `{"code":200,"objectId":"obj-n1"}`}
	return routes
}

// secretAt reads webHookNotification.headers[0].secretValue of a report value.
func secretAt(v map[string]interface{}) interface{} {
	wh, _ := v["webHookNotification"].(map[string]interface{})
	h, _ := wh["headers"].([]interface{})
	if len(h) == 0 {
		return nil
	}
	m, _ := h[0].(map[string]interface{})
	return m["secretValue"]
}

// envelopeResults reads the results of an agent envelope.
func envelopeResults(t *testing.T, stdout string) []writeResult {
	t.Helper()
	var e struct {
		Result writeReport `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	return e.Result.Results
}

func TestDryRunNeverPrintsASecret(t *testing.T) {
	file := notifFile(t, "real-value", "obj-n1")
	run := func(env map[string]string, args ...string) func(cfg string) cliResult {
		return func(cfg string) cliResult {
			return runCLIWithEnv(t, env, cfg, append([]string{"apply", "-f", file, "--dry-run"}, args...)...)
		}
	}
	cases := map[string]struct {
		run     func(cfg string) cliResult
		results func(t *testing.T, stdout string) []writeResult
	}{
		"json": {run(nil, "-o", "json"), func(t *testing.T, o string) []writeResult { return decodeReport(t, o).Results }},
		"yaml": {run(nil, "-o", "yaml"), func(t *testing.T, o string) []writeResult {
			var rep writeReport
			if err := yaml.Unmarshal([]byte(o), &rep); err != nil {
				t.Fatal(err)
			}
			return rep.Results
		}},
		"agent": {run(map[string]string{"CLAUDECODE": "1"}), envelopeResults},
		"table": {run(nil), nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, cfg := applyFake(t, "readwrite-all", dryRunRoutes())
			r := tc.run(cfg)
			if r.Code != 0 || strings.Contains(r.Stdout+r.Stderr, "real-value") || strings.Contains(r.Stdout+r.Stderr, maskValue) {
				t.Fatalf("%s", r)
			}
			if tc.results == nil {
				return
			}
			res := tc.results(t, r.Stdout)
			if len(res) != 1 || secretAt(res[0].Value) != secretPlaceholder || secretAt(res[0].Current) != secretPlaceholder {
				t.Errorf("want the placeholder in value and current: %s", r)
			}
		})
	}
}

// putBodyText is the first PUT body to obj-n1; the test fails if none was sent.
func putBodyText(t *testing.T, api *fakeAPI) string {
	t.Helper()
	bodies := api.Bodies(putPrefix + "obj-n1")
	if len(bodies) == 0 {
		t.Fatal("no PUT to obj-n1")
	}
	return bodies[0]
}

func TestRealWriteSendsTheRealSecret(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", dryRunRoutes())
	r, _ := applyJSON(t, cfg, notifFile(t, "real-value", "obj-n1"))
	if r.Code != 0 || strings.Contains(r.Stdout, "real-value") || !strings.Contains(putBodyText(t, api), "real-value") {
		t.Errorf("%s", r)
	}
}

// TestSecretCheckSharesTheValueSafelyAcrossContexts runs one value through
// several contexts planned in parallel: the check must only read it. Run
// with -race.
func TestSecretCheckSharesTheValueSafelyAcrossContexts(t *testing.T) {
	var envs []fakeEnv
	levels := map[string]string{}
	for _, n := range []string{"a", "b", "c", "d"} {
		envs = append(envs, fakeEnv{n, newFakeAPI(t, notifRoutes())})
		levels[n] = "readwrite-all"
	}
	cfg := withFakeEnvLevels(t, levels, envs...)
	create := valueFile(t, "type: WEBHOOK\nwebHookNotification:\n  url: https://example.invalid/x\n  secretUrl: real-url\n  headers:\n    - name: X\n      secretValue: real-value\n")
	base := []string{"create", "settings", "-f", create, "--schema", notifSchema, "--scope", "environment", "--env", "a;b;c;d"}
	apply := []string{"apply", "-f", notifFile(t, "real-value", ""), "--env", "a;b;c;d"}
	for name, args := range map[string][]string{
		"create dry run": append(base, "--dry-run"), "create": base,
		"apply dry run": append(apply, "--dry-run"), "apply": apply,
	} {
		if r := runCLI(t, cfg, args...); r.Code != 0 {
			t.Errorf("%s: %s", name, r)
		}
	}
	for _, e := range envs {
		if e.API.Hits(postObjects) != 2 {
			t.Errorf("%s: %d POSTs, want 2", e.Name, e.API.Hits(postObjects))
		}
	}
}

// TestApplyMaskedSecretOverTwoContextsIsAUsageError pins that when every
// context's plan fails with a usage error the run is a usage error (exit 2)
// with their common hint, not a partial failure.
func TestApplyMaskedSecretOverTwoContextsIsAUsageError(t *testing.T) {
	a, b := newFakeAPI(t, notifRoutes()), newFakeAPI(t, notifRoutes())
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all", "b": "readwrite-all"}, fakeEnv{"a", a}, fakeEnv{"b", b})
	file := notifFile(t, "'"+maskValue+"'", "")
	r := runCLI(t, cfg, "apply", "-f", file, "--env", "a;b")
	if r.Code != 2 || !strings.Contains(r.Stderr, "Exports mask secrets") || !strings.Contains(r.Stderr, "a:") || !strings.Contains(r.Stderr, "b:") {
		t.Errorf("human: %s", r)
	}
	r = runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "apply", "-f", file, "--env", "a;b")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("%v\n%s", err, r)
	}
	if r.Code != 2 || resp.Error == nil || !strings.Contains(strings.Join(resp.Error.Suggestions, "\n"), "Exports mask secrets") {
		t.Errorf("agent: %s", r)
	}
}

func TestGuardSecretsExecWriteKeepsItsDryRunValueWithoutASchemaID(t *testing.T) {
	ws := []plannedWrite{{Exec: func(*client.Client, bool) writeResult { return writeResult{} }, Value: map[string]interface{}{"setting": "ENABLED"}}}
	if err := guardSecrets(nil, ws, true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ws[0].ReportValue, ws[0].Value) {
		t.Errorf("ReportValue %v, want %v", ws[0].ReportValue, ws[0].Value)
	}
}

// listOfSecretsShape has one property: a list of plain secrets.
func listOfSecretsShape(t *testing.T) settingsSchemaShape {
	t.Helper()
	var s settingsSchemaShape
	if err := json.Unmarshal([]byte(`{"properties":{"tokens":{"type":"list","items":{"type":"secret"}}}}`), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSecretListOfPlainSecrets(t *testing.T) {
	s := listOfSecretsShape(t)
	if !s.hasSecrets() {
		t.Error("hasSecrets is false for a list of secrets")
	}
	v := map[string]interface{}{"tokens": []interface{}{"real", maskValue}}
	masked := maskSecrets(s, v)
	if want := []interface{}{secretPlaceholder, secretPlaceholder}; !reflect.DeepEqual(masked["tokens"], want) {
		t.Errorf("maskSecrets = %v, want %v", masked["tokens"], want)
	}
	if v["tokens"].([]interface{})[0] != "real" {
		t.Error("maskSecrets changed its input")
	}
	if got := maskedSecretPaths(s, v); !reflect.DeepEqual(got, []string{"tokens[1]"}) {
		t.Errorf("maskedSecretPaths = %v", got)
	}
}
