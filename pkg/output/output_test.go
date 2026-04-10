package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDetectAgent(t *testing.T) {
	// Clean state
	for _, v := range AgentEnvVars {
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
	p := &AgentPrinter{w: &buf, resource: "problems"}
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
