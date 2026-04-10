package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

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

// PrintSuccess writes a green-ish success message to stderr.
func PrintSuccess(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "✓ "+format+"\n", args...)
}

// PrintWarning writes a warning message to stderr.
func PrintWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
}

// PrintInfo writes an informational message to stderr.
func PrintInfo(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// PrintHumanError writes an error message to stderr.
func PrintHumanError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
}

// DescribeKV prints a key-value pair with right-aligned key in width w.
func DescribeKV(key string, w int, valueFormat string, args ...interface{}) {
	value := fmt.Sprintf(valueFormat, args...)
	fmt.Printf("%-*s %s\n", w, key, value)
}
