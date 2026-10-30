## dtmgd get slos

List Service Level Objectives

### Synopsis

List SLOs from the Dynatrace Managed environment.

The --selector flag accepts the Dynatrace sloSelector DSL for server-side filtering:
```
  name("...")            exact name match
  text("...")            substring name search
  managementZone("...")  filter by management zone name
  managementZoneID("...") filter by management zone numeric ID
  healthState("HEALTHY"|"UNHEALTHY")
```

Examples:
```
  dtmgd get slos
  dtmgd get slos --selector 'managementZone("production")' --evaluate
  dtmgd get slos --selector 'text("checkout")' --evaluate
  dtmgd get slos --enabled false
  dtmgd get slos --evaluate --from now-7d          # evaluate over the last 7 days
  dtmgd get slos --show-global=false               # leave out global SLOs
  dtmgd get slos --env ALL_ENVIRONMENTS
```

SLOs are evaluated over their own timeframe (--timeframe CURRENT, the
default). --from or --to evaluate over the given dates instead and imply
--timeframe GTF.

```
dtmgd get slos [flags]
```

### Options

```
      --demo               return demo SLOs only (sent only when given)
      --enabled string     filter by enabled status: true, false, or all
      --evaluate           evaluate SLO percentages (slower; max 25 per page, auto-paged)
      --from string        start time (implies --timeframe GTF)
  -h, --help               help for slos
      --limit int          maximum number of results to show
      --selector string    sloSelector DSL to filter SLOs (e.g. 'managementZone("production")')
      --show-global        include global SLOs whatever the filter (sent only when given; the API default is true) (default true)
      --sort string        sort order: name or -name (default: name)
      --timeframe string   time frame: CURRENT (the SLO's own, the default) or GTF (--from/--to)
      --to string          end time (implies --timeframe GTF)
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

