## dtmgd config set-context

Create or update a context entry

### Synopsis

Create or update a context that points to a Dynatrace Managed environment,
or, without --env-id, to a cluster's Cluster Management API.

A context stores three things:
```
  --host       Base URL of the Dynatrace Managed cluster
               (e.g. https://managed.company.com)
  --env-id     Environment identifier shown in the Managed UI
               (e.g. "abc12345")
  --token-ref  Name of the API token credential (see set-credentials)
```

Required API token scopes:
```
  DataExport, entities.read, events.read, logs.read, metrics.read, problems.read, securityProblems.read, slo.read
```

Without --env-id the context is a cluster context, for the Cluster
Management API: get environments, describe environment, get nodes, describe
cluster, describe license, get license-usage and describe backup.
Its --host is the cluster's own address, as used by the Cluster Management
Console, and its token needs the ServiceProviderAPI scope.

Examples:
```
  dtmgd config set-context prod \
    --host https://managed.company.com \
    --env-id abc12345 \
    --token-ref prod-token

  dtmgd config set-credentials prod-token   # prompts for the token
```

```
dtmgd config set-context <context-name> [flags]
```

### Options

```
      --description string   human-readable description
      --env-id string        environment ID (omit for a cluster context)
  -h, --help                 help for set-context
      --host string          Dynatrace Managed cluster URL (e.g. https://managed.company.com)
      --token-ref string     credential name (see set-credentials)
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config)
  -c, --context string    context to use (overrides current-context)
  -e, --env string        environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int     maximum number of pages to fetch (0 = all pages)
      --no-agent          disable auto-detected agent mode
  -o, --output string     output format: table|json|yaml|wide (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd config](dtmgd_config.md)	 - Manage dtmgd configuration

