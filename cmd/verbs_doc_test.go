package cmd

import (
	"os"
	"strings"
	"testing"
)

// readmeCommands returns the README's Commands section, up to the next
// heading.
func readmeCommands(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(data), "\n## Commands\n")
	if !ok {
		t.Fatal("README has no Commands section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	return section
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
