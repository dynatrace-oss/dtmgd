package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// newDescribeCmd builds the parent for all "describe" subcommands.
func newDescribeCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Show detailed information about a specific resource",
		Long: `Show detailed information about a resource by its ID.

Supported resources:
  problem <id>              Detailed problem with evidence and affected entities
  entity <id>               Entity details with properties and tags
  entity-type <type>        Entity type schema and properties
  entity-relations <id>     Relationships of an entity
  event <id>                Event details
  metric <id>               Metric descriptor details
  slo <id>                  SLO details with evaluation data
  security-problem <id>     Security vulnerability details with CVE info

Cluster resources (need a cluster context: one without env-id):
  environment <id>          An environment's properties, quotas and storage

Detail commands print a readable summary (or YAML) by default; use -o json for the raw API object.`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(
		newDescribeEntityCmd(a),
		newDescribeEntityTypeCmd(a),
		newDescribeEntityRelationsCmd(a),
		newDescribeEnvironmentCmd(a),
		newDescribeEventCmd(a),
		newDescribeMetricCmd(a),
		newDescribeProblemCmd(a),
		newDescribeSecurityProblemCmd(a),
		newDescribeSLOCmd(a),
	)
	return asGroup(cmd)
}

// describeFetch is the Fetch of a v2 endpoint that returns one object. A
// single object has no total, and no limit applies to it.
func describeFetch(path string, params map[string]string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, _ int) (fetched, error) {
		var m map[string]interface{}
		if err := c.GetV2(path, params, &m); err != nil {
			return fetched{}, err
		}
		return fetched{Body: m, Total: -1}, nil
	}
}

// detailHuman adapts a summary printer for a typed detail (ProblemDetail,
// SLODetail, …) to fetchSpec.Human: it decodes the API object into T first.
func detailHuman[T any](render func(io.Writer, T)) func(io.Writer, interface{}) error {
	return func(w io.Writer, body interface{}) error {
		var d T
		if err := client.DecodePaged(body.(map[string]interface{}), &d); err != nil {
			return err
		}
		render(w, d)
		return nil
	}
}
