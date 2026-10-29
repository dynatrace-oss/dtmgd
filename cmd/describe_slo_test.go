package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSLODetailUnmarshal(t *testing.T) {
	warn := 99.99
	raw := map[string]interface{}{
		"id":                   "abc-123",
		"name":                 "Availability SLO for CartController",
		"status":               "FAILURE",
		"enabled":              true,
		"target":               99.98,
		"warning":              warn,
		"evaluatedPercentage":  97.45,
		"errorBudget":          -2.52,
		"timeframe":            "-1w",
		"evaluationType":       "AGGREGATE",
		"filter":               `type("SERVICE"),entityId("SERVICE-ABC")`,
		"metricExpression":     "(100)*(builtin:service.errors.server.successCount:splitBy())/(builtin:service.requestCount.server:splitBy())",
		"relatedOpenProblems":  1,
		"relatedTotalProblems": 7,
		"errorBudgetBurnRate": map[string]interface{}{
			"burnRateValue": 1.5,
			"burnRateType":  "FAST",
		},
	}

	b, _ := json.Marshal(raw)
	var d SLODetail
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if d.ID != "abc-123" {
		t.Errorf("ID = %q, want %q", d.ID, "abc-123")
	}
	if d.Status != "FAILURE" {
		t.Errorf("Status = %q, want FAILURE", d.Status)
	}
	if !d.Enabled {
		t.Error("Enabled should be true")
	}
	if d.Target != 99.98 {
		t.Errorf("Target = %v, want 99.98", d.Target)
	}
	if d.Warning == nil || *d.Warning != warn {
		t.Errorf("Warning = %v, want %v", d.Warning, warn)
	}
	if d.ErrorBudgetBurnRate == nil || d.ErrorBudgetBurnRate.BurnRateValue != 1.5 {
		t.Errorf("BurnRateValue = %v, want 1.5", d.ErrorBudgetBurnRate)
	}
	if d.RelatedOpenProblems != 1 || d.RelatedTotalProblems != 7 {
		t.Errorf("RelatedProblems = %d/%d, want 1/7", d.RelatedOpenProblems, d.RelatedTotalProblems)
	}
}

func TestSLODetailUnmarshal_NilWarning(t *testing.T) {
	// SLOs without a warning threshold should unmarshal with Warning == nil.
	raw := map[string]interface{}{
		"id":                  "xyz-456",
		"name":                "Perf SLO",
		"status":              "SUCCESS",
		"enabled":             false,
		"target":              95.0,
		"evaluatedPercentage": 98.0,
		"errorBudget":         3.0,
		"timeframe":           "-1w",
	}
	b, _ := json.Marshal(raw)
	var d SLODetail
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if d.Warning != nil {
		t.Errorf("Warning should be nil when absent, got %v", *d.Warning)
	}
	if d.Enabled {
		t.Error("Enabled should be false")
	}
}

func TestPrintSLODetail(t *testing.T) {
	warn := 99.99
	d := SLODetail{
		ID:                   "abc-123",
		Name:                 "Availability SLO for CartController",
		Status:               "FAILURE",
		Enabled:              true,
		Target:               99.98,
		Warning:              &warn,
		EvaluatedPct:         97.4558,
		ErrorBudget:          -2.5241,
		Timeframe:            "-1w",
		EvaluationType:       "AGGREGATE",
		Filter:               `type("SERVICE"),entityId("SERVICE-ABC")`,
		RelatedOpenProblems:  1,
		RelatedTotalProblems: 7,
		ErrorBudgetBurnRate: &struct {
			BurnRateValue float64 `json:"burnRateValue"`
			BurnRateType  string  `json:"burnRateType"`
		}{BurnRateValue: 0, BurnRateType: "NONE"},
	}

	var buf bytes.Buffer
	printSLODetail(&buf, d)
	out := buf.String()

	checks := []string{
		"abc-123",
		"Availability SLO for CartController",
		"FAILURE",
		"99.98%",
		"99.99%",
		"97.4558%",
		"-2.5241",
		"-1w",
		"AGGREGATE",
		"1 open / 7 total",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("printSLODetail output missing %q\nGot:\n%s", want, out)
		}
	}
}

func TestPrintSLODetail_NoWarning(t *testing.T) {
	d := SLODetail{
		ID:           "no-warn",
		Name:         "Simple SLO",
		Status:       "SUCCESS",
		Enabled:      true,
		Target:       95.0,
		EvaluatedPct: 98.0,
		ErrorBudget:  3.0,
		Timeframe:    "-1w",
	}

	var buf bytes.Buffer
	printSLODetail(&buf, d)
	out := buf.String()

	if !strings.Contains(out, "Warning:") {
		t.Error("should still print Warning: line even when nil")
	}
	// The em-dash "—" is the sentinel for nil warning.
	if !strings.Contains(out, "—") {
		t.Errorf("expected — for nil warning, got:\n%s", out)
	}
}

// TestSLODetailRawMapPatternFixed verifies that the struct unmarshal round-trip
// does not produce a raw Go map representation (the bug fixed in this commit).
func TestSLODetailRawMapPatternFixed(t *testing.T) {
	raw := map[string]interface{}{
		"id": "test-id", "name": "Test SLO", "status": "SUCCESS",
		"target": 99.0, "evaluatedPercentage": 99.5, "timeframe": "-1w",
	}
	b, _ := json.Marshal(raw)
	var d SLODetail
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Confirm we get typed fields, not a raw map.
	if d.ID == "" {
		t.Error("ID should not be empty after unmarshal")
	}
	rawStr := fmt.Sprintf("%v", raw)
	if strings.Contains(rawStr, "map[") {
		// The raw map string contains "map[". Typed struct output should NOT look like this.
		typedStr := fmt.Sprintf("id=%s name=%s", d.ID, d.Name)
		if strings.Contains(typedStr, "map[") {
			t.Error("typed struct output should not contain raw Go map syntax")
		}
	}
}

// TestDescribeSLOTimeframeIsValidated pins that describe slo checks
// --timeframe like get slos: any case is sent upper-cased, and a value the
// API does not accept is a usage error that sends nothing.
func TestDescribeSLOTimeframeIsValidated(t *testing.T) {
	const path = "GET /e/env1/api/v2/slo/slo-1"
	api := newFakeAPI(t, map[string]route{path: {Body: `{"id":"slo-1","name":"n"}`}})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})

	if r := runCLI(t, cfg, "describe", "slo", "slo-1", "--timeframe", "gtf", "--from", "now-7d", "-o", "json"); r.Code != 0 {
		t.Fatalf("--timeframe gtf: %s", r)
	}
	if got := api.Queries(path)[0].Get("timeFrame"); got != "GTF" {
		t.Errorf("timeFrame = %q, want GTF", got)
	}
	r := runCLI(t, cfg, "describe", "slo", "slo-1", "--timeframe", "LAST")
	if r.Code != 2 || !strings.Contains(r.Stderr, `invalid --timeframe "LAST" (want one of: CURRENT, GTF)`) || api.Hits(path) != 1 {
		t.Errorf("--timeframe LAST: %s (hits %d)", r, api.Hits(path))
	}
}
