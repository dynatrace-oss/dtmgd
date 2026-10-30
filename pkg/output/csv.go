package output

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
)

// ErrNotTabular is what a CSVPrinter returns for a value without table
// columns: a map, a scalar, or a slice of either.
var ErrNotTabular = errors.New("this output has no table columns for csv or tsv; use -o json or -o yaml")

// CSVPrinter writes table rows as comma- (csv) or tab-separated (tsv)
// values with a header line, using the same struct tags as TablePrinter.
// Every tagged column is written, wide ones included; --columns narrows them.
type CSVPrinter struct {
	w       io.Writer
	comma   rune
	columns []string
}

func (p *CSVPrinter) Print(v interface{}) error { return p.PrintList(v) }

// PrintList writes v, a slice of structs or one struct, as a header line and
// one line per row. An empty slice still gets its header line.
func (p *CSVPrinter) PrintList(v interface{}) error {
	headers, fieldIdx, err := p.header(v)
	if err != nil {
		return err
	}
	w := p.writer()
	if err := w.Write(headers); err != nil {
		return err
	}
	if err := p.writeRows(w, "", v, fieldIdx); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// PrintSections writes the lists of several environments as one table whose
// first column, CONTEXT, names the environment each row came from. lists[i]
// belongs to names[i]; all lists hold the same row type.
func (p *CSVPrinter) PrintSections(names []string, lists []interface{}) error {
	if len(lists) == 0 {
		return nil
	}
	headers, fieldIdx, err := p.header(lists[0])
	if err != nil {
		return err
	}
	w := p.writer()
	if err := w.Write(append([]string{"CONTEXT"}, headers...)); err != nil {
		return err
	}
	for i, list := range lists {
		if err := p.writeRows(w, names[i], list, fieldIdx); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func (p *CSVPrinter) writer() *csv.Writer {
	w := csv.NewWriter(p.w)
	w.Comma = p.comma
	return w
}

// header returns the columns of v's row type, or ErrNotTabular.
func (p *CSVPrinter) header(v interface{}) ([]string, []int, error) {
	t := reflect.TypeOf(v)
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice) {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, nil, ErrNotTabular
	}
	headers, fieldIdx := (&TablePrinter{wide: true, columns: p.columns}).tableColumns(t)
	if len(headers) == 0 {
		return nil, nil, ErrNotTabular
	}
	return headers, fieldIdx, nil
}

// writeRows writes one line per row of v, led by context when it is not "".
func (p *CSVPrinter) writeRows(w *csv.Writer, context string, v interface{}, fieldIdx []int) error {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.Len() == 0 {
		return nil
	}
	for _, row := range tableRows(v) {
		if row.Kind() == reflect.Pointer {
			row = row.Elem()
		}
		var cells []string
		if context != "" {
			cells = append(cells, csvSafe(context))
		}
		for _, idx := range fieldIdx {
			cells = append(cells, csvSafe(fmt.Sprintf("%v", row.Field(idx).Interface())))
		}
		if err := w.Write(cells); err != nil {
			return err
		}
	}
	return nil
}

// plainAfterMinus is what may follow a leading "-" in a cell that is data, not
// a formula: a negative number or a negative problem ID.
var plainAfterMinus = regexp.MustCompile(`^[0-9A-Za-z_.:]*$`)

// csvSafe defuses a cell a spreadsheet would run as a formula (CSV
// injection), by prefixing "'": one starting with = + @ tab or carriage
// return, or with "-" followed by anything but a plain number or ID. Log
// content and entity names can hold text an attacker chose.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	case '-':
		if !plainAfterMinus.MatchString(s[1:]) {
			return "'" + s
		}
	}
	return s
}
