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

	// settingsEffectivePath is the effective values list, relative to /api/v2.
	settingsEffectivePath = "/settings/effectiveValues"
	// settingsEffectiveFields are the fields get settings --effective asks
	// for. An effective value has no objectId or updateToken; origin is the
	// scope it is stored at.
	settingsEffectiveFields = "schemaId,origin,summary,value"

	// hintSettingsNotFound is the next step after a 404 on a listing: the API
	// answers 404 for an unknown schema and for a scope it cannot use.
	hintSettingsNotFound = "Check the schema ID (dtmgd get settings-schemas --search <text>) and that the schema allows the scope (dtmgd describe settings-schema <id> lists its scopes)"
)

// getSettingsOptions holds the flags of get settings.
type getSettingsOptions struct {
	schemas   []string
	scope     string
	effective bool
	limit     int
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

// effectiveParams is the /settings/effectiveValues query for the flags. Its
// scope parameter is singular, unlike the object list's scopes.
func effectiveParams(o getSettingsOptions) map[string]string {
	params := map[string]string{"fields": settingsEffectiveFields, "scope": o.scope}
	if len(o.schemas) > 0 {
		params["schemaIds"] = strings.Join(o.schemas, ",")
	}
	return params
}

func newGetSettingsCmd(a *App) *cobra.Command {
	var opts getSettingsOptions
	cmd := &cobra.Command{
		Use:     "settings [object-id]",
		Aliases: []string{"setting"},
		Short:   "List Settings 2.0 objects, or the values in effect at a scope",
		Long: `List the Settings 2.0 objects stored in the environment, or one object.
With --effective, list the values that apply at --scope instead: inherited
values and schema defaults included, each with its origin (the scope it is
stored at, or "default").

A list needs --schema or --scope, as the API does. --schema takes one or more
schema IDs (repeat it or separate them with commas); --scope takes
"environment" or an entity ID such as HOST-…, HOST_GROUP-… or
ENVIRONMENT_ACTIVE_GATE-…. Find schema IDs with get settings-schemas.

json and yaml print the API's own objects, with objectId, schemaId, scope,
externalId, summary, updateToken, schemaVersion and value; effective values
have schemaId, origin, summary and value.

Examples:
  dtmgd get settings --schema builtin:deployment.management.update-windows
  dtmgd get settings --schema builtin:deployment.oneagent.updates --scope HOST_GROUP-0000000000000001
  dtmgd get settings --scope HOST-0000000000000001          # every schema stored on a host
  dtmgd get settings <object-id> -o yaml
  dtmgd get settings --effective --scope HOST-0000000000000001 --schema builtin:deployment.oneagent.updates`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runGetSettings(opts, args)
		},
	}
	cmd.Flags().StringSliceVar(&opts.schemas, "schema", nil, "schema ID(s) to list; repeat or separate with commas")
	cmd.Flags().StringVar(&opts.scope, "scope", "", `scope: "environment" or an entity ID (HOST-…, HOST_GROUP-…, ENVIRONMENT_ACTIVE_GATE-…)`)
	cmd.Flags().BoolVar(&opts.effective, "effective", false, "show the values that apply at --scope, inherited ones and schema defaults included")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of results to show")
	return withScopes(cmd, scopeSettingsRead)
}

// runGetSettings runs get settings for its flags and arguments.
func (a *App) runGetSettings(opts getSettingsOptions, args []string) error {
	fetch, list, err := settingsFetch(opts, args, a.Opts.MaxPages)
	if err != nil {
		return err
	}
	view := settingsView{names: &windowNames{}, effective: opts.effective}.forRequest(opts, len(args) == 1)
	return a.runSettingsFetch(fetchSpec{
		Verb:     "get",
		Resource: "settings",
		Fetch:    a.withWindowNames(fetch, view),
		Rows:     view.rows,
		Limit:    opts.limit,
		List:     list,
		Empty:    "No settings found.",
	})
}

// settingsFetch picks get settings' request: one object by ID, the effective
// values at a scope, or the stored objects; list reports whether the result
// is a list. It refuses flags that do not fit together.
func settingsFetch(opts getSettingsOptions, args []string, maxPages int) (fetch func(*client.Client, int) (fetched, error), list bool, err error) {
	switch {
	case len(args) == 1:
		if len(opts.schemas) > 0 || opts.scope != "" || opts.effective {
			return nil, false, &UsageError{Err: fmt.Errorf("--schema, --scope and --effective list objects; they cannot be combined with an object ID")}
		}
		return settingsObjectFetch(settingsObjectsPath + "/" + url.PathEscape(args[0])), false, nil
	case opts.effective:
		if opts.scope == "" {
			return nil, false, &UsageError{Err: fmt.Errorf(`--effective needs --scope: "environment" or the entity ID whose values to resolve`)}
		}
		return hintOnNotFound(settingsPagedFetch(settingsEffectivePath, effectiveParams(opts), "items", 500, maxPages), hintSettingsNotFound), true, nil
	case len(opts.schemas) == 0 && opts.scope == "":
		return nil, false, &UsageError{Err: fmt.Errorf("give --schema or --scope (the API needs one), or an object ID")}
	}
	return hintOnNotFound(settingsPagedFetch(settingsObjectsPath, settingsParams(opts), "items", 500, maxPages), hintSettingsNotFound), true, nil
}

// withWindowNames adds to fetch the lookup of update window names that the
// OneAgent and ActiveGate update tables show. Only a person's table needs
// them; a failed lookup leaves the object IDs, with a warning.
func (a *App) withWindowNames(fetch func(*client.Client, int) (fetched, error), view settingsView) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := fetch(c, limit)
		if err != nil || !a.humanOutput() || !view.usesWindows(f.Body) {
			return f, err
		}
		if err := lookupWindowNames(c, view.names); err != nil {
			f.Warnings = append(f.Warnings, "update window names unavailable: "+err.Error())
		}
		return f, nil
	}
}
