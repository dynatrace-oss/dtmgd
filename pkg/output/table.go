package output

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// TablePrinter formats data as a terminal table using struct field tags.
// Tag format: `table:"HEADER"`, plus options after commas: "wide" (only
// shown with --output wide) and "max=N" (shortened to N runes on one line, in
// tables only).
type TablePrinter struct {
	w       io.Writer
	wide    bool
	columns []string // if set, only show these columns (case-insensitive match on header)
}

func (p *TablePrinter) Print(v interface{}) error {
	return p.PrintList(v)
}

func (p *TablePrinter) columnRequested(header string) bool {
	h := strings.ToUpper(header)
	for _, c := range p.columns {
		if strings.ToUpper(c) == h {
			return true
		}
	}
	return false
}

// PrintList accepts either a slice or a single struct, printing rows to a table.
func (p *TablePrinter) PrintList(v interface{}) error {
	rows := tableRows(v)
	if len(rows) == 0 {
		_, err := fmt.Fprintln(p.w, "No items found.")
		return err
	}

	// Determine element type
	elemType := rows[0].Type()
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		// Maps, scalars and slices of them have no columns to lay out. YAML is
		// the readable form; Go's own formatting would print map[...] syntax.
		return p.printYAML(v)
	}

	headers, fieldIdx, maxes := p.tableColumns(elemType)
	if len(headers) == 0 {
		return p.printYAML(v)
	}

	// WithHeaderAutoFormat must precede WithHeader: WithHeader applies the
	// headers immediately, and tablewriter defaults AutoFormat to On at that
	// point. With AutoFormat on, headers are camel-split and rejoined with
	// spaces, turning "ENV-ID" into "ENV - ID".
	tbl := tablewriter.NewTable(p.w,
		tablewriter.WithHeaderAutoFormat(tw.Off),
		tablewriter.WithHeader(headers),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.BorderNone,
			Settings: tw.Settings{
				Lines:      tw.LinesNone,
				Separators: tw.SeparatorsNone,
			},
		}),
	)
	if tbl == nil {
		tbl = tablewriter.NewTable(os.Stdout)
	}

	for _, row := range rows {
		rv := row
		if rv.Kind() == reflect.Pointer {
			rv = rv.Elem()
		}
		var cells []string
		for i, idx := range fieldIdx {
			cells = append(cells, shorten(fmt.Sprintf("%v", rv.Field(idx).Interface()), maxes[i]))
		}
		if err := tbl.Append(cells); err != nil {
			return err
		}
	}

	return tbl.Render()
}

// tableRows is what PrintList prints a row for: the elements of a slice, or
// v itself, either through a pointer.
func tableRows(v interface{}) []reflect.Value {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return []reflect.Value{rv}
	}
	var rows []reflect.Value
	for i := 0; i < rv.Len(); i++ {
		rows = append(rows, rv.Index(i))
	}
	return rows
}

// tableColumns returns the headers, field indices and max=N lengths (0 for
// none) of the columns to show for struct type t, in field order: every
// field with a table tag except wide ones (unless wide output or --columns
// asks for them), and only the --columns ones when that is set.
func (p *TablePrinter) tableColumns(t reflect.Type) (headers []string, fieldIdx, maxes []int) {
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("table")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		header, isWide, maxLen := parts[0], false, 0
		for _, opt := range parts[1:] {
			if strings.EqualFold(opt, "wide") {
				isWide = true
			} else if n, ok := strings.CutPrefix(opt, "max="); ok {
				maxLen, _ = strconv.Atoi(n)
			}
		}
		if (isWide && !p.wide && !p.columnRequested(header)) || (len(p.columns) > 0 && !p.columnRequested(header)) {
			continue
		}
		headers = append(headers, header)
		fieldIdx = append(fieldIdx, i)
		maxes = append(maxes, maxLen)
	}
	return headers, fieldIdx, maxes
}

// shorten puts s on one line and cuts it to maxLen runes, ending in "…"
// when cut; maxLen 0 keeps s as it is.
func shorten(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if r := []rune(s); len(r) > maxLen {
		return string(r[:maxLen-1]) + "…"
	}
	return s
}

// printYAML renders v as YAML, the fallback for values without table columns.
func (p *TablePrinter) printYAML(v interface{}) error {
	return (&YAMLPrinter{w: p.w}).Print(v)
}
