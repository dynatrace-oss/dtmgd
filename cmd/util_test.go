package cmd

import "testing"

func TestMsToTime(t *testing.T) {
	// 2024-01-01 00:00:00 UTC = 1704067200000 ms
	result := msToTime(1704067200000)
	if result != "2024-01-01 00:00:00" {
		t.Errorf("expected '2024-01-01 00:00:00', got '%s'", result)
	}
}

func TestMsToTimeZero(t *testing.T) {
	if msToTime(0) != "" {
		t.Error("zero should return empty string")
	}
	if msToTime(-1) != "" {
		t.Error("negative should return empty string")
	}
}

func TestMsToTimeOrEmpty(t *testing.T) {
	if msToTimeOrEmpty(0) != "-" {
		t.Error("zero should return dash")
	}
	if msToTimeOrEmpty(-1) != "-" {
		t.Error("-1 should return dash")
	}
	if msToTimeOrEmpty(1704067200000) == "-" {
		t.Error("valid timestamp should not return dash")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" {
		t.Error("short string should not be truncated")
	}
	result := truncate("this is a long string", 10)
	if len([]rune(result)) != 10 {
		t.Errorf("expected 10 runes, got %d", len([]rune(result)))
	}
	if result[len(result)-3:] != "…" {
		t.Error("should end with ellipsis")
	}
}

func TestTruncateNewlines(t *testing.T) {
	result := truncate("line1\nline2\nline3", 50)
	if result != "line1 line2 line3" {
		t.Errorf("newlines should be replaced with spaces, got '%s'", result)
	}
}

func TestJoinSelector(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{
			name:  "single part",
			parts: []string{`status("OPEN")`},
			want:  `status("OPEN")`,
		},
		{
			name:  "two parts",
			parts: []string{`status("OPEN")`, `impactLevel("SERVICE")`},
			want:  `status("OPEN"),impactLevel("SERVICE")`,
		},
		{
			name:  "three parts — problems + security combined",
			parts: []string{`status("OPEN")`, `riskLevel("CRITICAL")`, `managementZones("bookstore")`},
			want:  `status("OPEN"),riskLevel("CRITICAL"),managementZones("bookstore")`,
		},
		{
			name:  "empty",
			parts: []string{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinSelector(tt.parts...)
			if got != tt.want {
				t.Errorf("joinSelector(%v) = %q, want %q", tt.parts, got, tt.want)
			}
		})
	}
}
