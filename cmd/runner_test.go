package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"gopkg.in/yaml.v3"
)

// problemPages builds routes serving total problems in pages of per, linked
// by nextPageKey the way the API pages.
func problemPages(total, per int) map[string]route {
	routes := map[string]route{}
	for page, start := 1, 0; start < total || page == 1; page, start = page+1, start+per {
		var items []string
		for i := start; i < start+per && i < total; i++ {
			items = append(items, fmt.Sprintf(`{"problemId":"P%d","displayId":"P-%d","title":"Problem %d","status":"OPEN"}`, i, i, i))
		}
		body := fmt.Sprintf(`{"totalCount":%d,"pageSize":%d,"problems":[%s]`, total, per, strings.Join(items, ","))
		if start+per < total {
			body += fmt.Sprintf(`,"nextPageKey":"PAGE%d"`, page+1)
		}
		body += "}"
		key := problemsPath
		if page > 1 {
			key += fmt.Sprintf(" nextPageKey=PAGE%d", page)
		}
		routes[key] = route{Body: body}
	}
	return routes
}

// closedPortAPI is a fake environment nothing listens on.
func closedPortAPI(t *testing.T) *fakeAPI {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return &fakeAPI{URL: "http://" + addr}
}

func problemCount(t *testing.T, body interface{}) int {
	t.Helper()
	m, ok := body.(map[string]interface{})
	if !ok {
		t.Fatalf("result is %T, want an object", body)
	}
	list, ok := m["problems"].([]interface{})
	if !ok {
		t.Fatalf("result has no problems list: %v", m)
	}
	return len(list)
}

// TestLimitHonouredInEveryFormat pins that --limit applies to every output
// format, not only the table, and that paging stops once it is met.
func TestLimitHonouredInEveryFormat(t *testing.T) {
	for _, format := range []string{"table", "json", "yaml", "agent"} {
		api := newFakeAPI(t, pagedProblems())
		args := []string{"get", "problems", "--limit", "12"}
		if format == "agent" {
			args = append(args, "-A")
		} else {
			args = append(args, "-o", format)
		}
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
		if r.Code != 0 {
			t.Fatalf("%s: code %d, stderr %q", format, r.Code, r.Stderr)
		}
		switch format {
		case "json":
			var body interface{}
			if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil {
				t.Fatal(err)
			}
			if n := problemCount(t, body); n != 12 {
				t.Errorf("json: %d problems, want 12", n)
			}
		case "yaml":
			var body map[string]interface{}
			if err := yaml.Unmarshal([]byte(r.Stdout), &body); err != nil {
				t.Fatal(err)
			}
			if n := len(body["problems"].([]interface{})); n != 12 {
				t.Errorf("yaml: %d problems, want 12", n)
			}
		case "agent":
			var resp agentmode.Response
			if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
				t.Fatal(err)
			}
			if n := problemCount(t, resp.Result); n != 12 {
				t.Errorf("agent: %d problems, want 12", n)
			}
		case "table":
			rows := strings.Count(strings.TrimSpace(r.Stdout), "\n") // header + rows - 1
			if rows != 12 {
				t.Errorf("table: %d data rows, want 12:\n%s", rows, r.Stdout)
			}
			if !strings.Contains(r.Stderr, "Showing 12 of 15 problems") {
				t.Errorf("table: stderr = %q", r.Stderr)
			}
		}
		if n := api.Hits(problemsPath); n != 2 {
			t.Errorf("%s: %d page requests, want 2", format, n)
		}
	}
}

// TestAgentDefaultLimit pins that agent mode caps lists at 50 when no
// --limit is given, and says the result is partial.
func TestAgentDefaultLimit(t *testing.T) {
	api := newFakeAPI(t, problemPages(60, 10))
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-A")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("%v\n%s", err, r.Stdout)
	}
	if n := problemCount(t, resp.Result); n != 50 {
		t.Errorf("%d problems, want 50", n)
	}
	if resp.Context == nil || !resp.Context.HasMore || resp.Context.Total == nil || *resp.Context.Total != 60 {
		t.Errorf("context = %+v; want has_more and total 60", resp.Context)
	}
}

