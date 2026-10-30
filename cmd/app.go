package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// GlobalOptions holds the root command's persistent flags for one invocation.
type GlobalOptions struct {
	ConfigPath, Context, Output, Env, Columns string
	Verbose, MaxPages, Concurrency            int
	Agent, NoAgent, Plain                     bool
}

// App is one dtmgd invocation: where its output goes and the global flags it
// was given. Run builds a fresh App and command tree for every call, so no
// flag value outlives the run that parsed it.
type App struct {
	Out, Err io.Writer
	Opts     GlobalOptions

	// args are the raw arguments, for decisions that must be made even when
	// flag parsing failed (see rawAgentMode).
	args []string

	// watchStop ends a --watch loop when it closes, as Ctrl+C does. Only
	// tests set it; nil never fires.
	watchStop <-chan struct{}
}

// Run executes dtmgd with args (without the program name), writing to stdout
// and stderr, and returns the process exit code. main passes os.Args[1:];
// tests pass their own arguments and writers.
func Run(args []string, stdout, stderr io.Writer) int {
	a := &App{Out: stdout, Err: stderr, args: args}
	// Until flags are parsed, plain mode follows the environment alone; the
	// persistent pre-run applies --plain once it is known.
	output.SetStderr(stderr)
	output.SetPlain(a.plain())
	defer func() {
		output.SetStderr(nil)
		output.SetPlain(false)
	}()
	root := newRootCmd(a)
	root.SetArgs(rewriteNegativeArgs(args))
	cmd, err := root.ExecuteC()
	if err != nil {
		err = classifyCobraError(root, err)
		a.reportError(cmd, err)
		return exitCode(err)
	}
	return client.ExitSuccess
}

// newRootCmd builds the dtmgd command tree over a, binding the global flags
// to a.Opts.
func newRootCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "dtmgd",
		Short:         "Dynatrace Managed CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
		// classifyCobraError offers its own did-you-mean as a suggestion;
		// cobra's would be appended to the error message itself.
		DisableSuggestions: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			output.SetPlain(a.plain())
			f, err := output.ParseFormat(a.Opts.Output)
			if err != nil {
				return &UsageError{Err: err}
			}
			// Normalised, so every format check below sees "yaml" for yml.
			a.Opts.Output = string(f)
			if n := a.agentNotice(); n != "" {
				output.PrintInfo("%s", n)
			}
			return nil
		},
		Long: `dtmgd is a kubectl-inspired CLI for Dynatrace Managed (self-hosted) clusters.

It provides read access to problems, entities, events, logs, metrics, SLOs,
and security vulnerabilities via the Dynatrace Managed classic API, and to a
cluster's environments, nodes, license and backup configuration via the
Cluster Management API.

API endpoints used:
  Environment API:  {host}/e/{env-id}/api/v2/  (environment contexts)
  Cluster API:      {host}/api/cluster/v2/ and {host}/api/v1.0/onpremise/
                    (cluster contexts: no env-id)

Quick start:
  dtmgd config set-context prod --host https://managed.company.com --env-id abc123 --token-ref prod-token
  dtmgd config set-credentials prod-token   # prompts for the token
  dtmgd get problems

Multi-environment:
  dtmgd get problems --env ALL_ENVIRONMENTS
  dtmgd get problems --env "prod;staging"

Cluster (a context without --env-id, with a ServiceProviderAPI token):
  dtmgd ctx set prod-cluster --host https://cluster.company.com --token-ref prod-cluster-token
  dtmgd get environments --context prod-cluster`,
	}
	cmd.SetOut(a.Out)
	cmd.SetErr(a.Err)
	cmd.SetFlagErrorFunc(flagError)

	f := cmd.PersistentFlags()
	f.StringVar(&a.Opts.ConfigPath, "config", "", `config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)`)
	f.StringVarP(&a.Opts.Context, "context", "c", "", "context to use (overrides current-context)")
	f.StringVarP(&a.Opts.Output, "output", "o", "", "output format: table|wide|json|yaml|csv|tsv (default: table)")
	f.CountVarP(&a.Opts.Verbose, "verbose", "v", "verbosity level (-v for summary, -vv for full request/response)")
	f.StringVarP(&a.Opts.Env, "env", "e", "", `environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS`)
	f.BoolVarP(&a.Opts.Agent, "agent", "A", false, "force agent envelope output mode")
	f.BoolVar(&a.Opts.NoAgent, "no-agent", false, "disable auto-detected agent mode")
	f.BoolVar(&a.Opts.Plain, "plain", false, "plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)")
	f.IntVar(&a.Opts.MaxPages, "max-pages", 0, "maximum number of pages to fetch (0 = all pages)")
	f.StringVar(&a.Opts.Columns, "columns", "", "comma-separated list of columns to show in table, csv and tsv output")
	f.IntVar(&a.Opts.Concurrency, "concurrency", client.DefaultConcurrency,
		"maximum environments queried at once with --env")

	cmd.AddCommand(
		newGetCmd(a),
		newDescribeCmd(a),
		newQueryCmd(a),
		newConfigCmd(a),
		newCtxCmd(a),
		newSkillsCmd(a),
		newDoctorCmd(a),
		newCommandsCmd(a),
		newVersionCmd(a),
		newCompletionCmd(a),
	)
	fillScopeHelp(cmd)
	return cmd
}

