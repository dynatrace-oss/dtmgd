package output

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
)

// ErrNotTabular is what a CSVPrinter returns for a value without table
// columns: a map, a scalar, or a slice of either.
var ErrNotTabular = errors.New("this output has no table columns for csv or tsv; use -o json or -o yaml")

// ErrUnknownColumns is what a CSVPrinter returns when no column matches
// --columns; the message lists the columns there are.
var ErrUnknownColumns = errors.New("unknown column")

// CSVPrinter writes table rows as comma- (csv) or tab-separated (tsv)
// values with a header line, using the same struct tags as TablePrinter.
// Every tagged column is written, wide ones included and never shortened;
// --columns narrows them. Fields are quoted as in RFC 4180, for tsv too.
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
	w := bufio.NewWriter(p.w)
	p.writeRecord(w, headers)
	p.writeRows(w, "", v, fieldIdx)
	return w.Flush()
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
	w := bufio.NewWriter(p.w)
	p.writeRecord(w, append([]string{"CONTEXT"}, headers...))
	for i, list := range lists {
		p.writeRows(w, names[i], list, fieldIdx)
	}
	return w.Flush()
}

// writeRecord writes one line of fields, quoting a field that holds the
// separator, a quote, a line break, leading or trailing space, or (in csv) a
// semicolon: a spreadsheet whose list separator is ";" splits an unquoted
// field there, and the part after it could start with "=".
func (p *CSVPrinter) writeRecord(w *bufio.Writer, fields []string) {
	for i, f := range fields {
		if i > 0 {
			_, _ = w.WriteRune(p.comma)
		}
		if p.needsQuotes(f) {
			_, _ = w.WriteString(`"` + strings.ReplaceAll(f, `"`, `""`) + `"`)
		} else {
			_, _ = w.WriteString(f)
		}
	}
	_ = w.WriteByte('\n')
}

func (p *CSVPrinter) needsQuotes(f string) bool {
	if f == "" {
		return false
	}
	special := string(p.comma) + "\"\r\n"
	if p.comma == ',' {
		special += ";"
	}
	return strings.ContainsAny(f, special) || f[0] == ' ' || f[len(f)-1] == ' '
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
	headers, fieldIdx, _ := (&TablePrinter{wide: true, columns: p.columns}).tableColumns(t)
	if len(headers) == 0 && len(p.columns) > 0 {
		all, _, _ := (&TablePrinter{wide: true}).tableColumns(t)
		if len(all) > 0 {
			return nil, nil, fmt.Errorf("%w: --columns %s matches none of: %s", ErrUnknownColumns, strings.Join(p.columns, ","), strings.Join(all, ", "))
		}
	}
	if len(headers) == 0 {
		return nil, nil, ErrNotTabular
	}
	return headers, fieldIdx, nil
}

// writeRows writes one line per row of v, led by context when it is not "".
func (p *CSVPrinter) writeRows(w *bufio.Writer, context string, v interface{}, fieldIdx []int) {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.Len() == 0 {
		return
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
			cells = append(cells, csvCell(fmt.Sprintf("%v", row.Field(idx).Interface())))
		}
		p.writeRecord(w, cells)
	}
}

// csvCell is a table cell as a csv or tsv field: the table's placeholders
// for "none" or "unknown" ("—", "-") become empty, and formulas are defused.
func csvCell(s string) string {
	if s == "—" || s == "-" {
		return ""
	}
	return csvSafe(s)
}

// negativeNumber is a cell starting with "-" that a spreadsheet reads as a
// number rather than a formula.
var negativeNumber = regexp.MustCompile(`^-[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// csvSafe defuses a cell a spreadsheet would read as a formula (CSV
// injection), by prefixing "'": one starting with = + @ tab or carriage
// return, or with "-" unless it is a plain number. Log content and entity
// names can hold text an attacker chose, and a negative problem ID would
// otherwise show as #NAME?.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	case '-':
		if !negativeNumber.MatchString(s) {
			return "'" + s
		}
	}
	return s
}
