package cmd

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// Checked live on 2026-10-30 (Dynatrace Managed 1.348): /events ignores an
// eventType parameter, /entities ignores mzSelector and /metrics ignores
// entitySelector, each returning the unfiltered result. These tests pin the
// documented parameters the flags send instead.

func TestGetEventsTypeSendsAnEventSelector(t *testing.T) {
	api := newFakeAPI(t, map[string]route{apiV2 + "/events": {Body: `{"totalCount":0,"events":[]}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "events", "--from", "now-1h", "--type", "CUSTOM_DEPLOYMENT", "-o", "json")
	qs := api.Queries(apiV2 + "/events")
	if r.Code != 0 || len(qs) != 1 {
		t.Fatalf("code %d, queries %v\nstderr: %s", r.Code, qs, r.Stderr)
	}
	if got := qs[0].Get("eventSelector"); got != `eventType("CUSTOM_DEPLOYMENT")` {
		t.Errorf("eventSelector = %q, want eventType(\"CUSTOM_DEPLOYMENT\")", got)
	}
	if qs[0].Has("eventType") {
		t.Errorf("still sends the ignored eventType parameter: %v", qs[0])
	}
}

func TestGetEntitiesMzJoinsTheEntitySelector(t *testing.T) {
	cases := []struct{ mz, want string }{
		{"Production", `type(HOST),mzName("Production")`},
		{`Prod "EU" ~1`, `type(HOST),mzName("Prod ~"EU~" ~~1")`},
		{`mzName("Production")`, `type(HOST),mzName("Production")`},
		{"mzId(123456789)", `type(HOST),mzId(123456789)`},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{apiV2 + "/entities": {Body: `{"totalCount":0,"entities":[]}`}})
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "entities", "--selector", "type(HOST)", "--mz", tc.mz, "-o", "json")
		qs := api.Queries(apiV2 + "/entities")
		want := url.Values{"entitySelector": {tc.want}, "pageSize": {"100"}}
		if r.Code != 0 || len(qs) != 1 || !reflect.DeepEqual(qs[0], want) {
			t.Errorf("--mz %q: code %d, queries %v, want %v\nstderr: %s", tc.mz, r.Code, qs, want, r.Stderr)
		}
	}
}

func TestGetMetricsEntityIsRefused(t *testing.T) {
	api := newFakeAPI(t, map[string]route{apiV2 + "/metrics": {Body: `{"totalCount":0,"metrics":[]}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "metrics", "--search", "cpu", "--entity", "type(HOST)")
	if r.Code != 2 {
		t.Errorf("exit %d, want 2 (usage error)", r.Code)
	}
	if api.totalHits() != 0 {
		t.Errorf("made %d requests; the refusal must come first", api.totalHits())
	}
	if !strings.Contains(r.Stderr, "query metrics") {
		t.Errorf("stderr does not point at query metrics:\n%s", r.Stderr)
	}
}

// A zone or event type the environment does not know is a 400 "Constraints
// violated." without detail (the API's constraintViolations are not shown);
// the hint says what to check.
func TestUnknownZoneOrEventTypeGetsAHint(t *testing.T) {
	const violated = `{"error":{"code":400,"message":"Constraints violated."}}`
	cases := []struct {
		name, path, want string
		args             []string
	}{
		{"zone", apiV2 + "/entities", "case-sensitive", []string{"get", "entities", "--selector", "type(HOST)", "--mz", "Nope"}},
		{"event type", apiV2 + "/events", "CUSTOM_DEPLOYMENT", []string{"get", "events", "--from", "now-1h", "--type", "nope"}},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{tc.path: {Status: 400, Body: violated}})
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), tc.args...)
		if r.Code == 0 || !strings.Contains(r.Stderr, tc.want) {
			t.Errorf("%s: exit %d, stderr does not mention %q:\n%s", tc.name, r.Code, tc.want, r.Stderr)
		}
	}
}

func TestGetEntitiesEmptyNoteNamesTheZone(t *testing.T) {
	api := newFakeAPI(t, map[string]route{apiV2 + "/entities": {Body: `{"totalCount":0,"entities":[]}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "entities", "--selector", "type(HOST)", "--mz", "Production")
	if !strings.Contains(r.Stderr, `type(HOST),mzName("Production")`) {
		t.Errorf("the empty note hides the zone:\n%s", r.Stderr)
	}
}

// With several environments the zone is often missing from some of them; the
// failed environment keeps its hint, for a person and in the envelope.
func TestMultiEnvironmentFailureKeepsItsHint(t *testing.T) {
	a := newFakeAPI(t, map[string]route{apiV2 + "/entities": {Body: `{"totalCount":0,"entities":[]}`}})
	b := newFakeAPI(t, map[string]route{apiV2 + "/entities": {Status: 400, Body: `{"error":{"code":400,"message":"Constraints violated."}}`}})
	cfg := withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b})
	args := []string{"get", "entities", "--selector", "type(HOST)", "--mz", "Prod", "--env", "a;b"}
	if r := runCLI(t, cfg, args...); r.Code == 0 || !strings.Contains(r.Stderr, "case-sensitive") {
		t.Errorf("human: exit %d, stderr:\n%s", r.Code, r.Stderr)
	}
	if r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, args...); r.Code == 0 || !strings.Contains(r.Stdout, "case-sensitive") {
		t.Errorf("agent: exit %d, stdout:\n%s", r.Code, r.Stdout)
	}
}
