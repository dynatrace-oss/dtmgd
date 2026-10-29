package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

// newUnknownCommandError is the UsageError for an unknown command or resource.
func newUnknownCommandError(err error, suggestions ...string) *UsageError {
	return &UsageError{Err: err, Suggestions: suggestions, unknownCommand: true}
}

// requireSubcommand is the RunE of the group commands (get, describe, query,
// config, skills): they only group subcommands, so reaching it means the
// subcommand was missing or unknown. A close match is offered as the command
// to run instead. The verbs call their subcommands resource types.
func requireSubcommand(cmd *cobra.Command, args []string) error {
	what := "subcommand"
	if slices.Contains(catalogVerbs, cmd.Name()) {
		what = "resource type"
	}
	help := fmt.Sprintf("Run '%s --help' for available %ss", cmd.CommandPath(), strings.TrimSuffix(what, " type"))
	if len(args) == 0 {
		return &UsageError{Err: fmt.Errorf("a %s is required", what), Suggestions: []string{help}}
	}
	err := fmt.Errorf("unknown %s %q for %q", what, args[0], cmd.CommandPath())
	byName := map[string]string{}
	var names []string
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		for _, n := range append([]string{sub.Name()}, sub.Aliases...) {
			byName[n] = sub.Name()
			names = append(names, n)
		}
	}
	if match, ok := closest(args[0], names); ok {
		return newUnknownCommandError(err, cmd.CommandPath()+" "+byName[match])
	}
	return newUnknownCommandError(err, help)
}

// flagError is the root's FlagErrorFunc (cobra hands it to every
// subcommand): flag mistakes become usage errors, with a close match offered
// for an unknown long flag.
func flagError(cmd *cobra.Command, err error) error {
	m := unknownFlagRe.FindStringSubmatch(err.Error())
	if m == nil {
		return &UsageError{Err: err}
	}
	var names []string
	collect := func(f *pflag.Flag) { names = append(names, f.Name) }
	cmd.LocalFlags().VisitAll(collect)
	cmd.InheritedFlags().VisitAll(collect)
	if match, ok := closest(m[1], names); ok {
		return &UsageError{Err: fmt.Errorf("unknown flag --%s, did you mean --%s?", m[1], match)}
	}
	return &UsageError{Err: err}
}

var unknownFlagRe = regexp.MustCompile(`^unknown flag: --([\w-]+)`)

// usagePrefixes are the starts of cobra's and pflag's own messages for a bad
// command line; classifyCobraError turns them into usage errors.
var usagePrefixes = []string{
	"unknown command", "unknown flag", "unknown shorthand flag", "accepts ",
	"requires at least", "requires at most", "invalid argument",
	"flag needs an argument", "required flag(s)",
}

// classifyCobraError turns cobra's untyped command-line errors into usage
// errors, so they exit 2 like the ones dtmgd raises itself.
func classifyCobraError(err error) error {
	var usage *UsageError
	if err == nil || errors.As(err, &usage) {
		return err
	}
	msg := err.Error()
	for _, p := range usagePrefixes {
		if strings.HasPrefix(msg, p) {
			if strings.HasPrefix(msg, "unknown command") {
				return newUnknownCommandError(err)
			}
			return &UsageError{Err: err}
		}
	}
	return err
}

// closest returns the candidate nearest to input (case-insensitive edit
// distance of at most 2, ties broken alphabetically), if any.
func closest(input string, candidates []string) (string, bool) {
	best, bestDist := "", 3
	in := strings.ToLower(input)
	for _, c := range candidates {
		d := levenshtein(in, strings.ToLower(c))
		if d < bestDist || (d == bestDist && c < best) {
			best, bestDist = c, d
		}
	}
	return best, bestDist <= 2
}

// levenshtein is the edit distance between a and b.
func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// rawAgentMode decides agent mode from the raw arguments, for errors raised
// before cobra finished parsing flags (an unknown flag stops parsing, so
// --agent or --no-agent later on the line was never read).
func rawAgentMode(args []string) bool {
	agent := false
	for _, arg := range args {
		switch {
		case arg == "--":
			return agent || agentmode.Detect().Detected
		case arg == "--no-agent", arg == "--no-agent=true":
			return false
		case arg == "--agent", arg == "--agent=true":
			agent = true
		case len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsRune(arg[1:], 'A'):
			agent = true
		}
	}
	return agent || agentmode.Detect().Detected
}

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
func (a *App) reportError(cmd *cobra.Command, err error) {
	if errors.Is(err, errReported) {
		return
	}
	hint := client.Diagnose(err)
	if scopes := requiredScopes(cmd); len(scopes) > 0 && isPermissionDenied(err) {
		hint += "\n  This command needs the token scope(s): " + strings.Join(scopes, ", ") + "."
	}
	var usage *UsageError
	agent := a.agentMode()
	if errors.As(err, &usage) {
		agent = rawAgentMode(a.args)
	}
	if agent {
		// Best effort: this is the last-resort reporter, so a failed write
		// has nowhere else to go.
		_ = output.WriteAgentError(a.Out, errorDetail(err, hint))
		return
	}
	output.PrintHumanError("%s", err)
	if hint != "" {
		output.PrintHint("%s", hint)
	}
	if usage != nil {
		for _, sug := range usage.Suggestions {
			if strings.HasPrefix(sug, "dtmgd ") {
				output.PrintHint("Did you mean: %s", sug)
			} else {
				output.PrintHint("%s", sug)
			}
		}
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

// isPermissionDenied reports whether err is, or includes, an HTTP 403.
func isPermissionDenied(err error) bool {
	var apiErr *client.ErrAPI
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 403
	}
	var partial *client.PartialFailureError
	if errors.As(err, &partial) {
		for _, r := range partial.Failed {
			if isPermissionDenied(r.Error) {
				return true
			}
		}
	}
	return false
}
