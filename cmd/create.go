package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// mutatingAnnotation marks a command that writes; the catalog shows it, so
// an agent can tell writes from reads.
const mutatingAnnotation = "dtmgd.io/mutating"

// withWrites marks cmd as a write command and declares the token scopes
// every settings write needs.
func withWrites(cmd *cobra.Command) *cobra.Command {
	withScopes(cmd, scopeSettingsRead, scopeSettingsWrite)
	cmd.Annotations[mutatingAnnotation] = "true"
	return cmd
}

// isMutating reports whether cmd writes.
func isMutating(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[mutatingAnnotation] == "true"
}

// newCreateCmd builds the parent for all "create" subcommands.
func newCreateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create resources",
		Long: `Create resources in a Dynatrace Managed environment.

Supported resources:
  settings              A Settings 2.0 object from a value file

Writes need a context whose safety-level allows them (see
dtmgd config set-context --help) and a token with settings.write.`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(newCreateSettingsCmd(a))
	return asGroup(cmd)
}

func newCreateSettingsCmd(a *App) *cobra.Command {
	var opts struct {
		file, schema, scope  string
		position             string
		dryRun, validateOnly bool
	}
	cmd := &cobra.Command{
		Use:     "settings -f <value-file> --schema <schema-id> --scope <scope>",
		Aliases: []string{"setting"},
		Short:   "Create a Settings 2.0 object from a value file",
		Long: `Create one Settings 2.0 object. The file (YAML or JSON, - for stdin) holds the
bare value; --schema and --scope say where it goes. To change an existing
object, or to create objects from a full {schemaId, scope, value} file, use
apply.

For a schema that holds one object per scope, create refuses when the scope
already has one.

In an ordered schema a new object goes last, or where --position says: first,
last, after:<ref> or before:<ref>, where <ref> is the objectId or externalId
of an object at the same scope.

--dry-run shows what would be created, with GETs only, at any write level.
--validate-only asks the cluster to check the value without saving it
(Dynatrace Managed 1.288 or later).

Examples:
  dtmgd create settings -f window.yaml --schema builtin:deployment.management.update-windows --scope environment --dry-run
  dtmgd create settings -f window.yaml --schema builtin:deployment.management.update-windows --scope environment
  dtmgd create settings -f window.yaml --schema builtin:deployment.management.update-windows --scope environment --env "stage;prod"`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readValueFile(opts.file, a.input())
			if err != nil {
				return err
			}
			pos, err := parsePosition(opts.position)
			if err != nil {
				return &UsageError{Err: err}
			}
			return a.runWrites(writeSpec{
				ClusterToo:   true,
				Verb:         "create",
				Resource:     "settings",
				DryRun:       opts.dryRun,
				ValidateOnly: opts.validateOnly,
				MultiEnv:     true,
				Plan: func(_ string, c *client.Client) ([]plannedWrite, error) {
					return planCreate(c, opts.schema, opts.scope, value, pos)
				},
			})
		},
	}
	cmd.Flags().StringVarP(&opts.file, "filename", "f", "", "file holding the value, YAML or JSON (- for stdin)")
	cmd.Flags().StringVar(&opts.schema, "schema", "", "schema ID of the new object")
	cmd.Flags().StringVar(&opts.scope, "scope", "", `scope of the new object: "environment" or an entity ID`)
	cmd.Flags().StringVar(&opts.position, "position", "", "place the object in an ordered schema: first, last, after:<ref> or before:<ref>")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show what would be created, sending GETs only")
	cmd.Flags().BoolVar(&opts.validateOnly, "validate-only", false, "have the cluster validate the value without saving it")
	_ = cmd.MarkFlagRequired("filename")
	_ = cmd.MarkFlagRequired("schema")
	_ = cmd.MarkFlagRequired("scope")
	return withWrites(cmd)
}

