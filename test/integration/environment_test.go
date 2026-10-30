//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// env is the environment context under test; the test skips without one.
func env(t *testing.T) string {
	t.Helper()
	return requireEnv(t, "DTMGD_IT_ENV")
}

// from is the start of the time range for events, problems and logs.
func from() string { return envOr("DTMGD_IT_FROM", "now-24h") }

// TestDoctorReportsTheContextUsable reads doctor's row whatever its exit
// code: doctor exits 1 for a failing context, and the row says why.
func TestDoctorReportsTheContextUsable(t *testing.T) {
	r := run(t, "doctor", "--context", env(t), "-o", "json")
	var rows []map[string]any
	if err := json.Unmarshal(r.Out, &rows); err != nil || len(rows) != 1 {
		t.Fatalf("doctor did not return one row (%s)", brief(r))
	}
	if s := rows[0]["status"]; s != "ok" && s != "warn" {
		t.Errorf("doctor status = %v, want ok or warn (detail is not printed here; run dtmgd doctor)", s)
	}
}

func TestListsReturnTheirItems(t *testing.T) {
	ctx := env(t)
	for _, c := range []struct {
		args []string
		key  string
	}{
		{[]string{"get", "problems", "--from", from()}, "problems"},
		{[]string{"get", "entities", "--selector", "type(HOST)"}, "entities"},
		{[]string{"get", "events", "--from", from()}, "events"},
		{[]string{"get", "metrics"}, "metrics"},
		{[]string{"get", "slos"}, "slo"},
		{[]string{"get", "security-problems"}, "securityProblems"},
		{[]string{"get", "entity-types"}, "types"},
	} {
		t.Run(c.args[1], func(t *testing.T) {
			args := append(c.args, "--context", ctx, "-o", "json", "--limit", "5")
			body := decode[map[string]any](t, args...)
			if n := len(items(t, body, c.key)); n > 5 {
				t.Errorf("--limit 5 returned %d %s", n, c.key)
			}
		})
	}
}

// TestLimitKeepsTheNextPageKey pins that a truncated result says so: the
// API's nextPageKey survives --limit.
func TestLimitKeepsTheNextPageKey(t *testing.T) {
	body := decode[map[string]any](t, "get", "metrics", "--context", env(t), "-o", "json", "--limit", "1")
	if n := len(items(t, body, "metrics")); n != 1 {
		t.Fatalf("--limit 1 returned %d metrics", n)
	}
	if total(t, body) > 1 {
		if key, _ := body["nextPageKey"].(string); key == "" {
			t.Error("a truncated result has no nextPageKey")
		}
	}
}

// TestEventTypeFilterIsHonoured re-checks what v0.4.0 fixed: --type must
// filter on the cluster (it used to send a parameter the cluster ignored).
func TestEventTypeFilterIsHonoured(t *testing.T) {
	ctx := env(t)
	all := decode[map[string]any](t, "get", "events", "--context", ctx, "--from", from(), "-o", "json", "--limit", "1")
	allTotal := total(t, all)
	if allTotal == 0 {
		t.Skipf("no events since %s, so the filter cannot be checked", from())
	}
	typ, _ := items(t, all, "events")[0].(map[string]any)["eventType"].(string)

	filtered := decode[map[string]any](t, "get", "events", "--context", ctx, "--from", from(), "--type", typ, "-o", "json", "--limit", "50")
	if n := total(t, filtered); n > allTotal || n == 0 {
		t.Errorf("--type %s matched %d of %d events", typ, n, allTotal)
	}
	for _, e := range items(t, filtered, "events") {
		if got := e.(map[string]any)["eventType"]; got != typ {
			t.Fatalf("--type %s returned an event of type %v", typ, got)
		}
	}

	r := run(t, "get", "events", "--context", ctx, "--from", from(), "--type", "ZZ_NOT_A_TYPE", "-o", "json")
	if r.Code == 0 || !strings.Contains(r.Err, "400") {
		t.Errorf("an unknown --type was not rejected by the cluster (%s)", brief(r))
	}
}

