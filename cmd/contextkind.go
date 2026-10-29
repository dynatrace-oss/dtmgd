package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// A context is an environment context or, without an env-id, a cluster
// context (config.Context.IsCluster). Environment commands call one
// environment's API and cluster commands the Cluster Management API, so each
// refuses the other kind before sending anything, and says which contexts to
// use instead. Contexts are never paired by host: an environment context's
// host is usually a node on port 9999, where there is no Cluster API.

// contextNamesOfKind returns the names of cfg's cluster contexts (cluster) or
// environment contexts (!cluster), in config order.
func contextNamesOfKind(cfg *config.Config, cluster bool) []string {
	var names []string
	for _, nc := range cfg.Contexts {
		if nc.Context.IsCluster() == cluster {
			names = append(names, nc.Name)
		}
	}
	return names
}

// checkEnvironmentContexts fails when the contexts an environment command
// runs against include a cluster context: the current context, or a name in
// an --env list. ALL_ENVIRONMENTS selects environment contexts only (see
// client.MultiRequest), and a name that does not exist is left to the
// not-found errors further on.
func (a *App) checkEnvironmentContexts(cfg *config.Config, s fetchSpec) error {
	names := []string{cfg.CurrentContext}
	if a.isMultiEnv() {
		if a.Opts.Env == "ALL_ENVIRONMENTS" {
			return nil
		}
		names = strings.Split(a.Opts.Env, ";")
	}
	for _, name := range names {
		nc, err := cfg.GetContext(strings.TrimSpace(name))
		if err == nil && nc.Context.IsCluster() {
			return wrongContextError(nc.Name, false, s, contextNamesOfKind(cfg, false))
		}
	}
	return nil
}

// checkClusterContext fails when a cluster command's context, the current
// one, is an environment context. A context that does not exist is left to
// client.NewForContext.
func (a *App) checkClusterContext(cfg *config.Config, s fetchSpec) error {
	nc, err := cfg.GetContext(cfg.CurrentContext)
	if err != nil || nc.Context.IsCluster() {
		return nil
	}
	return wrongContextError(nc.Name, true, s, contextNamesOfKind(cfg, true))
}

// wrongContextError is the error for running s against context name, which
// is of the wrong kind: wantCluster says what s needs. others are the
// configured contexts of the kind s needs, which the suggestion names.
func wrongContextError(name string, wantCluster bool, s fetchSpec, others []string) *UsageError {
	command := fmt.Sprintf("dtmgd %s %s", s.Verb, s.Resource)
	if wantCluster {
		err := fmt.Errorf("context %q is an environment context; %q needs a cluster context (one without env-id)", name, command)
		if len(others) == 0 {
			return &UsageError{Err: err, Suggestions: []string{"No cluster contexts are configured. Create one with: dtmgd ctx set <name> --host <cluster-url> --token-ref <ref> (no --env-id; its token needs the ServiceProviderAPI scope)"}}
		}
		return &UsageError{Err: err, Suggestions: []string{fmt.Sprintf("Cluster contexts: %s. Pass one with --context <name>", strings.Join(others, ", "))}}
	}
	err := fmt.Errorf("context %q is a cluster context (no env-id); %q needs an environment context", name, command)
	if len(others) == 0 {
		return &UsageError{Err: err, Suggestions: []string{"No environment contexts are configured. Create one with: dtmgd ctx set <name> --host <url> --env-id <id> --token-ref <ref>"}}
	}
	return &UsageError{Err: err, Suggestions: []string{fmt.Sprintf("Environment contexts: %s. Pass one with --context <name>, or make one current with: dtmgd ctx <name>", strings.Join(others, ", "))}}
}
