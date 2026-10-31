package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// settingsSchemasPath is the Settings 2.0 schema list, relative to /api/v2.
const settingsSchemasPath = "/settings/schemas"

// hintListSchemas is the next step after an unknown schema ID.
const hintListSchemas = "List schema IDs with: dtmgd get settings-schemas --search <text>"

// settingsSchemaRow is the table row for settings schemas.
type settingsSchemaRow struct {
	SchemaID string `table:"SCHEMA-ID"`
	Name     string `table:"NAME"`
	Version  string `table:"VERSION"`
}

func newGetSettingsSchemasCmd(a *App) *cobra.Command {
	var opts struct {
		search string
		limit  int
	}
	cmd := &cobra.Command{
		Use:     "settings-schemas [schema-id]",
		Aliases: []string{"schema", "schemas"},
		Short:   "List Settings 2.0 schemas",
		Long: `List the Settings 2.0 schemas the environment offers, or one schema.

--search keeps the schemas whose ID or display name contains the text,
ignoring case. describe settings-schema shows a schema's properties.

Examples:
  dtmgd get settings-schemas --search deployment
  dtmgd get settings-schemas builtin:deployment.oneagent.updates
  dtmgd describe settings-schema builtin:deployment.oneagent.updates`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := fetchSpec{
				Verb:     "get",
				Resource: "settings-schemas",
				Rows:     settingsSchemaRows,
				Limit:    opts.limit,
				Empty:    "No settings schemas found.",
			}
			if len(args) == 1 {
				if opts.search != "" {
					return &UsageError{Err: fmt.Errorf("--search lists schemas; it cannot be combined with a schema ID")}
				}
				spec.Fetch = hintOnNotFound(describeFetch(settingsSchemasPath+"/"+url.PathEscape(args[0]), nil), hintListSchemas)
				return a.runFetch(spec)
			}
			spec.Fetch = settingsSchemasFetch(opts.search)
			spec.List = true
			return a.runFetch(spec)
		},
	}
	cmd.Flags().StringVar(&opts.search, "search", "", "keep schemas whose ID or display name contains this text (ignoring case)")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of results to show")
	return withScopes(cmd, scopeSettingsRead)
}

// settingsSchemasFetch is get settings-schemas' Fetch for the list: one
// request (the endpoint has no paging), then --search and the limit, both in
// dtmgd. A search recounts totalCount, so json and yaml stay consistent.
func settingsSchemasFetch(search string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		var body map[string]interface{}
		if err := c.GetV2(settingsSchemasPath, nil, &body); err != nil {
			return fetched{}, err
		}
		if body == nil {
			body = map[string]interface{}{}
		}
		list, _ := body["items"].([]interface{})
		if search != "" {
			list = matchingSchemas(list, search)
			body["items"] = list
			body["totalCount"] = float64(len(list))
		}
		f := listFetched(body, "items")
		if limit > 0 && len(list) > limit {
			body["items"] = list[:limit]
			f.HasMore, f.Count = true, limit
		}
		return f, nil
	}
}

// matchingSchemas returns the schemas whose ID or display name contains
// search, ignoring case.
func matchingSchemas(list []interface{}, search string) []interface{} {
	needle := strings.ToLower(search)
	kept := []interface{}{}
	for _, item := range list {
		m, _ := item.(map[string]interface{})
		id, _ := m["schemaId"].(string)
		name, _ := m["displayName"].(string)
		if strings.Contains(strings.ToLower(id), needle) || strings.Contains(strings.ToLower(name), needle) {
			kept = append(kept, item)
		}
	}
	return kept
}

// settingsSchemaRows turns a schema list, or one schema definition, into
// table rows. A list entry names its version latestSchemaVersion, a
// definition version.
func settingsSchemaRows(body interface{}) (interface{}, int, error) {
	m, _ := body.(map[string]interface{})
	var entries []struct {
		SchemaID      string `json:"schemaId"`
		DisplayName   string `json:"displayName"`
		LatestVersion string `json:"latestSchemaVersion"`
		Version       string `json:"version"`
	}
	if err := decodeItems(m, &entries); err != nil {
		return nil, 0, err
	}
	rows := make([]settingsSchemaRow, 0, len(entries))
	for _, e := range entries {
		version := e.LatestVersion
		if version == "" {
			version = e.Version
		}
		rows = append(rows, settingsSchemaRow{SchemaID: e.SchemaID, Name: e.DisplayName, Version: version})
	}
	return rows, len(rows), nil
}

// decodeItems decodes a Settings API body into out, a pointer to a slice:
// the body's items when it is a list, else the body itself as the only
// element (a get by ID returns the bare object).
func decodeItems(body map[string]interface{}, out interface{}) error {
	var items interface{} = []interface{}{body}
	if list, isList := body["items"]; isList {
		items = list
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
