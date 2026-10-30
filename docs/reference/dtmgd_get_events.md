## dtmgd get events

List events from the Dynatrace Managed environment

### Synopsis

List events within a time range.

Examples:
```
  dtmgd get events --from now-1h --to now
  dtmgd get events --from now-6h --type CUSTOM_DEPLOYMENT
  dtmgd get events --env ALL_ENVIRONMENTS --from now-1h
```

```
dtmgd get events [flags]
```

### Options

```
      --entity string             entity selector to filter events
      --from string               start time (required), e.g. now-1h
  -h, --help                      help for events
      --limit int                 maximum number of events
      --to string                 end time (default: now)
      --type string               filter by event type (e.g. CUSTOM_DEPLOYMENT)
  -w, --watch                     re-run the command periodically
      --watch-interval duration   interval between watch refreshes (e.g. 5s, 30s, 1m) (default 5s)
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

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

