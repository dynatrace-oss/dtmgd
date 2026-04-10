package cmd

import "github.com/spf13/cobra"

// getCmd is the parent for all "get" subcommands.
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "List or retrieve resources",
	Long: `List resources from a Dynatrace Managed environment.

Supported resources:
  environments          Configured Managed environments and cluster info
  problems (prob)       Active and resolved problems
  entities (ent)        Monitored entities (requires --selector)
  entity-types (et)     All available entity types
  events (ev)           Events within a time range
  metrics (met)         Available metric descriptors
  slos                  Service Level Objectives
  security-problems (sp) Security vulnerabilities

Use 'dtmgd get <resource> --help' for resource-specific options.`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(getCmd)
}
