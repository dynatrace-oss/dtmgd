package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

// watchRun runs a watch loop at a 5ms interval that stops itself after fn's
// third call. fn fails on its first call. It returns the number of calls,
// stdout, what the stderr helpers wrote, what went to App.Err directly and
// the loop's error.
func watchRun(t *testing.T, env map[string]string) (calls int, out, helpers, appErr string, err error) {
	t.Helper()
	for _, v := range agentmode.EnvVars() {
		t.Setenv(v, "")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	for k, v := range env {
		t.Setenv(k, v)
	}
	var outBuf, helperBuf, errBuf bytes.Buffer
	output.SetStderr(&helperBuf)
	t.Cleanup(func() { output.SetStderr(nil); output.SetPlain(false) })
	a := newTestApp("")
	a.Out, a.Err = &outBuf, &errBuf
	output.SetPlain(a.plain())
	stop := make(chan struct{})
	a.watchStop = stop

	err = a.watchOrRun(watchOptions{on: true, interval: 5 * time.Millisecond}, func() error {
		calls++
		if calls == 3 {
			close(stop)
		}
		if calls == 1 {
			return errors.New("tick failed")
		}
		return nil
	})
	return calls, outBuf.String(), helperBuf.String(), errBuf.String(), err
}

// TestWatchLoopRunsUntilStopped pins the loop itself: it re-runs the command
// every interval with a header, reports a failed tick and carries on, and
// ends cleanly on Ctrl+C with the newline on the App's stderr.
func TestWatchLoopRunsUntilStopped(t *testing.T) {
	calls, out, helpers, appErr, err := watchRun(t, nil)
	if err != nil || calls < 3 {
		t.Fatalf("watch returned %v after %d calls, want nil after at least 3", err, calls)
	}
	if n := strings.Count(helpers, "Every 5ms — press Ctrl+C to stop"); n < 3 {
		t.Errorf("header printed %d times, want one per tick:\n%s", n, helpers)
	}
	if !strings.Contains(helpers, "Error: tick failed") {
		t.Errorf("failed tick not reported:\n%s", helpers)
	}
	if strings.Contains(out, "\033[H\033[2J") {
		t.Errorf("plain output cleared the screen: %q", out)
	}
	if appErr != "\n" {
		t.Errorf("App.Err = %q, want the newline that ends the watch", appErr)
	}
}

// TestWatchClearsTheScreenOnATerminal pins that a table on a terminal
// (FORCE_COLOR stands in for one) is redrawn from the top on every tick.
func TestWatchClearsTheScreenOnATerminal(t *testing.T) {
	calls, out, _, _, _ := watchRun(t, map[string]string{"FORCE_COLOR": "1"})
	if n := strings.Count(out, "\033[H\033[2J"); n != calls {
		t.Errorf("screen cleared %d times in %d ticks", n, calls)
	}
}
