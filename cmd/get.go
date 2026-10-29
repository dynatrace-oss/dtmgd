package cmd

import "github.com/spf13/cobra"

// newGetCmd builds the parent for all "get" subcommands.
func newGetCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.AddCommand(
		newGetEntityTypesCmd(a),
		newGetEntitiesCmd(a),
		newGetEnvironmentsCmd(a),
		newGetEventsCmd(a),
		newGetMetricsCmd(a),
		newGetProblemsCmd(a),
		newGetSecurityProblemsCmd(a),
		newGetSLOsCmd(a),
	)
	return asGroup(cmd)
}
