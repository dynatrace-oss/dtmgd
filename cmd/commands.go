package cmd

import (
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
	"github.com/dynatrace-oss/dtmgd/pkg/version"
)

// catalogSchemaVersion is raised on any breaking change to the shape of
// `dtmgd commands` output.
const catalogSchemaVersion = 1

// catalogVerbs are the verb-noun verbs whose resources --brief lists.
var catalogVerbs = []string{"get", "describe", "query"}

// groupAnnotation marks a command that only groups subcommands: run alone,
// it reports a missing subcommand or prints its help. The catalog lists the
// subcommands and leaves the group out.
const groupAnnotation = "dtmgd.io/group"

// asGroup marks cmd as a group (see groupAnnotation) and returns it.
func asGroup(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[groupAnnotation] = "true"
	return cmd
}

// catalog is the full `dtmgd commands` output.
type catalog struct {
	SchemaVersion int                    `json:"schema_version" yaml:"schema_version"`
	Tool          string                 `json:"tool" yaml:"tool"`
	Version       string                 `json:"version" yaml:"version"`
	CommandModel  string                 `json:"command_model" yaml:"command_model"`
	GlobalFlags   map[string]catalogFlag `json:"global_flags" yaml:"global_flags"`
	Commands      []catalogCommand       `json:"commands" yaml:"commands"`
}

// briefCatalog is the `dtmgd commands --brief` output.
type briefCatalog struct {
	SchemaVersion int                 `json:"schema_version" yaml:"schema_version"`
	Tool          string              `json:"tool" yaml:"tool"`
	Version       string              `json:"version" yaml:"version"`
	Verbs         map[string][]string `json:"verbs" yaml:"verbs"`
}

// catalogCommand is one runnable command in the catalog.
type catalogCommand struct {
	Path           string                 `json:"path" yaml:"path"`
	Aliases        []string               `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Short          string                 `json:"short" yaml:"short"`
	RequiredScopes []string               `json:"required_scopes,omitempty" yaml:"required_scopes,omitempty"`
	Flags          map[string]catalogFlag `json:"flags,omitempty" yaml:"flags,omitempty"`
}

// catalogFlag describes one flag. Default is the flag's default as cobra
// prints it in --help.
type catalogFlag struct {
	Type        string `json:"type" yaml:"type"`
	Default     string `json:"default" yaml:"default"`
	Description string `json:"description" yaml:"description"`
}

func newCommandsCmd(a *App) *cobra.Command {
	var brief bool
	cmd := &cobra.Command{
		Use:   "commands",
		Short: "Print a machine-readable catalog of dtmgd commands",
		Long: `Print every dtmgd command with its aliases, flags and the API token scopes it
needs, for AI agents and scripts to discover the CLI.

The catalog is JSON (or YAML with -o yaml) and is never wrapped in an agent
envelope. --brief lists only the resources of get, describe and query.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var v interface{}
			if brief {
				v = buildBriefCatalog(cmd.Root())
			} else {
				v = buildCatalog(cmd.Root())
			}
			format := string(output.FormatJSON)
			if a.format() == output.FormatYAML {
				format = string(output.FormatYAML)
			}
			return output.NewPrinterTo(format, a.Out).Print(v)
		},
	}
	cmd.Flags().BoolVar(&brief, "brief", false, "list only the verbs and their resources")
	return cmd
}

// buildCatalog describes every runnable command under root.
func buildCatalog(root *cobra.Command) catalog {
	c := catalog{
		SchemaVersion: catalogSchemaVersion,
		Tool:          root.Name(),
		Version:       version.Version,
		CommandModel:  "verb-noun",
		GlobalFlags:   map[string]catalogFlag{},
		Commands:      []catalogCommand{},
	}
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			c.GlobalFlags["--"+f.Name] = describeFlag(f)
		}
	})
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			if !listed(sub) {
				continue
			}
			if sub.Runnable() && sub.Annotations[groupAnnotation] == "" {
				c.Commands = append(c.Commands, describeCommand(sub))
			}
			walk(sub)
		}
	}
	walk(root)
	sort.Slice(c.Commands, func(i, j int) bool { return c.Commands[i].Path < c.Commands[j].Path })
	return c
}

// buildBriefCatalog lists the resources of each verb-noun verb, sorted.
func buildBriefCatalog(root *cobra.Command) briefCatalog {
	b := briefCatalog{
		SchemaVersion: catalogSchemaVersion,
		Tool:          root.Name(),
		Version:       version.Version,
		Verbs:         map[string][]string{},
	}
	for _, verb := range root.Commands() {
		if !slices.Contains(catalogVerbs, verb.Name()) || !listed(verb) {
			continue
		}
		names := []string{}
		for _, sub := range verb.Commands() {
			if listed(sub) && sub.Runnable() {
				names = append(names, sub.Name())
			}
		}
		sort.Strings(names)
		b.Verbs[verb.Name()] = names
	}
	return b
}

// listed reports whether cmd belongs in the catalog at all: not hidden, and
// not cobra's help, the completion generator or the catalog itself.
func listed(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "help", "completion", "commands":
		return false
	}
	return !cmd.Hidden
}

// describeCommand is cmd's catalog entry, with only the flags it declares
// itself (the global ones are listed once, under global_flags).
func describeCommand(cmd *cobra.Command) catalogCommand {
	e := catalogCommand{
		Path:           strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "),
		Aliases:        cmd.Aliases,
		Short:          cmd.Short,
		RequiredScopes: requiredScopes(cmd),
	}
	visitOwnFlags(cmd, func(f *pflag.Flag) {
		if e.Flags == nil {
			e.Flags = map[string]catalogFlag{}
		}
		e.Flags["--"+f.Name] = describeFlag(f)
	})
	return e
}

// visitOwnFlags calls fn for each visible flag cmd declares itself.
//
// It reads Flags and PersistentFlags directly instead of cobra's LocalFlags,
// which merges the parents' persistent flags into cmd as a side effect, and
// skips cobra's help flag, which cobra adds only to the command it runs.
func visitOwnFlags(cmd *cobra.Command, fn func(*pflag.Flag)) {
	seen := map[string]bool{}
	visit := func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" || seen[f.Name] || inheritedFlag(cmd, f) {
			return
		}
		seen[f.Name] = true
		fn(f)
	}
	cmd.Flags().VisitAll(visit)
	cmd.PersistentFlags().VisitAll(visit)
}

// inheritedFlag reports whether f is an ancestor's persistent flag rather than
// one cmd declares itself. Only the nearest ancestor declaring the name
// counts, since that is the flag cobra would merge in.
func inheritedFlag(cmd *cobra.Command, f *pflag.Flag) bool {
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		if pf := p.PersistentFlags().Lookup(f.Name); pf != nil {
			return pf == f
		}
	}
	return false
}

// describeFlag is f's catalog entry, with the type names dtctl's catalog uses.
func describeFlag(f *pflag.Flag) catalogFlag {
	t := f.Value.Type()
	switch t {
	case "bool":
		t = "boolean"
	case "int", "int32", "int64", "count":
		t = "integer"
	case "float32", "float64":
		t = "number"
	case "stringSlice", "stringArray":
		t = "string[]"
	}
	return catalogFlag{Type: t, Default: f.DefValue, Description: f.Usage}
}
