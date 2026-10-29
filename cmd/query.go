package cmd

import "github.com/spf13/cobra"

// newQueryCmd builds the parent for query subcommands.
func newQueryCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query time-series or log data",
		Long: `Query time-range data from a Dynatrace Managed environment.

Supported subcommands:
  metrics   Query metric time-series data
  logs      Search log records

Use 'dtmgd query <subcommand> --help' for details.`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(
		newQueryLogCountsCmd(a),
		newQueryLogsCmd(a),
		newQueryMetricsCmd(a),
	)
	return cmd
}
