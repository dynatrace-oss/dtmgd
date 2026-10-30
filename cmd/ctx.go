package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// Flag names shared between the `ctx set` and `config set-context` commands.
const (
	flagEnvID    = "env-id"
	flagTokenRef = "token-ref"
)

// ContextListItem is the table row for context listings.
type ContextListItem struct {
	Current     string `table:"CURRENT"`
	Name        string `table:"NAME"`
	Scope       string `table:"SCOPE"`
	Host        string `table:"HOST"`
	EnvID       string `table:"ENV-ID"`
	TokenRef    string `table:"TOKEN-REF"`
	Description string `table:"DESCRIPTION,wide"`
}

// newCtxCmd builds the top-level shortcut for context management.
func newCtxCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ctx [context-name]",
		Short: "Manage contexts (shortcut for config context commands)",
		Long: `Quick context management without the "config" prefix.

When called without arguments, lists all contexts.
When called with a context name, switches to that context.

Examples:
  dtmgd ctx                   # list all contexts
  dtmgd ctx production        # switch to context
  dtmgd ctx current           # show current context name`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: a.contextCompletionFn,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return a.listContexts()
			}
			return a.useContext(args[0])
		},
	}
	cmd.AddCommand(
		newCtxCurrentCmd(a),
		newCtxSetCmd(a),
		newCtxDeleteCmd(a),
	)
	return cmd
}

func newCtxCurrentCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Display the current context name",
		Long: `Display the current context name.

With -o json or -o yaml, and in agent mode, it is an object:
{"current-context": "<name>"}.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.printCurrentContext()
		},
	}
	return cmd
}

// currentContext is what config current-context and ctx current report in
// json, yaml, csv and tsv, and as the agent envelope's result, keyed as in
// the config file.
type currentContext struct {
	Name string `json:"current-context" yaml:"current-context" table:"CURRENT-CONTEXT"`
}

// printCurrentContext prints the config's current context: the bare name for
// a person, an object for an agent or a data format.
func (a *App) printCurrentContext() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if a.dataOutput() {
		return a.printer("contexts").Print(currentContext{Name: cfg.CurrentContext})
	}
	_, err = fmt.Fprintln(a.Out, cfg.CurrentContext)
	return err
}

func newCtxSetCmd(a *App) *cobra.Command {
	var opts struct {
		host, envID, tokenRef, description string
	}
	cmd := &cobra.Command{
		Use:   "set <context-name>",
		Short: "Create or update a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.setContext(args[0], opts.host, opts.envID, opts.tokenRef, opts.description)
		},
	}
	cmd.Flags().StringVar(&opts.host, "host", "", "Managed cluster host URL")
	cmd.Flags().StringVar(&opts.envID, flagEnvID, "", "environment ID (omit for a cluster context)")
	cmd.Flags().StringVar(&opts.tokenRef, flagTokenRef, "", "credential name")
	cmd.Flags().StringVar(&opts.description, "description", "", "description")
	return cmd
}

func newCtxDeleteCmd(a *App) *cobra.Command {
	var deleteCreds bool
	cmd := &cobra.Command{
		Use:               "delete <context-name>",
		Aliases:           []string{"rm"},
		Short:             "Delete a context",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: a.contextCompletionFn,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.deleteContext(args[0], deleteCreds)
		},
	}
	cmd.Flags().BoolVar(&deleteCreds, "delete-credentials", false, "also delete the context's token, unless another context uses it")
	return cmd
}

// contextsUsingToken returns the quoted names of the contexts whose
// token-ref is tokenRef.
func contextsUsingToken(cfg *config.Config, tokenRef string) []string {
	var names []string
	for _, nc := range cfg.Contexts {
		if nc.Context.TokenRef == tokenRef {
			names = append(names, strconv.Quote(nc.Name))
		}
	}
	return names
}

// contextCompletionFn provides shell completion for context names.
func (a *App) contextCompletionFn(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, nc := range cfg.Contexts {
		names = append(names, nc.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// ── Shared logic ──────────────────────────────────────────────────────────────

func (a *App) listContexts() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}

	var items []ContextListItem
	for _, nc := range cfg.Contexts {
		current := ""
		if nc.Name == cfg.CurrentContext {
			current = "*"
		}
		items = append(items, ContextListItem{
			Current:     current,
			Name:        nc.Name,
			Scope:       contextScope(nc.Context),
			Host:        nc.Context.Host,
			EnvID:       nc.Context.EnvID,
			TokenRef:    nc.Context.TokenRef,
			Description: nc.Context.Description,
		})
	}

	return a.printer("").PrintList(items)
}

// contextScope names c's kind, as the SCOPE columns show it.
func contextScope(c config.Context) string {
	if c.IsCluster() {
		return "cluster"
	}
	return "environment"
}

func (a *App) useContext(name string) error {
	cfg, err := a.loadConfigRaw()
	if err != nil {
		return err
	}

	found := false
	for _, nc := range cfg.Contexts {
		if nc.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("context %q not found", name)
	}

	cfg.CurrentContext = name
	if err := a.saveConfig(cfg); err != nil {
		return err
	}
	output.PrintSuccess("Switched to context %q", name)
	return nil
}

// warnClobberedContextRefs warns before a set-context call replaces a ${VAR}
// reference with a literal value. The user gets no other signal that the
// indirection is gone, and only the matched references are echoed — never the
// surrounding value, which may hold a literal secret.
//
// Does nothing when the context does not exist yet: there is nothing to clobber.
func warnClobberedContextRefs(cfg *config.Config, name, host, envID, tokenRef string) {
	existing, err := cfg.GetContext(name)
	if err != nil {
		return
	}
	for _, f := range []struct {
		newValue string
		oldValue string
		label    string
	}{
		{host, existing.Context.Host, "host"},
		{envID, existing.Context.EnvID, flagEnvID},
		{tokenRef, existing.Context.TokenRef, flagTokenRef},
	} {
		if f.newValue == "" || !config.HasPlaceholder(f.oldValue) {
			continue
		}
		refs := strings.Join(config.PlaceholderRefs(f.oldValue), ", ")
		output.PrintWarning("Replaced %s in the %s field — that environment variable no longer affects this context.", refs, f.label)
	}
}

func (a *App) setContext(name, host, envID, tokenRef, description string) error {
	if strings.Contains(name, ";") {
		return &UsageError{Err: fmt.Errorf("context name %q contains ';', which --env uses to separate names; choose another name", name)}
	}
	cfg, err := a.loadConfigForUpdate()
	if err != nil {
		return err
	}

	// A new context needs a host. Without an env-id it is a cluster context.
	isUpdate := false
	for _, nc := range cfg.Contexts {
		if nc.Name == name {
			isUpdate = true
			break
		}
	}
	if !isUpdate && host == "" {
		return fmt.Errorf("--host is required for new contexts")
	}
	warnClobberedContextRefs(cfg, name, host, envID, tokenRef)

	cfg.SetContext(name, host, envID, tokenRef, description)

	if cfg.CurrentContext == "" || len(cfg.Contexts) == 1 {
		cfg.CurrentContext = name
	}

	if err := a.saveConfig(cfg); err != nil {
		return err
	}
	output.PrintSuccess("Context %q set", name)
	if !isUpdate && envID == "" {
		output.PrintInfo("%q is a cluster context (no --env-id): use it with cluster commands, e.g. dtmgd get environments --context %s", name, name)
	}
	return nil
}

// deleteContext removes a context. With deleteCreds it also deletes the
// context's token, unless another context still uses it.
func (a *App) deleteContext(name string, deleteCreds bool) error {
	cfg, err := a.loadConfigRaw()
	if err != nil {
		return err
	}
	tokenRef := ""
	if nc, err := cfg.GetContext(name); err == nil {
		tokenRef = nc.Context.TokenRef
	}
	if err := cfg.DeleteContext(name); err != nil {
		return err
	}
	var deleteFromKeyring func() error
	if deleteCreds && tokenRef != "" {
		if deleteFromKeyring, err = dropContextToken(cfg, tokenRef); err != nil {
			return err
		}
	}
	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
		output.PrintWarning("Deleted current context. Use 'dtmgd ctx <name>' to set a new one.")
	}
	// The keyring entry goes only once the config no longer names it.
	if err := a.saveConfig(cfg); err != nil {
		return err
	}
	if deleteFromKeyring != nil {
		if err := deleteFromKeyring(); err != nil {
			return err
		}
		output.PrintSuccess("Credential %q deleted", tokenRef)
	}
	output.PrintSuccess("Context %q deleted", name)
	return nil
}

// dropContextToken removes the token of a deleted context from cfg, unless
// another context still uses it, and returns the function that removes it
// from the keyring (nil when nothing is left to delete there).
func dropContextToken(cfg *config.Config, tokenRef string) (func() error, error) {
	if users := contextsUsingToken(cfg, tokenRef); len(users) > 0 {
		output.PrintInfo("Kept credential %q: still used by %s.", tokenRef, strings.Join(users, ", "))
		return nil, nil
	}
	deleteFromKeyring, err := cfg.DeleteToken(tokenRef)
	if errors.Is(err, config.ErrTokenNotFound) {
		// Nothing to delete is no reason to keep the context.
		output.PrintInfo("Credential %q was already gone.", tokenRef)
		return nil, nil
	}
	return deleteFromKeyring, err
}
