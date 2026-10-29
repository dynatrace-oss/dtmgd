package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// TestParseColumns verifies the column filter parsing logic.
func TestParseColumns(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
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
			a := &App{Opts: GlobalOptions{Columns: tt.input}}
			got := a.columns()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("columns() = %v, want %v", got, tt.want)
			}
		})
	}
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
			a := &App{Opts: GlobalOptions{MaxPages: tt.globalPages}}
			got := a.maxPagesFor(tt.hasLimit)
			if got != tt.want {
				t.Errorf("maxPagesFor(%v) = %d, want %d", tt.hasLimit, got, tt.want)
			}
		})
	}
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
			a := &App{Opts: GlobalOptions{Env: tt.env}}
			got := a.isMultiEnv()
			if got != tt.want {
				t.Errorf("isMultiEnv() with --env %q = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

// TestRequireSubcommand verifies that requireSubcommand always returns a non-nil error
// (the "a subcommand is required" sentinel), regardless of the command passed in.
func TestRequireSubcommand(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Short: "test command"}
	err := requireSubcommand(cmd, nil)
	if err == nil {
		t.Fatal("expected non-nil error from requireSubcommand")
	}
	if err.Error() != "a subcommand is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// TestRewriteNegativeArgs verifies that rewriteNegativeArgs inserts "--" before
// negative-integer IDs and correctly moves trailing flags in front of "--".
func TestRewriteNegativeArgs(t *testing.T) {
	neg := "-6546711275898328738_1776193140000V2"
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "positive ID: no change",
			input: []string{"describe", "problem", "abc-uuid-123"},
			want:  []string{"describe", "problem", "abc-uuid-123"},
		},
		{
			name:  "negative ID alone: insert --",
			input: []string{"describe", "problem", neg},
			want:  []string{"describe", "problem", "--", neg},
		},
		{
			name:  "negative ID with -c after: move -c before --",
			input: []string{"describe", "problem", neg, "-c", "prod"},
			want:  []string{"describe", "problem", "-c", "prod", "--", neg},
		},
		{
			name:  "negative ID with -o and -c after: both moved before --",
			input: []string{"describe", "problem", neg, "-o", "json", "-c", "prod"},
			want:  []string{"describe", "problem", "-o", "json", "-c", "prod", "--", neg},
		},
		{
			name:  "flags before negative ID + flags after: after-flags moved before --",
			input: []string{"describe", "problem", "-v", neg, "-o", "json"},
			want:  []string{"describe", "problem", "-v", "-o", "json", "--", neg},
		},
		{
			name:  "boolean flag after negative ID: moved before --",
			input: []string{"describe", "problem", neg, "-v"},
			want:  []string{"describe", "problem", "-v", "--", neg},
		},
		{
			name:  "-- already present before negative ID: no change",
			input: []string{"describe", "problem", "--", neg},
			want:  []string{"describe", "problem", "--", neg},
		},
		{
			name:  "-- already present with -c before --: no change",
			input: []string{"describe", "problem", "-c", "prod", "--", neg},
			want:  []string{"describe", "problem", "-c", "prod", "--", neg},
		},
		{
			name:  "-- in after: hard stop, remaining passed through",
			input: []string{"describe", "problem", neg, "--", "extra"},
			want:  []string{"describe", "problem", "--", neg, "--", "extra"},
		},
		{
			name:  "alias prob: also rewritten",
			input: []string{"describe", "prob", neg, "-c", "prod"},
			want:  []string{"describe", "prob", "-c", "prod", "--", neg},
		},
		{
			name:  "non-describe-problem command: no change even with -digit arg",
			input: []string{"get", "problems", "-5"},
			want:  []string{"get", "problems", "-5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteNegativeArgs(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rewriteNegativeArgs(%v)\n  got:  %v\n  want: %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsDescribeProblemArgs verifies the subcommand-path detection helper.
func TestIsDescribeProblemArgs(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"describe", "problem", "abc"}, true},
		{[]string{"describe", "prob", "abc"}, true},
		{[]string{"desc", "problem", "abc"}, true},
		{[]string{"describe", "entity", "abc"}, false},
		{[]string{"get", "problems"}, false},
		{[]string{"problem"}, false},
		{[]string{}, false},
	}
	for _, tt := range tests {
		got := isDescribeProblemArgs(tt.args)
		if got != tt.want {
			t.Errorf("isDescribeProblemArgs(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

// TestAgentNoticeOnlyWhenAutoDetected pins pentest note 5: agent mode switched
// on by an environment variable is announced, but an explicit --agent or
// --no-agent is the user's own choice and needs no notice.
func TestAgentNoticeOnlyWhenAutoDetected(t *testing.T) {
	for _, v := range agentmode.EnvVars() {
		t.Setenv(v, "")
	}
	t.Setenv("CLAUDECODE", "1")
	a := &App{}

	a.Opts.Agent, a.Opts.NoAgent = false, false
	if got := a.agentNotice(); !strings.Contains(got, "detected claude") {
		t.Errorf("auto-detected: got %q", got)
	}
	a.Opts.Agent = true
	if got := a.agentNotice(); got != "" {
		t.Errorf("--agent given: want no notice, got %q", got)
	}
	a.Opts.Agent, a.Opts.NoAgent = false, true
	if got := a.agentNotice(); got != "" {
		t.Errorf("--no-agent given: want no notice, got %q", got)
	}
}
