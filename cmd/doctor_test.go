package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// The paths ClusterVersion tries, in order, and doctor's fallback.
const (
	clusterVersionPath = "GET /api/v1.0/onpremise/cluster"
	envVersionPath     = "GET /e/env1/api/v1/config/clusterversion"
	metricsPath        = "GET /e/env1/api/v2/metrics"
)

// doctorRowJSON decodes one doctor row.
type doctorRowJSON struct {
	Context, Host, EnvID, Status, Version, Detail string
}

// versionAPI is a fake cluster whose node list reports version v. The real
// /api/v1.0/onpremise/cluster answers with an array of nodes; these fakes
// used to serve an object, which no cluster returns.
func versionAPI(t *testing.T, v string) *fakeAPI {
	t.Helper()
	return newFakeAPI(t, map[string]route{clusterVersionPath: {Body: nodeList(v)}})
}

// nodeList is a node-list body with one node per version.
func nodeList(versions ...string) string {
	nodes := make([]string, len(versions))
	for i, v := range versions {
		nodes[i] = fmt.Sprintf(`{"id":%d,"buildVersion":%q,"operationState":"RUNNING"}`, i+1, v)
	}
	return "[" + strings.Join(nodes, ",") + "]"
}

// doctorRows runs doctor with args and decodes its JSON rows by context.
func doctorRows(t *testing.T, cfgPath string, args ...string) (map[string]doctorRowJSON, cliResult) {
	t.Helper()
	r := runCLI(t, cfgPath, append([]string{"doctor", "-o", "json"}, args...)...)
	var rows []doctorRowJSON
	if err := json.Unmarshal([]byte(r.Stdout), &rows); err != nil {
		t.Fatalf("stdout is not a JSON list: %v\n%s\nstderr: %s", err, r.Stdout, r.Stderr)
	}
	byName := map[string]doctorRowJSON{}
	for _, row := range rows {
		byName[row.Context] = row
	}
	return byName, r
}

// TestDoctorStatuses pins the three outcomes: a readable, recent version is
// ok; a token that cannot read the version but can read metrics is a warn
// (as the Managed MCP server assumes); a token that can read neither fails,
// and any failure makes doctor exit 1 without a second error report.
func TestDoctorStatuses(t *testing.T) {
	ok := versionAPI(t, "1.330.0.20251017-120000")
	warn := newFakeAPI(t, map[string]route{
		clusterVersionPath: {Status: 403, File: "error_403.json"},
		envVersionPath:     {Status: 403, File: "error_403.json"},
		metricsPath:        {Body: `{"totalCount":1,"metrics":[{"metricId":"builtin:host.cpu.usage"}]}`},
	})
	fail := newFakeAPI(t, map[string]route{
		clusterVersionPath: {Status: 401, File: "error_401.json"},
		envVersionPath:     {Status: 401, File: "error_401.json"},
		metricsPath:        {Status: 401, File: "error_401.json"},
	})
	cfg := withFakeEnvs(t, fakeEnv{"a", ok}, fakeEnv{"b", warn}, fakeEnv{"c", fail})
	rows, r := doctorRows(t, cfg)

	if r.Code != 1 {
		t.Errorf("code = %d, want 1", r.Code)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %v", rows)
	}
	if a := rows["a"]; a.Status != "ok" || a.Version != "1.330.0.20251017-120000" || a.Detail != "" {
		t.Errorf("a = %+v", a)
	}
	if b := rows["b"]; b.Status != "warn" || b.Version != "unknown" || !strings.Contains(b.Detail, "assumed") {
		t.Errorf("b = %+v", b)
	}
	if warn.Hits(metricsPath) != 1 {
		t.Errorf("metrics fallback hits = %d, want 1", warn.Hits(metricsPath))
	}
	if q := warn.Queries(metricsPath); len(q) == 1 && q[0].Get("pageSize") != "1" {
		t.Errorf("metrics fallback query = %v", q[0])
	}
	if c := rows["c"]; c.Status != "fail" || !strings.Contains(c.Detail, "401") {
		t.Errorf("c = %+v", c)
	}
	if rows["a"].Host != "http://FAKE" || rows["a"].EnvID != "env1" {
		t.Errorf("a host/env = %q %q", rows["a"].Host, rows["a"].EnvID)
	}
	if strings.Contains(r.Stderr, "already reported") || strings.Contains(r.Stderr, "Error") {
		t.Errorf("failure reported again on stderr: %q", r.Stderr)
	}
}

// TestDoctorOldVersionWarns pins the version floor of the Managed MCP server.
func TestDoctorOldVersionWarns(t *testing.T) {
	rows, r := doctorRows(t, withFakeEnvs(t, fakeEnv{"a", versionAPI(t, "1.320.0.20240101-000000")}))
	if r.Code != 0 {
		t.Errorf("code = %d, want 0 (a warning is not a failure); stderr %q", r.Code, r.Stderr)
	}
	if a := rows["a"]; a.Status != "warn" || !strings.Contains(a.Detail, "1.328.0") || a.Version != "1.320.0.20240101-000000" {
		t.Errorf("a = %+v", a)
	}
}

// TestDoctorOnlyNamedContext pins that -c checks just that context.
func TestDoctorOnlyNamedContext(t *testing.T) {
	a, b := versionAPI(t, "1.330.0.20251017-120000"), versionAPI(t, "1.330.0.20251017-120000")
	rows, r := doctorRows(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "-c", "b")
	if r.Code != 0 || len(rows) != 1 || rows["b"].Status != "ok" {
		t.Errorf("code %d, rows %v, stderr %q", r.Code, rows, r.Stderr)
	}
	if a.Hits(clusterVersionPath) != 0 {
		t.Error("doctor -c b also checked a")
	}
}

