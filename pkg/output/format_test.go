package output

import (
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in   string
		want Format
	}{
		{"", FormatTable},
		{"table", FormatTable},
		{"wide", FormatWide},
		{"json", FormatJSON},
		{"yaml", FormatYAML},
		{"yml", FormatYAML},
	}
	for _, tt := range tests {
		got, err := ParseFormat(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseFormat(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	_, err := ParseFormat("xml")
	if err == nil || !strings.Contains(err.Error(), `unknown output format "xml" (want one of: table, wide, json, yaml)`) {
		t.Errorf("ParseFormat(xml) error = %v", err)
	}
}
