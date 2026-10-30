package cmd

import (
	"strings"
	"time"
)

// msToTime converts a Unix millisecond timestamp to a human-readable UTC string.
func msToTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04:05")
}

// msToTimeOrEmpty converts a millisecond timestamp, returning "-" for -1 (open/ongoing).
func msToTimeOrEmpty(ms int64) string {
	if ms <= 0 || ms == -1 {
		return "-"
	}
	return msToTime(ms)
}

// truncate shortens a string to maxLen runes, appending "…" if needed.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// joinSelector joins selector criteria (problemSelector, securityProblemSelector
// or entitySelector) with a comma.
func joinSelector(parts ...string) string {
	return strings.Join(parts, ",")
}

// selectorString quotes v as a selector value: Dynatrace selectors escape a
// quote or a tilde inside a quoted value with a tilde.
func selectorString(v string) string {
	return `"` + strings.NewReplacer("~", "~~", `"`, `~"`).Replace(v) + `"`
}

// mzCriterion is the entitySelector criterion for --mz: the value as given
// when it already is an mzName(…) or mzId(…) criterion, else mzName of it.
func mzCriterion(mz string) string {
	if strings.HasPrefix(mz, "mzName(") || strings.HasPrefix(mz, "mzId(") {
		return mz
	}
	return "mzName(" + selectorString(mz) + ")"
}
