package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Printer is the common interface for all output formatters.
type Printer interface {
	Print(v interface{}) error
	PrintList(v interface{}) error
}

// NewPrinter returns a Printer for the given format ("table", "json", "yaml").
// Defaults to table.
func NewPrinter(format string) Printer {
	return NewPrinterTo(format, os.Stdout)
}

// NewPrinterTo returns a Printer writing to the given writer.
func NewPrinterTo(format string, w io.Writer) Printer {
	return NewPrinterToWithColumns(format, w, nil)
}

// NewPrinterToWithColumns returns a Printer with optional column filtering.
func NewPrinterToWithColumns(format string, w io.Writer, columns []string) Printer {
	if w == nil {
		w = os.Stdout
	}
	switch format {
	case "json":
		return &JSONPrinter{w: w}
	case "yaml", "yml":
		return &YAMLPrinter{w: w}
	default:
		return &TablePrinter{w: w, wide: format == "wide", columns: columns}
	}
}

// JSONPrinter formats as indented JSON.
type JSONPrinter struct{ w io.Writer }

func (p *JSONPrinter) Print(v interface{}) error {
	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func (p *JSONPrinter) PrintList(v interface{}) error { return p.Print(v) }

// YAMLPrinter formats as YAML.
type YAMLPrinter struct{ w io.Writer }

func (p *YAMLPrinter) Print(v interface{}) error {
	enc := yaml.NewEncoder(p.w)
	enc.SetIndent(2)
	return enc.Encode(v)
}
func (p *YAMLPrinter) PrintList(v interface{}) error { return p.Print(v) }

// Presentation settings for the stderr helpers below. They are process-wide
// because there is one terminal per process; cmd.Run sets them once per
// invocation. A nil stderr means os.Stderr as it is at the time of the call.
var (
	stderr io.Writer
	plain  bool
)

// SetStderr sets where the stderr helpers write; nil restores os.Stderr.
func SetStderr(w io.Writer) { stderr = w }

// SetPlain switches the stderr helpers between glyphs (⚠ ✗ ✓ 💡) and plain
// word prefixes, for scripts, screen readers and terminals without them.
func SetPlain(p bool) { plain = p }

func errOut() io.Writer {
	if stderr != nil {
		return stderr
	}
	return os.Stderr
}

func printPrefixed(glyph, word, format string, args ...interface{}) {
	prefix := glyph
	if plain {
		prefix = word
	}
	// Best effort: there is nowhere left to report a failed stderr write.
	_, _ = fmt.Fprintf(errOut(), prefix+format+"\n", args...)
}

// PrintSuccess writes a success message to stderr.
func PrintSuccess(format string, args ...interface{}) {
	printPrefixed("✓ ", "", format, args...)
}

// PrintWarning writes a warning message to stderr.
func PrintWarning(format string, args ...interface{}) {
	printPrefixed("⚠ ", "Warning: ", format, args...)
}

// PrintInfo writes an informational message to stderr.
func PrintInfo(format string, args ...interface{}) {
	printPrefixed("", "", format, args...)
}

// PrintHumanError writes an error message to stderr.
func PrintHumanError(format string, args ...interface{}) {
	printPrefixed("✗ ", "Error: ", format, args...)
}

// PrintHint writes a suggestion for what to do next to stderr.
func PrintHint(format string, args ...interface{}) {
	printPrefixed("💡 ", "Hint: ", format, args...)
}

// IsTerminal reports whether w is an interactive terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
