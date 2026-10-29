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

// DefaultConcurrency caps how many environments are queried at once.
//
// The fan-out used to be unbounded: one goroutine per context, each with its
// own resty client and connection pool, all launched at the same instant. A
// realistic Managed deployment hosts 20-50 environment IDs on one cluster, so
// `--env ALL_ENVIRONMENTS` sent that many simultaneous requests to a single
// endpoint. The cluster answers 429, and because every goroutine is retrying
// on the same 1-second schedule, each retry wave is as large as the original
// burst — three times over before anything fails. wg.Wait then holds all
// output until the slowest of them finishes.
//
// Ten is low enough to stay under typical rate limits and high enough that
// small configurations see no serialisation at all.
const DefaultConcurrency = 10

// MultiRequest fans out an API call to one or more environments in parallel.
// envSpec can be:
//   - "" (empty) → current context only
//   - "ALL_ENVIRONMENTS" → all configured contexts
//   - "prod;staging" → semicolon-separated context names
//
// maxConcurrency caps simultaneous requests; values below 1 mean
// DefaultConcurrency.
//
// The apiCall function receives a Client and should perform the request.
func MultiRequest(cfg *config.Config, envSpec string, maxConcurrency int, o Options, apiCall func(c *Client) (interface{}, error)) ([]EnvResult, error) {
	contexts, err := resolveContexts(cfg, envSpec)
	if err != nil {
		return nil, err
	}

	if maxConcurrency < 1 {
		maxConcurrency = DefaultConcurrency
	}

	results := make([]EnvResult, len(contexts))
	var wg sync.WaitGroup
	wg.Add(len(contexts))

	// A buffered channel is the semaphore. Acquiring inside the goroutine
	// rather than before launching it keeps this non-blocking for the caller:
	// all goroutines start, and the channel decides how many are in flight.
	sem := make(chan struct{}, maxConcurrency)

	for i, nc := range contexts {
		go func(idx int, nc config.NamedContext) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := EnvResult{Name: nc.Name}

			// The bare error is deliberate: UnwrapSingle returns it as-is
			// when there is exactly one result, so with a single-context
			// config the user sees "✗ ${EU_HOST} is not set" with no
			// context name — and none is needed, since there is no
			// ambiguity about which context failed. With multiple
			// contexts, EnvResult.Name (not this error text) identifies
			// the context: UnwrapSingle keys the merged result map by it.
			c, err := NewForContext(cfg, nc.Name, o)
			if err != nil {
				r.Error = err
				results[idx] = r
				return
			}

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
// otherwise a map of environment name to data, with {"error": msg} for each
// environment that failed. When any environment failed it also returns a
// *PartialFailureError, so the call exits non-zero; the map is still valid
// and callers print it.
func UnwrapSingle(results []EnvResult) (interface{}, error) {
	if len(results) == 1 {
		return results[0].Data, results[0].Error
	}

	merged := make(map[string]interface{})
	var failed []EnvResult
	for _, r := range results {
		if r.Error != nil {
			merged[r.Name] = map[string]string{"error": r.Error.Error()}
			failed = append(failed, r)
		} else {
			merged[r.Name] = r.Data
		}
	}
	if len(failed) > 0 {
		return merged, &PartialFailureError{Failed: failed, Total: len(results)}
	}
	return merged, nil
}

// IsSingleEnv returns true if the results are from a single environment.
func IsSingleEnv(results []EnvResult) bool {
	return len(results) == 1
}
