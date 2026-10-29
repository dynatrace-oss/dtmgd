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
