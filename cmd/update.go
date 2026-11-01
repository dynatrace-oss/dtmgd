package cmd

import "github.com/spf13/cobra"

// newUpdateCmd builds the parent for all "update" subcommands: resources
// that are changed whole from a file, outside apply.
func newUpdateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update resources from a file",
		Long: `Update a resource from a file, for resources apply does not cover.

Supported resources:
  activegate-updates [id]   The cluster ActiveGates' auto-update (cluster contexts)

Writes need a context whose safety-level allows them (see
dtmgd config set-context --help).`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(newUpdateActiveGateUpdatesCmd(a))
	return asGroup(cmd)
}

// markMutating marks a write command that declares its own scopes (a
// cluster command), so the catalog shows it as one.
func markMutating(cmd *cobra.Command) *cobra.Command {
	cmd.Annotations[mutatingAnnotation] = "true"
	return cmd
}