// readValueFile reads a settings value from path (YAML or JSON; "-" reads
// stdin) as a JSON object, so it compares and encodes like an API value.
func readValueFile(path string, stdin io.Reader) (map[string]interface{}, error) {
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	docs, err := yamlDocuments(data)
	if err != nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: not YAML or JSON: %w", path, err)}
	}
	if len(docs) != 1 {
		return nil, &UsageError{Err: fmt.Errorf("%s holds %d YAML documents; a value file holds one value", path, len(docs))}
	}
	var value map[string]interface{}
	if err := jsonRoundTrip(docs[0], &value); err != nil || value == nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: the value must be an object", path)}
	}
	return value, nil
}

// yamlDocuments decodes every document of a YAML (or JSON) stream, empty
// ones left out: a file of several "---" documents must not be read as its
// first one alone.
func yamlDocuments(data []byte) ([]interface{}, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []interface{}
	for {
		var doc interface{}
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
}

// readInput reads path, or stdin for "-".
func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the user names the file to apply
	if err != nil {
		return nil, &UsageError{Err: err}
	}
	return data, nil
}

// jsonRoundTrip converts v (as YAML decoded it) into out through JSON, so
// numbers and nested objects take the types an API response has.
func jsonRoundTrip(v, out interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// settingsSchemaShape is the part of a schema definition writes need.
type settingsSchemaShape struct {
	MultiObject bool                      `json:"multiObject"`
	KeyProperty string                    `json:"keyProperty"`
	Ordered     bool                      `json:"ordered"`
	Properties  map[string]schemaProperty `json:"properties"`
	Types       map[string]struct {
		Properties map[string]schemaProperty `json:"properties"`
	} `json:"types"`
}

// schemaShape reads schemaID's definition.
func schemaShape(c *client.Client, schemaID string) (settingsSchemaShape, error) {
	var shape settingsSchemaShape
	err := settingsGet(c, settingsSchemasPath+"/"+url.PathEscape(schemaID), nil, &shape)
	var apiErr *client.ErrAPI
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return shape, withHint(err, hintListSchemas)
	}
	return shape, err
}

// planCreate plans create settings: one create, refused when the POST would
// overwrite an object instead: the scope's object of a one-per-scope
// schema, or the object with the same key property.
func planCreate(c *client.Client, schemaID, scope string, value map[string]interface{}, pos settingsPosition) ([]plannedWrite, error) {
	shape, err := schemaShape(c, schemaID)
	if err != nil {
		return nil, err
	}
	if err := requireOrdered(schemaID, shape, pos); err != nil {
		return nil, err
	}
	w := plannedWrite{Op: config.OpCreate, Action: actionCreate, SchemaID: schemaID, Scope: scope, Value: value}
	if shape.MultiObject && shape.KeyProperty == "" && pos.Kind == "" {
		return []plannedWrite{w}, nil
	}
	listed, err := listSettings(c, schemaID, scope, "objectId,externalId,value")
	if err != nil {
		return nil, err
	}
	if m := findMatch(shape, applyObject{SchemaID: schemaID, Scope: scope, Value: value}, listed); m != nil {
		return nil, withHint(fmt.Errorf("creating this %s object at %s would overwrite object %s", schemaID, scope, m.ObjectID),
			"Use apply to change it: dtmgd describe settings "+m.ObjectID+" -o yaml > object.yaml, edit the value, then dtmgd apply -f object.yaml")
	}
	w.Existing = listedIDs(listed)
	if pos.Kind != "" {
		pl, err := placeObject(orderOf(listed), "", pos)
		if err != nil {
			return nil, err
		}
		w.setPlacement(pl)
	}
	return []plannedWrite{w}, nil
}

// listedIDs is the set of the listed objects' IDs.
func listedIDs(listed []settingsObject) map[string]bool {
	ids := make(map[string]bool, len(listed))
	for _, l := range listed {
		ids[l.ObjectID] = true
	}
	return ids
}

// listSettings returns every object of schemaID at scope, with fields.
func listSettings(c *client.Client, schemaID, scope, fields string) ([]settingsObject, error) {
	body, err := settingsPaged(c, settingsObjectsPath, map[string]string{
		"schemaIds": schemaID,
		"scopes":    scope,
		"fields":    fields,
		"pageSize":  "500",
	}, client.Paging{ListKey: "items"})
	if err != nil {
		return nil, err
	}
	var objs []settingsObject
	return objs, decodeItems(body, &objs)
}
