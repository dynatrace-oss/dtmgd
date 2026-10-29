package cmd

import (
	"errors"
	"fmt"
	"strings"
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
