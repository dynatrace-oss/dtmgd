## dtmgd get activegate-updates

Show how the cluster's ActiveGates update

### Synopsis

Show the cluster ActiveGates' automatic updates: the global setting, then
each cluster ActiveGate's own setting (ENABLED, DISABLED or INHERITED from the
global one), the setting in effect, and its update status. With an ID, that
ActiveGate's configuration.

TARGET-VERSION and WINDOWS appear only where the cluster returns them.

json and yaml print {"global": …, "activeGates": […]}, each as the Cluster API
returns it. Needs a cluster context whose token has ServiceProviderAPI.

Examples:
```
  dtmgd get activegate-updates --context prod-cluster
  dtmgd get activegate-updates <activegate-id> --context prod-cluster
```

```
dtmgd get activegate-updates [activegate-id] [flags]
```

### Options

```
  -h, --help[=true|false]   help for activegate-updates
```

### Options inherited from parent commands

```
  -A, --agent[=true|false]      force agent envelope output mode
      --columns string          comma-separated list of columns to show in table, csv and tsv output
      --concurrency int         maximum environments queried at once with --env (default 10)
      --config string           config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -c, --context string          context to use (overrides current-context)
  -e, --env string              environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int           maximum number of pages to fetch (0 = all pages)
      --no-agent[=true|false]   disable auto-detected agent mode
  -o, --output string           output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain[=true|false]      plain output: no colors, symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count           verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

