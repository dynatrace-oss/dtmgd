package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// ContextListItem is the table row for context listings.
type ContextListItem struct {
	Current     string `table:"CURRENT"`
	Name        string `table:"NAME"`
	Host        string `table:"HOST"`
	EnvID       string `table:"ENV-ID"`
	TokenRef    string `table:"TOKEN-REF"`
	Description string `table:"DESCRIPTION,wide"`
}

// ctxCmd is a top-level shortcut for context management.
var ctxCmd = &cobra.Command{
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
	ValidArgsFunction: contextCompletionFn,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return listContexts()
		}
		return useContext(args[0])
	},
}

var ctxCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Display the current context name",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		fmt.Println(cfg.CurrentContext)
		return nil
	},
}

var ctxSetCmd = &cobra.Command{
	Use:   "set <context-name>",
	Short: "Create or update a context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host, _ := cmd.Flags().GetString("host")
		envID, _ := cmd.Flags().GetString("env-id")
		tokenRef, _ := cmd.Flags().GetString("token-ref")
		description, _ := cmd.Flags().GetString("description")
		return setContext(args[0], host, envID, tokenRef, description)
	},
}

var ctxDeleteCmd = &cobra.Command{
	Use:               "delete <context-name>",
	Aliases:           []string{"rm"},
	Short:             "Delete a context",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: contextCompletionFn,
	RunE: func(cmd *cobra.Command, args []string) error {
		return deleteContext(args[0])
	},
}

// contextCompletionFn provides shell completion for context names.
func contextCompletionFn(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := LoadConfig()
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

func listContexts() error {
	cfg, err := LoadConfig()
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
			Host:        nc.Context.Host,
			EnvID:       nc.Context.EnvID,
			TokenRef:    nc.Context.TokenRef,
			Description: nc.Context.Description,
		})
	}

	return NewPrinter().PrintList(items)
}

func useContext(name string) error {
	cfg, err := loadConfigRaw()
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
	if err := saveConfig(cfg); err != nil {
		return err
	}
	output.PrintSuccess("Switched to context %q", name)
	return nil
}

func describeContext(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	var found *config.NamedContext
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == name {
			found = &cfg.Contexts[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("context %q not found", name)
	}

	const w = 12
	suffix := ""
	if found.Name == cfg.CurrentContext {
		suffix = " (current)"
	}
	output.DescribeKV("Name:", w, "%s%s", found.Name, suffix)
	output.DescribeKV("Host:", w, "%s", found.Context.Host)
	output.DescribeKV("Env-ID:", w, "%s", found.Context.EnvID)
	output.DescribeKV("Token-Ref:", w, "%s", found.Context.TokenRef)
	if found.Context.Description != "" {
		output.DescribeKV("Description:", w, "%s", found.Context.Description)
	}
	output.DescribeKV("API URL:", w, "%s/v2/", found.Context.APIBaseURL())
	return nil
}

func setContext(name, host, envID, tokenRef, description string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		cfg = config.NewConfig()
	}

	// For new contexts, host + env-id are required.
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
	if !isUpdate && envID == "" {
		return fmt.Errorf("--env-id is required for new contexts")
	}

	cfg.SetContext(name, host, envID, tokenRef, description)

	if cfg.CurrentContext == "" || len(cfg.Contexts) == 1 {
		cfg.CurrentContext = name
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}
	output.PrintSuccess("Context %q set", name)
	return nil
}

func deleteContext(name string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		return err
	}
	if err := cfg.DeleteContext(name); err != nil {
		return err
	}
	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
		output.PrintWarning("Deleted current context. Use 'dtmgd ctx <name>' to set a new one.")
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	output.PrintSuccess("Context %q deleted", name)
	return nil
}

func init() {
	rootCmd.AddCommand(ctxCmd)

	ctxCmd.AddCommand(ctxCurrentCmd)
	ctxCmd.AddCommand(ctxSetCmd)
	ctxCmd.AddCommand(ctxDeleteCmd)

	ctxSetCmd.Flags().String("host", "", "Managed cluster host URL")
	ctxSetCmd.Flags().String("env-id", "", "environment ID")
	ctxSetCmd.Flags().String("token-ref", "", "credential name")
	ctxSetCmd.Flags().String("description", "", "description")
}
