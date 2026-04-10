package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
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

// APIBaseURL returns the environment API base URL for this context.
// Format: {host}/e/{env-id}/api
func (c *Context) APIBaseURL() string {
	if c.Host == "" || c.EnvID == "" {
		return ""
	}
	host := c.Host
	// Ensure no trailing slash.
	for len(host) > 0 && host[len(host)-1] == '/' {
		host = host[:len(host)-1]
	}
	return fmt.Sprintf("%s/e/%s/api", host, c.EnvID)
}

// ClusterAPIBaseURL returns the cluster-level API base URL.
// Format: {host}/api
func (c *Context) ClusterAPIBaseURL() string {
	if c.Host == "" {
		return ""
	}
	host := c.Host
	for len(host) > 0 && host[len(host)-1] == '/' {
		host = host[:len(host)-1]
	}
	return fmt.Sprintf("%s/api", host)
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

// Load loads config from local file first, then global.
func Load() (*Config, error) {
	if local := FindLocalConfig(); local != "" {
		return LoadFrom(local)
	}
	return LoadFrom(DefaultConfigPath())
}

// LoadFrom loads config from the given path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s. Run 'dtmgd config set-context' to create one", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	expanded := []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(expanded, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
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

// Save saves the config to the default path.
func (c *Config) Save() error {
	return c.SaveTo(DefaultConfigPath())
}

// SaveTo saves the config to a specific path.
func (c *Config) SaveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
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
	if IsKeyringAvailable() {
		ts := NewTokenStore()
		token, err := ts.GetToken(tokenRef)
		if err == nil && token != "" {
			return token, nil
		}
	}
	for _, nt := range c.Tokens {
		if nt.Name == tokenRef {
			if nt.Token != "" {
				return nt.Token, nil
			}
			return "", fmt.Errorf("token %q not found in keyring (may need to re-add credentials)", tokenRef)
		}
	}
	return "", fmt.Errorf("token %q not found", tokenRef)
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
