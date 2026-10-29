package output

import "fmt"

// Format is a validated --output value.
type Format string

// The output formats dtmgd supports.
const (
	FormatTable Format = "table"
	FormatWide  Format = "wide"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// ParseFormat validates an --output value. An empty value is a table and yml
// is accepted for yaml; anything else is an error, so a typo such as -o xml
// fails instead of silently printing a table.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "", "table":
		return FormatTable, nil
	case "wide":
		return FormatWide, nil
	case "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	}
	return "", fmt.Errorf("unknown output format %q (want one of: table, wide, json, yaml)", s)
}
