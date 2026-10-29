package cmd

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// UsageError is a mistake in how dtmgd was invoked: an unknown command,
// resource or flag, or missing arguments. It exits with client.ExitUsage.
type UsageError struct {
	Err         error
	Suggestions []string

	// unknownCommand marks an unknown command or resource, as opposed to
	// a bad flag or argument; agents see unknown_command for it.
	unknownCommand bool
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
// do about it: a human error and hint on stderr, or one error envelope on
// stdout in agent mode. cmd is the command that failed (the root when parsing
// failed).
func (a *App) reportError(_ *cobra.Command, err error) {
	if errors.Is(err, errReported) {
		return
	}
	hint := client.Diagnose(err)
	if a.agentMode() {
		// Best effort: this is the last-resort reporter, so a failed write
		// has nowhere else to go.
		_ = output.WriteAgentError(a.Out, errorDetail(err, hint))
		return
	}
	output.PrintHumanError("%s", err)
	if hint != "" {
		output.PrintHint("%s", hint)
	}
}

// errorDetail classifies err for the agent envelope, using dtctl's codes so
// agents can branch on them the same way for both tools.
func errorDetail(err error, hint string) *agentmode.ErrorDetail {
	d := &agentmode.ErrorDetail{Code: "error", Message: err.Error()}

	var (
		apiErr   *client.ErrAPI
		usage    *UsageError
		partial  *client.PartialFailureError
		notFound *config.NotFoundError
		unres    *config.UnresolvedVarsError
		netErr   net.Error
	)
	switch {
	case errors.As(err, &partial):
		d.Code = "partial_failure"
	case errors.As(err, &apiErr):
		d.Code = agentmode.ClassifyHTTPError(apiErr.StatusCode)
		d.StatusCode = apiErr.StatusCode
	case errors.As(err, &usage):
		d.Code = "validation_error"
		if usage.unknownCommand {
			d.Code = "unknown_command"
		}
		d.Suggestions = append(d.Suggestions, usage.Suggestions...)
	case errors.As(err, &notFound), errors.As(err, &unres),
		strings.Contains(err.Error(), "no current context"),
		strings.Contains(err.Error(), "context") && strings.Contains(err.Error(), "not found"):
		d.Code = "config_error"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		d.Code = "timeout"
	}
	if hint != "" {
		d.Suggestions = append([]string{hint}, d.Suggestions...)
	}
	return d
}
