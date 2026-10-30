package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type csvItem struct {
	ID    string `table:"ID"`
	Name  string `table:"NAME"`
	Note  string `table:"NOTE,wide"`
	Count int    `table:"COUNT"`
	Inner string // no tag: never a column
}

func printCSV(t *testing.T, comma rune, columns []string, v interface{}) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := (&CSVPrinter{w: &buf, comma: comma, columns: columns}).PrintList(v)
	return buf.String(), err
}

func TestCSVPrinterWritesEveryColumnWithAHeader(t *testing.T) {
	// Wide columns too: a spreadsheet has room for them. Commas, quotes and
	// line breaks are quoted per RFC 4180.
	got, err := printCSV(t, ',', nil, []csvItem{
		{ID: "a", Name: "plain", Note: "n", Count: 1},
		{ID: "b", Name: `has, comma and "quotes"`, Note: "two\nlines", Count: 2},
	})
	want := "ID,NAME,NOTE,COUNT\na,plain,n,1\nb,\"has, comma and \"\"quotes\"\"\",\"two\nlines\",2\n"
	if err != nil || got != want {
		t.Errorf("got %q, %v\nwant %q", got, err, want)
	}
}

func TestCSVPrinterHonoursColumns(t *testing.T) {
	got, err := printCSV(t, ',', []string{"count", "id"}, []csvItem{{ID: "a", Name: "x", Count: 3}})
	if want := "ID,COUNT\na,3\n"; err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestCSVPrinterWritesTheHeaderForAnEmptyList(t *testing.T) {
	got, err := printCSV(t, ',', nil, []csvItem{})
	if want := "ID,NAME,NOTE,COUNT\n"; err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestTSVUsesTabs(t *testing.T) {
	got, err := printCSV(t, '\t', nil, []csvItem{{ID: "a", Name: "b c", Count: 1}})
	if want := "ID\tNAME\tNOTE\tCOUNT\na\tb c\t\t1\n"; err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestCSVPrinterRefusesValuesWithoutColumns(t *testing.T) {
	for _, v := range []interface{}{map[string]string{"a": "b"}, []string{"x"}, "scalar"} {
		if _, err := printCSV(t, ',', nil, v); !errors.Is(err, ErrNotTabular) {
			t.Errorf("%T: err = %v, want ErrNotTabular", v, err)
		}
	}
}

// Log content and entity names can carry text an attacker chose; a
// spreadsheet would run a cell starting with = + @ tab or CR, or a - that is
// not a plain number, as a formula. Plain negative numbers stay numbers.
func TestCSVCellsThatLookLikeFormulasAreDefused(t *testing.T) {
	cases := map[string]string{
		"=1+1":                   "'=1+1",
		"+cmd":                   "'+cmd",
		"@SUM(A1)":               "'@SUM(A1)",
		"-2+3+cmd|' /C calc'!A0": "'-2+3+cmd|' /C calc'!A0",
		"\t=x":                   "'\t=x",
		"\r=x":                   "'\r=x",
		"-A1":                    "'-A1",
		"-1.5":                   "-1.5",
		"-42":                    "-42",
		"-1e3":                   "-1e3",
		// Not a number, so a spreadsheet would read a formula (#NAME?); as
		// text it shows as the ID.
		"-6546711275898328738_1776193140000V2": "'-6546711275898328738_1776193140000V2",
		"plain":                                "plain",
		"":                                     "",
	}
	for in, want := range cases {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

// In a locale whose list separator is ";" (German, French…), a spreadsheet
// splits an unquoted cell at ";", and the part after it could start with =.
func TestCSVQuotesSemicolons(t *testing.T) {
	got, err := printCSV(t, ',', []string{"name"}, []csvItem{{Name: "x;=cmd"}})
	if want := "NAME\n\"x;=cmd\"\n"; err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestCSVTablePlaceholdersAreEmpty(t *testing.T) {
	got, err := printCSV(t, ',', []string{"id", "name"}, []csvItem{{ID: "—", Name: "-"}})
	if want := "ID,NAME\n,\n"; err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestCSVUnknownColumnsAreNamed(t *testing.T) {
	_, err := printCSV(t, ',', []string{"NOPE"}, []csvItem{{ID: "a"}})
	if err == nil || !strings.Contains(err.Error(), "NOPE") || !strings.Contains(err.Error(), "ID, NAME, NOTE, COUNT") {
		t.Errorf("err = %v", err)
	}
}

// max=N shortens a column to N runes, on one line, in tables only: csv keeps
// the full text.
func TestMaxTagShortensTablesOnly(t *testing.T) {
	type item struct {
		Desc string `table:"DESC,max=8"`
	}
	v := []item{{Desc: "a long\ndescription"}}
	var table bytes.Buffer
	if err := (&TablePrinter{w: &table}).PrintList(v); err != nil || !strings.Contains(table.String(), "a long …") {
		t.Errorf("table: %v\n%s", err, table.String())
	}
	if got, err := printCSV(t, ',', nil, v); err != nil || got != "DESC\n\"a long\ndescription\"\n" {
		t.Errorf("csv: %q, %v", got, err)
	}
}
