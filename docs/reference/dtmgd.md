## dtmgd

Dynatrace Managed CLI

### Synopsis

dtmgd is a kubectl-inspired CLI for Dynatrace Managed (self-hosted) clusters.

It provides read access to problems, entities, events, logs, metrics, SLOs,
and security vulnerabilities via the Dynatrace Managed classic API, and to a
cluster's environments, nodes, license and backup configuration via the
Cluster Management API.

API endpoints used:
```
  Environment API:  {host}/e/{env-id}/api/v2/  (environment contexts)
  Cluster API:      {host}/api/cluster/v2/ and {host}/api/v1.0/onpremise/
                    (cluster contexts: no env-id)
```

Quick start:
```
  dtmgd config set-context prod --host https://managed.company.com --env-id abc123 --token-ref prod-token
  dtmgd config set-credentials prod-token   # prompts for the token
  dtmgd get problems
```

Multi-environment:
```
  dtmgd get problems --env ALL_ENVIRONMENTS
  dtmgd get problems --env "prod;staging"
```

Cluster (a context without --env-id, with a ServiceProviderAPI token):
```
  dtmgd ctx set prod-cluster --host https://cluster.company.com --token-ref prod-cluster-token
  dtmgd get environments --context prod-cluster
```

### Options

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table, csv and tsv output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -c, --context string    context to use (overrides current-context)
  -e, --env string        environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
  -h, --help              help for dtmgd
      --max-pages int     maximum number of pages to fetch (0 = all pages)
      --no-agent          disable auto-detected agent mode
  -o, --output string     output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd commands](dtmgd_commands.md)	 - Print a machine-readable catalog of dtmgd commands
* [dtmgd completion](dtmgd_completion.md)	 - Generate shell completion script
* [dtmgd config](dtmgd_config.md)	 - Manage dtmgd configuration
* [dtmgd ctx](dtmgd_ctx.md)	 - Manage contexts (shortcut for config context commands)
* [dtmgd describe](dtmgd_describe.md)	 - Show detailed information about a specific resource
* [dtmgd doctor](dtmgd_doctor.md)	 - Check connectivity, token and cluster version of each context
* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources
* [dtmgd query](dtmgd_query.md)	 - Query time-series or log data
* [dtmgd skills](dtmgd_skills.md)	 - Manage AI coding assistant skill files
* [dtmgd version](dtmgd_version.md)	 - Show dtmgd version

