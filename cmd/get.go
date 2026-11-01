package cmd

import "github.com/spf13/cobra"

// newGetCmd builds the parent for all "get" subcommands.
func newGetCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List or retrieve resources",
		Long: `List resources from a Dynatrace Managed environment.

Supported resources:
  problems (prob)       Active and resolved problems
  entities (ent)        Monitored entities (requires --selector)
  entity-types (et)     All available entity types
  events (ev)           Events within a time range
  metrics (met)         Available metric descriptors
  slos                  Service Level Objectives
  security-problems (sp) Security vulnerabilities
  settings-schemas (schemas) Settings 2.0 schemas
  settings              Settings 2.0 objects (requires --schema or --scope)

Cluster resources (need a cluster context: one without env-id):
  environments (envs)   The cluster's environments
  nodes (node)          The cluster's nodes, their state, version and roles
  license-usage         Each environment's total billed license usage
  activegate-updates    How the cluster's ActiveGates update automatically

Use 'dtmgd get <resource> --help' for resource-specific options.`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(
		newGetActiveGateUpdatesCmd(a),
		newGetEntityTypesCmd(a),
		newGetEntitiesCmd(a),
		newGetEnvironmentsCmd(a),
		newGetEventsCmd(a),
		newGetLicenseUsageCmd(a),
		newGetMetricsCmd(a),
		newGetNodesCmd(a),
		newGetProblemsCmd(a),
		newGetSecurityProblemsCmd(a),
		newGetSettingsCmd(a),
		newGetSettingsSchemasCmd(a),
		newGetSLOsCmd(a),
	)
	return asGroup(cmd)
}
