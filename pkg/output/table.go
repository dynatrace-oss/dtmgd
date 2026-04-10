package output

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/olekukonko/tablewriter"
)

// TablePrinter formats data as a terminal table using struct field tags.
// Tag format: `table:"HEADER"` or `table:"HEADER,wide"` (only shown with --output wide).
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
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	var rows []reflect.Value
	if rv.Kind() == reflect.Slice {
		for i := 0; i < rv.Len(); i++ {
			rows = append(rows, rv.Index(i))
		}
	} else {
		rows = []reflect.Value{rv}
	}

	if len(rows) == 0 {
		fmt.Fprintln(p.w, "No items found.")
		return nil
	}

	// Determine element type
	elemType := rows[0].Type()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		// Fallback: just print each value
		for _, r := range rows {
			fmt.Fprintln(p.w, r.Interface())
		}
		return nil
	}

	// Collect headers and field indices
	var headers []string
	var fieldIdx []int
	for i := 0; i < elemType.NumField(); i++ {
		f := elemType.Field(i)
		tag := f.Tag.Get("table")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		header := parts[0]
		isWide := len(parts) > 1 && strings.ToLower(parts[1]) == "wide"
		if isWide && !p.wide && !p.columnRequested(header) {
			continue
		}
		if len(p.columns) > 0 && !p.columnRequested(header) {
			continue
		}
		headers = append(headers, header)
		fieldIdx = append(fieldIdx, i)
	}

	if len(headers) == 0 {
		fmt.Fprintln(p.w, v)
		return nil
	}

	tw := tablewriter.NewWriter(p.w)
	if tw == nil {
		tw = tablewriter.NewWriter(os.Stdout)
	}
	tw.SetHeader(headers)
	tw.SetAutoWrapText(false)
	tw.SetBorder(false)
	tw.SetHeaderLine(false)
	tw.SetColumnSeparator("  ")
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, row := range rows {
		rv := row
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		var cells []string
		for _, idx := range fieldIdx {
			cells = append(cells, fmt.Sprintf("%v", rv.Field(idx).Interface()))
		}
		tw.Append(cells)
	}

	tw.Render()
	return nil
}