// TestEmptyResultJSONAndAgent pins that an empty result still prints the API
// object with an empty list (it used to print nothing in json mode).
func TestEmptyResultJSONAndAgent(t *testing.T) {
	empty := map[string]route{problemsPath: {File: "problems_empty.json"}}
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, empty)})

	r := runCLI(t, cfg, "get", "problems", "-o", "json")
	var body interface{}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil || problemCount(t, body) != 0 || r.Code != 0 {
		t.Errorf("json: stdout %q, code %d", r.Stdout, r.Code)
	}

	r = runCLI(t, cfg, "get", "problems", "-A")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil || !resp.OK || problemCount(t, resp.Result) != 0 {
		t.Errorf("agent: stdout %q", r.Stdout)
	}

	r = runCLI(t, cfg, "get", "problems")
	if r.Stdout != "" || !strings.Contains(r.Stderr, "No problems found") {
		t.Errorf("table: stdout %q, stderr %q", r.Stdout, r.Stderr)
	}
}

func TestMultiEnvTableSections(t *testing.T) {
	small := map[string]route{problemsPath: {File: "problems_small.json"}}
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, small)}, fakeEnv{"b", newFakeAPI(t, small)}), "get", "problems", "--env", "a;b")
	if !strings.Contains(r.Stdout, "== a ==") || !strings.Contains(r.Stdout, "== b ==") || strings.Count(r.Stdout, "PROBLEM-ID") != 2 {
		t.Errorf("stdout:\n%s", r.Stdout)
	}
}

// TestMultiEnvOneEnvUnreachable pins that one dead environment neither hides
// the others' results nor stalls the call, and still fails it.
func TestMultiEnvOneEnvUnreachable(t *testing.T) {
	small := map[string]route{problemsPath: {File: "problems_small.json"}}
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, small)}, fakeEnv{"b", closedPortAPI(t)})
	start := time.Now()
	r := runCLI(t, cfg, "get", "problems", "--env", "a;b", "-o", "json")
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s, want under 5s", d)
	}
	var out map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(r.Stdout), &out); err != nil {
		t.Fatalf("%v\n%s", err, r.Stdout)
	}
	if problemCount(t, out["a"]) != 2 || out["b"]["error"] == nil || r.Code != 1 {
		t.Errorf("stdout %s, code %d", r.Stdout, r.Code)
	}
}

func TestWatchFlagOnlyOnWatchableCommands(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "get", "slos", "-w")
	if r.Code != 2 || !strings.Contains(r.Stderr, "unknown shorthand flag") {
		t.Errorf("code %d, stderr %q", r.Code, r.Stderr)
	}
}

// TestMultiEnvAgentTruncationVisible pins that a multi-environment agent
// result says when an environment's list was cut short: the merged envelope
// used to drop each environment's has_more, so an agent took 50 of 60 for
// the whole story.
func TestMultiEnvAgentTruncationVisible(t *testing.T) {
	cfg := withFakeEnvs(t,
		fakeEnv{"a", newFakeAPI(t, problemPages(60, 10))},
		fakeEnv{"b", newFakeAPI(t, problemPages(60, 10))})
	r := runCLI(t, cfg, "get", "problems", "--env", "a;b", "-A")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("%v\n%s", err, r.Stdout)
	}
	if resp.Context == nil || !resp.Context.HasMore {
		t.Fatalf("context = %+v; want has_more", resp.Context)
	}
	warnings := strings.Join(resp.Context.Warnings, "\n")
	for _, env := range []string{"a", "b"} {
		if !strings.Contains(warnings, env+": Showing 50 of 60 problems") {
			t.Errorf("warnings %q do not say %s was truncated", resp.Context.Warnings, env)
		}
	}
}

// multiEnvPartialConfig is four environments for partial-failure tests: a
// has no problems, b denies the request, c holds 1 of 5 (one page, more
// behind nextPageKey) and d holds 1 of 1.
func multiEnvPartialConfig(t *testing.T) string {
	t.Helper()
	body := func(b string) map[string]route { return map[string]route{problemsPath: {Body: b}} }
	return withFakeEnvs(t,
		fakeEnv{"a", newFakeAPI(t, map[string]route{problemsPath: {File: "problems_empty.json"}})},
		fakeEnv{"b", newFakeAPI(t, map[string]route{problemsPath: {Status: 403, File: "error_403.json"}})},
		fakeEnv{"c", newFakeAPI(t, body(`{"totalCount":5,"problems":[{"problemId":"P1"}],"nextPageKey":"K"}`))},
		fakeEnv{"d", newFakeAPI(t, body(`{"totalCount":1,"problems":[{"problemId":"P1","displayId":"P-1","title":"t","status":"OPEN"}]}`))},
	)
}

