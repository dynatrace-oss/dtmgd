## dtmgd query log-counts

Aggregate log counts by service and level (INFO/WARN/ERROR)

### Synopsis

Aggregate log records from /api/v2/logs/aggregate, grouped by process group and log level.

Returns INFO, WARN, and ERROR counts per process group, named by its display name. The
--entity selector finds the process groups; the aggregate call groups by
dt.entity.process_group, and the result is filtered to the groups the selector found.

On Dynatrace Managed Classic, logs are attributed to PROCESS_GROUP entities (not SERVICE
entities). If the given --entity selector targets SERVICE entities (type(SERVICE),...), the
command automatically derives an equivalent PROCESS_GROUP selector by substituting the type.

Each level's aggregation returns at most --max-services process groups. When a level
reaches that limit, a process group missing from it may still have records at that level;
its count is shown as "-" (null in json and yaml) rather than 0.

NOTE: On Dynatrace Managed Classic, structured field queries (e.g. loglevel:ERROR) are
not supported. Log level counts are approximated by searching for "INFO", "WARN", "ERROR"
as full-text terms in log content. This is accurate for standard Spring Boot / Java logs
where the level appears in each log line.

Examples:
```
  dtmgd query log-counts --entity 'type(SERVICE),tag("env:prod")' --from now-1h
  dtmgd query log-counts --entity 'type(PROCESS_GROUP),tag("env:prod")' --from now-30m --to now
```

```
dtmgd query log-counts [flags]
```

### Options

```
      --entity string      entitySelector to scope log aggregation (required)
      --from string        start time (default: now-1h), e.g. now-2h
  -h, --help               help for log-counts
      --max-services int   maximum number of process groups the aggregation returns per level (max: 100) (default 100)
      --to string          end time (default: now)
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

* [dtmgd query](dtmgd_query.md)	 - Query time-series or log data

