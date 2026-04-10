package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show dtmgd version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dtmgd %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
