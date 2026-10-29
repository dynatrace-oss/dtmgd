package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// newConfigCmd builds the top-level config management command.
func newConfigCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage dtmgd configuration",
		Long:  `View and modify dtmgd configuration including contexts (Managed environments) and credentials.`,
		RunE:  requireSubcommand,
	}
	cmd.AddCommand(
		newConfigViewCmd(a),
		newConfigInitCmd(a),
		newConfigGetContextsCmd(a),
		newConfigCurrentContextCmd(a),
		newConfigUseContextCmd(a),
		newConfigSetContextCmd(a),
		newConfigSetCredentialsCmd(a),
		newConfigDeleteContextCmd(a),
		newConfigDeleteCredentialsCmd(a),
		newConfigMigrateTokensCmd(a),
		newConfigImportCmd(a),
	)
	return asGroup(cmd)
}

// newConfigViewCmd builds `config view`, which displays the current config.
func newConfigViewCmd(a *App) *cobra.Command {
	var opts struct {
		showTokens bool
	}
	cmd := &cobra.Command{
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
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			if !opts.showTokens {
				cfg = withMaskedTokens(cfg)
			}
			return a.printer("").Print(cfg)
		},
	}
	cmd.Flags().BoolVar(&opts.showTokens, "show-tokens", false, "print token values instead of masking them")
	return cmd
}

// tokenMask replaces a stored token value in displayed output.
const tokenMask = "*** (use --show-tokens to reveal)"

// withMaskedTokens returns a copy of cfg with literal token values and proxy
// passwords masked.
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
	masked.Contexts = make([]config.NamedContext, len(cfg.Contexts))
	copy(masked.Contexts, cfg.Contexts)
	for i := range masked.Contexts {
		c := &masked.Contexts[i].Context
		c.HTTPProxyURL = maskProxyPassword(c.HTTPProxyURL)
		c.HTTPSProxyURL = maskProxyPassword(c.HTTPSProxyURL)
	}
	return &masked
}

// maskProxyPassword masks the password in a proxy URL (user:xxxxx@host) and
// keeps the rest, so the output still shows which proxy a context uses. It is
// client.RedactProxy, which fails closed on URLs url.Parse rejects or
// misreads; a ${VAR} reference with no userinfo is left as written.
func maskProxyPassword(proxy string) string {
	if proxy == "" {
		return proxy
	}
	return client.RedactProxy(proxy)
}

// newConfigInitCmd builds `config init`, which creates a .dtmgd.yaml template
// in the current directory.
func newConfigInitCmd(a *App) *cobra.Command {
	var opts struct {
		force   bool
		context string
	}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a .dtmgd.yaml template in the current directory",
		Long: `Create a project-local .dtmgd.yaml configuration template.

Environment variables can be used with ${VAR_NAME} syntax.

Examples:
  dtmgd config init
  dtmgd config init --context my-cluster`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxName := opts.context

			if _, err := os.Stat(config.LocalConfigName); err == nil && !opts.force {
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
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite existing .dtmgd.yaml")
	cmd.Flags().StringVar(&opts.context, "context", "", "context name to pre-fill in template")
	return cmd
}

// newConfigGetContextsCmd builds `config get-contexts`, which lists all contexts.
func newConfigGetContextsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get-contexts",
		Short:   "List all available contexts",
		Aliases: []string{"get-ctx"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listContexts()
		},
	}
	return cmd
}

// newConfigCurrentContextCmd builds `config current-context`, which shows the
// current context name.
func newConfigCurrentContextCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current-context",
		Short: "Display the current context",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(a.Out, cfg.CurrentContext)
			return nil
		},
	}
	return cmd
}

// newConfigUseContextCmd builds `config use-context`, which switches to a
// different context.
func newConfigUseContextCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use-context <context-name>",
		Short: "Switch to a different context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.useContext(args[0])
		},
	}
	return cmd
}

// newConfigSetContextCmd builds `config set-context`, which creates or updates
// a context.
func newConfigSetContextCmd(a *App) *cobra.Command {
	var opts struct {
		host, envID, tokenRef, description string
	}
	cmd := &cobra.Command{
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
  %SCOPES%

Examples:
  dtmgd config set-context prod \
    --host https://managed.company.com \
    --env-id abc12345 \
    --token-ref prod-token

  dtmgd config set-credentials prod-token --token <api-token>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.setContext(args[0], opts.host, opts.envID, opts.tokenRef, opts.description)
		},
	}
	cmd.Flags().StringVar(&opts.host, "host", "", "Dynatrace Managed cluster URL (e.g. https://managed.company.com)")
	cmd.Flags().StringVar(&opts.envID, flagEnvID, "", "Environment ID")
	cmd.Flags().StringVar(&opts.tokenRef, flagTokenRef, "", "credential name (see set-credentials)")
	cmd.Flags().StringVar(&opts.description, "description", "", "human-readable description")
	return cmd
}

