package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// agentLinks runs an agent-mode command and returns its envelope's
// context.links (nil when absent), failing the test on a malformed envelope.
func agentLinks(t *testing.T, r cliResult) map[string]string {
	t.Helper()
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("stdout is not one envelope: %v\n%s\nstderr: %s", err, r.Stdout, r.Stderr)
	}
	if resp.Context == nil {
		t.Fatalf("envelope has no context:\n%s", r.Stdout)
	}
	return resp.Context.Links
}

// A single environment's agent envelope links its result in the Dynatrace UI
// under "ui".
func TestAgentLinksSingleEnv(t *testing.T) {
	api := newFakeAPI(t, map[string]route{apiV2 + "/problems/P": {Body: `{"problemId":"P","displayId":"P-1","title":"CPU saturation"}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "describe", "problem", "P", "-A")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	want := map[string]string{"ui": "http://FAKE/e/env1/#problems/problemdetails;pid=P"}
	if got := agentLinks(t, r); !reflect.DeepEqual(got, want) {
		t.Errorf("context.links = %v, want %v", got, want)
	}
}

// Review Focus 5: with several environments, context.links has one entry per
// environment that answered, each built from that environment's own URL, and
// none for a failed environment. A command without a link has no links.
func TestAgentLinksPerEnvironment(t *testing.T) {
	ids := map[string]string{"a": "envA", "b": "envB"}
	problems := func(envID string) map[string]route {
		return map[string]route{"GET /e/" + envID + "/api/v2/problems": {File: "problems_small.json"}}
	}

	t.Run("every environment answers", func(t *testing.T) {
		a, b := newFakeAPI(t, problems("envA")), newFakeAPI(t, problems("envB"))
		r := runCLI(t, withFakeEnvIDs(t, ids, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-A")
		if r.Code != 0 {
			t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
		}
		want := map[string]string{"a": "http://FAKE/e/envA/ui/problems", "b": "http://FAKE/e/envB/ui/problems"}
		if got := agentLinks(t, r); !reflect.DeepEqual(got, want) {
			t.Errorf("context.links = %v, want %v", got, want)
		}
	})

	t.Run("a failed environment gets no link", func(t *testing.T) {
		a := newFakeAPI(t, problems("envA"))
		b := newFakeAPI(t, map[string]route{"GET /e/envB/api/v2/problems": {Status: 403, File: "error_403.json"}})
		r := runCLI(t, withFakeEnvIDs(t, ids, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-A")
		if r.Code != 1 {
			t.Fatalf("code %d, want 1 (partial failure); stderr %q", r.Code, r.Stderr)
		}
		want := map[string]string{"a": "http://FAKE/e/envA/ui/problems"}
		if got := agentLinks(t, r); !reflect.DeepEqual(got, want) {
			t.Errorf("context.links = %v, want %v", got, want)
		}
	})

	t.Run("describe links each environment's own item", func(t *testing.T) {
		a := newFakeAPI(t, map[string]route{"GET /e/envA/api/v2/entities/HOST-1": {Body: `{"entityId":"HOST-1"}`}})
		b := newFakeAPI(t, map[string]route{"GET /e/envB/api/v2/entities/HOST-1": {Body: `{"entityId":"HOST-1"}`}})
		r := runCLI(t, withFakeEnvIDs(t, ids, fakeEnv{"a", a}, fakeEnv{"b", b}), "describe", "entity", "HOST-1", "--env", "a;b", "-A")
		if r.Code != 0 {
			t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
		}
		want := map[string]string{"a": "http://FAKE/e/envA/ui/entity/HOST-1", "b": "http://FAKE/e/envB/ui/entity/HOST-1"}
		if got := agentLinks(t, r); !reflect.DeepEqual(got, want) {
			t.Errorf("context.links = %v, want %v", got, want)
		}
	})

	t.Run("a command without a link has none", func(t *testing.T) {
		empty := `{"totalCount":0,"entities":[]}`
		a := newFakeAPI(t, map[string]route{"GET /e/envA/api/v2/entities": {Body: empty}})
		b := newFakeAPI(t, map[string]route{"GET /e/envB/api/v2/entities": {Body: empty}})
		r := runCLI(t, withFakeEnvIDs(t, ids, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "entities", "--selector", "type(HOST)", "--env", "a;b", "-A")
		if r.Code != 0 {
			t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
		}
		if got := agentLinks(t, r); got != nil {
			t.Errorf("context.links = %v, want none", got)
		}
	})
}

// In human mode a describe command names its Dynatrace UI page on stderr,
// after the detail; stdout holds only the detail. json and yaml print no
// link anywhere, and list commands print none in human mode.
func TestDescribeHumanPrintsLink(t *testing.T) {
	const path = apiV2 + "/problems/P"
	const link = "Open in Dynatrace: http://FAKE/e/env1/#problems/problemdetails;pid=P\n"
	body := `{"problemId":"P","displayId":"P-1","title":"CPU saturation","status":"OPEN"}`
	api := newFakeAPI(t, map[string]route{path: {Body: body}, problemsPath: {File: "problems_small.json"}})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})

	r := runCLI(t, cfg, "describe", "problem", "P")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if r.Stderr != link {
		t.Errorf("stderr = %q, want %q", r.Stderr, link)
	}
	if !strings.HasPrefix(r.Stdout, "ID:") || strings.Contains(r.Stdout, "Open in Dynatrace") {
		t.Errorf("stdout is not the detail alone:\n%s", r.Stdout)
	}

	for _, format := range []string{"json", "yaml"} {
		r := runCLI(t, cfg, "describe", "problem", "P", "-o", format)
		if r.Code != 0 || r.Stderr != "" {
			t.Errorf("-o %s: code %d, stderr %q, want 0 and nothing", format, r.Code, r.Stderr)
		}
		if strings.Contains(r.Stdout, "problemdetails") || strings.Contains(r.Stdout, "links") {
			t.Errorf("-o %s: stdout carries a link:\n%s", format, r.Stdout)
		}
	}

	r = runCLI(t, cfg, "get", "problems")
	if r.Code != 0 || strings.Contains(r.Stderr, "Open in Dynatrace") {
		t.Errorf("get problems: code %d, stderr %q, want no link", r.Code, r.Stderr)
	}
}

// With several environments in human mode, each describe section's link is
// its own environment's.
func TestDescribeHumanLinkPerEnvironment(t *testing.T) {
	body := `{"problemId":"P","title":"CPU saturation"}`
	a := newFakeAPI(t, map[string]route{"GET /e/envA/api/v2/problems/P": {Body: body}})
	b := newFakeAPI(t, map[string]route{"GET /e/envB/api/v2/problems/P": {Body: body}})
	cfg := withFakeEnvIDs(t, map[string]string{"a": "envA", "b": "envB"}, fakeEnv{"a", a}, fakeEnv{"b", b})
	r := runCLI(t, cfg, "describe", "problem", "P", "--env", "a;b")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	want := "Open in Dynatrace: http://FAKE/e/envA/#problems/problemdetails;pid=P\n" +
		"Open in Dynatrace: http://FAKE/e/envB/#problems/problemdetails;pid=P\n"
	if r.Stderr != want {
		t.Errorf("stderr = %q, want %q", r.Stderr, want)
	}
}

// Each command's UI link, as the Dynatrace Managed MCP server gives it, and
// none for commands it gives no page for. "" = no links in the context.
func TestAgentLinkPatterns(t *testing.T) {
	const base = "http://FAKE/e/env1"
	tests := []struct {
		name   string
		args   []string
		routes map[string]route
		want   string
	}{
		{"get problems", []string{"get", "problems"},
			map[string]route{problemsPath: {File: "problems_small.json"}},
			base + "/ui/problems"},
		{"describe problem with a negative ID", []string{"describe", "problem", "-1234_1730000000000V2"},
			map[string]route{apiV2 + "/problems/-1234_1730000000000V2": {Body: `{"problemId":"-1234_1730000000000V2"}`}},
			base + "/#problems/problemdetails;pid=-1234_1730000000000V2"},
		{"describe entity", []string{"describe", "entity", "SERVICE-ABC"},
			map[string]route{entitiesPath + "/SERVICE-ABC": {Body: `{"entityId":"SERVICE-ABC","displayName":"checkout"}`}},
			base + "/ui/entity/SERVICE-ABC"},
		{"describe entity-relations", []string{"describe", "entity-relations", "HOST-1"},
			map[string]route{entitiesPath + "/HOST-1": {Body: `{"entityId":"HOST-1","fromRelationships":{"isProcessOf":[{"id":"PGI-1"}]}}`}},
			base + "/ui/entity/HOST-1"},
		{"get security-problems", []string{"get", "security-problems"},
			map[string]route{apiV2 + "/securityProblems": {Body: `{"totalCount":0,"securityProblems":[]}`}},
			base + "/ui/security/vulnerabilities"},
		// A display ID is resolved first; the link uses the internal ID.
		{"describe security-problem by display ID", []string{"describe", "security-problem", "S-1"},
			map[string]route{
				apiV2 + "/securityProblems":      {Body: `{"securityProblems":[{"securityProblemId":"7001","displayId":"S-1"}]}`},
				apiV2 + "/securityProblems/7001": {Body: `{"securityProblemId":"7001","displayId":"S-1","title":"CVE"}`},
			},
			base + "/ui/security/vulnerabilities/7001"},
		{"get slos", []string{"get", "slos"},
			map[string]route{sloPath: {Body: `{"totalCount":0,"slo":[]}`}},
			base + "/ui/slo"},
		{"query logs", []string{"query", "logs", "--query", "error", "--from", "now-1h"},
			map[string]route{logsPath: {File: "logs.json"}},
			base + "/ui/log-monitoring"},
		{"get metrics", []string{"get", "metrics"},
			map[string]route{apiV2 + "/metrics": {Body: `{"totalCount":0,"metrics":[]}`}},
			base + "/ui/metrics"},
		{"query metrics", []string{"query", "metrics", "--metric", "builtin:host.cpu.usage", "--from", "now-1h"},
			map[string]route{metricsQueryPath: {Body: metricQueryBody}},
			base + "/ui/data-explorer"},
		{"get entities", []string{"get", "entities", "--selector", "type(HOST)"},
			map[string]route{entitiesPath: {Body: `{"totalCount":0,"entities":[]}`}},
			""},
		{"describe slo", []string{"describe", "slo", "X"},
			map[string]route{apiV2 + "/slo/X": {Body: `{"id":"X","name":"slo x"}`}},
			""},
		{"describe event", []string{"describe", "event", "E1"},
			map[string]route{apiV2 + "/events/E1": {Body: `{"eventId":"E1"}`}},
			""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.routes)
			r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append(tt.args, "-A")...)
			if r.Code != 0 {
				t.Fatalf("code %d, stderr %q\n%s", r.Code, r.Stderr, r.Stdout)
			}
			var want map[string]string
			if tt.want != "" {
				want = map[string]string{"ui": tt.want}
			}
			if got := agentLinks(t, r); !reflect.DeepEqual(got, want) {
				t.Errorf("context.links = %v, want %v", got, want)
			}
		})
	}
}

// An item link path-escapes the ID, and there is no link without a base URL
// (an environment the client cannot address) or without an ID in the body.
func TestUILinkBuilders(t *testing.T) {
	item := uiItem("/ui/entity/", "entityId")
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"page", uiPage("/ui/problems")("https://h/e/x", nil), "https://h/e/x/ui/problems"},
		{"item", item("https://h/e/x", map[string]interface{}{"entityId": "SERVICE-1"}), "https://h/e/x/ui/entity/SERVICE-1"},
		{"item escapes", item("https://h/e/x", map[string]interface{}{"entityId": "a/b;c#d e?"}), "https://h/e/x/ui/entity/a%2Fb%3Bc%23d%20e%3F"},
		{"item without ID", item("https://h/e/x", map[string]interface{}{"displayName": "x"}), ""},
		{"item with empty ID", item("https://h/e/x", map[string]interface{}{"entityId": ""}), ""},
		{"item of a non-object", item("https://h/e/x", []interface{}{}), ""},
		{"fixed ID", uiID("/ui/entity/", "HOST 1")("https://h/e/x", nil), "https://h/e/x/ui/entity/HOST%201"},
		{"fixed empty ID", uiID("/ui/entity/", "")("https://h/e/x", nil), ""},
		{"no link func", fetchSpec{}.uiLink(fetched{UIBase: "https://h/e/x"}), ""},
		{"no base", fetchSpec{Link: uiPage("/ui/problems")}.uiLink(fetched{}), ""},
		{"spec", fetchSpec{Link: uiPage("/ui/problems")}.uiLink(fetched{UIBase: "https://h/e/x"}), "https://h/e/x/ui/problems"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
