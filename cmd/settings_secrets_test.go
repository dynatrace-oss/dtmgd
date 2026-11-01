package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
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
			routes[objPrefix+"obj-gone"] = route{Status: 404, File: "error_403.json"}
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
	if r.Code != 0 || actions(rep) != "updated" || !strings.Contains(api.Bodies(putPrefix + "obj-n1")[0], maskValue) {
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

func TestDryRunNeverPrintsASecret(t *testing.T) {
	file := notifFile(t, "real-value", "obj-n1")
	for name, run := range map[string]func(cfg string) cliResult{
		"json": func(cfg string) cliResult { return runCLI(t, cfg, "apply", "-f", file, "--dry-run", "-o", "json") },
		"yaml": func(cfg string) cliResult { return runCLI(t, cfg, "apply", "-f", file, "--dry-run", "-o", "yaml") },
		"agent": func(cfg string) cliResult {
			return runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "apply", "-f", file, "--dry-run")
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, cfg := applyFake(t, "readwrite-all", dryRunRoutes())
			r := run(cfg)
			if r.Code != 0 || strings.Contains(r.Stdout, "real-value") || strings.Contains(r.Stdout, maskValue) {
				t.Fatalf("%s", r)
			}
			if strings.Count(unescapeAngles(r.Stdout), secretPlaceholder) < 2 {
				t.Errorf("want the placeholder in value and current: %s", r)
			}
		})
	}
}

func TestRealWriteSendsTheRealSecret(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", dryRunRoutes())
	r, _ := applyJSON(t, cfg, notifFile(t, "real-value", "obj-n1"))
	if r.Code != 0 || strings.Contains(r.Stdout, "real-value") || !strings.Contains(api.Bodies(putPrefix + "obj-n1")[0], "real-value") {
		t.Errorf("%s", r)
	}
}

// unescapeAngles undoes encoding/json's \u003c and \u003e.
func unescapeAngles(s string) string {
	return strings.NewReplacer(`\u003c`, "<", `\u003e`, ">").Replace(s)
}
