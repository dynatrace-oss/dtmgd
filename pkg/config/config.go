package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

// Config represents the dtmgd configuration.
type Config struct {
	APIVersion     string            `yaml:"apiVersion"`
	Kind           string            `yaml:"kind"`
	CurrentContext string            `yaml:"current-context"`
	Contexts       []NamedContext    `yaml:"contexts"`
	Tokens         []NamedToken      `yaml:"tokens"`
	Preferences    Preferences       `yaml:"preferences"`
	Aliases        map[string]string `yaml:"aliases,omitempty"`

	// expandedFrom is not set by any production code path: LoadFrom no longer
	// expands ${VAR} references at load, so there is nothing left to guard
	// against there. The field and the SaveTo check that reads it are kept
	// as a backstop for a library caller that constructs a Config directly
	// with already-expanded values — setting this field is how such a
	// caller opts into SaveTo's refusal to overwrite the source file.
	expandedFrom string
}

// NamedContext holds a named context entry.
type NamedContext struct {
	Name    string  `yaml:"name"`
	Context Context `yaml:"context"`
}

// Context holds the connection information for one Dynatrace Managed environment.
type Context struct {
	// Host is the base URL of the Dynatrace Managed cluster, e.g. https://managed.company.com
	Host string `yaml:"host"`
	// EnvID is the environment identifier, e.g. "abc12345"
	EnvID string `yaml:"env-id"`
	// TokenRef is the name of the token entry in the Tokens list (or OS keyring).
	TokenRef      string `yaml:"token-ref"`
	Description   string `yaml:"description,omitempty"`
	HTTPProxyURL  string `yaml:"http-proxy,omitempty"`
	HTTPSProxyURL string `yaml:"https-proxy,omitempty"`
}

// NamedToken holds a named API token.
type NamedToken struct {
	Name  string `yaml:"name"`
	Token string `yaml:"token"`
}

// Preferences holds user preferences.
type Preferences struct {
	Output string `yaml:"output,omitempty"`
}

// DefaultConfigPath returns the default config file path (~/.config/dtmgd/config).
func DefaultConfigPath() string {
	return filepath.Join(xdg.ConfigHome, "dtmgd", "config")
}

// ConfigDir returns the config directory path.
func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, "dtmgd")
}

// LocalConfigName is the name of the per-project config file.
const LocalConfigName = ".dtmgd.yaml"

// FindLocalConfig searches for a .dtmgd.yaml file starting from cwd, walking up to root.
func FindLocalConfig() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		p := filepath.Join(dir, LocalConfigName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// NotFoundError reports that no config file exists at Path.
//
// It unwraps to fs.ErrNotExist so callers can tell "no config yet" apart from
// every other load failure. Commands that create a config on first use must
// start from an empty one only in this case: falling back on any error turned
// a single YAML typo into a silently wiped config.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("config file not found at %s. Run 'dtmgd config set-context' to create one", e.Path)
}

// Unwrap makes errors.Is(err, fs.ErrNotExist) true.
func (e *NotFoundError) Unwrap() error { return fs.ErrNotExist }

// LoadFrom loads config from the given path.
func LoadFrom(path string) (*Config, error) {
	// #nosec G304 -- reading a caller-supplied config path is this function's
	// purpose. The path comes from --config, a discovered .dtmgd.yaml, or the
	// XDG default, and dtmgd runs with the invoking user's own privileges, so
	// there is no privilege boundary to cross here.
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{Path: path}
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Deliberately no os.ExpandEnv here. The in-memory Config is a faithful
	// image of the file, so a save can never write an expanded secret back.
	// ${VAR} references are expanded at the point of use — see Context.Resolve
	// and Config.GetToken.
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, parseError(path, err)
	}
	if err := checkContextNames(path, cfg.Contexts); err != nil {
		return nil, err
	}
	// Apply defaults for fields that may be absent in minimal configs.
	if cfg.APIVersion == "" {
		cfg.APIVersion = "dtmgd.io/v1"
	}
	if cfg.Kind == "" {
		cfg.Kind = "Config"
	}
	return &cfg, nil
}

