## dtmgd get metrics

List available metric descriptors

### Synopsis

List available metrics in the Managed environment, optionally filtered by text.

The metrics API cannot list the metrics of an entity. To read a metric for an
entity, use query metrics with --entity.

Examples:
```
  dtmgd get metrics --search response.time
  dtmgd get metrics --env ALL_ENVIRONMENTS --search cpu
```

```
dtmgd get metrics [flags]
```

### Options

```
  -h, --help            help for metrics
      --limit int       maximum number of metrics
      --search string   text search in metric names/descriptions
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

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

