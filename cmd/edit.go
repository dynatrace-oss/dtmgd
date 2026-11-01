package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// newEditCmd builds the parent for all "edit" subcommands.
func newEditCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit resources in an editor",
		Long: `Edit a resource's value in an editor, for a person at a terminal. Agents and
scripts use apply.

Supported resources:
  setting <object-id>   A Settings 2.0 object's value`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(newEditSettingCmd(a))
	return asGroup(cmd)
}

func newEditSettingCmd(a *App) *cobra.Command {
	var validateOnly bool
	cmd := &cobra.Command{
		Use:     "setting <object-id>",
		Aliases: []string{"settings"},
		Short:   "Edit a Settings 2.0 object's value in an editor",
		Long: `Open a Settings 2.0 object's value, as YAML, in $EDITOR (else $VISUAL, else
vi), and update the object when the file changed. Leaving it unchanged
cancels.

It needs a person at a terminal: in agent mode or without one it refuses;
use describe settings -o yaml and apply instead.

--validate-only asks the cluster to check the edited value without saving it
(Dynatrace Managed 1.288 or later).

Examples:
  dtmgd edit setting <object-id>
  EDITOR="code --wait" dtmgd edit setting <object-id> --validate-only`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !a.isInteractive() {
				return withHint(&UsageError{Err: fmt.Errorf("edit opens an editor and needs a person at a terminal")},
					"Use: dtmgd describe settings "+args[0]+" -o yaml > object.yaml, change the value, then dtmgd apply -f object.yaml")
			}
			id := args[0]
			return a.runWrites(writeSpec{
				Verb:         "edit",
				Resource:     "setting",
				ValidateOnly: validateOnly,
				Plan: func(_ string, c *client.Client) ([]plannedWrite, error) {
					return a.planEdit(c, id)
				},
			})
		},
	}
	cmd.Flags().BoolVar(&validateOnly, "validate-only", false, "have the cluster validate the edited value without saving it")
	return withWrites(cmd)
}

// planEdit reads object id, lets the person edit its value, and plans the
// update (unchanged when they saved nothing new).
func (a *App) planEdit(c *client.Client, id string) ([]plannedWrite, error) {
	var cur settingsObject
	if err := c.GetV2(settingsObjectsPath+"/"+url.PathEscape(id), nil, &cur); err != nil {
		return nil, err
	}
	value, changed, err := a.editValue(cur.Value)
	if err != nil {
		return nil, err
	}
	w := plannedWrite{
		Op: config.OpUpdate, Action: actionUpdate,
		ObjectID: id, SchemaID: cur.SchemaID, Scope: cur.Scope, Summary: cur.Summary,
		UpdateToken: cur.UpdateToken, Value: value, Current: cur.Value,
	}
	if !changed {
		output.PrintInfo("Edit cancelled: the value is unchanged.")
		w.Op, w.Action = "", actionUnchanged
	}
	return []plannedWrite{w}, nil
}

// editorCommand is the editor to run, with its arguments: $EDITOR, else
// $VISUAL, else vi (notepad on Windows).
func editorCommand() []string {
	for _, v := range []string{"EDITOR", "VISUAL"} {
		if fields := strings.Fields(os.Getenv(v)); len(fields) > 0 {
			return fields
		}
	}
	if runtime.GOOS == "windows" {
		return []string{"notepad"}
	}
	return []string{"vi"}
}

// editValue writes value to a temporary YAML file, runs the editor on it,
// and returns what the file holds afterwards and whether it changed.
func (a *App) editValue(value map[string]interface{}) (map[string]interface{}, bool, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, false, err
	}
	f, err := os.CreateTemp("", "dtmgd-edit-*.yaml")
	if err != nil {
		return nil, false, err
	}
	path := f.Name()
	defer func() { _ = os.Remove(path) }()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, false, err
	}
	editor := editorCommand()
	cmd := exec.Command(editor[0], append(editor[1:], path)...) // #nosec G204 -- the user's own editor
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, a.Err
	if err := cmd.Run(); err != nil {
		return nil, false, fmt.Errorf("editor %s: %w", editor[0], err)
	}
	edited, err := readValueFile(path, nil)
	if err != nil {
		return nil, false, err
	}
	return edited, !reflect.DeepEqual(edited, value), nil
}
