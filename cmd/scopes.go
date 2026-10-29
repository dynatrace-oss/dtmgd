package cmd

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// scopesAnnotation is the cobra annotation holding the API token scopes a
// command needs, comma-separated. It is the single source for the 403 hint,
// the README's scope list (checked by a test) and the commands catalog.
const scopesAnnotation = "dtmgd.io/scopes"

// scopesPlaceholder in a command's Long help is replaced with the full scope
// list once the tree is built (see fillScopeHelp).
const scopesPlaceholder = "%SCOPES%"

// fillScopeHelp replaces scopesPlaceholder in every command's help with the
// scopes declared under root, so help text cannot drift from the commands.
func fillScopeHelp(root *cobra.Command) {
	list := strings.Join(allScopes(root), ", ")
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Long = strings.ReplaceAll(c.Long, scopesPlaceholder, list)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// withScopes records the token scopes cmd needs and returns cmd.
func withScopes(cmd *cobra.Command, scopes ...string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[scopesAnnotation] = strings.Join(scopes, ",")
	return cmd
}

// requiredScopes returns the token scopes cmd declared.
func requiredScopes(cmd *cobra.Command) []string {
	if cmd == nil || cmd.Annotations[scopesAnnotation] == "" {
		return nil
	}
	return strings.Split(cmd.Annotations[scopesAnnotation], ",")
}

// allScopes returns every scope declared anywhere under root, sorted and
// without duplicates.
func allScopes(root *cobra.Command) []string {
	seen := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range requiredScopes(c) {
			seen[s] = true
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
