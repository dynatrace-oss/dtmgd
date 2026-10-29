package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// backupConfigPath is the Cluster API's backup configuration, relative to
// {host}/api. /backup/config/status is not used: it reports the status of a
// configuration change request, not of backups.
const backupConfigPath = "/v1.0/onpremise/backup/config"

// backupConfigOnly is what the Cluster API cannot tell about backups.
const backupConfigOnly = "The Cluster API reports the backup configuration only. It has no last-backup time and no backup health; check those in the Cluster Management Console."

// BackupDetail is the backup configuration describe backup's summary shows.
// Pointers tell an absent field from false or 0.
type BackupDetail struct {
	Enabled               *bool  `json:"enabled"`
	PauseBackups          *bool  `json:"pauseBackups"`
	StoragePath           string `json:"storagePath"`
	Datacenter            string `json:"datacenter"`
	CassandraHour         *int   `json:"cassandraScheduledTime"`
	BandwidthLimitMbits   *int   `json:"bandwidthLimitMbits"`
	IncludeTsMetricData   *bool  `json:"includeTsMetricData"`
	IncludeRumData        *bool  `json:"includeRumData"`
	IncludeLm20Data       *bool  `json:"includeLm20Data"`
	MaxEsSnapshotsToClean *int   `json:"maxEsSnapshotsToClean"`
}

func newDescribeBackupCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "backup",
		Args:    noArgs,
		Aliases: []string{"backups"},
		Short:   "Show a Dynatrace Managed cluster's backup configuration (cluster context)",
		Long: `Show the cluster's backup configuration from the Cluster Management API:
whether backups are enabled or paused, where they go, when Cassandra backups
start, and what they include. Needs a cluster context: one without env-id,
whose token has the ServiceProviderAPI scope.

` + backupConfigOnly + `

Examples:
  dtmgd describe backup --context prod-cluster
  dtmgd describe backup -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runClusterFetch(fetchSpec{
				Verb:     "describe",
				Resource: "backup",
				Fetch:    clusterObjectFetch(backupConfigPath, nil),
				Human: func(w io.Writer, body interface{}) error {
					if err := detailHuman(printBackupDetail)(w, body); err != nil {
						return err
					}
					output.PrintInfo("%s", backupConfigOnly)
					return nil
				},
			})
		},
	}
	return clusterCommand(cmd)
}

func printBackupDetail(w io.Writer, d BackupDetail) {
	line := func(label, value string) {
		if value != "" {
			_, _ = fmt.Fprintf(w, "%-23s %s\n", label+":", value)
		}
	}
	line("Enabled", optYesNo(d.Enabled))
	line("Paused", optYesNo(d.PauseBackups))
	line("Storage path", d.StoragePath)
	line("Datacenter", d.Datacenter)
	if d.CassandraHour != nil {
		line("Cassandra backup start", fmt.Sprintf("%02d:00 (hour of day, cluster time)", *d.CassandraHour))
	}
	if d.BandwidthLimitMbits != nil {
		line("Bandwidth limit", fmt.Sprintf("%d Mbit/s", *d.BandwidthLimitMbits))
	}
	line("Includes", backupIncludes(d))
	if d.MaxEsSnapshotsToClean != nil {
		_, _ = fmt.Fprintf(w, "Elasticsearch snapshots to clean: %d\n", *d.MaxEsSnapshotsToClean)
	}
}

// backupIncludes lists which optional data the backups include, or "" when
// the configuration says nothing about it.
func backupIncludes(d BackupDetail) string {
	var parts []string
	for _, inc := range []struct {
		what string
		v    *bool
	}{
		{"time-series metric data", d.IncludeTsMetricData},
		{"user sessions", d.IncludeRumData},
		{"Log Monitoring v2 data", d.IncludeLm20Data},
	} {
		if inc.v != nil {
			parts = append(parts, inc.what+": "+yesNo(*inc.v))
		}
	}
	return strings.Join(parts, ", ")
}

// optYesNo is yesNo(*b), or "" for nil.
func optYesNo(b *bool) string {
	if b == nil {
		return ""
	}
	return yesNo(*b)
}
