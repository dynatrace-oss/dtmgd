package cmd

import (
	"reflect"
	"testing"
)

// TestParseColumns verifies the column filter parsing logic.
func TestParseColumns(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single column",
			input: "TITLE",
			want:  []string{"TITLE"},
		},
		{
			name:  "multiple columns",
			input: "PROBLEM-ID,TITLE,STATUS",
			want:  []string{"PROBLEM-ID", "TITLE", "STATUS"},
		},
		{
			name:  "strips spaces",
			input: " PROBLEM-ID , TITLE , STATUS ",
			want:  []string{"PROBLEM-ID", "TITLE", "STATUS"},
		},
		{
			name:  "skips empty segments",
			input: "TITLE,,STATUS",
			want:  []string{"TITLE", "STATUS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns = tt.input
			got := parseColumns()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseColumns() = %v, want %v", got, tt.want)
			}
		})
	}
	// Reset global state.
	columns = ""
}

// TestEffectiveMaxPages verifies the page-limit logic.
func TestEffectiveMaxPages(t *testing.T) {
	tests := []struct {
		name        string
		globalPages int
		hasLimit    bool
		want        int
	}{
		{
			name:        "no limit → use global maxPages=0",
			globalPages: 0,
			hasLimit:    false,
			want:        0,
		},
		{
			name:        "no limit → use global maxPages=5",
			globalPages: 5,
			hasLimit:    false,
			want:        5,
		},
		{
			name:        "has limit → always 1 page",
			globalPages: 0,
			hasLimit:    true,
			want:        1,
		},
		{
			name:        "has limit overrides global maxPages",
			globalPages: 10,
			hasLimit:    true,
			want:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxPages = tt.globalPages
			got := effectiveMaxPages(tt.hasLimit)
			if got != tt.want {
				t.Errorf("effectiveMaxPages(%v) = %d, want %d", tt.hasLimit, got, tt.want)
			}
		})
	}
	// Reset global state.
	maxPages = 0
}

// TestIsMultiEnv verifies multi-environment detection.
func TestIsMultiEnv(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{"", false},
		{"prod", false},
		{"ALL_ENVIRONMENTS", true},
		{"prod;staging", true},
		{"prod;staging;dev", true},
		{"prod;", true}, // semicolon present → multi
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			envSpec = tt.env
			got := isMultiEnv()
			if got != tt.want {
				t.Errorf("isMultiEnv() with envSpec=%q = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
	// Reset global state.
	envSpec = ""
}
