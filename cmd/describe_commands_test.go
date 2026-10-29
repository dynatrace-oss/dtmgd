package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

const apiV2 = "GET /e/env1/api/v2"

// Describe commands without a summary of their own print YAML in table mode,
// never Go's map[...] syntax.
func TestDescribeTableIsReadable(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		route   string
		body    string
		want    string // the fixture's ID as YAML
		notWant string // "" = nothing extra to rule out
	}{
		{
			name:  "entity",
			args:  []string{"describe", "entity", "HOST-1"},
			route: apiV2 + "/entities/HOST-1",
			body:  `{"entityId":"HOST-1","displayName":"host-1","type":"HOST","properties":{"osType":"LINUX"}}`,
			want:  "entityId: HOST-1",
		},
		{
			name:  "entity-type",
			args:  []string{"describe", "entity-type", "HOST"},
			route: apiV2 + "/entityTypes/HOST",
			body:  `{"type":"HOST","displayName":"Host","properties":[{"id":"osType","type":"String"}]}`,
			want:  "type: HOST",
		},
		{
			// Only the two relationship keys are kept.
			name:    "entity-relations",
			args:    []string{"describe", "entity-relations", "HOST-1"},
			route:   apiV2 + "/entities/HOST-1",
			body:    `{"entityId":"HOST-1","displayName":"host-1","fromRelationships":{"isProcessOf":[{"id":"PGI-1","type":"PROCESS_GROUP_INSTANCE"}]},"toRelationships":{}}`,
			want:    "id: PGI-1",
			notWant: "entityId",
		},
		{
			name:  "event",
			args:  []string{"describe", "event", "E1"},
			route: apiV2 + "/events/E1",
			body:  `{"eventId":"E1","eventType":"CUSTOM_INFO","title":"deploy","properties":[{"key":"version","value":"1.2"}]}`,
			want:  "eventId: E1",
		},
		{
			name:  "metric",
			args:  []string{"describe", "metric", "builtin:host.cpu.usage"},
			route: apiV2 + "/metrics/builtin:host.cpu.usage",
			body:  `{"metricId":"builtin:host.cpu.usage","displayName":"CPU usage %","unit":"Percent","dimensionDefinitions":[{"key":"dt.entity.host","type":"ENTITY"}]}`,
			want:  "metricId: builtin:host.cpu.usage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, map[string]route{tt.route: {Body: tt.body}})
			r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), tt.args...)
			if r.Code != 0 {
				t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
			}
			if strings.Contains(r.Stdout, "map[") {
				t.Errorf("stdout has Go map syntax:\n%s", r.Stdout)
			}
			if !strings.Contains(r.Stdout, tt.want) {
				t.Errorf("stdout lacks %q:\n%s", tt.want, r.Stdout)
			}
			if tt.notWant != "" && strings.Contains(r.Stdout, tt.notWant) {
				t.Errorf("stdout has %q:\n%s", tt.notWant, r.Stdout)
			}
		})
	}
}

// Without timeFrame=GTF the API ignores from/to and evaluates the SLO over
// its own timeframe, so explicit dates must bring GTF with them.
func TestDescribeSLOSetsGTF(t *testing.T) {
	const path = apiV2 + "/slo/X"
	api := newFakeAPI(t, map[string]route{path: {Body: `{"id":"X","name":"slo x","status":"SUCCESS"}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "describe", "slo", "X", "--from", "now-7d")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	qs := api.Queries(path)
	if len(qs) != 1 {
		t.Fatalf("requests = %d, want 1", len(qs))
	}
	if got := qs[0].Get("timeFrame"); got != "GTF" {
		t.Errorf("timeFrame = %q, want GTF", got)
	}
	if got := qs[0].Get("from"); got != "now-7d" {
		t.Errorf("from = %q, want now-7d", got)
	}
}

// --env with several environments prints a map keyed by context name.
func TestDescribeMultiEnvJSON(t *testing.T) {
	const path = apiV2 + "/problems/P"
	body := `{"problemId":"P","displayId":"P-1","title":"CPU saturation","status":"OPEN"}`
	a := newFakeAPI(t, map[string]route{path: {Body: body}})
	b := newFakeAPI(t, map[string]route{path: {Body: body}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "describe", "problem", "P", "--env", "a;b", "-o", "json")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var got map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil {
		t.Fatalf("stdout is not a JSON object of objects: %v\n%s", err, r.Stdout)
	}
	if len(got) != 2 {
		t.Errorf("keys = %d, want 2 (a, b):\n%s", len(got), r.Stdout)
	}
	for _, env := range []string{"a", "b"} {
		if id := got[env]["problemId"]; id != "P" {
			t.Errorf("%s.problemId = %v, want P", env, id)
		}
	}
}

// describe problem keeps its human summary in table mode, and -A prints one
// envelope whose context names the verb and resource.
func TestDescribeProblemHumanSummary(t *testing.T) {
	const path = apiV2 + "/problems/P"
	body := `{"problemId":"P","displayId":"P-1","title":"CPU saturation","status":"OPEN","severityLevel":"RESOURCE_CONTENTION","impactLevel":"INFRASTRUCTURE","startTime":1730000000000}`
	api := newFakeAPI(t, map[string]route{path: {Body: body}})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})

	r := runCLI(t, cfg, "describe", "problem", "P")
	if r.Code != 0 {
		t.Fatalf("table: code %d, stderr %q", r.Code, r.Stderr)
	}
	for _, want := range []string{fmt.Sprintf("%-22s %s", "Title:", "CPU saturation"), fmt.Sprintf("%-22s %s", "Display ID:", "P-1")} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("table: stdout lacks %q:\n%s", want, r.Stdout)
		}
	}

	r = runCLI(t, cfg, "describe", "problem", "P", "-A")
	if r.Code != 0 {
		t.Fatalf("agent: code %d, stderr %q", r.Code, r.Stderr)
	}
	var resp agentmode.Response
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("agent: stdout is not an envelope: %v\n%s", err, r.Stdout)
	}
	if dec.More() {
		t.Errorf("agent: more than one envelope:\n%s", r.Stdout)
	}
	if !resp.OK || resp.Context == nil {
		t.Fatalf("agent: ok=%v context=%v", resp.OK, resp.Context)
	}
	if resp.Context.Verb != "describe" || resp.Context.Resource != "problem" {
		t.Errorf("agent: context verb=%q resource=%q, want describe/problem", resp.Context.Verb, resp.Context.Resource)
	}
	if resp.Context.Total != nil {
		t.Errorf("agent: context total = %d, want none for a single object", *resp.Context.Total)
	}
	if m, ok := resp.Result.(map[string]interface{}); !ok || m["title"] != "CPU saturation" {
		t.Errorf("agent: result = %v, want the API object", resp.Result)
	}
}
