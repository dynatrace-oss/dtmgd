package cmd

import "testing"

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
