package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

const (
	// settingsObjectsPath is the Settings 2.0 object list, relative to /api/v2.
	settingsObjectsPath = "/settings/objects"
	// settingsObjectFields are the fields get settings asks for: with them an
	// exported object (-o yaml) carries what apply needs to write it back.
	settingsObjectFields = "objectId,schemaId,scope,externalId,summary,updateToken,schemaVersion,value"
)

// getSettingsOptions holds the flags of get settings.
type getSettingsOptions struct {
	schemas []string
	scope   string
	limit   int
}

// settingsParams is the /settings/objects query for the flags. The API needs
// schemaIds or scopes; fields is always sent.
func settingsParams(o getSettingsOptions) map[string]string {
	params := map[string]string{"fields": settingsObjectFields}
	if len(o.schemas) > 0 {
		params["schemaIds"] = strings.Join(o.schemas, ",")
	}
	if o.scope != "" {
		params["scopes"] = o.scope
	}
	return params
}

func newGetSettingsCmd(a *App) *cobra.Command {
	var opts getSettingsOptions
	cmd := &cobra.Command{
		Use:     "settings [object-id]",
		Aliases: []string{"setting"},
		Short:   "List Settings 2.0 objects",
		Long: `List the Settings 2.0 objects stored in the environment, or one object.

A list needs --schema or --scope, as the API does. --schema takes one or more
schema IDs (repeat it or separate them with commas); --scope takes
"environment" or an entity ID such as HOST-…, HOST_GROUP-… or
ENVIRONMENT_ACTIVE_GATE-…. Find schema IDs with get settings-schemas.

json and yaml print the API's own objects, with objectId, schemaId, scope,
externalId, summary, updateToken, schemaVersion and value.

Examples:
  dtmgd get settings --schema builtin:deployment.management.update-windows
  dtmgd get settings --schema builtin:deployment.oneagent.updates --scope HOST_GROUP-0000000000000001
  dtmgd get settings --scope HOST-0000000000000001          # every schema stored on a host
  dtmgd get settings <object-id> -o yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			names := &windowNames{}
			spec := fetchSpec{
				Verb:     "get",
				Resource: "settings",
				Rows:     settingsView{names: names}.rows,
				Limit:    opts.limit,
				Empty:    "No settings found.",
			}
			if len(args) == 1 {
				if len(opts.schemas) > 0 || opts.scope != "" {
					return &UsageError{Err: fmt.Errorf("--schema and --scope list objects; they cannot be combined with an object ID")}
				}
				spec.Fetch = a.withWindowNames(describeFetch(settingsObjectsPath+"/"+url.PathEscape(args[0]), nil), names)
				return a.runFetch(spec)
			}
			if len(opts.schemas) == 0 && opts.scope == "" {
				return &UsageError{Err: fmt.Errorf("give --schema or --scope (the API needs one), or an object ID")}
			}
			spec.Fetch = a.withWindowNames(hintOnNotFound(pagedFetch(settingsObjectsPath, settingsParams(opts), "items", 500, a.Opts.MaxPages), hintListSchemas), names)
			spec.List = true
			return a.runFetch(spec)
		},
	}
	cmd.Flags().StringSliceVar(&opts.schemas, "schema", nil, "schema ID(s) to list; repeat or separate with commas")
	cmd.Flags().StringVar(&opts.scope, "scope", "", `scope: "environment" or an entity ID (HOST-…, HOST_GROUP-…, ENVIRONMENT_ACTIVE_GATE-…)`)
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of results to show")
	return withScopes(cmd, scopeSettingsRead)
}

// withWindowNames adds to fetch the lookup of update window names that the
// OneAgent and ActiveGate update tables show. Only a person's table needs
// them; a failed lookup leaves the object IDs, with a warning.
func (a *App) withWindowNames(fetch func(*client.Client, int) (fetched, error), names *windowNames) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := fetch(c, limit)
		if err != nil || !a.humanOutput() || !usesWindows(f.Body) {
			return f, err
		}
		if err := lookupWindowNames(c, names); err != nil {
			f.Warnings = append(f.Warnings, "update window names unavailable: "+err.Error())
		}
		return f, nil
	}
}
