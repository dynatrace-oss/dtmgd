## dtmgd describe slo

Show detailed information about a specific SLO

### Synopsis

Show SLO details including evaluation, error budget, and burn rate.

Examples:
```
  dtmgd describe slo <id>
  dtmgd describe slo <id> --timeframe CURRENT
  dtmgd describe slo <id> --from now-2w --to now
```

--from/--to imply --timeframe GTF when --timeframe is not given.

```
dtmgd describe slo <slo-id> [flags]
```

### Options

```
      --from string        start time (implies --timeframe GTF)
  -h, --help               help for slo
      --timeframe string   time frame: CURRENT or GTF
      --to string          end time (implies --timeframe GTF)
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

* [dtmgd describe](dtmgd_describe.md)	 - Show detailed information about a specific resource

