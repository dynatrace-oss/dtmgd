package output

import (
	"io"
	"os"
	"strings"
)

// ANSI codes for the few colors dtmgd uses, the ones dtctl uses for the same
// things: bold headers, and red, yellow and green for errors, warnings and
// success.
const (
	ansiReset      = "\x1b[0m"
	ansiBold       = "\x1b[1m"
	ansiRed        = "\x1b[31m"
	ansiGreen      = "\x1b[32m"
	ansiYellow     = "\x1b[33m"
	ansiBoldRed    = "\x1b[1;31m"
	ansiBoldGreen  = "\x1b[1;32m"
	ansiBoldYellow = "\x1b[1;33m"
)

// Whether table output and the stderr helpers use color. Process-wide, like
// plain; cmd.Run decides them once per invocation with SetColor.
var (
	tableColor  bool
	stderrColor bool
)

// SetColor turns color on or off for tables (on stdout) and for the stderr
// helpers' glyphs. Plain mode overrides it for the glyphs.
func SetColor(stdout, stderr bool) {
	tableColor, stderrColor = stdout, stderr
}

func colorize(code, s string) string {
	return code + s + ansiReset
}

// statusColors maps a status column's values, compared case-insensitively,
// to their color. Any other value stays uncolored.
var statusColors = map[string]string{
	"ok":       ansiGreen,
	"success":  ansiGreen,
	"healthy":  ansiGreen,
	"resolved": ansiGreen,
	"closed":   ansiGreen,
	"warn":     ansiYellow,
	"warning":  ansiYellow,
	"fail":     ansiRed,
	"failure":  ansiRed,
	"open":     ansiRed,
}

// statusCell colors a status column's cell by its value.
func statusCell(s string) string {
	if code, ok := statusColors[strings.ToLower(s)]; ok {
		return colorize(code, s)
	}
	return s
}

// EnableVT prepares w for escape codes and reports whether it can show them.
// Only a Windows console needs preparing (its virtual-terminal mode must be
// switched on); anything else, a pipe or a file included, reports true, so
// FORCE_COLOR works there.
func EnableVT(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || !IsTerminal(w) {
		return true
	}
	return enableVT(f)
}
