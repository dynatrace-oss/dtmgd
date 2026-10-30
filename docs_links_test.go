package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

var (
	mdLink     = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	codeSpan   = regexp.MustCompile("`[^`]*`")
	mdHeading  = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	fenceStart = regexp.MustCompile("^\\s*(```|~~~)")
)

// markdownLines returns md's lines outside fenced code blocks.
func markdownLines(md string) []string {
	var lines []string
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		if fenceStart.MatchString(line) {
			inFence = !inFence
			continue
		}
		if !inFence {
			lines = append(lines, line)
		}
	}
	return lines
}

// headingSlugs returns the anchors GitHub gives md's headings: lowercase,
// punctuation dropped except '-' and '_', each space a '-', and -1, -2 … for
// repeats.
func headingSlugs(md string) map[string]bool {
	slugs := map[string]bool{}
	seen := map[string]int{}
	for _, line := range markdownLines(md) {
		m := mdHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var b strings.Builder
		for _, r := range strings.ToLower(m[1]) {
			switch {
			case r == ' ':
				b.WriteRune('-')
			case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
				b.WriteRune(r)
			}
		}
		slug := b.String()
		if n := seen[slug]; n > 0 {
			slugs[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			slugs[slug] = true
		}
		seen[slug]++
	}
	return slugs
}

// checkMarkdownLinks reports each relative link in files (paths relative to
// root) whose target does not exist, or whose #anchor names no heading of
// the target, as "file: target". Links in code blocks and code spans, and
// http(s) and mailto links, are not checked. Neither are forms this
// repository does not use: reference-style links ([text][ref]), titles in
// single quotes or parentheses, and HTML <a href>. A heading that contains a
// link or emphasis gets its slug from the raw text, which differs from
// GitHub's.
func checkMarkdownLinks(root string, files []string) []string {
	var broken []string
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			broken = append(broken, file+": "+err.Error())
			continue
		}
		for _, line := range markdownLines(string(data)) {
			for _, m := range mdLink.FindAllStringSubmatch(codeSpan.ReplaceAllString(line, ""), -1) {
				if target := m[1]; !linkResolves(root, file, string(data), target) {
					broken = append(broken, file+": "+target)
				}
			}
		}
	}
	return broken
}

// linkResolves reports whether target, a link in file (whose content is
// md), points at an existing file and, if it has one, an existing anchor.
func linkResolves(root, file, md, target string) bool {
	for _, scheme := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(target, scheme) {
			return true
		}
	}
	path, anchor, _ := strings.Cut(target, "#")
	if path == "" {
		return anchor == "" || headingSlugs(md)[anchor]
	}
	full := filepath.Join(root, filepath.Dir(file), filepath.FromSlash(path))
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	if anchor == "" || info.IsDir() || !strings.HasSuffix(path, ".md") {
		return true
	}
	data, err := os.ReadFile(full)
	return err == nil && headingSlugs(string(data))[anchor]
}

func TestCheckMarkdownLinksFindsBrokenFilesAndAnchors(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a.md": "# Local Heading\n\n" +
			"[ok](b.md) [missing](c.md) [anchor](b.md#real-heading) [bad anchor](b.md#nope)\n" +
			"[local](#local-heading) [web](https://example.com/x.md) [mail](mailto:a@example.com)\n" +
			"```\n[in code](nowhere.md)\n```\n",
		"b.md": "## Real Heading\n",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := checkMarkdownLinks(root, []string{"a.md", "b.md"})
	if want := []string{"a.md: c.md", "a.md: b.md#nope"}; !reflect.DeepEqual(got, want) {
		t.Errorf("checkMarkdownLinks = %q, want %q", got, want)
	}
}

func TestHeadingSlugsFollowGitHub(t *testing.T) {
	got := headingSlugs("# dtmgd — Dynatrace Managed CLI\n## `get problems`\n## Exit Codes\n## Exit Codes\n```\n## not a heading\n```\n")
	for _, want := range []string{"dtmgd--dynatrace-managed-cli", "get-problems", "exit-codes", "exit-codes-1"} {
		if !got[want] {
			t.Errorf("no slug %q in %v", want, got)
		}
	}
	if got["not-a-heading"] {
		t.Error("a heading inside a code block got a slug")
	}
}

// TestRepositoryMarkdownLinksResolve checks every relative link in the
// repository's Markdown: README, docs/, AGENTS.md, the skill and its
// references, the GitHub templates. Plans and specs under docs/superpowers
// are records of their time and are left out.
func TestRepositoryMarkdownLinksResolve(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	out, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "--", "*.md").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f != "" && !strings.HasPrefix(f, "docs/superpowers/") {
			files = append(files, f)
		}
	}
	for _, broken := range checkMarkdownLinks(".", files) {
		t.Errorf("broken link: %s", broken)
	}
}
