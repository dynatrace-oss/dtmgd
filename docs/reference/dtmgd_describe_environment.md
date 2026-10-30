## dtmgd describe environment

Show one environment of a Dynatrace Managed cluster (cluster context)

### Synopsis

Show an environment's properties, quotas and storage limits from the Cluster
Management API. Needs a cluster context: one without env-id, whose token has
the ServiceProviderAPI scope. Find IDs with "dtmgd get environments".

--consumption adds current usage to the quotas. It is not available on a
cluster licensed with the Dynatrace Platform Subscription (DPS).

Examples:
```
  dtmgd describe environment 0b5c4e1a-2f3d-4c6b-9a7e-1d2c3b4a5f60 --context prod-cluster
  dtmgd describe environment <id> --consumption -o json
```

```
dtmgd describe environment <environment-id> [flags]
```

### Options

```
      --consumption   include current consumption (not on DPS-licensed clusters)
  -h, --help          help for environment
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table, csv and tsv output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -c, --context string    context to use (overrides current-context)
  -e, --env string        environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int     maximum number of pages to fetch (0 = all pages)
      --no-agent          disable auto-detected agent mode
  -o, --output string     output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain             plain output: no colors, symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd describe](dtmgd_describe.md)	 - Show detailed information about a specific resource

