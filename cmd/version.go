package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/version"
)

func newVersionCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show dtmgd version",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintf(a.Out, "dtmgd %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
		},
	}
}
