package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"gopkg.in/yaml.v3"
)

const (
	metricsQueryPath  = apiV2 + "/metrics/query"
	logsAggregatePath = apiV2 + "/logs/aggregate"
)

// Small fixtures for the query commands.
const (
	metricQueryBody = `{"totalCount":1,"resolution":"1h","result":[{"metricId":"builtin:host.cpu.usage","data":[` +
		`{"dimensions":["HOST-1"],"dimensionMap":{"dt.entity.host":"HOST-1"},"timestamps":[1730000000000,1730003600000],"values":[12.5,null]}]}]}`
	logCountEntitiesBody = `{"totalCount":3,"pageSize":500,"entities":[` +
		`{"entityId":"PROCESS_GROUP-AAA","displayName":"checkout"},` +
		`{"entityId":"PROCESS_GROUP-BBB","displayName":"cart"},` +
		`{"entityId":"PROCESS_GROUP-CCC","displayName":"search"}]}`
	logAggregateBody = `{"aggregationResult":{"dt.entity.process_group":{"1730000000000":{"process_group-aaa":7,"process_group-bbb":3}}}}`
)

// TestYAMLOutputParses pins Review Focus 1: -o yaml prints nothing but YAML
// on stdout, for every list, describe and query command. query log-counts
// used to print its header lines there.
func TestYAMLOutputParses(t *testing.T) {
	one := func(key, body string) map[string]route { return map[string]route{key: {Body: body}} }
	cases := []struct {
		name   string
		args   []string
		routes map[string]route
		format string // "" = yaml
	}{
		{"get problems", []string{"get", "problems"}, map[string]route{problemsPath: {File: "problems_small.json"}}, ""},
		{"get entities", []string{"get", "entities", "--selector", "type(HOST)"},
			one(entitiesPath, `{"totalCount":1,"entities":[{"entityId":"HOST-1","type":"HOST","displayName":"host-1"}]}`), ""},
		{"get entity-types", []string{"get", "entity-types"},
			one(apiV2+"/entityTypes", `{"totalCount":1,"types":[{"type":"HOST","displayName":"Host"}]}`), ""},
		{"get events", []string{"get", "events", "--from", "now-1h"},
			one(apiV2+"/events", `{"totalCount":1,"events":[{"eventId":"E1","eventType":"CUSTOM_INFO","title":"deploy","startTime":1730000000000}]}`), ""},
		{"get metrics", []string{"get", "metrics"},
			one(apiV2+"/metrics", `{"totalCount":1,"metrics":[{"metricId":"builtin:host.cpu.usage","displayName":"CPU usage %","unit":"Percent"}]}`), ""},
		{"get slos", []string{"get", "slos"}, one(sloPath, `{"totalCount":1,"slo":[`+sloItem(0)+`]}`), ""},
		{"get security-problems", []string{"get", "security-problems"},
			one(apiV2+"/securityProblems", `{"totalCount":1,"securityProblems":[{"securityProblemId":"SP-1","displayId":"S-1","status":"OPEN"}]}`), ""},
		{"describe problem", []string{"describe", "problem", "P"},
			one(apiV2+"/problems/P", `{"problemId":"P","displayId":"P-1","title":"CPU saturation","status":"OPEN"}`), ""},
		{"describe entity", []string{"describe", "entity", "HOST-1"}, map[string]route{entitiesPath + "/HOST-1": {File: "entity.json"}}, ""},
		{"describe entity-type", []string{"describe", "entity-type", "HOST"},
			one(apiV2+"/entityTypes/HOST", `{"type":"HOST","displayName":"Host","properties":[{"id":"osType","type":"String"}]}`), ""},
		{"describe entity-relations", []string{"describe", "entity-relations", "HOST-1"},
			one(entitiesPath+"/HOST-1", `{"entityId":"HOST-1","fromRelationships":{"isProcessOf":[{"id":"PGI-1","type":"PROCESS_GROUP_INSTANCE"}]},"toRelationships":{}}`), ""},
		{"describe event", []string{"describe", "event", "E1"},
			one(apiV2+"/events/E1", `{"eventId":"E1","eventType":"CUSTOM_INFO","title":"deploy"}`), ""},
		{"describe metric", []string{"describe", "metric", "builtin:host.cpu.usage"},
			one(apiV2+"/metrics/builtin:host.cpu.usage", `{"metricId":"builtin:host.cpu.usage","displayName":"CPU usage %","unit":"Percent"}`), ""},
		{"describe slo", []string{"describe", "slo", "X"}, one(sloPath+"/X", `{"id":"X","name":"slo x","status":"SUCCESS"}`), ""},
		{"describe security-problem", []string{"describe", "security-problem", "SP-UUID"},
			one(apiV2+"/securityProblems/SP-UUID", `{"securityProblemId":"SP-UUID","displayId":"S-1","title":"CVE in lib","status":"OPEN"}`), ""},
		{"query metrics", []string{"query", "metrics", "--metric", "builtin:host.cpu.usage", "--from", "now-1h"},
			one(metricsQueryPath, metricQueryBody), ""},
		{"query metrics yml", []string{"query", "metrics", "--metric", "builtin:host.cpu.usage", "--from", "now-1h"},
			one(metricsQueryPath, metricQueryBody), "yml"},
		{"query logs", []string{"query", "logs", "--query", "error", "--from", "now-1h"}, map[string]route{logsPath: {File: "logs.json"}}, ""},
		{"query log-counts", []string{"query", "log-counts", "--entity", "type(SERVICE)", "--from", "now-1h"},
			map[string]route{entitiesPath: {Body: logCountEntitiesBody}, logsAggregatePath: {Body: logAggregateBody}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format := tc.format
			if format == "" {
				format = "yaml"
			}
			api := newFakeAPI(t, tc.routes)
			args := append(append([]string(nil), tc.args...), "-o", format)
			r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
			if r.Code != 0 {
				t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
			}
			if strings.TrimSpace(r.Stdout) == "" {
				t.Fatalf("no stdout; stderr %q", r.Stderr)
			}
			var v interface{}
			if err := yaml.Unmarshal([]byte(r.Stdout), &v); err != nil {
				t.Fatalf("stdout is not YAML: %v\n%s", err, r.Stdout)
			}
			if _, ok := v.(map[string]interface{}); !ok {
				t.Errorf("stdout is %T, want a YAML mapping (the API-shaped object):\n%s", v, r.Stdout)
			}
			first, _, _ := strings.Cut(r.Stdout, "\n")
			for _, sentence := range []string{"Found", "Showing", "Log counts", "Time range", "No ", "Every"} {
				if strings.HasPrefix(first, sentence) {
					t.Errorf("first stdout line is a human sentence: %q", first)
				}
			}
		})
	}
}

