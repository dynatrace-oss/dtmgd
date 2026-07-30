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
