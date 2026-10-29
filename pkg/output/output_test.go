package output

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// captureStderr runs fn and returns everything written to os.Stderr.
func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	fn()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = old
	return buf.String()
}

func TestDetectAgent(t *testing.T) {
	// Clean state
	for _, v := range agentmode.EnvVars() {
		os.Unsetenv(v)
	}
	if DetectAgent() {
		t.Error("should be false with no env vars set")
	}

	os.Setenv("KIRO", "1")
	defer os.Unsetenv("KIRO")
	if !DetectAgent() {
		t.Error("should be true with KIRO set")
	}
}

func TestAgentPrinterPrint(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &agentmode.ResponseContext{Resource: "problems"})
	p.Print(map[string]string{"key": "value"})

	out := buf.String()
	if !strings.Contains(out, `"ok": true`) {
		t.Error("should contain ok: true")
	}
	if !strings.Contains(out, `"resource": "problems"`) {
		t.Error("should contain resource")
	}
	if !strings.Contains(out, `"key": "value"`) {
		t.Error("should contain result data")
	}
}

func TestJSONPrinter(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{w: &buf}
	p.Print(map[string]int{"count": 42})

	if !strings.Contains(buf.String(), `"count": 42`) {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestYAMLPrinter(t *testing.T) {
	var buf bytes.Buffer
	p := &YAMLPrinter{w: &buf}
	p.Print(map[string]string{"name": "test"})

	if !strings.Contains(buf.String(), "name: test") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

// --- NewPrinterTo / NewPrinterToWithColumns ---

func TestNewPrinterToJSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterTo("json", &buf)
	_ = p.Print(map[string]int{"count": 5})
	if !strings.Contains(buf.String(), `"count": 5`) {
		t.Errorf("unexpected JSON output: %s", buf.String())
	}
}

func TestNewPrinterToYAML(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterTo("yaml", &buf)
	_ = p.Print(map[string]string{"k": "v"})
	if !strings.Contains(buf.String(), "k: v") {
		t.Errorf("unexpected YAML output: %s", buf.String())
	}
}

func TestNewPrinterToYMLAlias(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterTo("yml", &buf)
	if _, ok := p.(*YAMLPrinter); !ok {
		t.Errorf("expected *YAMLPrinter for 'yml', got %T", p)
	}
}

func TestNewPrinterToTable(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterTo("table", &buf)
	if _, ok := p.(*TablePrinter); !ok {
		t.Errorf("expected *TablePrinter, got %T", p)
	}
}

func TestNewPrinterToWide(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterToWithColumns("wide", &buf, nil)
	tp, ok := p.(*TablePrinter)
	if !ok {
		t.Fatalf("expected *TablePrinter, got %T", p)
	}
	if !tp.wide {
		t.Error("expected wide=true for 'wide' format")
	}
}

func TestNewPrinterToWithColumns(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterToWithColumns("table", &buf, []string{"NAME", "STATUS"})
	tp, ok := p.(*TablePrinter)
	if !ok {
		t.Fatalf("expected *TablePrinter, got %T", p)
	}
	if len(tp.columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(tp.columns))
	}
}

func TestNewPrinterToNilWriter(t *testing.T) {
	// nil writer should default to os.Stdout (no panic).
	p := NewPrinterToWithColumns("json", nil, nil)
	if p == nil {
		t.Error("expected non-nil printer")
	}
}

// --- PrintSuccess / PrintWarning / PrintInfo / PrintHumanError ---

func TestPrintSuccess(t *testing.T) {
	out := captureStderr(func() { PrintSuccess("done %s", "ok") })
	if !strings.Contains(out, "done ok") {
		t.Errorf("missing message in: %q", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("missing checkmark in: %q", out)
	}
}

func TestPrintWarning(t *testing.T) {
	out := captureStderr(func() { PrintWarning("beware %d items", 3) })
	if !strings.Contains(out, "beware 3 items") {
		t.Errorf("missing message in: %q", out)
	}
	if !strings.Contains(out, "⚠") {
		t.Errorf("missing warning symbol in: %q", out)
	}
}

func TestPrintInfo(t *testing.T) {
	out := captureStderr(func() { PrintInfo("resolution: %s", "1m") })
	if !strings.Contains(out, "resolution: 1m") {
		t.Errorf("missing message in: %q", out)
	}
}

func TestPrintHumanError(t *testing.T) {
	out := captureStderr(func() { PrintHumanError("failed: %s", "timeout") })
	if !strings.Contains(out, "failed: timeout") {
		t.Errorf("missing message in: %q", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("missing error symbol in: %q", out)
	}
}

// TestPlainPrefixes pins plain mode: words instead of glyphs, so output read
// by scripts, screen readers or dumb terminals carries no symbols.
func TestPlainPrefixes(t *testing.T) {
	var buf bytes.Buffer
	SetStderr(&buf)
	SetPlain(true)
	t.Cleanup(func() { SetStderr(nil); SetPlain(false) })

	PrintWarning("x")
	PrintHumanError("y")
	PrintHint("z")
	if want := "Warning: x\nError: y\nHint: z\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
