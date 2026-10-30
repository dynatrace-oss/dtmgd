## dtmgd describe backup

Show a Dynatrace Managed cluster's backup configuration (cluster context)

### Synopsis

Show the cluster's backup configuration from the Cluster Management API:
whether backups are enabled or paused, where they go, when Cassandra backups
start, and what they include. Needs a cluster context: one without env-id,
whose token has the ServiceProviderAPI scope.

The Cluster API reports the backup configuration only. It has no last-backup time and no backup health; check those in the Cluster Management Console.

Examples:
```
  dtmgd describe backup --context prod-cluster
  dtmgd describe backup -o json
```

```
dtmgd describe backup [flags]
```

### Options

```
  -h, --help   help for backup
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

* [dtmgd describe](dtmgd_describe.md)	 - Show detailed information about a specific resource

