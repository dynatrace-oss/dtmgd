package cmd

import (
	"errors"
	"strings"
	"testing"
)

// TestTokenInputResolve covers the input-selection matrix for set-credentials.
//
// resolve is exercised directly rather than through cobra because the branch
// that matters most — no flags and no terminal — cannot be reached from a test
// binary whose stdin is a pipe if the terminal check is left to os.Stdin.
func TestTokenInputResolve(t *testing.T) {
	const prompted = "dt0c01.FROMPROMPT"

	tests := []struct {
		name    string
		in      tokenInput
		want    string
		wantErr string
	}{
		{
			name: "stdin",
			in:   tokenInput{fromStdin: true, stdin: strings.NewReader("dt0c01.PIPED\n")},
			want: "dt0c01.PIPED",
		},
		{
			name: "stdin strips the newline a here-string or echo adds",
			in:   tokenInput{fromStdin: true, stdin: strings.NewReader("  dt0c01.PADDED  \r\n")},
			want: "dt0c01.PADDED",
		},
		{
			name:    "stdin with no content",
			in:      tokenInput{fromStdin: true, stdin: strings.NewReader("   \n")},
			wantErr: "no token found on stdin",
		},
		{
			name:    "both inputs at once",
			in:      tokenInput{fromStdin: true, flagChanged: true, flagValue: "x", stdin: strings.NewReader("y")},
			wantErr: "cannot be combined",
		},
		{
			name:    "empty --token is a user error, not a prompt",
			in:      tokenInput{flagChanged: true, flagValue: ""},
			wantErr: "empty value",
		},
		{
			name: "no flags, terminal attached",
			in:   tokenInput{prompt: func() (string, error) { return prompted, nil }},
			want: prompted,
		},
		{
			name:    "no flags, no terminal",
			in:      tokenInput{},
			wantErr: "no token supplied",
		},
		{
			name:    "prompt failure surfaces",
			in:      tokenInput{prompt: func() (string, error) { return "", errors.New("boom") }},
			wantErr: "boom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			var err error
			// The --token branch warns; capturing keeps test output readable
			// and, for that case, is what the dedicated test below asserts on.
			captureStderr(t, func() { got, err = tc.in.resolve() })

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("resolve() = %q, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("resolve() error = %q, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("resolve() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTokenInputFlagStillWorksAndWarns pins the compatibility contract: an
// existing --token invocation keeps working, so no user's script breaks, but
// it now says why that invocation is a bad idea.
func TestTokenInputFlagStillWorksAndWarns(t *testing.T) {
	in := tokenInput{flagChanged: true, flagValue: "dt0c01.FROMFLAG"}

	var got string
	var err error
	stderr := captureStderr(t, func() { got, err = in.resolve() })

	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if got != "dt0c01.FROMFLAG" {
		t.Errorf("resolve() = %q, want the flag value", got)
	}
	for _, want := range []string{"process list", flagTokenStdin} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning did not mention %q:\n%s", want, stderr)
		}
	}
	// The warning must not repeat the secret it is warning about.
	if strings.Contains(stderr, "dt0c01.FROMFLAG") {
		t.Errorf("the token value was echoed in the warning:\n%s", stderr)
	}
}

// TestTokenInputStdinNotProvided covers the nil-reader guard, which is
// unreachable through the CLI but would otherwise panic in io.ReadAll.
func TestTokenInputStdinNotProvided(t *testing.T) {
	_, err := tokenInput{fromStdin: true}.resolve()
	if err == nil || !strings.Contains(err.Error(), "no input is available") {
		t.Errorf("resolve() error = %v, want a missing-input error", err)
	}
}
