package cmd

import "github.com/spf13/cobra"

// newQueryCmd builds the parent for query subcommands.
func newQueryCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query time-series or log data",
		Long: `Query time-range data from a Dynatrace Managed environment.

Supported subcommands:
  metrics      Query metric time-series data
  logs         Search log records
  log-counts   Count log records per process group and level (INFO/WARN/ERROR)

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
