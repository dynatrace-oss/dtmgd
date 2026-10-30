## dtmgd query metrics

Query metric time-series data

### Synopsis

Query metric data for a specific metric selector and time range.

Use 'dtmgd get metrics --search <keyword>' to find available metric IDs first.

Examples:
```
  dtmgd query metrics --metric builtin:service.response.time --from now-1h --to now
  dtmgd query metrics --metric builtin:host.cpu.usage --from now-24h --resolution 1h
  dtmgd query metrics --env ALL_ENVIRONMENTS --metric builtin:host.cpu.usage --from now-1h
```

```
dtmgd query metrics [flags]
```

### Options

```
      --entity string       entity selector to filter metric data
      --from string         start time (required), e.g. now-1h
  -h, --help                help for metrics
      --metric string       metric selector (required), e.g. builtin:service.response.time
      --resolution string   data resolution (e.g. 5m, 1h, 1d)
      --to string           end time (default: now)
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

* [dtmgd query](dtmgd_query.md)	 - Query time-series or log data

