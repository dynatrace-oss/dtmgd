package config

import (
	"reflect"
	"testing"
)

func TestExpand(t *testing.T) {
	t.Setenv("SET_VAR", "VALUE")
	t.Setenv("OTHER_VAR", "OTHER")
	t.Setenv("EMPTY_VAR", "")

	tests := []struct {
		name      string
		input     string
		want      string
		wantUnset []string
	}{
		{"no references", "plain value", "plain value", nil},
		{"single set", "${SET_VAR}", "VALUE", nil},
		{"embedded", "a-${SET_VAR}-b", "a-VALUE-b", nil},
		{"two distinct", "${SET_VAR}/${OTHER_VAR}", "VALUE/OTHER", nil},
		{"repeated same", "${SET_VAR}${SET_VAR}", "VALUEVALUE", nil},
		{"set but empty is not unset", "${EMPTY_VAR}", "", nil},
		{"single unset", "${MISSING_VAR}", "", []string{"MISSING_VAR"}},
		{"unset deduped", "${MISSING_VAR}${MISSING_VAR}", "", []string{"MISSING_VAR"}},
		{"unset ordered by first use", "${B_MISSING}${A_MISSING}", "", []string{"B_MISSING", "A_MISSING"}},
		{"mixed set and unset", "${SET_VAR}:${MISSING_VAR}", "VALUE:", []string{"MISSING_VAR"}},

		// Literal $ must survive untouched. These are the strings that
		// os.ExpandEnv corrupts today.
		{"double dollar password", "pa$$w0rd", "pa$$w0rd", nil},
		{"dollar digit", "cost is $5", "cost is $5", nil},
		{"bare ref not expanded", "a$b-c", "a$b-c", nil},
		{"bare uppercase ref not expanded", "$SET_VAR", "$SET_VAR", nil},
		{"unclosed brace", "${SET_VAR", "${SET_VAR", nil},
		{"lone dollar", "$", "$", nil},
		{"empty braces", "${}", "${}", nil},
		{"name starting with digit", "${1BAD}", "${1BAD}", nil},
		{"proxy url with dollars", "http://user:pa$$w0rd@proxy.corp:8080", "http://user:pa$$w0rd@proxy.corp:8080", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, unset := expand(tt.input)
			if got != tt.want {
				t.Errorf("expand(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !reflect.DeepEqual(unset, tt.wantUnset) {
				t.Errorf("expand(%q) unset = %v, want %v", tt.input, unset, tt.wantUnset)
			}
		})
	}
}

func TestHasPlaceholder(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"${DT_API_TOKEN}", true},
		{"prefix-${VAR}-suffix", true},
		{"plain", false},
		{"", false},
		{"pa$$w0rd", false},
		{"$BARE", false},
		{"${unclosed", false},
		{"${}", false},
	}
	for _, tt := range tests {
		if got := HasPlaceholder(tt.input); got != tt.want {
			t.Errorf("HasPlaceholder(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBareRefs(t *testing.T) {
	t.Setenv("DT_API_TOKEN", "secret")
	t.Setenv("SET_VAR", "VALUE")

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"bare ref to set var", "$DT_API_TOKEN", []string{"DT_API_TOKEN"}},
		{"braced form is not bare", "${DT_API_TOKEN}", nil},
		{"mixed forms", "${SET_VAR}/$DT_API_TOKEN", []string{"DT_API_TOKEN"}},
		{"unset name is not reported", "$NOT_A_REAL_VAR", nil},
		{"password is not reported", "pa$$w0rd", nil},
		{"deduped", "$SET_VAR $SET_VAR", []string{"SET_VAR"}},
		{"plain text", "no refs here", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BareRefs(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BareRefs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
