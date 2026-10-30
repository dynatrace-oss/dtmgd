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
