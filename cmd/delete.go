package cmd

import (
	"bufio"
	"errors"
	"fmt"
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
required in agent mode or without a terminal. An object that other
settings still reference (a schema's "setting" properties, such as an
alerting profile a notification uses) is not deleted: change or delete
those settings first. On a cluster context, an update window that the
ActiveGates' auto-update uses is not deleted either. Only Settings 2.0 is checked; dashboards and other
configuration are not.

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

// planDelete plans deleting object id, refusing an object that other
// settings still reference (the cluster would delete it and leave them
// pointing at nothing), and a window the cluster's ActiveGates still use.
func planDelete(c *client.Client, id string) ([]plannedWrite, error) {
	var cur settingsObject
	if err := getSettingsObject(c, id, &cur); err != nil {
		return nil, err
	}
	cur.ObjectID = id
	if cur.SchemaID == schemaUpdateWindows {
		if err := checkWindowUnused(c, id); err != nil {
			return nil, err
		}
	}
	warnings, err := checkUnreferenced(c, cur)
	if err != nil {
		return nil, err
	}
	return []plannedWrite{{
		Warnings: warnings,
		Op:       config.OpDelete, Action: actionDelete,
		ObjectID: id, SchemaID: cur.SchemaID, Scope: cur.Scope, Summary: cur.Summary,
		UpdateToken: cur.UpdateToken, Current: cur.Value,
	}}, nil
}

// checkWindowUnused refuses deleting update window id while a cluster's
// ActiveGate auto-update uses it. (Settings objects that reference it are
// found by checkUnreferenced, for every schema.)
func checkWindowUnused(c *client.Client, id string) error {
	if c.ClusterScoped() {
		refs, err := clusterWindowReferences(c, id)
		if isForbidden(err) {
			return withHint(err, "Checking whether the cluster's ActiveGates still use this window needs ServiceProviderAPI on the cluster token, besides settings.write.")
		}
		if err != nil || len(refs) == 0 {
			return err
		}
		return withHint(fmt.Errorf("update window %s is still used by the cluster ActiveGates' auto-update: %s", id, strings.Join(refs, ", ")),
			"Point those configurations at another window first (dtmgd update activegate-updates), then delete the window.")
	}
	return nil
}

// isForbidden reports whether err is an HTTP 403.
func isForbidden(err error) bool {
	var apiErr *client.ErrAPI
	return errors.As(err, &apiErr) && apiErr.StatusCode == 403
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
