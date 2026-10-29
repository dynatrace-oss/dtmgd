package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// mcpEnvVar is the variable the Dynatrace Managed MCP server reads its
// environments from when no DT_CONFIG_FILE is set.
const mcpEnvVar = "DT_ENVIRONMENT_CONFIGS"

// mcpEnvironment is one environment of a Dynatrace Managed MCP server config,
// in the MCP server's own field names.
type mcpEnvironment struct {
	Alias, APIEndpointURL, EnvironmentID, APIToken, HTTPProxyURL, HTTPSProxyURL string
}

// mcpFields are the MCP fields import maps to a dtmgd setting. Any other
// field (dynatraceUrl, …) is ignored and named once on stderr.
var mcpFields = map[string]func(*mcpEnvironment) *string{
	"alias":          func(e *mcpEnvironment) *string { return &e.Alias },
	"apiEndpointUrl": func(e *mcpEnvironment) *string { return &e.APIEndpointURL },
	"environmentId":  func(e *mcpEnvironment) *string { return &e.EnvironmentID },
	"apiToken":       func(e *mcpEnvironment) *string { return &e.APIToken },
	"httpProxyUrl":   func(e *mcpEnvironment) *string { return &e.HTTPProxyURL },
	"httpsProxyUrl":  func(e *mcpEnvironment) *string { return &e.HTTPSProxyURL },
}

// importOptions are the arguments and flags of `config import`.
type importOptions struct {
	file                       string
	fromEnv, overwrite, dryRun bool
}

// importAction is one environment import will write, and whether it
// replaces an existing context.
type importAction struct {
	env     mcpEnvironment
	replace bool
}

// importRow is one line of the --dry-run listing.
type importRow struct {
	Name       string `table:"NAME"`
	Action     string `table:"ACTION"`
	Host       string `table:"HOST"`
	EnvID      string `table:"ENV-ID"`
	TokenRef   string `table:"TOKEN-REF"`
	Token      string `table:"TOKEN"`
	HTTPProxy  string `table:"HTTP-PROXY,wide"`
	HTTPSProxy string `table:"HTTPS-PROXY,wide"`
}

// newConfigImportCmd builds `config import`, which creates contexts from a
// Dynatrace Managed MCP server config.
func newConfigImportCmd(a *App) *cobra.Command {
	var opts importOptions
	cmd := &cobra.Command{
		Use:   "import (<file> | --from-env)",
		Short: "Import contexts from a Dynatrace Managed MCP server config",
		Long: `Import contexts from a Dynatrace Managed MCP server configuration: the file
DT_CONFIG_FILE names (a YAML or JSON array; the extension decides which), or,
with --from-env, the JSON in DT_ENVIRONMENT_CONFIGS.

Each environment becomes a context named after its alias:
  alias           context name
  apiEndpointUrl  host (trailing / trimmed)
  environmentId   env-id (trailing / trimmed)
  apiToken        a token named <alias>-token, the context's token-ref
  httpProxyUrl    http-proxy
  httpsProxyUrl   https-proxy
Other fields, such as dynatraceUrl, are ignored.

A literal apiToken is stored as set-credentials stores one: in the OS keyring
when available, otherwise in the config file. An apiToken that is or contains
a ${VAR} reference is written to the config as the reference. It is never
expanded, so the secret it names reaches neither the file nor the keyring.

An alias that names an existing context is skipped unless --overwrite is
given. Nothing is written if any environment is invalid.

Examples:
  dtmgd config import dt-config.yaml --dry-run
  dtmgd config import dt-config.yaml
  dtmgd config import --from-env --overwrite`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.file = args[0]
			}
			return a.importMCPConfig(opts)
		},
	}
	cmd.Flags().BoolVar(&opts.fromEnv, "from-env", false, "read the environments from "+mcpEnvVar)
	cmd.Flags().BoolVar(&opts.overwrite, "overwrite", false, "replace existing contexts with the same name")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show what would be imported without writing anything")
	return cmd
}

