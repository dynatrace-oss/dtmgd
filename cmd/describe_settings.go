package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

func newDescribeSettingsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "settings <object-id>",
		Aliases: []string{"setting"},
		Short:   "Show a Settings 2.0 object",
		Long: `Show one Settings 2.0 object: its schema, scope and summary, then its value.

-o json or -o yaml prints the API's object, which apply accepts as it is.

Examples:
  dtmgd describe settings <object-id>
  dtmgd describe settings <object-id> -o yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "settings",
				Fetch:    describeFetch(settingsObjectsPath+"/"+url.PathEscape(args[0]), nil),
				Human:    printSettingsObject,
			})
		},
	}
	return withScopes(cmd, scopeSettingsRead)
}

// printSettingsObject renders one settings object for a person: its
// identity, then its value as YAML.
func printSettingsObject(w io.Writer, body interface{}) error {
	var o settingsObject
	if err := client.DecodePaged(body.(map[string]interface{}), &o); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Object ID:       %s\n", o.ObjectID)
	_, _ = fmt.Fprintf(w, "Schema:          %s\n", o.SchemaID)
	_, _ = fmt.Fprintf(w, "Scope:           %s\n", o.Scope)
	_, _ = fmt.Fprintf(w, "Summary:         %s\n", o.Summary)
	if o.ExternalID != "" {
		_, _ = fmt.Fprintf(w, "External ID:     %s\n", o.ExternalID)
	}
	_, _ = fmt.Fprintf(w, "Schema version:  %s\n", o.SchemaVersion)
	_, _ = fmt.Fprintln(w, "Value:")
	var buf bytes.Buffer
	if err := output.NewPrinterTo("yaml", &buf).Print(o.Value); err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
	return nil
}
