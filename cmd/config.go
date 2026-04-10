package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// configCmd is the top-level config management command.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage dtmgd configuration",
	Long:  `View and modify dtmgd configuration including contexts (Managed environments) and credentials.`,
}

// configViewCmd displays the current config.
var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Display the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		return NewPrinter().Print(cfg)
	},
}

// configInitCmd creates a .dtmgd.yaml template in the current directory.
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a .dtmgd.yaml template in the current directory",
	Long: `Create a project-local .dtmgd.yaml configuration template.

Environment variables can be used with ${VAR_NAME} syntax.

Examples:
  dtmgd config init
  dtmgd config init --context my-cluster`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		ctxName, _ := cmd.Flags().GetString("context")

		if _, err := os.Stat(config.LocalConfigName); err == nil && !force {
			return fmt.Errorf("%s already exists. Use --force to overwrite", config.LocalConfigName)
		}

		if ctxName == "" {
			ctxName = "my-cluster"
		}

		template := &config.Config{
			APIVersion:     "dtmgd.io/v1",
			Kind:           "Config",
			CurrentContext: ctxName,
			Contexts: []config.NamedContext{
				{
					Name: ctxName,
					Context: config.Context{
						Host:        "${DT_MANAGED_HOST}",
						EnvID:       "${DT_ENV_ID}",
						TokenRef:    "my-token",
						Description: "Dynatrace Managed environment",
					},
				},
			},
			Tokens: []config.NamedToken{
				{Name: "my-token", Token: "${DT_API_TOKEN}"},
			},
			Preferences: config.Preferences{Output: "table"},
		}

		if err := template.SaveTo(config.LocalConfigName); err != nil {
			return fmt.Errorf("failed to write %s: %w", config.LocalConfigName, err)
		}

		output.PrintSuccess("Created %s", config.LocalConfigName)
		output.PrintInfo("Edit this file to configure your Dynatrace Managed connection.")
		output.PrintInfo("Environment variables can be used with ${VAR_NAME} syntax.")
		return nil
	},
}

// configGetContextsCmd lists all contexts.
var configGetContextsCmd = &cobra.Command{
	Use:     "get-contexts",
	Short:   "List all available contexts",
	Aliases: []string{"get-ctx"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return listContexts()
	},
}

// configCurrentContextCmd shows the current context name.
var configCurrentContextCmd = &cobra.Command{
	Use:   "current-context",
	Short: "Display the current context",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		fmt.Println(cfg.CurrentContext)
		return nil
	},
}

// configUseContextCmd switches to a different context.
var configUseContextCmd = &cobra.Command{
	Use:   "use-context <context-name>",
	Short: "Switch to a different context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return useContext(args[0])
	},
}

// configSetContextCmd creates or updates a context.
var configSetContextCmd = &cobra.Command{
	Use:   "set-context <context-name>",
	Short: "Create or update a context entry",
	Long: `Create or update a context that points to a Dynatrace Managed environment.

A context stores three things:
  --host       Base URL of the Dynatrace Managed cluster
               (e.g. https://managed.company.com)
  --env-id     Environment identifier shown in the Managed UI
               (e.g. "abc12345")
  --token-ref  Name of the API token credential (see set-credentials)

Required API token scopes:
  DataExport, ReadConfig, ReadSyntheticData, ReadLogContent,
  ReadEvents, ReadProblems, ReadSecurityProblems, ReadSLO

Examples:
  dtmgd config set-context prod \
    --host https://managed.company.com \
    --env-id abc12345 \
    --token-ref prod-token

  dtmgd config set-credentials prod-token --token <api-token>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host, _ := cmd.Flags().GetString("host")
		envID, _ := cmd.Flags().GetString("env-id")
		tokenRef, _ := cmd.Flags().GetString("token-ref")
		description, _ := cmd.Flags().GetString("description")

		return setContext(args[0], host, envID, tokenRef, description)
	},
}

// configSetCredentialsCmd stores an API token.
var configSetCredentialsCmd = &cobra.Command{
	Use:     "set-credentials <name>",
	Short:   "Store an API token credential",
	Aliases: []string{"set-creds"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token, _ := cmd.Flags().GetString("token")
		if token == "" {
			return fmt.Errorf("--token is required")
		}

		cfg, err := loadConfigRaw()
		if err != nil {
			cfg = config.NewConfig()
		}

		if err := cfg.SetToken(args[0], token); err != nil {
			return err
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}

		if config.IsKeyringAvailable() {
			output.PrintSuccess("Credential %q stored securely in %s", args[0], config.KeyringBackend())
		} else {
			output.PrintWarning("Credential %q saved (plaintext — keyring not available)", args[0])
		}
		return nil
	},
}

// configDeleteContextCmd removes a context.
var configDeleteContextCmd = &cobra.Command{
	Use:     "delete-context <context-name>",
	Short:   "Delete a context from the config",
	Aliases: []string{"rm-ctx"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return deleteContext(args[0])
	},
}

// configMigrateTokensCmd migrates plaintext tokens to the OS keyring.
var configMigrateTokensCmd = &cobra.Command{
	Use:   "migrate-tokens",
	Short: "Migrate tokens from config file to OS keyring",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !config.IsKeyringAvailable() {
			return fmt.Errorf("keyring not available. Tokens will remain in config file")
		}
		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}
		migrated, err := config.MigrateTokensToKeyring(cfg)
		if err != nil {
			return err
		}
		if migrated == 0 {
			output.PrintInfo("No tokens to migrate")
			return nil
		}
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config after migration: %w", err)
		}
		output.PrintSuccess("Migrated %d token(s) to %s", migrated, config.KeyringBackend())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configGetContextsCmd)
	configCmd.AddCommand(configCurrentContextCmd)
	configCmd.AddCommand(configUseContextCmd)
	configCmd.AddCommand(configSetContextCmd)
	configCmd.AddCommand(configSetCredentialsCmd)
	configCmd.AddCommand(configDeleteContextCmd)
	configCmd.AddCommand(configMigrateTokensCmd)

	configInitCmd.Flags().Bool("force", false, "overwrite existing .dtmgd.yaml")
	configInitCmd.Flags().String("context", "", "context name to pre-fill in template")

	configSetContextCmd.Flags().String("host", "", "Dynatrace Managed cluster URL (e.g. https://managed.company.com)")
	configSetContextCmd.Flags().String("env-id", "", "Environment ID")
	configSetContextCmd.Flags().String("token-ref", "", "credential name (see set-credentials)")
	configSetContextCmd.Flags().String("description", "", "human-readable description")

	configSetCredentialsCmd.Flags().String("token", "", "API token value")
}
