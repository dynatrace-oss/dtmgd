package cmd

import (
	"os"
	"strings"
	"testing"
)

// readmeCommands returns the README's Commands section.
func readmeCommands(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	section, ok := commandsSection(string(data))
	if !ok {
		t.Fatal("README has no Commands section")
	}
	return section
}

// commandsSection returns the "## Commands" section of a README, up to the
// next heading, with CRLF line endings read as LF.
func commandsSection(readme string) (string, bool) {
	readme = strings.ReplaceAll(readme, "\r\n", "\n")
	_, section, ok := strings.Cut(readme, "\n## Commands\n")
	if !ok {
		return "", false
	}
	section, _, _ = strings.Cut(section, "\n## ")
	return section, true
}

// TestCommandsSectionWithCRLF: the Windows CI runner checks the README out
// with CRLF line endings.
func TestCommandsSectionWithCRLF(t *testing.T) {
	got, ok := commandsSection("# dtmgd\r\n\r\n## Commands\r\n\r\n`get`\r\n\r\n## Contexts\r\n`ctx`\r\n")
	if !ok || !strings.Contains(got, "`get`") || strings.Contains(got, "`ctx`") {
		t.Errorf("commandsSection = %q, %v", got, ok)
	}
}

// TestEveryTopLevelCommandIsInTheREADME keeps the README's verb table in
// step with the command tree, and pins architecture guideline 1: verbs
// beyond the standard get and describe are documented as intentional.
func TestEveryTopLevelCommandIsInTheREADME(t *testing.T) {
	section := readmeCommands(t)
	for _, c := range newRootCmd(&App{}).Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		if !strings.Contains(section, "`"+c.Name()+"`") {
			t.Errorf("the README's Commands section does not mention `%s`", c.Name())
		}
	}
	if !strings.Contains(section, "intentional extensions") {
		t.Error("the README does not say which verbs are intentional extensions of the portfolio's grammar")
	}
}
