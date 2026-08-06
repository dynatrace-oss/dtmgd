package cmd

import (
	"fmt"
	"os"
	"strings"

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
	Long: `Display the current configuration.

Token values are masked. Pass --show-tokens to print them verbatim.

Masking matters only where the OS keyring is unavailable — headless Linux,
containers, CI runners — because that is where SetToken falls back to writing
the token into the config file rather than leaving an empty placeholder. That
is also where the output of this command is most likely to be captured in a
build log.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		showTokens, _ := cmd.Flags().GetBool("show-tokens")
		if !showTokens {
			cfg = withMaskedTokens(cfg)
		}
		return NewPrinter().Print(cfg)
	},
}

// tokenMask replaces a stored token value in displayed output.
const tokenMask = "*** (use --show-tokens to reveal)"

// withMaskedTokens returns a copy of cfg with literal token values masked.
//
// The copy is what keeps this safe to call from a command that shares the
// loaded Config: masking in place would put the mask string one saveConfig
// away from replacing the real credential on disk.
//
// A ${VAR} placeholder is left alone. It is a reference to a value, not the
// value, and showing it is how a user confirms which variable a context reads
// — masking it would hide configuration while protecting nothing.
func withMaskedTokens(cfg *config.Config) *config.Config {
	masked := *cfg
	masked.Tokens = make([]config.NamedToken, len(cfg.Tokens))
	copy(masked.Tokens, cfg.Tokens)
	for i, nt := range masked.Tokens {
		if nt.Token == "" || config.HasPlaceholder(nt.Token) {
			continue
		}
		masked.Tokens[i].Token = tokenMask
	}
	return &masked
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
		envID, _ := cmd.Flags().GetString(flagEnvID)
		tokenRef, _ := cmd.Flags().GetString(flagTokenRef)
		description, _ := cmd.Flags().GetString("description")

		return setContext(args[0], host, envID, tokenRef, description)
	},
}

// configSetCredentialsCmd stores an API token.
var configSetCredentialsCmd = &cobra.Command{
	Use:     "set-credentials <name>",
	Short:   "Store an API token credential",
	Aliases: []string{"set-creds"},
	Long: `Store an API token under a name that contexts refer to with --token-ref.

The token is read from the terminal by default, with echo disabled, so it
reaches neither the screen nor your shell history:

  dtmgd config set-credentials prod-token

For scripts and CI, pipe it in on stdin:

  echo "$DT_API_TOKEN" | dtmgd config set-credentials prod-token --token-stdin
  dtmgd config set-credentials prod-token --token-stdin < /run/secrets/dt-token

--token is still accepted, but a value passed there is copied into the process
argument vector by the kernel and is readable from /proc/<pid>/cmdline by any
process running as you, as well as being recorded in your shell history.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := tokenInputFromCmd(cmd).resolve()
		if err != nil {
			return err
		}
		return setCredentials(args[0], token)
	},
}

// setCredentials stores an API token under the given name.
func setCredentials(name, token string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		cfg = config.NewConfig()
	}

	// Warn before the placeholder disappears. This matters most in the keyring
	// case, where SetToken writes token: "" and the ${VAR} reference vanishes
	// without the new token ever appearing in the file.
	for _, nt := range cfg.Tokens {
		if nt.Name == name && config.HasPlaceholder(nt.Token) {
			refs := strings.Join(config.PlaceholderRefs(nt.Token), ", ")
			output.PrintWarning("Replaced %s in the config — that environment variable no longer affects this token.", refs)
			break
		}
	}

	if err := cfg.SetToken(name, token); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	if config.IsKeyringAvailable() {
		output.PrintSuccess("Credential %q stored securely in %s", name, config.KeyringBackend())
	} else {
		output.PrintWarning("Credential %q saved (plaintext — keyring not available)", name)
	}
	return nil
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
		migrated, skipped, err := config.MigrateTokensToKeyring(cfg)
		if err != nil {
			return err
		}
		for _, name := range skipped {
			output.PrintWarning("Skipped %q: it resolves from an environment variable; migrating would freeze the current value", name)
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
	configSetContextCmd.Flags().String(flagEnvID, "", "Environment ID")
	configSetContextCmd.Flags().String(flagTokenRef, "", "credential name (see set-credentials)")
	configSetContextCmd.Flags().String("description", "", "human-readable description")

	configViewCmd.Flags().Bool("show-tokens", false, "print token values instead of masking them")

	configSetCredentialsCmd.Flags().String(flagToken, "", "API token value (exposed in the process list — prefer --token-stdin)")
	configSetCredentialsCmd.Flags().Bool(flagTokenStdin, false, "read the API token from stdin")
}
