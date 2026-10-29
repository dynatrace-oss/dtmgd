package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// UsageError is a mistake in how dtmgd was invoked: an unknown command,
// resource or flag, or missing arguments. It exits with client.ExitUsage.
type UsageError struct {
	Err         error
	Suggestions []string
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// errReported marks an error whose output a command has already written
// (for example an agent envelope carrying partial results). Run reports
// nothing more for it and only computes the exit code.
var errReported = errors.New("already reported")

// exitCode maps an error from a command to the process exit code.
func exitCode(err error) int {
	if err == nil {
		return client.ExitSuccess
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return client.ExitUsage
	}
	var partial *client.PartialFailureError
	if errors.As(err, &partial) {
		return partial.Code()
	}
	var apiErr *client.ErrAPI
	if errors.As(err, &apiErr) {
		return client.ExitCodeForStatus(apiErr.StatusCode)
	}
	return client.ExitError
}

// reportError tells the user what went wrong and, when there is one, what to
// do about it. cmd is the command that failed (it may be the root when
// parsing failed).
func (a *App) reportError(_ *cobra.Command, err error) {
	if errors.Is(err, errReported) {
		return
	}
	hint := client.Diagnose(err)
	if a.agentMode() {
		if hint != "" {
			err = fmt.Errorf("%w\n\n  💡 %s", err, hint)
		}
		output.PrintAgentError(err)
		return
	}
	output.PrintHumanError("%s", err)
	if hint != "" {
		output.PrintHint("%s", hint)
	}
}
