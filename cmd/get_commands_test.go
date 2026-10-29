package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const sloPath = "GET /e/env1/api/v2/slo"

// listPages builds routes serving total items under listKey in pages of per,
// linked by nextPageKey the way the API pages. item renders item i as JSON.
func listPages(key, listKey string, total, per int, item func(i int) string) map[string]route {
	routes := map[string]route{}
	for page, start := 1, 0; start < total || page == 1; page, start = page+1, start+per {
		var items []string
		for i := start; i < start+per && i < total; i++ {
			items = append(items, item(i))
		}
		body := fmt.Sprintf(`{"totalCount":%d,"pageSize":%d,%q:[%s]`, total, per, listKey, strings.Join(items, ","))
		if start+per < total {
			body += fmt.Sprintf(`,"nextPageKey":"PAGE%d"`, page+1)
		}
		body += "}"
		k := key
		if page > 1 {
			k += fmt.Sprintf(" nextPageKey=PAGE%d", page)
		}
		routes[k] = route{Body: body}
	}
	return routes
}

func sloItem(i int) string {
	return fmt.Sprintf(`{"id":"SLO-%d","name":"SLO %d","enabled":true,"status":"SUCCESS","evaluatedPercentage":99.5,"target":95}`, i, i)
}

// listLen decodes a json object and returns the length of its listKey list.
func listLen(t *testing.T, stdout, listKey string) int {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	list, ok := body[listKey].([]interface{})
	if !ok {
		t.Fatalf("no %q list in %s", listKey, stdout)
	}
	return len(list)
}

// TestGetListCommandsHonourLimit pins that every get list command applies
// --limit to json output, not only to the table.
func TestGetListCommandsHonourLimit(t *testing.T) {
	cases := []struct {
		name, path, listKey string
		args                []string
		item                func(i int) string
	}{
		{"entities", entitiesPath, "entities", []string{"get", "entities", "--selector", "type(HOST)"},
			func(i int) string {
				return fmt.Sprintf(`{"entityId":"HOST-%d","type":"HOST","displayName":"host-%d"}`, i, i)
			}},
		{"entity-types", "GET /e/env1/api/v2/entityTypes", "types", []string{"get", "entity-types"},
			func(i int) string { return fmt.Sprintf(`{"type":"TYPE_%d","displayName":"Type %d"}`, i, i) }},
		{"events", "GET /e/env1/api/v2/events", "events", []string{"get", "events", "--from", "now-1h"},
			func(i int) string {
				return fmt.Sprintf(`{"eventId":"EV-%d","eventType":"CUSTOM_INFO","title":"Event %d","startTime":1700000000000}`, i, i)
			}},
		{"metrics", "GET /e/env1/api/v2/metrics", "metrics", []string{"get", "metrics"},
			func(i int) string {
				return fmt.Sprintf(`{"metricId":"builtin:m%d","displayName":"Metric %d","unit":"Count"}`, i, i)
			}},
		{"slos", sloPath, "slo", []string{"get", "slos"}, sloItem},
		{"security-problems", "GET /e/env1/api/v2/securityProblems", "securityProblems", []string{"get", "security-problems"},
			func(i int) string {
				return fmt.Sprintf(`{"securityProblemId":"SP-%d","displayId":"S-%d","status":"OPEN"}`, i, i)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t, listPages(tc.path, tc.listKey, 5, 5, tc.item))
			args := append(append([]string(nil), tc.args...), "--limit", "3", "-o", "json")
			r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
			if r.Code != 0 {
				t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
			}
			if n := listLen(t, r.Stdout, tc.listKey); n != 3 {
				t.Errorf("%d %s, want 3", n, tc.listKey)
			}

			table := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append(append([]string(nil), tc.args...), "--limit", "3")...)
			if rows := strings.Count(strings.TrimSpace(table.Stdout), "\n"); table.Code != 0 || rows != 3 {
				t.Errorf("table: code %d, %d data rows, want 3:\n%s", table.Code, rows, table.Stdout)
			}
			if want := "Showing 3 of 5 " + tc.name; !strings.Contains(table.Stderr, want) {
				t.Errorf("table: stderr %q, want %q", table.Stderr, want)
			}
		})
	}
}

// TestSlosEvaluateLimitAbove25 pins that --evaluate pages in 25s (the API's
// limit for evaluate=true) until --limit is met, instead of stopping at 25.
func TestSlosEvaluateLimitAbove25(t *testing.T) {
	api := newFakeAPI(t, listPages(sloPath, "slo", 60, 25, sloItem))
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "slos", "--evaluate", "--limit", "50", "-o", "json")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if n := listLen(t, r.Stdout, "slo"); n != 50 {
		t.Errorf("%d SLOs, want 50", n)
	}
	if n := api.Hits(sloPath); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
	q := api.Queries(sloPath)
	if len(q) == 0 || q[0].Get("pageSize") != "25" || q[0].Get("evaluate") != "true" {
		t.Errorf("first request query = %v, want pageSize=25 and evaluate=true", q)
	}
}

// TestSlosEvaluateSummaryHumanOnly pins that the --evaluate status summary
// goes to stderr after the table, and stays out of json output.
func TestSlosEvaluateSummaryHumanOnly(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, listPages(sloPath, "slo", 3, 3, sloItem))})

	r := runCLI(t, cfg, "get", "slos", "--evaluate")
	if r.Code != 0 || !strings.Contains(r.Stdout, "SLO-2") || !strings.Contains(r.Stderr, "Status: 3 SUCCESS") {
		t.Errorf("table: code %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}

	r = runCLI(t, cfg, "get", "slos", "--evaluate", "-o", "json")
	if r.Code != 0 || strings.Contains(r.Stderr, "Status:") || listLen(t, r.Stdout, "slo") != 3 {
		t.Errorf("json: code %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}

	empty := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, listPages(sloPath, "slo", 0, 25, sloItem))})
	r = runCLI(t, empty, "get", "slos", "--evaluate")
	if r.Code != 0 || r.Stdout != "" || !strings.Contains(r.Stderr, "No SLOs found") || strings.Contains(r.Stderr, "Status:") {
		t.Errorf("empty: code %d, stdout %q, stderr %q", r.Code, r.Stdout, r.Stderr)
	}
}

func TestEntitiesWithoutSelectorIsUsageError(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "get", "entities")
	if r.Code != 2 || !strings.Contains(r.Stderr, "--selector is required") {
		t.Errorf("code %d, stderr %q", r.Code, r.Stderr)
	}
}