// checkContextNames rejects context names that no dtmgd command could select
// or repair: a name defined more than once (only the first would ever be
// used), and a name containing ';', which --env uses to separate names. Names
// match exactly, so "prod" and "Prod" are distinct.
func checkContextNames(path string, contexts []NamedContext) error {
	count := make(map[string]int, len(contexts))
	for _, nc := range contexts {
		if strings.Contains(nc.Name, ";") {
			return fmt.Errorf("config file %s: context name %q contains ';', which --env uses to separate names; rename it by editing the file", path, nc.Name)
		}
		count[nc.Name]++
	}
	// In config order, so the first duplicated name is the one reported.
	for _, nc := range contexts {
		if n := count[nc.Name]; n > 1 {
			return fmt.Errorf("config file %s: context %q is defined %d times; rename or remove the extras by editing the file", path, nc.Name, n)
		}
	}
	return nil
}

// parseError converts a yaml decode failure into a message safe to show.
//
// yaml.v3 quotes the offending value in type errors, truncated to ten
// characters. Because --config accepts any readable path, forwarding that text
// turns a config flag into a short read primitive: `dtmgd --config /etc/hostname`
// printed the machine's hostname, and in agent mode the same text lands in the
// JSON error.message that agent frameworks record. Ten characters with no way
// to page through more is too constrained to be a security finding, but the
// error has no reason to echo file content at all.
//
// Syntax errors are forwarded unchanged: they report a line number and a
// structural problem ("did not find expected key") without quoting values, and
// they are the errors a user most often needs to act on.
func parseError(path string, err error) error {
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		return fmt.Errorf("failed to parse config file at %s: it is not valid dtmgd configuration", path)
	}
	return fmt.Errorf("failed to parse config file at %s: %w", path, err)
}

// Save saves the config to the default path.
func (c *Config) Save() error {
	return c.SaveTo(DefaultConfigPath())
}

// SaveTo saves the config to a specific path.
func (c *Config) SaveTo(path string) error {
	if c.expandedFrom != "" && sameFile(c.expandedFrom, path) {
		return fmt.Errorf("refusing to overwrite %s: it references environment variables, and saving would replace them with their current values. Edit the file directly instead", path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return writeFileAtomic(path, data)
}

// writeFileAtomic replaces path with data in one rename, so an interrupted
// save never leaves a truncated config, and leaves it mode 0600 even if the
// file it replaces was looser: a config can hold a plaintext token. A symlink
// is followed, so a config kept elsewhere (a dotfiles repo) stays linked.
func writeFileAtomic(path string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if tmp != "" {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	tmp = ""
	return nil
}

// CurrentContextObj returns the current context.
func (c *Config) CurrentContextObj() (*Context, error) {
	if c.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set. Run 'dtmgd config set-context' to configure one")
	}
	for _, nc := range c.Contexts {
		if nc.Name == c.CurrentContext {
			ctx := nc.Context
			return &ctx, nil
		}
	}
	return nil, fmt.Errorf("current context %q not found in config", c.CurrentContext)
}

// GetContext returns a named context.
func (c *Config) GetContext(name string) (*NamedContext, error) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], nil
		}
	}
	return nil, fmt.Errorf("context %q not found", name)
}

// GetToken retrieves a token by reference name.
// Tries OS keyring first, then config file plaintext.
func (c *Config) GetToken(tokenRef string) (string, error) {
	// kerr is why the keyring could not supply the token, if it failed for a
	// reason other than not having it. A token whose config entry is empty
	// lives in the keyring, so that reason is the real error, not "not found".
	fromKeyring, kerr := keyringToken(tokenRef)
	if fromKeyring != "" {
		return fromKeyring, nil
	}
	for _, nt := range c.Tokens {
		if nt.Name == tokenRef {
			if nt.Token == "" {
				if kerr != nil {
					return "", fmt.Errorf("keyring: %w (token %q is stored there)", kerr, tokenRef)
				}
				return "", fmt.Errorf("token %q not found in keyring (may need to re-add credentials)", tokenRef)
			}
			// The stored value may be a ${VAR} reference. Expand it here, at
			// the point of use, rather than at load.
			token, unset := expand(nt.Token)
			if len(unset) > 0 {
				return "", &UnresolvedVarsError{Vars: unset}
			}
			return token, nil
		}
	}
	return "", fmt.Errorf("token %q not found", tokenRef)
}