// importMCPConfig imports the environments opts names. The source is read
// and validated in full before the config is loaded, so an invalid source
// writes nothing.
func (a *App) importMCPConfig(opts importOptions) error {
	envs, ignored, err := readMCPSource(opts.file, opts.fromEnv)
	if err != nil {
		return err
	}
	if err := validateMCPEnvironments(envs); err != nil {
		return err
	}
	cfg, err := a.loadConfigForUpdate()
	if err != nil {
		return err
	}
	if len(ignored) > 0 {
		output.PrintInfo("Ignored MCP field(s) dtmgd has no setting for: %s", strings.Join(ignored, ", "))
	}
	plan := planMCPImport(cfg, envs, opts.overwrite)
	if opts.dryRun {
		return a.printImportPlan(plan)
	}
	if len(plan) == 0 {
		output.PrintInfo("Nothing imported.")
		return nil
	}
	keyringDeletes, err := applyMCPImport(cfg, plan)
	if err != nil {
		return err
	}
	// Stale keyring entries go only once the config no longer relies on them.
	if err := a.saveConfig(cfg); err != nil {
		return err
	}
	for _, del := range keyringDeletes {
		if err := del(); err != nil {
			return err
		}
	}
	for _, act := range plan {
		reportImported(cfg, act)
	}
	return nil
}

// readMCPSource reads the MCP environments from file or, with fromEnv, from
// DT_ENVIRONMENT_CONFIGS, and returns them with the sorted names of the
// fields it ignored.
func readMCPSource(file string, fromEnv bool) ([]mcpEnvironment, []string, error) {
	data, name, ext, err := mcpSourceData(file, fromEnv)
	if err != nil {
		return nil, nil, err
	}
	raw, err := decodeMCPArray(data, name, ext)
	if err != nil {
		return nil, nil, err
	}
	return parseMCPEnvironments(raw)
}

// mcpSourceData returns the raw source, a name for it in messages, and the
// extension that decides its format.
func mcpSourceData(file string, fromEnv bool) ([]byte, string, string, error) {
	switch {
	case file != "" && fromEnv:
		return nil, "", "", &UsageError{Err: errors.New("give a file or --from-env, not both")}
	case fromEnv:
		v := os.Getenv(mcpEnvVar)
		if v == "" {
			return nil, "", "", &UsageError{Err: fmt.Errorf("--from-env: %s is not set", mcpEnvVar)}
		}
		return []byte(v), mcpEnvVar, ".json", nil
	case file == "":
		return nil, "", "", &UsageError{Err: fmt.Errorf("name an MCP config file, or pass --from-env to read %s", mcpEnvVar)}
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return nil, "", "", &UsageError{Err: fmt.Errorf("unsupported file format %q for %s: use .json, .yaml or .yml", ext, file)}
	}
	// #nosec G304 -- reading the file the user named is this command's purpose.
	data, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read MCP config: %w", err)
	}
	return data, file, ext, nil
}

// decodeMCPArray parses data as JSON or YAML and checks that it is an array,
// as the MCP server requires.
func decodeMCPArray(data []byte, name, ext string) ([]interface{}, error) {
	var v interface{}
	var err error
	if ext == ".json" {
		err = json.Unmarshal(data, &v)
	} else {
		err = yaml.Unmarshal(data, &v)
	}
	if err != nil {
		return nil, &UsageError{Err: fmt.Errorf("failed to parse %s: %w", name, err)}
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil, &UsageError{Err: fmt.Errorf("%s: configuration must be an array of environments", name)}
	}
	return arr, nil
}

// parseMCPEnvironments converts the decoded array to environments, collecting
// the names of fields dtmgd has no setting for.
func parseMCPEnvironments(raw []interface{}) ([]mcpEnvironment, []string, error) {
	envs := make([]mcpEnvironment, 0, len(raw))
	ignored := map[string]bool{}
	for i, item := range raw {
		fields, ok := item.(map[string]interface{})
		if !ok {
			return nil, nil, &UsageError{Err: fmt.Errorf("environment #%d must be an object", i+1)}
		}
		var env mcpEnvironment
		for key, value := range fields {
			if field, known := mcpFields[key]; known {
				*field(&env) = mcpString(value)
			} else {
				ignored[key] = true
			}
		}
		envs = append(envs, env)
	}
	names := make([]string, 0, len(ignored))
	for key := range ignored {
		names = append(names, key)
	}
	sort.Strings(names)
	return envs, names, nil
}

