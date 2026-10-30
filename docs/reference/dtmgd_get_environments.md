## dtmgd get environments

List the environments of a Dynatrace Managed cluster (cluster context)

### Synopsis

List the environments of a cluster, from the Cluster Management API. Needs a
cluster context: one without env-id, whose token has the ServiceProviderAPI
scope.

"get environments" used to check each configured context's connectivity;
that check is now "dtmgd doctor".

Filters combine; an environment must match all of them:
```
  --name     part of the environment name, case-insensitive
  --state    ENABLED or DISABLED
  --filter   a raw API filter, e.g. 'trial(true)', 'tag(prod)', or
             'tag(a,b)' for either tag
```

--consumption adds host-unit and yearly DDU usage, as usage / limit. It is
not available on a cluster licensed with the Dynatrace Platform Subscription
(DPS).

Examples:
```
  dtmgd get environments --context prod-cluster
  dtmgd get environments --state enabled --name shop
  dtmgd get environments --consumption -o json
```

```
dtmgd get environments [flags]
```

### Options

```
      --consumption     include host-unit and DDU consumption (not on DPS-licensed clusters)
      --filter string   raw API filter, e.g. 'tag(prod)' or 'trial(false)'
  -h, --help            help for environments
      --limit int       maximum number of environments to return
      --name string     only environments whose name contains this (case-insensitive)
      --sort string     sort order: name, -name, creationDate or -creationDate
      --state string    only environments in this state: ENABLED or DISABLED
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -c, --context string    context to use (overrides current-context)
  -e, --env string        environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int     maximum number of pages to fetch (0 = all pages)
      --no-agent          disable auto-detected agent mode
  -o, --output string     output format: table|json|yaml|wide (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

