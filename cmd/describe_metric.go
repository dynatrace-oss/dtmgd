package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func newDescribeMetricCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "metric <metric-id>",
		Aliases: []string{"met"},
		Short:   "Show detailed information about a specific metric descriptor",
		Long: `Show all metadata for a metric including aggregation types, dimensions,
impact maps, and display units.

Example:
  dtmgd describe metric builtin:service.response.time`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "metric",
				Fetch:    describeFetch(fmt.Sprintf("/metrics/%s", url.PathEscape(args[0])), nil),
			})
		},
	}
	return withScopes(cmd, scopeMetricsRead)
}