// mcpString renders a field value as the MCP server does with toString: an
// environmentId may be written as a number.
func mcpString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// validateMCPEnvironments reports every environment the MCP server would
// reject, plus aliases dtmgd cannot use as context names, as one usage error.
// Messages for problems the MCP server also checks are the MCP server's own.
func validateMCPEnvironments(envs []mcpEnvironment) error {
	var problems []string
	seen := map[string]bool{}
	for i, env := range envs {
		problems = append(problems, missingMCPFields(i, env)...)
		switch {
		case strings.Contains(env.Alias, ";"):
			problems = append(problems, fmt.Sprintf(`Invalid alias found: "%s". Aliases are mandatory and cannot contain semicolons.`, env.Alias))
		case config.HasPlaceholder(env.Alias):
			problems = append(problems, fmt.Sprintf(`Invalid alias found: "%s". dtmgd does not expand ${VAR} in context names; write the name out.`, env.Alias))
		case env.Alias != "" && seen[env.Alias]:
			problems = append(problems, fmt.Sprintf(`Duplicate alias found: "%s". Aliases must be unique across environments.`, env.Alias))
		}
		seen[env.Alias] = true
	}
	if len(problems) == 0 {
		return nil
	}
	return &UsageError{Err: errors.New(strings.Join(problems, "\n"))}
}

// missingMCPFields reports the required fields env lacks. apiToken is not
// required: the MCP server's HTTP mode takes tokens from a request header.
func missingMCPFields(i int, env mcpEnvironment) []string {
	alias := env.Alias
	if alias == "" {
		alias = "N/A"
	}
	var problems []string
	for _, f := range []struct{ key, value string }{
		{"apiEndpointUrl", env.APIEndpointURL},
		{"environmentId", env.EnvironmentID},
		{"alias", env.Alias},
	} {
		if f.value == "" {
			problems = append(problems, fmt.Sprintf(`Key "%s" is empty or missing (environment #%d, alias: %s).`, f.key, i+1, alias))
		}
	}
	return problems
}

// importTokenName is the token an imported context refers to.
func importTokenName(alias string) string {
	return alias + "-token"
}

// planMCPImport decides which environments to write, warning about each one
// it skips.
func planMCPImport(cfg *config.Config, envs []mcpEnvironment, overwrite bool) []importAction {
	var plan []importAction
	for _, env := range envs {
		_, err := cfg.GetContext(env.Alias)
		exists := err == nil
		if reason := importConflict(cfg, env, exists, overwrite); reason != "" {
			output.PrintWarning("Skipped %q: %s", env.Alias, reason)
			continue
		}
		plan = append(plan, importAction{env: env, replace: exists})
	}
	return plan
}

// importConflict returns why env must not be written, or "" if it may be.
//
// The token name is checked as well as the context name. A token another
// context uses is never replaced, even with --overwrite: that would silently
// change the other context's credential.
func importConflict(cfg *config.Config, env mcpEnvironment, exists, overwrite bool) string {
	tokenName := importTokenName(env.Alias)
	if env.APIToken != "" {
		if others := otherContextsUsingToken(cfg, tokenName, env.Alias); len(others) > 0 {
			return fmt.Sprintf("its token %q is also used by context(s) %s; importing would change their credential", tokenName, strings.Join(others, ", "))
		}
	}
	if overwrite {
		return ""
	}
	if exists {
		return "a context with that name already exists; pass --overwrite to replace it"
	}
	if _, found := configToken(cfg, tokenName); found && env.APIToken != "" {
		return fmt.Sprintf("a token named %q already exists; pass --overwrite to replace it", tokenName)
	}
	return ""
}

// otherContextsUsingToken returns the quoted names of the contexts other than
// self whose token-ref is tokenRef.
func otherContextsUsingToken(cfg *config.Config, tokenRef, self string) []string {
	var names []string
	for _, nc := range cfg.Contexts {
		if nc.Name != self && nc.Context.TokenRef == tokenRef {
			names = append(names, strconv.Quote(nc.Name))
		}
	}
	return names
}

// configToken returns the config's entry for the named token, if any.
func configToken(cfg *config.Config, name string) (string, bool) {
	for _, nt := range cfg.Tokens {
		if nt.Name == name {
			return nt.Token, true
		}
	}
	return "", false
}

// importedContext is the context env becomes.
func importedContext(env mcpEnvironment) config.NamedContext {
	return config.NamedContext{
		Name: env.Alias,
		Context: config.Context{
			Host:          strings.TrimRight(env.APIEndpointURL, "/"),
			EnvID:         strings.TrimRight(env.EnvironmentID, "/"),
			TokenRef:      importTokenName(env.Alias),
			HTTPProxyURL:  env.HTTPProxyURL,
			HTTPSProxyURL: env.HTTPSProxyURL,
		},
	}
}

