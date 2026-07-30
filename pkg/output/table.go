package output

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
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
	if rv.Kind() == reflect.Pointer {
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
		_, err := fmt.Fprintln(p.w, "No items found.")
		return err
	}

	// Determine element type
	elemType := rows[0].Type()
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		// Fallback: print each value on its own line
		for _, r := range rows {
			if _, err := fmt.Fprintln(p.w, r.Interface()); err != nil {
				return err
			}
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
		_, err := fmt.Fprintln(p.w, v)
		return err
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
		for _, idx := range fieldIdx {
			cells = append(cells, fmt.Sprintf("%v", rv.Field(idx).Interface()))
		}
		if err := tbl.Append(cells); err != nil {
			return err
		}
	}

	return tbl.Render()
}
