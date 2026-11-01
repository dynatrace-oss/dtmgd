package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// settingsSchemaDetail is the part of a Settings 2.0 schema definition that
// describe settings-schema shows.
type settingsSchemaDetail struct {
	SchemaID      string                    `json:"schemaId"`
	DisplayName   string                    `json:"displayName"`
	Description   string                    `json:"description"`
	Version       string                    `json:"version"`
	MultiObject   bool                      `json:"multiObject"`
	Ordered       bool                      `json:"ordered"`
	MaxObjects    int                       `json:"maxObjects"`
	AllowedScopes []string                  `json:"allowedScopes"`
	Properties    map[string]schemaProperty `json:"properties"`
	Types         map[string]struct {
		Properties map[string]schemaProperty `json:"properties"`
	} `json:"types"`
	Enums map[string]struct {
		Items []struct {
			Value interface{} `json:"value"`
		} `json:"items"`
	} `json:"enums"`
}

// schemaProperty is one property of a schema or of one of its types. Type is
// a name ("text", "set", …) or a {"$ref": "#/enums/X"} or
// {"$ref": "#/types/Y"} object.
type schemaProperty struct {
	Type    json.RawMessage `json:"type"`
	Default interface{}     `json:"default"`
	Items   *struct {
		Type json.RawMessage `json:"type"`
	} `json:"items"`
}

// schemaPropertyRow is a row of describe settings-schema's property tables.
type schemaPropertyRow struct {
	Property string `table:"PROPERTY"`
	Type     string `table:"TYPE"`
	Default  string `table:"DEFAULT,max=40"`
	Values   string `table:"VALUES,max=80"`
}

func newDescribeSettingsSchemaCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "settings-schema <schema-id>",
		Aliases: []string{"settings-schemas", "schema"},
		Short:   "Show a Settings 2.0 schema: scopes, properties and allowed values",
		Long: `Show a Settings 2.0 schema: its version, the scopes it can be set at, how
many objects a scope holds, and its properties with their types, defaults
and allowed values.

Examples:
  dtmgd describe settings-schema builtin:deployment.oneagent.updates
  dtmgd describe settings-schema builtin:deployment.management.update-windows -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "settings-schema",
				Fetch:    hintOnNotFound(describeFetch(settingsSchemasPath+"/"+url.PathEscape(args[0]), nil), hintListSchemas),
				Human:    printSettingsSchemaDetail,
			})
		},
	}
	return withScopes(cmd, scopeSettingsRead)
}

// printSettingsSchemaDetail renders a schema definition for a person: a
// summary, then a property table for the schema and one for each of its
// types.
func printSettingsSchemaDetail(w io.Writer, body interface{}) error {
	var d settingsSchemaDetail
	if err := client.DecodePaged(body.(map[string]interface{}), &d); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Schema:       %s\n", d.SchemaID)
	_, _ = fmt.Fprintf(w, "Name:         %s\n", d.DisplayName)
	_, _ = fmt.Fprintf(w, "Version:      %s\n", d.Version)
	_, _ = fmt.Fprintf(w, "Scopes:       %s\n", strings.Join(d.AllowedScopes, ", "))
	_, _ = fmt.Fprintf(w, "Objects:      %s\n", schemaObjects(d))
	if d.Description != "" {
		_, _ = fmt.Fprintf(w, "Description:  %s\n", d.Description)
	}
	_, _ = fmt.Fprintln(w)
	if err := printPropertyTable(w, d, d.Properties); err != nil {
		return err
	}
	for _, name := range sortedKeys(d.Types) {
		_, _ = fmt.Fprintf(w, "\nType %s:\n", name)
		if err := printPropertyTable(w, d, d.Types[name].Properties); err != nil {
			return err
		}
	}
	return nil
}

// schemaObjects says how many objects one scope holds.
func schemaObjects(d settingsSchemaDetail) string {
	if !d.MultiObject {
		return "one per scope"
	}
	s := fmt.Sprintf("up to %d per scope", d.MaxObjects)
	if d.Ordered {
		s += ", ordered"
	}
	return s
}

// printPropertyTable prints props as a table, sorted by name.
func printPropertyTable(w io.Writer, d settingsSchemaDetail, props map[string]schemaProperty) error {
	rows := make([]schemaPropertyRow, 0, len(props))
	for _, name := range sortedKeys(props) {
		p := props[name]
		typ, enum := propertyType(p)
		rows = append(rows, schemaPropertyRow{
			Property: name,
			Type:     typ,
			Default:  defaultText(p.Default),
			Values:   enumValues(d, enum),
		})
	}
	return output.NewPrinterTo("table", w).PrintList(rows)
}

// propertyType names p's type for a person, and the enum whose values it
// takes, if any: "enum" for an enum, the type's name for a type, and
// "set of …" or "list of …" for a collection.
func propertyType(p schemaProperty) (typ, enum string) {
	typ, enum = typeRef(p.Type)
	if (typ == "set" || typ == "list") && p.Items != nil {
		item, itemEnum := typeRef(p.Items.Type)
		return typ + " of " + item, itemEnum
	}
	return typ, enum
}

// typeRef reads a type: a plain name, or a $ref to an enum or a type.
func typeRef(raw json.RawMessage) (typ, enum string) {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name, ""
	}
	var ref struct {
		Ref string `json:"$ref"`
	}
	if json.Unmarshal(raw, &ref) != nil || ref.Ref == "" {
		return "—", ""
	}
	if e, ok := strings.CutPrefix(ref.Ref, "#/enums/"); ok {
		return "enum", e
	}
	return strings.TrimPrefix(ref.Ref, "#/types/"), ""
}

// enumValues lists the values of d's enum named enum, or "" for none.
func enumValues(d settingsSchemaDetail, enum string) string {
	if enum == "" {
		return ""
	}
	var values []string
	for _, item := range d.Enums[enum].Items {
		values = append(values, fmt.Sprint(item.Value))
	}
	return strings.Join(values, ", ")
}

// defaultText shows a property's default: a string as is, anything else as
// JSON, and "—" when there is none.
func defaultText(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "—"
	case string:
		return t
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// sortedKeys returns m's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