// An entity lookup that fails is an error, not "No entities found": a 401
// exits 3.
func TestLogCountsEntityLookupErrorFails(t *testing.T) {
	api := newFakeAPI(t, map[string]route{
		entitiesPath:      {Status: 401, File: "error_401.json"},
		logsAggregatePath: {Body: logAggregateBody},
	})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "log-counts", "--entity", "type(SERVICE)")
	if r.Code != 3 {
		t.Errorf("code = %d, want 3; stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	if strings.Contains(r.Stderr, "No entities found") {
		t.Errorf("stderr reports an empty result for a failed lookup: %q", r.Stderr)
	}
	if n := api.Hits(logsAggregatePath); n != 0 {
		t.Errorf("%d aggregate requests after a failed lookup, want 0", n)
	}
}

// logCountsAPI is a fake environment for query log-counts that answers
// /logs/aggregate by the request's query (the log level), which fakeAPI's
// path-only routing cannot do. It records every aggregate query string.
func logCountsAPI(t *testing.T, entities string, byLevel map[string]string) (*fakeAPI, func() []url.Values) {
	t.Helper()
	var mu sync.Mutex
	var aggQueries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/e/env1/api/v2/entities":
			_, _ = io.WriteString(w, entities)
		case "/e/env1/api/v2/logs/aggregate":
			mu.Lock()
			aggQueries = append(aggQueries, r.URL.Query())
			mu.Unlock()
			body, ok := byLevel[r.URL.Query().Get("query")]
			if !ok {
				body = `{"aggregationResult":{}}`
			}
			_, _ = io.WriteString(w, body)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":404,"message":"not found"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return &fakeAPI{URL: srv.URL}, func() []url.Values {
		mu.Lock()
		defer mu.Unlock()
		return append([]url.Values(nil), aggQueries...)
	}
}

