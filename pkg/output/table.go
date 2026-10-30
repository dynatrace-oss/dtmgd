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
// shown with --output wide), "max=N" (shortened to N runes on one line, in
// tables only) and "status" (colored by value when colors are on, see
// statusColors). With colors on, headers are bold.
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

	headers, fieldIdx, maxes, status := p.tableColumns(elemType)
	if len(headers) == 0 {
		return p.printYAML(v)
	}
	headers = tableHeaders(headers)

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
		if err := tbl.Append(rowCells(row, fieldIdx, maxes, status)); err != nil {
			return err
		}
	}

	return tbl.Render()
}

// tableHeaders returns the headers as the table shows them: bold when table
// colors are on.
func tableHeaders(headers []string) []string {
	if !tableColor {
		return headers
	}
	bold := make([]string, len(headers))
	for i, h := range headers {
		bold[i] = colorize(ansiBold, h)
	}
	return bold
}

// rowCells returns one row's cells for the columns tableColumns chose:
// shortened to their max=N, and colored by value in a status column when
// table colors are on.
func rowCells(row reflect.Value, fieldIdx, maxes []int, status []bool) []string {
	if row.Kind() == reflect.Pointer {
		row = row.Elem()
	}
	cells := make([]string, len(fieldIdx))
	for i, idx := range fieldIdx {
		cells[i] = shorten(fmt.Sprintf("%v", row.Field(idx).Interface()), maxes[i])
		if tableColor && status[i] {
			cells[i] = statusCell(cells[i])
		}
	}
	return cells
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

// tableColumns returns the headers, field indices, max=N lengths (0 for
// none) and status flags of the columns to show for struct type t, in field
// order: every field with a table tag except wide ones (unless wide output or
// --columns asks for them), and only the --columns ones when that is set.
func (p *TablePrinter) tableColumns(t reflect.Type) (headers []string, fieldIdx, maxes []int, status []bool) {
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("table")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		header, isWide, maxLen, isStatus := parts[0], false, 0, false
		for _, opt := range parts[1:] {
			switch n, isMax := strings.CutPrefix(opt, "max="); {
			case strings.EqualFold(opt, "wide"):
				isWide = true
			case strings.EqualFold(opt, "status"):
				isStatus = true
			case isMax:
				maxLen, _ = strconv.Atoi(n)
			}
		}
		if (isWide && !p.wide && !p.columnRequested(header)) || (len(p.columns) > 0 && !p.columnRequested(header)) {
			continue
		}
		headers = append(headers, header)
		fieldIdx = append(fieldIdx, i)
		maxes = append(maxes, maxLen)
		status = append(status, isStatus)
	}
	return headers, fieldIdx, maxes, status
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
