## dtmgd get license-usage

List each environment's total billed license usage (cluster context)

### Synopsis

List each environment's total billed usage of host units, DEM units and Davis
data units, including usage from past contracts, from the Cluster Management
API. Needs a cluster context: one without env-id, whose token has the
ServiceProviderAPI scope. The table names each environment from the
cluster's environment list.

The API needs Dynatrace Managed 1.326+ and works only with Dynatrace classic
licensing; a cluster on the Dynatrace Platform Subscription (DPS) has no
classic license usage. "dtmgd describe license" shows the cluster's totals.

Examples:
```
  dtmgd get license-usage --context prod-cluster
  dtmgd get license-usage -o json
```

```
dtmgd get license-usage [flags]
```

### Options

```
  -h, --help        help for license-usage
      --limit int   maximum number of environments to return
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
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

