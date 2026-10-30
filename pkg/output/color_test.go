package output

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// withPresentation sets the stderr helpers' writer, plain mode and colors
// for one test and returns the stderr buffer.
func withPresentation(t *testing.T, plainMode, stdoutColor, stderrColor bool) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	SetStderr(&buf)
	SetPlain(plainMode)
	SetColor(stdoutColor, stderrColor)
	t.Cleanup(func() { SetStderr(nil); SetPlain(false); SetColor(false, false) })
	return &buf
}

func printAllPrefixes() {
	PrintHumanError("x")
	PrintWarning("y")
	PrintSuccess("z")
	PrintHint("h")
}

func TestStderrGlyphsAreColored(t *testing.T) {
	buf := withPresentation(t, false, false, true)
	printAllPrefixes()
	want := "\x1b[1;31m✗\x1b[0m x\n\x1b[1;33m⚠\x1b[0m y\n\x1b[1;32m✓\x1b[0m z\n💡 h\n"
	if got := buf.String(); got != want {
		t.Errorf("stderr = %q\nwant     %q", got, want)
	}
}

func TestStderrWithoutColorKeepsPlainGlyphs(t *testing.T) {
	buf := withPresentation(t, false, true, false)
	printAllPrefixes()
	if got, want := buf.String(), "✗ x\n⚠ y\n✓ z\n💡 h\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestPlainModeWinsOverColor(t *testing.T) {
	buf := withPresentation(t, true, true, true)
	printAllPrefixes()
	if got, want := buf.String(), "Error: x\nWarning: y\nz\nHint: h\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

type colorRow struct {
	Name   string `table:"NAME"`
	Status string `table:"STATUS,status"`
	Note   string `table:"NOTE"`
}

var colorRows = []colorRow{
	{"alpha", "OPEN", "x"},
	{"b", "ok", "a longer note"},
	{"charlie-long-name", "WARNING", ""},
	{"d", "UNKNOWN", "y"},
	{"e", "resolved", "z"},
}

func render(t *testing.T, format string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := NewPrinterTo(format, &buf).PrintList(colorRows); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestTableColorsHeadersAndStatus(t *testing.T) {
	withPresentation(t, false, true, false)
	out := render(t, "table")
	for _, want := range []string{
		"\x1b[1mNAME\x1b[0m",
		"\x1b[31mOPEN\x1b[0m",
		"\x1b[32mok\x1b[0m",
		"\x1b[33mWARNING\x1b[0m",
		"\x1b[32mresolved\x1b[0m",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%q", want, out)
		}
	}
	if strings.Contains(out, "mUNKNOWN") || strings.Contains(out, "\x1b[31mx") {
		t.Errorf("an unknown status or a non-status column was colored:\n%q", out)
	}
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestColoredTableKeepsItsAlignment pins that escape codes take no width:
// without them, a colored table is byte for byte the uncolored one.
func TestColoredTableKeepsItsAlignment(t *testing.T) {
	withPresentation(t, false, false, false)
	plainTable := render(t, "table")
	SetColor(true, false)
	colored := render(t, "table")
	if colored == plainTable {
		t.Fatal("colors are on but the table has none")
	}
	if got := ansi.ReplaceAllString(colored, ""); got != plainTable {
		t.Errorf("colored table misaligned:\n%s\nwant\n%s", got, plainTable)
	}
}

func TestOnlyTablesAreColored(t *testing.T) {
	withPresentation(t, false, true, true)
	for _, format := range []string{"csv", "tsv", "json", "yaml"} {
		if out := render(t, format); strings.Contains(out, "\x1b") {
			t.Errorf("%s output has escape codes:\n%q", format, out)
		}
	}
}

// TestEnableVTAcceptsWhatIsNotAConsole pins that only a console is ever
// prepared: a buffer or a file (FORCE_COLOR into a pipe or a file) can take
// escape codes as they are.
func TestEnableVTAcceptsWhatIsNotAConsole(t *testing.T) {
	if !EnableVT(&bytes.Buffer{}) {
		t.Error("EnableVT refused a buffer")
	}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !EnableVT(f) {
		t.Error("EnableVT refused a regular file")
	}
}
