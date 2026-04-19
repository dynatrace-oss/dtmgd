package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

var (
	cfgFile      string
	contextName  string
	outputFormat string
	verbosity    int
	envSpec      string
	agentFlag    bool
	noAgentFlag  bool
	maxPages     int
	columns      string
)

// agentMode returns true if agent envelope output should be used.
func agentMode() bool {
	if noAgentFlag {
		return false
	}
	return agentFlag || output.DetectAgent()
}

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:           "dtmgd",
	Short:         "Dynatrace Managed CLI",
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `dtmgd is a kubectl-inspired CLI for Dynatrace Managed (self-hosted) clusters.

It provides read access to problems, entities, events, logs, metrics, SLOs,
and security vulnerabilities via the Dynatrace Managed classic API.

API endpoints used:
  Environment API:  {host}/e/{env-id}/api/v2/
  Cluster API:      {host}/api/v1.0/onpremise/

Quick start:
  dtmgd config set-context prod --host https://managed.company.com --env-id abc123 --token-ref prod-token
  dtmgd config set-credentials prod-token --token <your-api-token>
  dtmgd get problems

Multi-environment:
  dtmgd get problems --env ALL_ENVIRONMENTS
  dtmgd get problems --env "prod;staging"`,
}

// isDescribeProblemArgs returns true when args contains a "describe problem"
// (or "describe prob") subcommand prefix, indicating that a positional
// argument may be a negative-integer problem ID.
func isDescribeProblemArgs(args []string) bool {
	for i, a := range args {
		if (a == "describe" || a == "desc") && i+1 < len(args) {
			next := args[i+1]
			if next == "problem" || next == "prob" {
				return true
			}
		}
	}
	return false
}

// rewriteNegativeArgs inserts "--" before the first argument that begins with
// "-" followed by a digit (e.g. a negative-integer problem ID such as
// "-6546711275898328738_1776193140000V2"), preventing cobra/pflag from
// misinterpreting it as a flag.  Any flag-like arguments that appear after
// the negative-integer arg are moved in front of the "--" so that global
// flags such as -c, -o, and -v continue to work in their natural position.
//
// This rewrite is only applied for "describe problem" subcommands, where
// negative-integer IDs can legitimately appear as positional arguments.
func rewriteNegativeArgs(args []string) []string {
	if !isDescribeProblemArgs(args) {
		return args
	}
	negIdx := -1
	for i, a := range args {
		if len(a) >= 2 && a[0] == '-' && a[1] >= '0' && a[1] <= '9' {
			negIdx = i
			break
		}
	}
	if negIdx == -1 {
		return args
	}
	// "--" already present before the negative ID — nothing to do.
	for i := 0; i < negIdx; i++ {
		if args[i] == "--" {
			return args
		}
	}
	before := args[:negIdx]
	negID := args[negIdx]
	after := args[negIdx+1:]

	// Move flag-like tokens (starting with -<letter> or --<word>) that come
	// after the negative ID to before the "--", so they are still parsed by
	// cobra.  Stop at a bare "--" or any non-flag token.
	var movedFlags []string
	var positionalAfter []string
	for i := 0; i < len(after); {
		a := after[i]
		if a == "--" {
			// Hard stop: pass remaining args (including this "--") through as-is.
			positionalAfter = append(positionalAfter, after[i:]...)
			break
		}
		isFlagLike := len(a) >= 2 && a[0] == '-' &&
			(a[1] == '-' || (a[1] >= 'a' && a[1] <= 'z') || (a[1] >= 'A' && a[1] <= 'Z'))
		if isFlagLike {
			movedFlags = append(movedFlags, a)
			// If the next token does not look like a flag, treat it as the flag value.
			if i+1 < len(after) && (len(after[i+1]) == 0 || after[i+1][0] != '-') {
				movedFlags = append(movedFlags, after[i+1])
				i += 2
				continue
			}
			i++
		} else {
			positionalAfter = append(positionalAfter, after[i:]...)
			break
		}
	}

	out := make([]string, 0, len(args)+1)
	out = append(out, before...)
	out = append(out, movedFlags...)
	out = append(out, "--")
	out = append(out, negID)
	out = append(out, positionalAfter...)
	return out
}

