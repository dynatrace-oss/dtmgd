package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// newDeleteCmd builds the parent for all "delete" subcommands.
func newDeleteCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete resources",
		Long: `Delete resources from a Dynatrace Managed environment.

Supported resources:
  settings <object-id>  A Settings 2.0 object

Writes need a context whose safety-level allows them (see
dtmgd config set-context --help) and a token with settings.write.`,
		RunE: requireSubcommand,
	}
	cmd.AddCommand(newDeleteSettingsCmd(a))
	return asGroup(cmd)
}

func newDeleteSettingsCmd(a *App) *cobra.Command {
	var opts struct{ yes, dryRun bool }
	cmd := &cobra.Command{
		Use:     "settings <object-id>",
		Aliases: []string{"setting"},
		Short:   "Delete a Settings 2.0 object",
		Long: `Delete one Settings 2.0 object. Deleting the object at a scope makes that
scope inherit its value again.

A person at a terminal confirms the delete; --yes skips the question, and is
required in agent mode or without a terminal. An update window that a
OneAgent or ActiveGate update setting still uses is not deleted: change
those settings first.

--dry-run shows what would be deleted, with GETs only, at any write level.

Examples:
  dtmgd delete settings <object-id> --dry-run
  dtmgd delete settings <object-id> --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			return a.runWrites(writeSpec{
				ClusterToo: true,
				Verb:       "delete",
				Resource:   "settings",
				DryRun:     opts.dryRun,
				Plan: func(_ string, c *client.Client) ([]plannedWrite, error) {
					return planDelete(c, id)
				},
				Confirm: func(ws []plannedWrite) error {
					return a.confirmDelete(deletePrompt(ws), opts.yes)
				},
			})
		},
	}
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "delete without asking (required in agent mode and without a terminal)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show what would be deleted, sending GETs only")
	return withWrites(cmd)
}

// planDelete plans deleting object id, refusing an update window that
// update settings still use: the cluster would delete it and leave them
// pointing at nothing.
func planDelete(c *client.Client, id string) ([]plannedWrite, error) {
	var cur settingsObject
	if err := settingsGet(c, settingsObjectsPath+"/"+url.PathEscape(id), nil, &cur); err != nil {
		return nil, err
	}
	// The OneAgent and ActiveGate update schemas exist in environments
	// only; a cluster's windows are checked against its ActiveGates' auto-update.
	if cur.SchemaID == schemaUpdateWindows && !c.ClusterScoped() {
		refs, err := windowReferences(c, id)
		if err != nil {
			return nil, err
		}
		if len(refs) > 0 {
			return nil, windowInUse(id, refs)
		}
	}
	return []plannedWrite{{
		Op: config.OpDelete, Action: actionDelete,
		ObjectID: id, SchemaID: cur.SchemaID, Scope: cur.Scope, Summary: cur.Summary,
		UpdateToken: cur.UpdateToken, Current: cur.Value,
	}}, nil
}

// windowReferences lists the OneAgent and ActiveGate update settings, at
// every scope, that name update window windowID.
func windowReferences(c *client.Client, windowID string) ([]settingsObject, error) {
	body, err := settingsPaged(c, settingsObjectsPath, map[string]string{
		"schemaIds": schemaOneAgentUpdates + "," + schemaActiveGateUpdates,
		"fields":    "objectId,schemaId,scope,value",
		"pageSize":  "500",
	}, client.Paging{ListKey: "items"})
	if err != nil {
		return nil, err
	}
	var objs []settingsObject
	if err := decodeItems(body, &objs); err != nil {
		return nil, err
	}
	var refs []settingsObject
	for _, o := range objs {
		if referencesWindow(o.Value, windowID) {
			refs = append(refs, o)
		}
	}
	return refs, nil
}

// referencesWindow reports whether an update setting's value names the
// window, in either schema's spelling.
func referencesWindow(value map[string]interface{}, windowID string) bool {
	for listKey, itemKey := range map[string]string{"maintenanceWindows": "maintenanceWindow", "updateWindows": "updateWindow"} {
		list, _ := value[listKey].([]interface{})
		for _, item := range list {
			if ref, _ := item.(map[string]interface{}); ref[itemKey] == windowID {
				return true
			}
		}
	}
	return false
}

// windowInUse is the refusal to delete a window that refs still use.
func windowInUse(id string, refs []settingsObject) error {
	users := make([]string, len(refs))
	for i, r := range refs {
		users[i] = fmt.Sprintf("%s at %s (%s)", r.SchemaID, r.Scope, r.ObjectID)
	}
	return withHint(fmt.Errorf("update window %s is still used by %s", id, strings.Join(users, ", ")),
		"Point those settings at another window, or change their update mode, then delete the window.")
}

// deletePrompt is the question a person answers before ws are deleted.
func deletePrompt(ws []plannedWrite) string {
	if len(ws) == 1 {
		w := ws[0]
		return fmt.Sprintf("Delete %s object %s (%s) at %s?", w.SchemaID, w.ObjectID, w.Summary, w.Scope)
	}
	return fmt.Sprintf("Delete %d settings objects?", len(ws))
}

// confirmDelete asks a person to confirm a delete. --yes answers for them;
// without it, agent mode and a missing terminal are usage errors, since
// nobody can answer. Anything but y or yes cancels.
func (a *App) confirmDelete(prompt string, yes bool) error {
	if yes {
		return nil
	}
	if !a.isInteractive() {
		return &UsageError{Err: fmt.Errorf("nothing can confirm this delete here (agent mode, or no terminal): add --yes")}
	}
	_, _ = fmt.Fprintf(a.Err, "%s [y/N] ", prompt)
	answer, _ := bufio.NewReader(a.input()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errCancelled
}