// applyMCPImport writes the planned contexts and tokens into cfg, and returns
// the keyring deletions to run once cfg is saved (see storeImportedToken). The
// first imported context becomes current when none is set.
func applyMCPImport(cfg *config.Config, plan []importAction) ([]func() error, error) {
	var keyringDeletes []func() error
	for _, act := range plan {
		putContext(cfg, importedContext(act.env))
		del, err := storeImportedToken(cfg, importTokenName(act.env.Alias), act.env.APIToken)
		if err != nil {
			return nil, err
		}
		if del != nil {
			keyringDeletes = append(keyringDeletes, del)
		}
	}
	if cfg.CurrentContext == "" {
		cfg.CurrentContext = plan[0].env.Alias
	}
	return keyringDeletes, nil
}

// putContext replaces the context of the same name in place, or appends nc.
func putContext(cfg *config.Config, nc config.NamedContext) {
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == nc.Name {
			cfg.Contexts[i] = nc
			return
		}
	}
	cfg.Contexts = append(cfg.Contexts, nc)
}

// storeImportedToken stores an imported apiToken under name.
//
// A ${VAR} reference is written into the config's tokens list as written,
// never expanded and never sent to the keyring. Any keyring entry already
// under that name must go, because GetToken reads the keyring before the
// config, so a stale value there would be used instead of the reference; the
// returned function deletes it, and the caller runs it once the config is
// saved, so a failed save loses nothing. A literal token takes the
// set-credentials path, cfg.SetToken. An empty one leaves any existing token
// alone.
func storeImportedToken(cfg *config.Config, name, token string) (deleteFromKeyring func() error, err error) {
	switch {
	case token == "":
		return nil, nil
	case config.HasPlaceholder(token):
		deleteFromKeyring, err := cfg.DeleteToken(name)
		if err != nil && !errors.Is(err, config.ErrTokenNotFound) {
			return nil, err
		}
		cfg.Tokens = append(cfg.Tokens, config.NamedToken{Name: name, Token: token})
		return deleteFromKeyring, nil
	default:
		return nil, cfg.SetToken(name, token)
	}
}

// importTokenDisplay says where a token would go, never what it is. For a
// reference only the ${VAR} parts are shown: the rest of a mixed value may
// be a literal secret.
func importTokenDisplay(token string) string {
	switch {
	case token == "":
		return "<none>"
	case config.HasPlaceholder(token):
		return "<reference " + strings.Join(config.PlaceholderRefs(token), ", ") + ">"
	case config.IsKeyringAvailable():
		return "<keyring>"
	default:
		return "<plaintext>"
	}
}

// printImportPlan is --dry-run's output: the contexts import would write.
func (a *App) printImportPlan(plan []importAction) error {
	rows := make([]importRow, 0, len(plan))
	for _, act := range plan {
		nc := importedContext(act.env)
		action := "create"
		if act.replace {
			action = "replace"
		}
		rows = append(rows, importRow{
			Name:       nc.Name,
			Action:     action,
			Host:       nc.Context.Host,
			EnvID:      nc.Context.EnvID,
			TokenRef:   nc.Context.TokenRef,
			Token:      importTokenDisplay(act.env.APIToken),
			HTTPProxy:  nc.Context.HTTPProxyURL,
			HTTPSProxy: nc.Context.HTTPSProxyURL,
		})
	}
	output.PrintInfo("Dry run: nothing was written.")
	return a.printer("").PrintList(rows)
}

// reportImported tells the user what import did for one context and where
// its token went.
func reportImported(cfg *config.Config, act importAction) {
	verb := "Created"
	if act.replace {
		verb = "Replaced"
	}
	output.PrintSuccess("%s context %q", verb, act.env.Alias)
	name, token := importTokenName(act.env.Alias), act.env.APIToken
	switch {
	case config.HasPlaceholder(token):
		output.PrintInfo("Token %q reads %s when used.", name, strings.Join(config.PlaceholderRefs(token), ", "))
	case token != "":
		reportStoredCredential(name)
	default:
		if _, found := configToken(cfg, name); !found {
			output.PrintInfo("The MCP config has no apiToken for %q; store one with: dtmgd config set-credentials %s", act.env.Alias, name)
		}
	}
}