// Execute runs the CLI.
func Execute() {
	if len(os.Args) > 1 {
		os.Args = append(os.Args[:1], rewriteNegativeArgs(os.Args[1:])...)
	}
	if err := rootCmd.Execute(); err != nil {
		err = client.WrapWithDiagnosis(err)
		if agentMode() {
			output.PrintAgentError(err)
		} else {
			output.PrintHumanError("%s", err)
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/dtmgd/config)")
	rootCmd.PersistentFlags().StringVarP(&contextName, "context", "c", "", "context to use (overrides current-context)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "output format: table|json|yaml|wide (default: table)")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "verbosity level (-v for summary, -vv for full request/response)")
	rootCmd.PersistentFlags().StringVarP(&envSpec, "env", "e", "", `environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS`)
	rootCmd.PersistentFlags().BoolVarP(&agentFlag, "agent", "A", false, "force agent envelope output mode")
	rootCmd.PersistentFlags().BoolVar(&noAgentFlag, "no-agent", false, "disable auto-detected agent mode")
	rootCmd.PersistentFlags().IntVar(&maxPages, "max-pages", 0, "maximum number of pages to fetch (0 = all pages)")
	rootCmd.PersistentFlags().StringVar(&columns, "columns", "", "comma-separated list of columns to show in table output")
}

// LoadConfig loads the configuration, respecting --config and --context flags.
func LoadConfig() (*config.Config, error) {
	var cfg *config.Config
	var err error

	if cfgFile != "" {
		cfg, err = config.LoadFrom(cfgFile)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil, err
	}

	// Override current context from --context flag (single-env mode)
	if contextName != "" {
		cfg.CurrentContext = contextName
	}

	// If --env is a single named context (not ALL_ENVIRONMENTS or semicolon-separated),
	// treat it like --context so table output works correctly.
	if envSpec != "" && envSpec != "ALL_ENVIRONMENTS" && !strings.Contains(envSpec, ";") {
		cfg.CurrentContext = envSpec
	}

	return cfg, nil
}

// loadConfigRaw loads config without applying the --context override.
// Used by config management commands.
func loadConfigRaw() (*config.Config, error) {
	if cfgFile != "" {
		return config.LoadFrom(cfgFile)
	}
	return config.Load()
}

// saveConfig saves config, respecting the --config flag and local config presence.
func saveConfig(cfg *config.Config) error {
	if cfgFile != "" {
		return cfg.SaveTo(cfgFile)
	}
	if local := config.FindLocalConfig(); local != "" {
		return cfg.SaveTo(local)
	}
	return cfg.Save()
}

// NewClientFromConfig creates an API client from the current config.
func NewClientFromConfig(cfg *config.Config) (*client.Client, error) {
	c, err := client.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	c.SetVerbosity(verbosity)
	return c, nil
}

// NewPrinter returns a Printer based on the --output flag and agent mode.
// For agent mode, resource should be passed to identify the resource type.
func NewPrinter() output.Printer {
	if agentMode() {
		return output.NewAgentPrinter("")
	}
	format := outputFormat
	if format == "" {
		format = "table"
	}
	return output.NewPrinterToWithColumns(format, nil, parseColumns())
}

// NewPrinterForResource returns a Printer with resource context (for agent mode).
func NewPrinterForResource(resource string) output.Printer {
	if agentMode() {
		return output.NewAgentPrinter(resource)
	}
	format := outputFormat
	if format == "" {
		format = "table"
	}
	return output.NewPrinterToWithColumns(format, nil, parseColumns())
}

func parseColumns() []string {
	if columns == "" {
		return nil
	}
	parts := strings.Split(columns, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// NewClientWithHostEnv creates an API client for a specific host/env/token.
func NewClientWithHostEnv(host, envID, token string) (*client.Client, error) {
	c, err := client.New(host, envID, token)
	if err != nil {
		return nil, err
	}
	c.SetVerbosity(verbosity)
	return c, nil
}

// isMultiEnv returns true if the --env flag specifies multiple environments.
func isMultiEnv() bool {
	return envSpec == "ALL_ENVIRONMENTS" || strings.Contains(envSpec, ";")
}

// effectiveMaxPages returns the page limit. When --limit is set, fetch only 1 page
// (the API's pageSize already limits results). Otherwise use --max-pages.
func effectiveMaxPages(hasLimit bool) int {
	if hasLimit {
		return 1
	}
	return maxPages
}

// effectiveEnvSpec returns the env spec from the --env flag.
func effectiveEnvSpec() string {
	return envSpec
}

// multiExec runs an API call against one or more environments based on --env flag.
// Returns the unwrapped result (single value or map of name→value).
func multiExec(cfg *config.Config, apiCall func(c *client.Client) (interface{}, error)) (interface{}, error) {
	spec := effectiveEnvSpec()
	if spec == "" {
		// Single environment — use current context directly
		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return nil, err
		}
		return apiCall(c)
	}
	results, err := client.MultiRequest(cfg, spec, apiCall)
	if err != nil {
		return nil, err
	}
	return client.UnwrapSingle(results)
}

func requireSubcommand(cmd *cobra.Command, args []string) error {
	if err := cmd.Help(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr)
	return fmt.Errorf("a subcommand is required")
}