// plain reports whether output should avoid glyphs and terminal control
// sequences: when asked with --plain or NO_COLOR, or when stdout is not a
// terminal (a pipe, a file, an agent's shell) unless FORCE_COLOR is set.
func (a *App) plain() bool {
	if a.Opts.Plain || os.Getenv("NO_COLOR") != "" {
		return true
	}
	return !output.IsTerminal(a.Out) && os.Getenv("FORCE_COLOR") == ""
}

// format returns the validated --output value.
func (a *App) format() output.Format {
	f, _ := output.ParseFormat(a.Opts.Output)
	return f
}

// agentMode returns true if agent envelope output should be used.
func (a *App) agentMode() bool {
	if a.Opts.NoAgent {
		return false
	}
	return a.Opts.Agent || output.DetectAgent()
}

// agentNotice returns the stderr line announcing that agent mode was switched
// on by an environment variable, or "" when no notice is due.
//
// Pentest note 5: with CLAUDECODE (or similar) exported in a CI job, every
// dtmgd call there silently switched to JSON envelopes, and table-parsing
// scripts broke without an error. An explicit --agent or --no-agent is the
// user's own choice, so only auto-detection is announced.
func (a *App) agentNotice() string {
	if a.Opts.Agent || a.Opts.NoAgent {
		return ""
	}
	info := agentmode.Detect()
	if !info.Detected {
		return ""
	}
	return fmt.Sprintf("Agent mode is on (detected %s). Use --no-agent for human-readable output.", info.Name)
}

// loadConfig loads the configuration, respecting --config and --context flags.
func (a *App) loadConfig() (*config.Config, error) {
	cfg, err := a.loadFromSource()
	if err != nil {
		return nil, err
	}

	// Context precedence: --env naming one context, then --context, then
	// DTMGD_CONTEXT, then the file's current-context.
	if ctx := os.Getenv("DTMGD_CONTEXT"); ctx != "" {
		cfg.CurrentContext = ctx
	}
	// Override current context from --context flag (single-env mode)
	if a.Opts.Context != "" {
		cfg.CurrentContext = a.Opts.Context
	}

	// If --env is a single named context (not ALL_ENVIRONMENTS or semicolon-separated),
	// treat it like --context so table output works correctly.
	if a.singleEnvName() != "" {
		cfg.CurrentContext = a.Opts.Env
	}
	return cfg, nil
}

// singleEnvName is --env when it names one context, "" otherwise.
func (a *App) singleEnvName() string {
	if env := a.Opts.Env; env != "" && !a.isMultiEnv() {
		return env
	}
	return ""
}

// checkEnvVarContext fails when DTMGD_CONTEXT chose the current context and
// cfg lacks it. Nothing on the command line names that context, so the error
// names the variable and the file. Only commands that call the API for the
// current context check it: ctx, config view and doctor must keep working so
// the user can find the right name.
func (a *App) checkEnvVarContext(cfg *config.Config) error {
	ctx := os.Getenv("DTMGD_CONTEXT")
	if ctx == "" || a.Opts.Context != "" || a.Opts.Env != "" {
		return nil
	}
	if _, err := cfg.GetContext(ctx); err != nil {
		return fmt.Errorf("DTMGD_CONTEXT names context %q, which is not in %s", ctx, config.ResolveSource(a.configPath()).Path)
	}
	return nil
}

