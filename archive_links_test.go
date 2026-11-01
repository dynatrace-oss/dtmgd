package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// archiveFile is one entry of an archive's files list in .goreleaser.yaml:
// a glob, or a {src, dst, strip_parent} object.
type archiveFile struct {
	Src         string `yaml:"src"`
	Dst         string `yaml:"dst"`
	StripParent bool   `yaml:"strip_parent"`
}

func (f *archiveFile) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		f.Src = n.Value
		return nil
	}
	type plain archiveFile
	return n.Decode((*plain)(f))
}

// archiveLayout copies the files the release archive ships into a temporary
// directory, at the paths the archive gives them, and returns it with the
// Markdown files among them.
func archiveLayout(t *testing.T) (root string, markdown []string) {
	t.Helper()
	data, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Archives []struct {
			Files []archiveFile `yaml:"files"`
		} `yaml:"archives"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil || len(cfg.Archives) == 0 {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}
	root = t.TempDir()
	for _, f := range cfg.Archives[0].Files {
		matches, err := filepath.Glob(f.Src)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matches {
			dest := m
			if f.StripParent {
				dest = filepath.Base(m)
			}
			dest = filepath.Join(f.Dst, dest)
			copyFile(t, m, filepath.Join(root, dest))
			if strings.HasSuffix(dest, ".md") {
				markdown = append(markdown, dest)
			}
		}
	}
	return root, markdown
}

// copyFile copies src to dst, creating dst's directory.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseArchiveLinksResolve checks that every relative link in the
// Markdown the release archive ships (README, the docs, the skill) points at
// a file the archive also ships, so a downloaded archive reads offline.
func TestReleaseArchiveLinksResolve(t *testing.T) {
	root, markdown := archiveLayout(t)
	if len(markdown) == 0 {
		t.Fatal("the archive ships no Markdown")
	}
	for _, broken := range checkMarkdownLinks(root, markdown) {
		t.Errorf("broken in the release archive: %s", broken)
	}
}
