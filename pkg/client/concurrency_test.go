package client

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// configWithContexts builds a config of n contexts sharing one token, so
// MultiRequest can construct a Client for each without reaching the network.
func configWithContexts(n int) *config.Config {
	cfg := &config.Config{
		Tokens: []config.NamedToken{{Name: "tok", Token: "dt0c01.TEST"}},
	}
	for i := 0; i < n; i++ {
		cfg.Contexts = append(cfg.Contexts, config.NamedContext{
			Name: fmt.Sprintf("env%02d", i),
			Context: config.Context{
				Host:     "https://managed.example.com",
				EnvID:    fmt.Sprintf("e%02d", i),
				TokenRef: "tok",
			},
		})
	}
	return cfg
}

// peakTracker records the highest number of simultaneous calls observed.
type peakTracker struct {
	mu      sync.Mutex
	current int
	peak    int
	total   atomic.Int64
}

func (p *peakTracker) enter() {
	p.mu.Lock()
	p.current++
	if p.current > p.peak {
		p.peak = p.current
	}
	p.mu.Unlock()
	p.total.Add(1)
}

func (p *peakTracker) exit() {
	p.mu.Lock()
	p.current--
	p.mu.Unlock()
}

// TestMultiRequestCapsConcurrency is the finding: 30 contexts used to produce
// 30 simultaneous requests to the same cluster, which answers 429 and then
// receives three equally large retry waves.
func TestMultiRequestCapsConcurrency(t *testing.T) {
	const contexts, limit = 30, 4

	var p peakTracker
	results, err := MultiRequest(configWithContexts(contexts), "ALL_ENVIRONMENTS", limit,
		func(c *Client) (interface{}, error) {
			p.enter()
			// Long enough that unbounded goroutines would overlap; the cap is
			// what keeps the observed peak at or below the limit.
			time.Sleep(20 * time.Millisecond)
			p.exit()
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("MultiRequest() error = %v", err)
	}

	if got := int(p.total.Load()); got != contexts {
		t.Errorf("apiCall ran %d times, want once per context (%d)", got, contexts)
	}
	if p.peak > limit {
		t.Errorf("peak concurrency = %d, want at most %d", p.peak, limit)
	}
	if p.peak < 2 {
		t.Errorf("peak concurrency = %d — the fan-out serialised instead of running in parallel", p.peak)
	}
	if len(results) != contexts {
		t.Errorf("len(results) = %d, want %d", len(results), contexts)
	}
	for _, r := range results {
		if r.Error != nil {
			t.Errorf("context %s errored: %v", r.Name, r.Error)
		}
	}
}

// TestMultiRequestConcurrencyFallback: a caller that passes nothing sensible
// still gets a bound rather than the old unbounded behaviour.
func TestMultiRequestConcurrencyFallback(t *testing.T) {
	const contexts = DefaultConcurrency + 12

	var p peakTracker
	_, err := MultiRequest(configWithContexts(contexts), "ALL_ENVIRONMENTS", 0,
		func(c *Client) (interface{}, error) {
			p.enter()
			time.Sleep(20 * time.Millisecond)
			p.exit()
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("MultiRequest() error = %v", err)
	}

	if p.peak > DefaultConcurrency {
		t.Errorf("peak concurrency = %d with limit 0, want at most DefaultConcurrency (%d)", p.peak, DefaultConcurrency)
	}
}

// TestMultiRequestPreservesOrder guards the property the semaphore could
// plausibly have broken: results are indexed by context position, not by
// completion order.
func TestMultiRequestPreservesOrder(t *testing.T) {
	cfg := configWithContexts(8)

	results, err := MultiRequest(cfg, "ALL_ENVIRONMENTS", 3,
		func(c *Client) (interface{}, error) { return c.APIBaseURL(), nil })
	if err != nil {
		t.Fatalf("MultiRequest() error = %v", err)
	}

	for i, r := range results {
		if r.Name != cfg.Contexts[i].Name {
			t.Errorf("results[%d].Name = %q, want %q", i, r.Name, cfg.Contexts[i].Name)
		}
	}
}
