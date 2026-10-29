package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Values the API accepts for the enum flags, as env2temp.json lists them.
var (
	problemStatuses         = []string{"OPEN", "CLOSED"}
	problemImpactLevels     = []string{"APPLICATION", "ENVIRONMENT", "INFRASTRUCTURE", "SERVICES"}
	securityProblemStatuses = []string{"OPEN", "RESOLVED"}
	securityRiskLevels      = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "NONE"}
)

// enumFlag checks the value of --flag against allowed, ignoring case, and
// returns its canonical form: upper-cased when upper is set, otherwise as
// allowed spells it. An empty value (flag not given) is returned as is. A
// value not in allowed is a UsageError naming the allowed values.
func enumFlag(flag, value string, allowed []string, upper bool) (string, error) {
	if value == "" {
		return "", nil
	}
	for _, a := range allowed {
		if strings.EqualFold(value, a) {
			if upper {
				return strings.ToUpper(a), nil
			}
			return a, nil
		}
	}
	return "", &UsageError{Err: fmt.Errorf("invalid --%s %q (want one of: %s)", flag, value, strings.Join(allowed, ", "))}
}

// limitFlag checks --limit: 0 means no limit, and a negative value is a
// UsageError rather than a way around the agent-mode default.
func limitFlag(n int) error {
	if n < 0 {
		return &UsageError{Err: errors.New("--limit must be 0 (no limit) or positive")}
	}
	return nil
}

// noArgs is the Args of commands that take no positional arguments. A stray
// word is most often a bool flag's value written with a space
// (--show-global false), which cobra would otherwise drop, leaving the flag
// true and sending it.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return &UsageError{Err: fmt.Errorf("%s takes no arguments, got %q; give a bool flag's value with '=', e.g. --show-global=false", cmd.CommandPath(), args[0])}
}
