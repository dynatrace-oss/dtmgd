package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// TestReportWatchErrorSkipsReported pins that a watch tick reports an error
// the way a single run would: nothing more for one the runner already
// printed (a partial failure's sections or envelope), and "Error: …" for the
// rest. Watch used to print every error itself, so a partial failure showed
// "already reported: …" under the output that had reported it.
func TestReportWatchErrorSkipsReported(t *testing.T) {
	for _, v := range agentmode.EnvVars() {
		t.Setenv(v, "")
	}
	var buf bytes.Buffer
	output.SetStderr(&buf)
	output.SetPlain(true)
	t.Cleanup(func() { output.SetStderr(nil); output.SetPlain(false) })
	a := newTestApp("")

	a.reportWatchError(fmt.Errorf("%w: x", errReported))
	if buf.Len() != 0 {
		t.Errorf("reported error printed again: %q", buf.String())
	}

	buf.Reset()
	a.reportWatchError(errors.New("boom"))
	if got := buf.String(); !strings.HasPrefix(got, "Error: boom") {
		t.Errorf("stderr = %q, want Error: boom", got)
	}
}

// TestWatchIntervalMustBePositive pins that a zero --watch-interval is a
// usage error raised before any request. time.NewTicker panics on it, after
// the first tick had already run.
func TestWatchIntervalMustBePositive(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-w", "--watch-interval", "0")
	if r.Code != 2 || !strings.Contains(r.Stderr, "--watch-interval must be positive") {
		t.Errorf("code %d, stderr %q", r.Code, r.Stderr)
	}
	if n := api.Hits(problemsPath); n != 0 {
		t.Errorf("%d API requests, want none", n)
	}
}
