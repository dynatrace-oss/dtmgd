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