// TestDoctorAgentEnvelope pins agent output: one envelope whose result is the
// rows, and ok:false with exit 1 when a context fails.
func TestDoctorAgentEnvelope(t *testing.T) {
	ok := versionAPI(t, "1.330.0.20251017-120000")
	fail := newFakeAPI(t, map[string]route{
		clusterVersionPath: {Status: 401, File: "error_401.json"},
		envVersionPath:     {Status: 401, File: "error_401.json"},
		metricsPath:        {Status: 401, File: "error_401.json"},
	})
	for _, tc := range []struct {
		name   string
		envs   []fakeEnv
		wantOK bool
		code   int
	}{
		{"all ok", []fakeEnv{{"a", ok}}, true, 0},
		{"one fails", []fakeEnv{{"a", ok}, {"c", fail}}, false, 1},
	} {
		r := runCLI(t, withFakeEnvs(t, tc.envs...), "doctor", "-A")
		var env struct {
			OK      bool            `json:"ok"`
			Result  []doctorRowJSON `json:"result"`
			Context struct {
				Verb, Resource string
			} `json:"context"`
		}
		if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
			t.Fatalf("%s: stdout is not one envelope: %v\n%s", tc.name, err, r.Stdout)
		}
		if env.OK != tc.wantOK || r.Code != tc.code || len(env.Result) != len(tc.envs) {
			t.Errorf("%s: ok %v code %d rows %d\n%s", tc.name, env.OK, r.Code, len(env.Result), r.Stdout)
		}
		if env.Context.Verb != "doctor" || env.Context.Resource != "contexts" {
			t.Errorf("%s: context = %+v", tc.name, env.Context)
		}
	}
}

// TestDoctorTable pins the human table's columns.
func TestDoctorTable(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", versionAPI(t, "1.330.0.20251017-120000")}), "doctor")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	for _, h := range []string{"CONTEXT", "HOST", "ENV-ID", "STATUS", "VERSION", "DETAIL"} {
		if !strings.Contains(r.Stdout, h) {
			t.Errorf("table has no %s column:\n%s", h, r.Stdout)
		}
	}
}

// TestJudgeVersion pins the version floor comparison on the first three
// numeric parts.
func TestJudgeVersion(t *testing.T) {
	for v, want := range map[string]string{
		"1.328.0.20240101-000000":  doctorOK,
		"1.330.18.20251017-123456": doctorOK,
		"2.0.0":                    doctorOK,
		"1.327.99.20240101-000000": doctorWarn,
		"1.9.0":                    doctorWarn,
		"":                         doctorWarn,
		"1.330":                    doctorWarn,
		"v1.330.0":                 doctorWarn,
	} {
		if got, _ := judgeVersion(v); got != want {
			t.Errorf("judgeVersion(%q) = %s, want %s", v, got, want)
		}
	}
}

// TestDoctorMissingTokenFails pins that a context whose token is not stored
// fails with a detail that says how to store it.
func TestDoctorMissingTokenFails(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", versionAPI(t, "1.330.0.20251017-120000")})
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(strings.Replace(string(data), "token-ref: tok-a", "token-ref: missing", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	rows, r := doctorRows(t, cfg)
	a := rows["a"]
	if r.Code != 1 || a.Status != "fail" || !strings.Contains(a.Detail, "no token") || !strings.Contains(a.Detail, "dtmgd config set-credentials") {
		t.Errorf("code %d, a = %+v", r.Code, a)
	}
}

// TestDoctorChecksContextsConcurrently pins that doctor checks contexts in
// parallel, so a config of hung hosts takes one timeout rather than one per
// context. The version endpoint answers only once both contexts' requests
// have arrived; checked one after the other, the first gives up at the
// barrier and fails.
func TestDoctorChecksContextsConcurrently(t *testing.T) {
	var mu sync.Mutex
	arrived := 0
	both := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if "GET "+r.URL.Path != clusterVersionPath {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":404,"message":"not found"}}`)
			return
		}
		mu.Lock()
		arrived++
		if arrived == 2 {
			close(both)
		}
		mu.Unlock()
		select {
		case <-both:
			_, _ = io.WriteString(w, nodeList("1.330.0.20251017-120000"))
		case <-time.After(3 * time.Second):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"the other context never arrived"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	api := &fakeAPI{URL: srv.URL}

	rows, r := doctorRows(t, withFakeEnvs(t, fakeEnv{"a", api}, fakeEnv{"b", api}))
	if r.Code != 0 {
		t.Errorf("code %d, stderr %q", r.Code, r.Stderr)
	}
	for _, name := range []string{"a", "b"} {
		if rows[name].Status != "ok" {
			t.Errorf("%s: %+v, want ok", name, rows[name])
		}
	}
	if !strings.Contains(r.Stdout, `"context": "a"`) || strings.Index(r.Stdout, `"context": "a"`) > strings.Index(r.Stdout, `"context": "b"`) {
		t.Errorf("rows not in config order:\n%s", r.Stdout)
	}
}

// TestDoctorDeclaresScopes pins the token scopes doctor needs: DataExport for
// the cluster version, and metrics.read for the fallback check. They feed the
// 403 hint, the commands catalog and the README's scope list.
func TestDoctorDeclaresScopes(t *testing.T) {
	doctor, _, err := testRoot().Find([]string{"doctor"})
	if err != nil {
		t.Fatal(err)
	}
	if got := requiredScopes(doctor); !reflect.DeepEqual(got, []string{"DataExport", "metrics.read"}) {
		t.Errorf("doctor scopes = %q, want [DataExport metrics.read]", got)
	}
}
