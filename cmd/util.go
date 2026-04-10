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
