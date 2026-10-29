package cmd

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// TestChooseAgentNonInteractive pins pentest note 5's second half: without a
// terminal, an environment variable alone must not pick the directory a skill
// is installed into. An explicit --for, or --cross-client (which does not
// depend on detection), still works.
func TestChooseAgentNonInteractive(t *testing.T) {
	for _, v := range output.AgentEnvVars {
		t.Setenv(v, "")
	}
	t.Setenv("CLAUDECODE", "1")

	if _, err := chooseAgent("", false, false); err == nil || !strings.Contains(err.Error(), "--for claude") {
		t.Errorf("want --for required naming claude, got %v", err)
	}
	if a, err := chooseAgent("", false, true); err != nil || a.Name != "claude" {
		t.Errorf("interactive: got %v, %v", a, err)
	}
	if _, err := chooseAgent("", true, false); err != nil {
		t.Errorf("--cross-client must not need --for: %v", err)
	}
	if a, err := chooseAgent("cursor", false, false); err != nil || a.Name != "cursor" {
		t.Errorf("explicit --for: got %v, %v", a, err)
	}
}
