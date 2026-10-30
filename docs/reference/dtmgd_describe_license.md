## dtmgd describe license

Show a Dynatrace Managed cluster's license and billed usage (cluster context)

### Synopsis

Show the cluster license and its billed usage of host units, DEM units and
Davis data units, from the Cluster Management API. Needs a cluster context:
one without env-id, whose token has the ServiceProviderAPI scope.

The license key is masked in every output format: only its last four
characters are shown.

The API needs Dynatrace Managed 1.326+ and works only with Dynatrace classic
licensing; a cluster on the Dynatrace Platform Subscription (DPS) has none to
show. Billed usage covers the current contract and lags about two hours.
"dtmgd get license-usage" shows the usage per environment.

Examples:
```
  dtmgd describe license --context prod-cluster
  dtmgd describe license -o json
```

```
dtmgd describe license [flags]
```

### Options

```
  -h, --help   help for license
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

