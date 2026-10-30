## dtmgd query

Query time-series or log data

### Synopsis

Query time-range data from a Dynatrace Managed environment.

Supported subcommands:
```
  metrics      Query metric time-series data
  logs         Search log records
  log-counts   Count log records per process group and level (INFO/WARN/ERROR)
```

Use 'dtmgd query &lt;subcommand> --help' for details.

```
dtmgd query [flags]
```

### Options

```
  -h, --help   help for query
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
  -o, --output string     output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI
* [dtmgd query log-counts](dtmgd_query_log-counts.md)	 - Aggregate log counts by service and level (INFO/WARN/ERROR)
* [dtmgd query logs](dtmgd_query_logs.md)	 - Search log records
* [dtmgd query metrics](dtmgd_query_metrics.md)	 - Query metric time-series data

