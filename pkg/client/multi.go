package client

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// EnvResult holds the result of a single environment request.
type EnvResult struct {
	Name  string
	Data  interface{}
	Error error
}

// MultiRequest fans out an API call to one or more environments in parallel.
// envSpec can be:
//   - "" (empty) → current context only
//   - "ALL_ENVIRONMENTS" → all configured contexts
//   - "prod;staging" → semicolon-separated context names
//
// The apiCall function receives a Client and should perform the request.
func MultiRequest(cfg *config.Config, envSpec string, apiCall func(c *Client) (interface{}, error)) ([]EnvResult, error) {
	contexts, err := resolveContexts(cfg, envSpec)
	if err != nil {
		return nil, err
	}

	results := make([]EnvResult, len(contexts))
	var wg sync.WaitGroup
	wg.Add(len(contexts))

	for i, nc := range contexts {
		go func(idx int, nc config.NamedContext) {
			defer wg.Done()
			r := EnvResult{Name: nc.Name}

			token, tokenErr := cfg.GetToken(nc.Context.TokenRef)
			if tokenErr != nil {
				r.Error = fmt.Errorf("token error: %w", tokenErr)
				results[idx] = r
				return
			}

			c, clientErr := New(nc.Context.Host, nc.Context.EnvID, token)
			if clientErr != nil {
				r.Error = fmt.Errorf("client error: %w", clientErr)
				results[idx] = r
				return
			}
			c.SetProxy(nc.Context.HTTPProxyURL, nc.Context.HTTPSProxyURL)

			data, callErr := apiCall(c)
			r.Data = data
			r.Error = callErr
			results[idx] = r
		}(i, nc)
	}

	wg.Wait()
	return results, nil
}

func resolveContexts(cfg *config.Config, envSpec string) ([]config.NamedContext, error) {
	if envSpec == "" {
		// Current context only
		if cfg.CurrentContext == "" {
			return nil, fmt.Errorf("no current context set. Run 'dtmgd config set-context' to configure one")
		}
		for _, nc := range cfg.Contexts {
			if nc.Name == cfg.CurrentContext {
				return []config.NamedContext{nc}, nil
			}
		}
		return nil, fmt.Errorf("current context %q not found in config", cfg.CurrentContext)
	}

	if envSpec == "ALL_ENVIRONMENTS" {
		if len(cfg.Contexts) == 0 {
			return nil, fmt.Errorf("no contexts configured")
		}
		return cfg.Contexts, nil
	}

	// Semicolon-separated names
	names := strings.Split(envSpec, ";")
	var result []config.NamedContext
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		found := false
		for _, nc := range cfg.Contexts {
			if nc.Name == name {
				result = append(result, nc)
				found = true
				break
			}
		}
		if !found {
			available := make([]string, len(cfg.Contexts))
			for i, nc := range cfg.Contexts {
				available[i] = nc.Name
			}
			return nil, fmt.Errorf("context %q not found. Available: %s", name, strings.Join(available, ", "))
		}
	}
	return result, nil
}

// UnwrapSingle returns the data directly if there's only one result,
// or a map of name→data if there are multiple.
func UnwrapSingle(results []EnvResult) (interface{}, error) {
	if len(results) == 1 {
		return results[0].Data, results[0].Error
	}

	// Multiple environments — collect into a map
	merged := make(map[string]interface{})
	var firstErr error
	for _, r := range results {
		if r.Error != nil {
			merged[r.Name] = map[string]string{"error": r.Error.Error()}
			if firstErr == nil {
				firstErr = r.Error
			}
		} else {
			merged[r.Name] = r.Data
		}
	}
	return merged, nil
}

// IsSingleEnv returns true if the results are from a single environment.
func IsSingleEnv(results []EnvResult) bool {
	return len(results) == 1
}