// loadConfigRaw loads config without applying the --context override.
// Used by config management commands.
func (a *App) loadConfigRaw() (*config.Config, error) {
	return a.loadFromSource()
}

// loadConfigForUpdate loads the config that a command is about to modify and
// save. Only a missing file yields a fresh config; any other failure — a parse
// error, a permission problem — is returned, because saving over a file that
// could not be read would replace every context and token in it.
func (a *App) loadConfigForUpdate() (*config.Config, error) {
	cfg, err := a.loadConfigRaw()
	if errors.Is(err, fs.ErrNotExist) {
		return config.NewConfig(), nil
	}
	return cfg, err
}

// loadFromSource reads whichever config file ResolveSource selects, reporting
// the choice when the user did not make it themselves.
func (a *App) loadFromSource() (*config.Config, error) {
	src := config.ResolveSource(a.configPath())
	a.reportConfigSource(src)

	cfg, err := config.LoadFrom(src.Path)
	if err != nil {
		return nil, err
	}
	warnBareRefs(cfg)
	return cfg, nil
}

// reportConfigSource names the config file in effect when that choice is not
// visible to the user.
//
// FindLocalConfig walks upward from the working directory, so a .dtmgd.yaml
// planted in any shared ancestor — /tmp, a group-writable project root, a CI
// workspace — redirects every dtmgd run, host and token included, with
// nothing on screen to show for it. Naming the file is what makes that
// visible; it is the disclosure half of the fix, the other half being that
// LoadFrom no longer expands ${VAR} into attacker-chosen fields.
//
// The remaining sources are announced only under -v. A .dtmgd.yaml in the
// working directory is the documented project-local workflow that
// `dtmgd config init` creates, and an explicit --config path was typed by the
// user; warning on either would train people to ignore the warning that
// matters.
func (a *App) reportConfigSource(src config.Source) {
	if src.Invisible() {
		output.PrintWarning(
			"Using config %s — found in a parent directory, not %s. Check it before trusting this run.",
			src.Path, config.DefaultConfigPath())
		return
	}
	if a.Opts.Verbose > 0 {
		output.PrintInfo("Using config %s", src.Path)
	}
}

// configPath is the config file the user named: --config, else
// DTMGD_CONFIG, else "" (discover a local .dtmgd.yaml or use the default).
func (a *App) configPath() string {
	if a.Opts.ConfigPath != "" {
		return a.Opts.ConfigPath
	}
	return os.Getenv("DTMGD_CONFIG")
}

// saveConfig saves config, respecting the --config flag and local config presence.
func (a *App) saveConfig(cfg *config.Config) error {
	if p := a.configPath(); p != "" {
		return cfg.SaveTo(p)
	}
	if local := config.FindLocalConfig(); local != "" {
		return cfg.SaveTo(local)
	}
	return cfg.Save()
}

// clientOptions are the client settings this invocation's flags ask for.
func (a *App) clientOptions() client.Options {
	return client.Options{Verbosity: a.Opts.Verbose}
}

// printer returns a Printer writing to a.Out, chosen by the --output flag and
// agent mode. In agent mode, resource identifies the resource type in the
// envelope.
func (a *App) printer(resource string) output.Printer {
	if a.agentMode() {
		return output.NewAgentPrinter(a.Out, &agentmode.ResponseContext{Resource: resource})
	}
	format := a.Opts.Output
	if format == "" {
		format = "table"
	}
	return output.NewPrinterToWithColumns(format, a.Out, a.columns())
}

// columns returns the --columns list, trimmed and without empty entries.
func (a *App) columns() []string {
	if a.Opts.Columns == "" {
		return nil
	}
	parts := strings.Split(a.Opts.Columns, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isMultiEnv returns true if the --env flag specifies multiple environments.
func (a *App) isMultiEnv() bool {
	return a.Opts.Env == "ALL_ENVIRONMENTS" || strings.Contains(a.Opts.Env, ";")
}