// TestZoneFilterIsHonoured re-checks the other v0.4.0 fix: --mz must reach
// the entity selector. A zone that cannot exist is a 400; a real one, named
// in DTMGD_IT_MZ, narrows the result.
func TestZoneFilterIsHonoured(t *testing.T) {
	ctx := env(t)
	noZone := fmt.Sprintf("zz-no-such-zone-%d", time.Now().UnixNano())
	args := []string{"get", "entities", "--context", ctx, "--selector", "type(HOST)", "--mz", noZone, "-o", "json"}
	r := run(t, args...)
	skipWithoutScope(t, r, args)
	if r.Code == 0 || !strings.Contains(r.Err, "400") {
		t.Errorf("a nonexistent --mz was not rejected by the cluster (%s)", brief(r))
	}

	zone := envOr("DTMGD_IT_MZ", "")
	if zone == "" {
		t.Skip("DTMGD_IT_MZ is not set, so the narrowing is not checked")
	}
	all := total(t, decode[map[string]any](t, "get", "entities", "--context", ctx, "--selector", "type(HOST)", "-o", "json", "--limit", "1"))
	inZone := total(t, decode[map[string]any](t, "get", "entities", "--context", ctx, "--selector", "type(HOST)", "--mz", zone, "-o", "json", "--limit", "1"))
	narrowed(t, "hosts in DTMGD_IT_MZ", inZone, all)
}

// TestLogsEntityFilterIsHonoured pins that /logs/search honours its
// undocumented entitySelector: an entity that cannot exist matches nothing.
func TestLogsEntityFilterIsHonoured(t *testing.T) {
	ctx := env(t)
	q := envOr("DTMGD_IT_LOG_QUERY", "error")
	all := decode[map[string]any](t, "query", "logs", "--context", ctx, "--query", q, "--from", from(), "-o", "json", "--limit", "10")
	if len(items(t, all, "results")) == 0 {
		t.Skipf("no log records match %q since %s; set DTMGD_IT_LOG_QUERY", q, from())
	}
	none := decode[map[string]any](t, "query", "logs", "--context", ctx, "--query", q, "--from", from(),
		"--entity", `entityId("HOST-0000000000000000")`, "-o", "json", "--limit", "10")
	if n := len(items(t, none, "results")); n != 0 {
		t.Errorf("an impossible --entity matched %d records: the filter is ignored", n)
	}
}

// TestProblemsStatusFilter needs problems that are not open in the window:
// if every problem is open, the filter cannot be told from no filter.
func TestProblemsStatusFilter(t *testing.T) {
	ctx := env(t)
	all := decode[map[string]any](t, "get", "problems", "--context", ctx, "--from", from(), "-o", "json", "--limit", "1")
	open := decode[map[string]any](t, "get", "problems", "--context", ctx, "--status", "OPEN", "--from", from(), "-o", "json", "--limit", "50")
	narrowed(t, "open problems", total(t, open), total(t, all))
	for _, p := range items(t, open, "problems") {
		if s := p.(map[string]any)["status"]; s != "OPEN" {
			t.Fatalf("--status OPEN returned a problem with status %v", s)
		}
	}
}

func TestQueryMetrics(t *testing.T) {
	body := decode[map[string]any](t, "query", "metrics", "--context", env(t), "--metric", "builtin:host.cpu.usage", "--from", "now-1h", "-o", "json")
	if _, ok := body["result"].([]any); !ok {
		t.Error("query metrics returned no result list")
	}
}

// TestMultiEnvironmentFanOut runs one command against two environment
// contexts at once, as --env "a;b" does, when DTMGD_IT_ENV2 names a second.
func TestMultiEnvironmentFanOut(t *testing.T) {
	ctx := env(t)
	ctx2 := requireEnv(t, "DTMGD_IT_ENV2")
	byContext := decode[map[string]map[string]any](t, "get", "problems", "--env", ctx+";"+ctx2, "--from", from(), "-o", "json", "--limit", "5")
	for _, c := range []string{ctx, ctx2} {
		if _, ok := byContext[c]["problems"].([]any); !ok {
			t.Errorf("fan-out has no problems list for one of the two contexts")
		}
	}
}

// TestMetricsEntityIsRefused needs no network: get metrics cannot list an
// entity's metrics, so --entity is a usage error pointing at query metrics.
func TestMetricsEntityIsRefused(t *testing.T) {
	r := run(t, "get", "metrics", "--context", env(t), "--entity", "HOST-0000000000000000")
	if r.Code != 2 {
		t.Errorf("get metrics --entity: %s, want exit 2", brief(r))
	}
}