// /logs/aggregate returns at most maxGroupValues groups per level. When a
// level's result is full, a process group missing from it is unknown, not 0:
// its cell shows "-" and stderr says why. When the result has room left, a
// missing group really has no records, so its count is 0.
func TestLogCountsTopNWindowShowsUnknown(t *testing.T) {
	api, aggQueries := logCountsAPI(t, logCountEntitiesBody, map[string]string{
		// Full: 2 groups with --max-services 2, so "search" is unknown.
		"INFO": logAggregateBody,
		// Room left: "cart" and "search" have no WARN records.
		"WARN": `{"aggregationResult":{"dt.entity.process_group":{"1730000000000":{"process_group-aaa":2}}}}`,
	})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	args := []string{"query", "log-counts", "--entity", `type(SERVICE),tag("team:shop")`, "--max-services", "2"}

	r := runCLI(t, cfg, args...)
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		f := strings.Fields(line)
		if len(f) == 5 {
			rows[f[0]] = f[1:]
		}
	}
	want := map[string][]string{ // INFO WARN ERROR TOTAL
		"checkout": {"7", "2", "0", "9"},
		"cart":     {"3", "0", "0", "3"},
		"search":   {"-", "0", "0", "-"},
	}
	for name, cells := range want {
		if got := strings.Join(rows[name], " "); got != strings.Join(cells, " ") {
			t.Errorf("%s row = %q, want %q\n%s", name, got, strings.Join(cells, " "), r.Stdout)
		}
	}
	note := `Some process groups are outside the top 2 for INFO; their counts are shown as "-".`
	if !strings.Contains(r.Stderr, note) {
		t.Errorf("stderr lacks the top-N note:\n%s", r.Stderr)
	}
	for _, level := range []string{"WARN", "ERROR"} {
		if strings.Contains(r.Stderr, "top 2 for "+level) {
			t.Errorf("stderr has a note for %s, whose result was not full:\n%s", level, r.Stderr)
		}
	}
	if strings.Contains(r.Stdout, "Log counts") || !strings.Contains(r.Stderr, "Log counts for entity selector") {
		t.Errorf("header lines belong on stderr; stdout %q, stderr %q", r.Stdout, r.Stderr)
	}

	// DT Managed Classic ignores entitySelector on /logs/aggregate and
	// attributes logs to process groups, so the command never sends one and
	// looks up process groups instead of services.
	for _, q := range aggQueries() {
		if q.Has("entitySelector") || q.Get("maxGroupValues") != "2" || q.Get("groupBy") != "dt.entity.process_group" {
			t.Errorf("aggregate query = %v", q)
		}
	}

	// json carries null, not 0, for an unknown count.
	r = runCLI(t, cfg, append(args, "-o", "json")...)
	var body struct {
		LogCounts []map[string]interface{} `json:"logCounts"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil {
		t.Fatalf("json: %v\n%s", err, r.Stdout)
	}
	for _, row := range body.LogCounts {
		if row["service"] != "search" {
			continue
		}
		if row["info"] != nil || row["total"] != nil || row["warn"] != float64(0) {
			t.Errorf("json search row = %v, want info and total null, warn 0", row)
		}
	}
}

// The command shows each process group's own display name; no demo
// application's naming scheme is special-cased.
func TestLogCountsNoDemoNames(t *testing.T) {
	src, err := os.ReadFile("query_log_counts.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(src)), "bookstore") {
		t.Error("query_log_counts.go mentions BookStore")
	}
}

// /logs/search pages by slice: a nextSliceKey means more records exist.
func TestQueryLogsHasMoreFromSliceKey(t *testing.T) {
	body := `{"sliceSize":1,"nextSliceKey":"SLICE2","results":[{"timestamp":1730000000000,"status":"ERROR","content":"error: payment failed"}]}`
	api := newFakeAPI(t, map[string]route{logsPath: {Body: body}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "logs", "--query", "error", "--from", "now-1h", "-A")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("%v\n%s", err, r.Stdout)
	}
	if !resp.OK || resp.Context == nil || !resp.Context.HasMore {
		t.Errorf("ok=%v context=%+v, want has_more", resp.OK, resp.Context)
	}
	if resp.Context != nil && (resp.Context.Verb != "query" || resp.Context.Resource != "logs") {
		t.Errorf("context verb=%q resource=%q, want query/logs", resp.Context.Verb, resp.Context.Resource)
	}
}

// query logs makes one /logs/search request whose limit is --limit capped at
// the API's 1000, or 100 without --limit.
func TestQueryLogsLimitParam(t *testing.T) {
	for _, tc := range []struct {
		limit []string
		want  string
	}{
		{nil, "100"},
		{[]string{"--limit", "20"}, "20"},
		{[]string{"--limit", "5000"}, "1000"},
	} {
		api := newFakeAPI(t, map[string]route{logsPath: {File: "logs.json"}})
		args := append([]string{"query", "logs", "--query", "error", "--from", "now-1h"}, tc.limit...)
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
		qs := api.Queries(logsPath)
		if r.Code != 0 || len(qs) != 1 || qs[0].Get("limit") != tc.want {
			t.Errorf("%v: code %d, queries %v, want one with limit=%s", tc.limit, r.Code, qs, tc.want)
		}
	}
}

// query metrics prints its whole summary on stdout. A failed entity-name
// lookup is one warning, not a failure: the IDs are still shown.
func TestQueryMetricsLookupErrorWarns(t *testing.T) {
	api := newFakeAPI(t, map[string]route{
		metricsQueryPath: {Body: metricQueryBody},
		entitiesPath:     {Status: 403, File: "error_403.json"},
	})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "metrics", "--metric", "builtin:host.cpu.usage", "--from", "now-1h")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	for _, want := range []string{"Resolution: 1h", "Metric: builtin:host.cpu.usage", "Entity: HOST-1", "12.5000"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.Stdout)
		}
	}
	if n := strings.Count(r.Stderr, "entity names"); n != 1 {
		t.Errorf("stderr has %d entity-name warnings, want 1:\n%s", n, r.Stderr)
	}
}

// query log-counts goes through the shared runner, so --env with several
// environments prints a map keyed by context name.
func TestLogCountsMultiEnv(t *testing.T) {
	routes := map[string]route{entitiesPath: {Body: logCountEntitiesBody}, logsAggregatePath: {Body: logAggregateBody}}
	a, b := newFakeAPI(t, routes), newFakeAPI(t, routes)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}),
		"query", "log-counts", "--entity", "type(SERVICE)", "--env", "a;b", "-o", "json")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var got map[string]LogCounts
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil {
		t.Fatalf("%v\n%s", err, r.Stdout)
	}
	for _, env := range []string{"a", "b"} {
		if n := len(got[env].LogCounts); n != 3 {
			t.Errorf("%s: %d rows, want 3:\n%s", env, n, r.Stdout)
		}
	}
	if a.Hits(entitiesPath) != 1 || b.Hits(entitiesPath) != 1 {
		t.Errorf("entity lookups: a=%d b=%d, want 1 each", a.Hits(entitiesPath), b.Hits(entitiesPath))
	}
}
