package cmd

import "github.com/spf13/cobra"

// queryCmd is the parent for query subcommands.
var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query time-series or log data",
	Long: `Query time-range data from a Dynatrace Managed environment.

Supported subcommands:
  metrics   Query metric time-series data
  logs      Search log records

Use 'dtmgd query <subcommand> --help' for details.`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(queryCmd)
}
