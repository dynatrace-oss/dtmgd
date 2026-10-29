package cmd

import (
	"strings"
	"testing"
)

// TestExitCodes pins the exit-code contract: 3 for 401, 4 for 404, 5 for 403,
// and a non-zero code when any environment of a multi-environment call fails.
func TestExitCodes(t *testing.T) {
	single := []struct {
		fixture string
		status  int
		want    int
	}{
		{"error_401.json", 401, 3},
		{"error_404.json", 404, 4},
		{"error_403.json", 403, 5},
	}
	for _, tt := range single {
		api := newFakeAPI(t, map[string]route{problemsPath: {Status: tt.status, File: tt.fixture}})
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems")
		if r.Code != tt.want {
			t.Errorf("HTTP %d: code = %d, want %d (stderr %q)", tt.status, r.Code, tt.want, r.Stderr)
		}
	}

	deny := map[string]route{problemsPath: {Status: 403, File: "error_403.json"}}
	a, b := newFakeAPI(t, deny), newFakeAPI(t, deny)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-o", "json")
	if r.Code != 5 || !strings.Contains(r.Stderr, "a: API error 403") || !strings.Contains(r.Stderr, "b: API error 403") {
		t.Errorf("both 403: code = %d, stderr = %q; want 5 naming a and b", r.Code, r.Stderr)
	}

	ok := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	denied := newFakeAPI(t, deny)
	r = runCLI(t, withFakeEnvs(t, fakeEnv{"a", ok}, fakeEnv{"b", denied}), "get", "problems", "--env", "a;b", "-o", "json")
	if r.Code != 1 {
		t.Errorf("one ok, one 403: code = %d, want 1", r.Code)
	}
}
