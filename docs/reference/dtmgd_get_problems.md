## dtmgd get problems

List problems from the Dynatrace Managed environment

### Synopsis

List active or resolved problems. Defaults to the last 24 hours.

Filter examples:
```
  --status OPEN          only active problems
  --impact SERVICES       service-level impact
  --entity type(SERVICE),entityName.contains("checkout")
  --sort "+status"        open first
```

Multi-environment:
```
  dtmgd get problems --env ALL_ENVIRONMENTS
  dtmgd get problems --env "prod;staging"
```

```
dtmgd get problems [flags]
```

### Options

```
      --entity string             entitySelector to filter problems by entity
      --from string               start time (e.g. now-24h, 2024-01-01T00:00:00Z)
  -h, --help                      help for problems
      --impact string             filter by impact level: APPLICATION, ENVIRONMENT, INFRASTRUCTURE or SERVICES (added to problemSelector)
      --limit int                 maximum number of problems to return
      --selector string           problemSelector DSL (e.g. managementZones("production"))
      --sort string               sort order (e.g. +status, -startTime)
      --status string             filter by status: OPEN or CLOSED (added to problemSelector)
      --to string                 end time (default: now)
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
  -o, --output string     output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

