package cmd

import "github.com/spf13/cobra"

// describeCmd is the parent for all "describe" subcommands.
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show detailed information about a specific resource",
	Long: `Show detailed information about a resource by its ID.

Supported resources:
  problem <id>              Detailed problem with evidence and affected entities
  entity <id>               Entity details with properties and tags
  entity-type <type>        Entity type schema and properties
  entity-relations <id>     Relationships of an entity
  event <id>                Event details
  metric <id>               Metric descriptor details
  slo <id>                  SLO details with evaluation data
  security-problem <id>     Security vulnerability details with CVE info

Output defaults to JSON for detail commands (use -o yaml for YAML).`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
