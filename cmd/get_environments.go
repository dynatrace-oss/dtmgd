package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// EnvListItem is the table row for environments.
type EnvListItem struct {
	Name    string `table:"NAME"`
	Host    string `table:"HOST"`
	EnvID   string `table:"ENV-ID"`
	Version string `table:"VERSION"`
	Status  string `table:"STATUS"`
}

// envDeprecationNote announces that the connectivity check moves to doctor.
const envDeprecationNote = `Note: "get environments" will list environments from the Cluster API in a coming release. Use "dtmgd doctor" for this connectivity check.`

func newGetEnvironmentsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "environments",
		Args:    noArgs,
		Aliases: []string{"envs", "env"},
		Short:   "List configured Dynatrace Managed environments",
		Long: `List all contexts and verify connectivity to each Dynatrace Managed cluster.

Fetches the cluster version to confirm that the API token and network path are working.

This connectivity check is now "dtmgd doctor". In a coming release,
"get environments" will list environments from the Cluster API instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintInfo("%s", envDeprecationNote)
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Contexts) == 0 {
				output.PrintInfo("No contexts configured. Run 'dtmgd config set-context' to add one.")
				return nil
			}

			items := make([]EnvListItem, 0, len(cfg.Contexts))
			for _, nc := range cfg.Contexts {
				items = append(items, envListItem(a.checkContext(cfg, nc)))
			}
			return a.printer("environments").PrintList(items)
		},
	}
	return withScopes(cmd, scopeDataExport)
}

// envListItem is a doctor row in the shape get environments has always
// printed: OK, or what went wrong, in STATUS, and "—" for a missing version.
func envListItem(r DoctorRow) EnvListItem {
	item := EnvListItem{Name: r.Context, Host: r.Host, EnvID: r.EnvID, Version: r.Version, Status: "OK"}
	if item.Version == "" {
		item.Version = "—"
	}
	switch r.Status {
	case doctorWarn:
		item.Status = "warn: " + r.Detail
	case doctorFail:
		item.Status = r.Detail
	}
	return item
}