// TestMultiEnvAgentPartialFailure pins the merged agent envelope when one
// environment fails: ok=false with a partial_failure error, every result
// keyed by context, warnings naming the failure and the truncated list in
// request order, and a UI link for each environment that answered. Nothing
// goes to stderr, and the exit code is 1.
func TestMultiEnvAgentPartialFailure(t *testing.T) {
	r := runCLI(t, multiEnvPartialConfig(t), "get", "problems", "--env", "a;b;c", "-A", "--limit", "1")
	want := "== exit code: 1\n" +
		"== stdout\n" +
		"{\n" +
		"  \"ok\": false,\n" +
		"  \"result\": {\n" +
		"    \"a\": {\n" +
		"      \"pageSize\": 10,\n" +
		"      \"problems\": [],\n" +
		"      \"totalCount\": 0\n" +
		"    },\n" +
		"    \"b\": {\n" +
		"      \"error\": \"API error 403: Token is missing required scope\"\n" +
		"    },\n" +
		"    \"c\": {\n" +
		"      \"nextPageKey\": \"K\",\n" +
		"      \"problems\": [\n" +
		"        {\n" +
		"          \"problemId\": \"P1\"\n" +
		"        }\n" +
		"      ],\n" +
		"      \"totalCount\": 5\n" +
		"    }\n" +
		"  },\n" +
		"  \"error\": {\n" +
		"    \"code\": \"partial_failure\",\n" +
		"    \"message\": \"1 of 3 environments failed: b: API error 403: Token is missing required scope\"\n" +
		"  },\n" +
		"  \"context\": {\n" +
		"    \"has_more\": true,\n" +
		"    \"verb\": \"get\",\n" +
		"    \"resource\": \"problems\",\n" +
		"    \"warnings\": [\n" +
		"      \"b: API error 403: Token is missing required scope\",\n" +
		"      \"c: Showing 1 of 5 problems\"\n" +
		"    ],\n" +
		"    \"links\": {\n" +
		"      \"a\": \"http://FAKE/e/env1/ui/problems\",\n" +
		"      \"c\": \"http://FAKE/e/env1/ui/problems\"\n" +
		"    }\n" +
		"  }\n" +
		"}\n" +
		"== stderr\n"
	if got := r.String(); got != want {
		t.Errorf("--- want\n%s\n--- got\n%s", want, got)
	}
}

// TestMultiEnvTablePartialFailure pins table output when one environment
// fails: a section per environment in request order, separated by blank
// lines; the failure reported on stderr under its context name (and not
// again as a summary); a truncation note for a cut-short list; exit code 1.
func TestMultiEnvTablePartialFailure(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"failure last", []string{"--env", "d;b"}, "== exit code: 1\n" +
			"== stdout\n" +
			"== d ==\n" +
			" PROBLEM-ID  DISPLAY-ID  TITLE  STATUS  SEVERITY  IMPACT \n" +
			" P1          P-1         t      OPEN                     \n" +
			"\n" +
			"== b ==\n" +
			"== stderr\n" +
			"Error: b: API error 403: Token is missing required scope\n"},
		{"failure first, then a truncated list", []string{"--env", "b;d;c", "--limit", "1"}, "== exit code: 1\n" +
			"== stdout\n" +
			"== b ==\n" +
			"\n" +
			"== d ==\n" +
			" PROBLEM-ID  DISPLAY-ID  TITLE  STATUS  SEVERITY  IMPACT \n" +
			" P1          P-1         t      OPEN                     \n" +
			"\n" +
			"== c ==\n" +
			" PROBLEM-ID  DISPLAY-ID  TITLE  STATUS  SEVERITY  IMPACT \n" +
			" P1                                                      \n" +
			"== stderr\n" +
			"Error: b: API error 403: Token is missing required scope\n" +
			"Showing 1 of 5 problems. Use --limit or narrower filters.\n"},
	}
	for _, tc := range cases {
		r := runCLI(t, multiEnvPartialConfig(t), append([]string{"get", "problems"}, tc.args...)...)
		if got := r.String(); got != tc.want {
			t.Errorf("%s:\n--- want\n%s\n--- got\n%s", tc.name, tc.want, got)
		}
	}
}