// newConfigSetCredentialsCmd builds `config set-credentials`, which stores an
// API token. Its token flags are read by tokenInputFromCmd, which also needs
// to know whether --token was given at all.
func newConfigSetCredentialsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
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
			return a.setCredentials(args[0], token)
		},
	}
	cmd.Flags().String(flagToken, "", "API token value (exposed in the process list — prefer --token-stdin)")
	cmd.Flags().Bool(flagTokenStdin, false, "read the API token from stdin")
	return cmd
}

// setCredentials stores an API token under the given name.
func (a *App) setCredentials(name, token string) error {
	cfg, err := a.loadConfigForUpdate()
	if err != nil {
		return err
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

	undo, err := cfg.SetTokenUndoable(name, token)
	if err != nil {
		return err
	}
	if err := a.saveConfig(cfg); err != nil {
		return errors.Join(err, undoKeyring(undo))
	}
	reportStoredCredential(name)
	return nil
}

// undoKeyring runs a keyring undo after a failed save. It adds to the error
// only when the undo itself fails, because the keyring then holds a token the
// config does not.
func undoKeyring(undo func() error) error {
	if err := undo(); err != nil {
		return fmt.Errorf("the keyring could not be restored either, so it may hold a token the config does not: %w", err)
	}
	return nil
}

// reportStoredCredential says where cfg.SetToken put a literal token: the OS
// keyring, or the config file in plaintext when no keyring is available.
func reportStoredCredential(name string) {
	if config.IsKeyringAvailable() {
		output.PrintSuccess("Credential %q stored securely in %s", name, config.KeyringBackend())
	} else {
		output.PrintWarning("Credential %q saved (plaintext — keyring not available)", name)
	}
}

// newConfigDeleteContextCmd builds `config delete-context`, which removes a
// context.
func newConfigDeleteContextCmd(a *App) *cobra.Command {
	var deleteCreds bool
	cmd := &cobra.Command{
		Use:     "delete-context <context-name>",
		Short:   "Delete a context from the config",
		Aliases: []string{"rm-ctx"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.deleteContext(args[0], deleteCreds)
		},
	}
	cmd.Flags().BoolVar(&deleteCreds, "delete-credentials", false, "also delete the context's token, unless another context uses it")
	return cmd
}

// newConfigDeleteCredentialsCmd builds `config delete-credentials`, which
// removes a stored token from the OS keyring and the config.
func newConfigDeleteCredentialsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "delete-credentials <token-name>",
		Short: "Delete a stored API token from the keyring and the config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.deleteCredentials(args[0])
		},
	}
}

// deleteCredentials deletes a token and names the contexts that still
// point at it, which will fail until they get a new one.
func (a *App) deleteCredentials(name string) error {
	cfg, err := a.loadConfigRaw()
	if err != nil {
		return err
	}
	deleteFromKeyring, err := cfg.DeleteToken(name)
	if err != nil {
		return err
	}
	if users := contextsUsingToken(cfg, name); len(users) > 0 {
		output.PrintWarning("Context(s) %s still use token-ref %q; they need a new credential.", strings.Join(users, ", "), name)
	}
	// The keyring entry goes only once the config no longer names it.
	if err := a.saveConfig(cfg); err != nil {
		return err
	}
	if err := deleteFromKeyring(); err != nil {
		return err
	}
	output.PrintSuccess("Credential %q deleted", name)
	return nil
}

// newConfigMigrateTokensCmd builds `config migrate-tokens`, which migrates
// plaintext tokens to the OS keyring.
func newConfigMigrateTokensCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-tokens",
		Short: "Migrate tokens from config file to OS keyring",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !config.IsKeyringAvailable() {
				return fmt.Errorf("keyring not available. Tokens will remain in config file")
			}
			cfg, err := a.loadConfigRaw()
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
			if err := a.saveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config after migration: %w", err)
			}
			output.PrintSuccess("Migrated %d token(s) to %s", migrated, config.KeyringBackend())
			return nil
		},
	}
	return cmd
}
