package cmd

import (
	"net/url"
	"strings"
	"testing"
)

// TestFormatEvaluatedPct verifies the SLO evaluated-percentage formatter.
// Dynatrace returns -1 when it cannot evaluate an SLO; we show "N/A" instead
// of the confusing "-1.00%" that was shown before the fix.
func TestFormatEvaluatedPct(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		{"DT sentinel -1", -1, "N/A"},
		{"any negative", -0.01, "N/A"},
		{"very negative", -999.9, "N/A"},
		{"zero is valid", 0.0, "0.00%"},
		{"small positive", 0.01, "0.01%"},
		{"typical green SLO", 99.95, "99.95%"},
		{"exactly 100", 100.0, "100.00%"},
		{"just below target", 89.99, "89.99%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEvaluatedPct(tt.input)
			if got != tt.want {
				t.Errorf("formatEvaluatedPct(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// slosQuery runs get slos with args against a fresh fake API and returns the
// one query it sent.
func slosQuery(t *testing.T, args ...string) url.Values {
	t.Helper()
	api := newFakeAPI(t, map[string]route{sloPath: {Body: `{"totalCount":0,"slo":[]}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append([]string{"get", "slos", "-o", "json"}, args...)...)
	qs := api.Queries(sloPath)
	if r.Code != 0 || len(qs) != 1 {
		t.Fatalf("get slos %v: %s (%d requests)", args, r, len(qs))
	}
	return qs[0]
}

// TestSlosTimeframeFlags pins --timeframe, --from, --to and --sort on get
// slos: dates without --timeframe imply GTF, as on describe slo, and an
// explicit --timeframe is kept.
func TestSlosTimeframeFlags(t *testing.T) {
	if q := slosQuery(t, "--from", "now-7d"); q.Get("timeFrame") != "GTF" || q.Get("from") != "now-7d" || q.Has("to") {
		t.Errorf("--from only: %v", q)
	}
	if q := slosQuery(t, "--to", "now-1d"); q.Get("timeFrame") != "GTF" || q.Get("to") != "now-1d" {
		t.Errorf("--to only: %v", q)
	}
	if q := slosQuery(t, "--timeframe", "current", "--from", "now-7d"); q.Get("timeFrame") != "CURRENT" || q.Get("from") != "now-7d" {
		t.Errorf("--timeframe current --from: %v", q)
	}
	if q := slosQuery(t, "--sort", "-NAME", "--enabled", "ALL"); q.Get("sort") != "-name" || q.Get("enabledSlos") != "all" {
		t.Errorf("--sort -NAME --enabled ALL: %v", q)
	}
	if q := slosQuery(t); q.Has("timeFrame") || q.Has("from") || q.Has("to") || q.Has("sort") {
		t.Errorf("no flags: %v", q)
	}
}

// TestSlosBooleanFlagsSentOnlyWhenSet pins that --show-global and --demo are
// sent exactly when given, false included (the MCP server drops a false
// value), and not at all otherwise.
func TestSlosBooleanFlagsSentOnlyWhenSet(t *testing.T) {
	if q := slosQuery(t, "--show-global=false", "--demo=false"); q.Get("showGlobalSlos") != "false" || q.Get("demo") != "false" {
		t.Errorf("=false: %v", q)
	}
	if q := slosQuery(t, "--show-global", "--demo"); q.Get("showGlobalSlos") != "true" || q.Get("demo") != "true" {
		t.Errorf("bare flags: %v", q)
	}
	if q := slosQuery(t); q.Has("showGlobalSlos") || q.Has("demo") {
		t.Errorf("no flags: %v", q)
	}
}

// TestSlosInvalidEnums pins the usage errors for the get slos enum flags.
func TestSlosInvalidEnums(t *testing.T) {
	api := newFakeAPI(t, map[string]route{sloPath: {Body: `{"totalCount":0,"slo":[]}`}})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	for args, want := range map[string]string{
		"--timeframe=LAST":  `invalid --timeframe "LAST" (want one of: CURRENT, GTF)`,
		"--sort=status":     `invalid --sort "status" (want one of: name, -name)`,
		"--enabled=enabled": `invalid --enabled "enabled" (want one of: true, false, all)`,
	} {
		if r := runCLI(t, cfg, "get", "slos", args); r.Code != 2 || !strings.Contains(r.Stderr, want) {
			t.Errorf("%s: want exit 2 and %q, got %s", args, want, r)
		}
	}
	if n := api.Hits(sloPath); n != 0 {
		t.Errorf("an invalid flag still reached the API %d times", n)
	}
}
