package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func newDescribeEventCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "event <event-id>",
		Aliases: []string{"ev"},
		Short:   "Show detailed information about a specific event",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "event",
				Fetch:    describeFetch(fmt.Sprintf("/events/%s", url.PathEscape(args[0])), nil),
			})
		},
	}
	return withScopes(cmd, "events.read")
}
