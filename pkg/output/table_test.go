package output

import (
	"bytes"
	"strings"
	"testing"
)

type testRow struct {
	Name   string `table:"NAME"`
	Status string `table:"STATUS"`
	Detail string `table:"DETAIL,wide"`
}

func TestTablePrinterBasic(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	rows := []testRow{
		{Name: "alpha", Status: "OK", Detail: "d1"},
		{Name: "beta", Status: "FAIL", Detail: "d2"},
	}
	p.PrintList(rows)

	out := buf.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") {
		t.Error("should contain headers")
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Error("should contain row data")
	}
	// Detail is wide-only, should NOT appear
	if strings.Contains(out, "DETAIL") {
		t.Error("wide column should not appear in normal mode")
	}
}

func TestTablePrinterWide(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf, wide: true}
	rows := []testRow{{Name: "alpha", Status: "OK", Detail: "d1"}}
	p.PrintList(rows)

	if !strings.Contains(buf.String(), "DETAIL") {
		t.Error("wide column should appear in wide mode")
	}
}

func TestTablePrinterColumns(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf, columns: []string{"NAME"}}
	rows := []testRow{{Name: "alpha", Status: "OK", Detail: "d1"}}
	p.PrintList(rows)

	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Error("requested column should appear")
	}
	if strings.Contains(out, "STATUS") {
		t.Error("non-requested column should not appear")
	}
}

func TestTablePrinterColumnsShowsWide(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf, columns: []string{"DETAIL"}}
	rows := []testRow{{Name: "alpha", Status: "OK", Detail: "d1"}}
	p.PrintList(rows)

	out := buf.String()
	if !strings.Contains(out, "DETAIL") {
		t.Error("explicitly requested wide column should appear")
	}
	if strings.Contains(out, "NAME") {
		t.Error("non-requested column should not appear")
	}
}

func TestTablePrinterEmpty(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	p.PrintList([]testRow{})

	if !strings.Contains(buf.String(), "No items") {
		t.Error("should show empty message")
	}
}

func TestTablePrinterPrintSingleStruct(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	p.Print(testRow{Name: "gamma", Status: "OK", Detail: "d3"})
	out := buf.String()
	if !strings.Contains(out, "gamma") {
		t.Errorf("single struct should appear in table: %s", out)
	}
}

func TestTablePrinterPrimitiveSlice(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	p.Print([]string{"item1", "item2"})
	out := buf.String()
	if !strings.Contains(out, "item1") || !strings.Contains(out, "item2") {
		t.Errorf("primitive slice items should appear: %s", out)
	}
}

type noTagRow struct {
	Name   string
	Status string
}

func TestTablePrinterNoTagsFallback(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	rows := []noTagRow{{Name: "x", Status: "y"}}
	p.PrintList(rows) // no `table:` tags → fallback to fmt.Fprintln(w, v)
	// Should not panic; may print struct or fallback
}

func TestTablePrinterColumnsNoMatch(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf, columns: []string{"NONEXISTENT"}}
	rows := []testRow{{Name: "alpha", Status: "OK"}}
	p.PrintList(rows)
	// No headers match → falls through to fmt.Fprintln fallback
}

type hyphenatedRow struct {
	ProblemID string `table:"PROBLEM-ID"`
	DisplayID string `table:"DISPLAY-ID"`
}

// Headers containing hyphens must render verbatim. tablewriter's AutoFormat
// camel-splits and rejoins with spaces ("PROBLEM-ID" → "PROBLEM - ID"), and it
// defaults to On at the moment WithHeader is applied — so WithHeaderAutoFormat
// has to be passed before WithHeader.
func TestTablePrinterHyphenatedHeaders(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	rows := []hyphenatedRow{{ProblemID: "123_456V2", DisplayID: "P-1001"}}
	if err := p.PrintList(rows); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"PROBLEM-ID", "DISPLAY-ID"} {
		if !strings.Contains(out, want) {
			t.Errorf("header %q missing from output:\n%s", want, out)
		}
	}
	if strings.Contains(out, " - ID") {
		t.Errorf("header hyphen was padded with spaces:\n%s", out)
	}
}

