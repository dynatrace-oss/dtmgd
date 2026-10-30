package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/version"
)

// versionInfo is what dtmgd version reports in json, yaml, csv and tsv, and
// as the agent envelope's result.
type versionInfo struct {
	Version string `json:"version" yaml:"version" table:"VERSION"`
	Commit  string `json:"commit" yaml:"commit" table:"COMMIT"`
	Date    string `json:"date" yaml:"date" table:"BUILT"`
}

func newVersionCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show dtmgd version",
		Long: `Show dtmgd's version, commit and build date.

With -o json, -o yaml, -o csv or -o tsv they are fields (version, commit,
date); in agent mode they are the envelope's result.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := versionInfo{Version: version.Version, Commit: version.Commit, Date: version.Date}
			if a.dataOutput() {
				return a.printer("version").Print(info)
			}
			_, err := fmt.Fprintf(a.Out, "dtmgd %s (commit: %s, built: %s)\n", info.Version, info.Commit, info.Date)
			return err
		},
	}
}
