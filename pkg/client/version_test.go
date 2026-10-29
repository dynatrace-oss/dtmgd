package client

import "testing"

// TestLowestClusterVersion pins the version a cluster in the middle of an
// upgrade reports: its lowest node's, ignoring versions that do not parse.
func TestLowestClusterVersion(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"1.330.1.20251017-120000", "1.329.9.20250901-090000"}, "1.329.9.20250901-090000"},
		{[]string{"garbage", "1.330.1.20251017-120000"}, "1.330.1.20251017-120000"},
		{[]string{"1.330.10.x", "1.330.9.y"}, "1.330.9.y"},
		{nil, ""},
		{[]string{"garbage"}, ""},
	}
	for _, tc := range cases {
		if got := LowestClusterVersion(tc.in); got != tc.want {
			t.Errorf("LowestClusterVersion(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestParseClusterVersion pins the parse doctor's version floor relies on.
func TestParseClusterVersion(t *testing.T) {
	if got, ok := ParseClusterVersion("1.330.18.20251017-123456"); !ok || got != [3]int{1, 330, 18} {
		t.Errorf("full version: %v %v", got, ok)
	}
	for _, bad := range []string{"", "1.330", "1.x.3", "1.-1.0"} {
		if _, ok := ParseClusterVersion(bad); ok {
			t.Errorf("ParseClusterVersion(%q) ok, want not ok", bad)
		}
	}
}
