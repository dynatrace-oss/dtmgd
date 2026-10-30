package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// clusterAnnotation marks a cluster command: one that calls the Cluster
// Management API through a cluster context (see clusterCommand).
const clusterAnnotation = "dtmgd.io/cluster"

// clusterCommand marks cmd as a cluster command, which needs a cluster
// context whose token has the ServiceProviderAPI scope, and returns it.
func clusterCommand(cmd *cobra.Command) *cobra.Command {
	withScopes(cmd, scopeServiceProviderAPI)
	cmd.Annotations[clusterAnnotation] = "true"
	return cmd
}

// isClusterCommand reports whether cmd is a cluster command.
func isClusterCommand(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[clusterAnnotation] == "true"
}

// Hints for the Cluster API's errors, which the generic ones (client.Diagnose)
// get wrong: its 401 hint does not say that an environment token cannot work,
// and its 404 hint ("check the ID") misleads when the whole API is missing.
const (
	hintClusterAuth = "Authentication failed. A cluster context needs a Cluster API token with the ServiceProviderAPI scope, created in the Cluster Management Console; an environment API token does not work here.\n  Store it with: dtmgd config set-credentials <token-ref>"

	hintNoClusterAPI = "No Cluster API answered at context %q's host. A cluster context's host must be the cluster's own address, the one the Cluster Management Console uses: not a node address on port 9999, and without /e/<env-id>."

	hintDPSConsumption = "Consumption information is not available on a cluster licensed with the Dynatrace Platform Subscription (DPS). Run without --consumption."
)

// runClusterFetch is runFetch for a cluster command: it fetches from the one
// cluster context the flags select and renders the result for the chosen
// format. There is no --env fan-out; a multi-environment --env is a usage
// error, and so is an environment context.
func (a *App) runClusterFetch(s fetchSpec) error {
	if err := limitFlag(s.Limit); err != nil {
		return err
	}
	if err := a.checkDelimited(s); err != nil {
		return err
	}
	if a.isMultiEnv() {
		return &UsageError{Err: fmt.Errorf("--env %q selects environments; \"dtmgd %s %s\" runs against one cluster context. Use --context <name>", a.Opts.Env, s.Verb, s.Resource)}
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if err := a.checkEnvVarContext(cfg); err != nil {
		return err
	}
	if err := a.checkClusterContext(cfg, s); err != nil {
		var usage *UsageError
		if errors.As(err, &usage) && s.WrongContext != "" {
			usage.Suggestions = append(usage.Suggestions, s.WrongContext)
		}
		return err
	}
	c, err := client.NewForContext(cfg, cfg.CurrentContext, a.clientOptions())
	if err != nil {
		return err
	}
	limit := s.Limit
	if limit == 0 && s.List && a.agentMode() {
		limit = agentDefaultLimit
	}
	f, err := s.Fetch(c, limit)
	if err != nil {
		return clusterHint(err, cfg.CurrentContext, s)
	}
	return a.renderOne(s, f)
}

// clusterHint gives a Cluster API error the hint that fits it: what kind of
// token a 401 needs, and, for a 404, s.NotFound or that the context's host
// has no Cluster API. Other errors pass through unchanged.
func clusterHint(err error, ctxName string, s fetchSpec) error {
	var apiErr *client.ErrAPI
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.StatusCode {
	case 401:
		return withHint(err, hintClusterAuth)
	case 404:
		noAPI := fmt.Sprintf(hintNoClusterAPI, ctxName)
		if s.NotFound != "" {
			// A command's own reading of a 404 cannot rule out a wrong
			// host, which makes every cluster call 404.
			return withHint(err, s.NotFound+"\n  If every cluster command answers 404: "+noAPI)
		}
		return withHint(err, noAPI)
	}
	return err
}

// hintOnStatus wraps fetch so that an API error with the given status gains
// hint (see withHint).
func hintOnStatus(fetch func(*client.Client, int) (fetched, error), status int, hint string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := fetch(c, limit)
		var apiErr *client.ErrAPI
		if errors.As(err, &apiErr) && apiErr.StatusCode == status {
			return f, withHint(err, hint)
		}
		return f, err
	}
}

// clusterPagedFetch is pagedFetch for a Cluster API list endpoint; path is
// relative to {host}/api.
func clusterPagedFetch(path string, params map[string]string, listKey string, pageCap, maxPages int) func(*client.Client, int) (fetched, error) {
	return pagedWith((*client.Client).GetClusterPaged, path, params, listKey, pageCap, maxPages)
}

// clusterObjectFetch is describeFetch for a Cluster API endpoint that returns
// one object; path is relative to {host}/api.
func clusterObjectFetch(path string, params map[string]string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, _ int) (fetched, error) {
		var m map[string]interface{}
		if err := c.GetCluster(path, params, &m); err != nil {
			return fetched{}, err
		}
		return fetched{Body: m, Total: -1}, nil
	}
}