// keyringToken looks tokenRef up in the OS keyring, if the keyring is usable.
// It returns the token when the keyring holds a non-empty one; otherwise it
// returns why the keyring could not supply it, or nil if it simply does not
// have it.
func keyringToken(tokenRef string) (string, error) {
	if kerr := keyringStatus(); kerr != nil {
		return "", kerr
	}
	token, err := NewTokenStore().GetToken(tokenRef)
	if err == nil && token != "" {
		return token, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "", err
	}
	return "", nil
}

// SetContext creates or updates a context entry.
func (c *Config) SetContext(name, host, envID, tokenRef, description string) {
	for i, nc := range c.Contexts {
		if nc.Name == name {
			if host != "" {
				c.Contexts[i].Context.Host = host
			}
			if envID != "" {
				c.Contexts[i].Context.EnvID = envID
			}
			if tokenRef != "" {
				c.Contexts[i].Context.TokenRef = tokenRef
			}
			if description != "" {
				c.Contexts[i].Context.Description = description
			}
			return
		}
	}
	c.Contexts = append(c.Contexts, NamedContext{
		Name: name,
		Context: Context{
			Host:        host,
			EnvID:       envID,
			TokenRef:    tokenRef,
			Description: description,
		},
	})
}

// DeleteContext removes a context by name.
func (c *Config) DeleteContext(name string) error {
	for i, nc := range c.Contexts {
		if nc.Name == name {
			c.Contexts = append(c.Contexts[:i], c.Contexts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("context %q not found", name)
}

// SetToken creates or updates a token.
// Uses OS keyring when available; stores empty string in config as reference.
func (c *Config) SetToken(name, token string) error {
	if IsKeyringAvailable() {
		ts := NewTokenStore()
		if err := ts.SetToken(name, token); err != nil {
			return fmt.Errorf("failed to store token in keyring: %w", err)
		}
		token = ""
	}
	for i, nt := range c.Tokens {
		if nt.Name == name {
			c.Tokens[i].Token = token
			return nil
		}
	}
	c.Tokens = append(c.Tokens, NamedToken{Name: name, Token: token})
	return nil
}

// ErrTokenNotFound is DeleteToken's error when neither the OS keyring nor
// the config holds the token, so callers can treat an already-deleted token
// as done.
var ErrTokenNotFound = errors.New("token not found")

// DeleteToken removes the named token from the OS keyring and from the
// config's tokens list. It is an error wrapping ErrTokenNotFound if neither
// holds it.
func (c *Config) DeleteToken(name string) error {
	found := false
	if IsKeyringAvailable() {
		switch err := NewTokenStore().DeleteToken(name); {
		case err == nil:
			found = true
		case !errors.Is(err, keyring.ErrNotFound):
			return fmt.Errorf("keyring: %w", err)
		}
	}
	for i, nt := range c.Tokens {
		if nt.Name == name {
			c.Tokens = append(c.Tokens[:i], c.Tokens[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("token %q not found: %w", name, ErrTokenNotFound)
	}
	return nil
}

// NewConfig returns a minimal default Config.
func NewConfig() *Config {
	return &Config{
		APIVersion:  "dtmgd.io/v1",
		Kind:        "Config",
		Contexts:    []NamedContext{},
		Tokens:      []NamedToken{},
		Preferences: Preferences{Output: "table"},
	}
}

// sameFile reports whether two paths point at the same config file.
func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}