// TestTableFallbackRendersYAML pins that values the table printer cannot lay
// out as rows (maps, scalars, untagged structs) come out as YAML, never as
// Go's map[...] syntax.
func TestTableFallbackRendersYAML(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{w: &buf}
	v := map[string]interface{}{"entityId": "HOST-1", "properties": map[string]interface{}{"a": 1}}
	if err := p.PrintList(v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "entityId: HOST-1") || strings.Contains(buf.String(), "map[") {
		t.Errorf("want YAML, got:\n%s", buf.String())
	}
}

// shapeRow has a column of each kind: skipped ("-" and untagged), a
// non-string value, a wide column whose tag option is upper case, and an
// unknown tag option, which leaves a column ordinary.
type shapeRow struct {
	Name   string `table:"NAME"`
	Skip   string `table:"-"`
	Untag  string
	Count  int    `table:"COUNT"`
	Detail string `table:"DETAIL,WIDE"`
	Note   string `table:"NOTE,extra"`
}

// TestTablePrinterExactOutput pins PrintList's exact output for every shape
// it accepts: a slice of structs or of pointers to them, a pointer to such a
// slice, a struct or a pointer to one. Columns follow the struct's field
// order whatever order --columns names them in; --columns matches headers
// case-insensitively and shows a wide column it names; with no column left,
// the original value is printed as YAML.
func TestTablePrinterExactOutput(t *testing.T) {
	rows := []shapeRow{{Name: "a", Skip: "s", Untag: "u", Count: 1, Detail: "d", Note: "n"}, {Name: "b", Count: 2}}
	shapes := map[string]interface{}{
		"slice":             rows,
		"pointer to slice":  &rows,
		"slice of pointers": []*shapeRow{&rows[0], &rows[1]},
	}
	single := map[string]interface{}{"struct": rows[0], "pointer to struct": &rows[0]}

	cases := []struct {
		name string
		p    TablePrinter
		want string // for both rows; the single-row shapes print its first two lines
	}{
		{"default", TablePrinter{}, " NAME  COUNT  NOTE \n a     1      n    \n b     2           \n"},
		{"wide", TablePrinter{wide: true}, " NAME  COUNT  DETAIL  NOTE \n a     1      d       n    \n b     2                   \n"},
		{"columns in another order", TablePrinter{columns: []string{"count", "name"}}, " NAME  COUNT \n a     1     \n b     2     \n"},
		{"columns name a wide one", TablePrinter{columns: []string{"detail"}}, " DETAIL \n d      \n        \n"},
		{"columns with wide", TablePrinter{wide: true, columns: []string{"NOTE"}}, " NOTE \n n    \n      \n"},
	}
	for _, tc := range cases {
		for shape, v := range shapes {
			var buf bytes.Buffer
			p := tc.p
			p.w = &buf
			if err := p.PrintList(v); err != nil || buf.String() != tc.want {
				t.Errorf("%s, %s: err %v\n--- want\n%s--- got\n%s", tc.name, shape, err, tc.want, buf.String())
			}
		}
		lines := strings.SplitAfter(tc.want, "\n")
		wantOne := lines[0] + lines[1]
		for shape, v := range single {
			var buf bytes.Buffer
			p := tc.p
			p.w = &buf
			if err := p.PrintList(v); err != nil || buf.String() != wantOne {
				t.Errorf("%s, %s: err %v\n--- want\n%s--- got\n%s", tc.name, shape, err, wantOne, buf.String())
			}
		}
	}

	for shape, v := range map[string]interface{}{"empty slice of pointers": []*shapeRow{}, "pointer to empty slice": &[]shapeRow{}} {
		var buf bytes.Buffer
		if err := (&TablePrinter{w: &buf}).PrintList(v); err != nil || buf.String() != "No items found.\n" {
			t.Errorf("%s: err %v, output %q", shape, err, buf.String())
		}
	}

	for shape, v := range map[string]interface{}{"no column matches": shapes["slice of pointers"], "pointer to scalars": &[]string{"x"}} {
		var want, got bytes.Buffer
		if err := (&YAMLPrinter{w: &want}).Print(v); err != nil {
			t.Fatal(err)
		}
		if err := (&TablePrinter{w: &got, columns: []string{"NONE"}}).PrintList(v); err != nil || got.String() != want.String() {
			t.Errorf("%s: err %v\n--- want\n%s--- got\n%s", shape, err, want.String(), got.String())
		}
	}
}
