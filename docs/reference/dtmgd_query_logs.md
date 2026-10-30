## dtmgd query logs

Search log records

### Synopsis

Search log records using simple text queries.

Managed clusters support basic text search. Do NOT use structured syntax
like "content:error" — just use the plain search term.

Examples:
```
  dtmgd query logs --query "error" --from now-1h --to now
  dtmgd query logs --query "timeout" --from now-30m --limit 50
  dtmgd query logs --query "error" --from now-1h --entity 'type(PROCESS_GROUP),tag("env:prod")'
  dtmgd query logs --env ALL_ENVIRONMENTS --query "error" --from now-1h
```

```
dtmgd query logs [flags]
```

### Options

```
      --entity string   entitySelector to scope log search (e.g. 'type(PROCESS_GROUP),tag("env:prod")'); not a documented /logs/search parameter, so check results against an unscoped search
      --from string     start time (required), e.g. now-1h
  -h, --help            help for logs
      --limit int       maximum number of records, at most 1000 (default: 100)
      --query string    text to search for in log content (required)
      --sort string     sort order: -timestamp (newest first, the default) or +timestamp
      --to string       end time (default: now)
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

* [dtmgd query](dtmgd_query.md)	 - Query time-series or log data

