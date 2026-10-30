## dtmgd get security-problems

List security vulnerabilities

### Synopsis

List security problems (CVE vulnerabilities) in the Dynatrace Managed environment.

Examples:
```
  dtmgd get security-problems
  dtmgd get security-problems --risk CRITICAL
  dtmgd get security-problems --selector 'managementZones("production")'
  dtmgd get security-problems --from now-7d --sort +firstSeenTimestamp
  dtmgd get security-problems --env ALL_ENVIRONMENTS
```

Without --from the API returns the last 30 days. The default sort is the
highest risk score first (-riskAssessment.riskScore).

Scoping to entities: the security problems API has no entitySelector, so
use --selector criteria instead:
```
  --selector 'relatedHostIds("HOST-0123456789ABCDEF")'
  --selector 'affectedPgIds("PROCESS_GROUP-0123456789ABCDEF")'
  --selector 'managementZoneIds("1234567890")'
```

```
dtmgd get security-problems [flags]
```

### Options

```
      --from string       start time (default: now-30d)
  -h, --help              help for security-problems
      --limit int         maximum number of results
      --risk string       filter by risk level: CRITICAL, HIGH, MEDIUM, LOW or NONE
      --selector string   security problem selector, e.g. managementZones("production")
      --sort string       sort order, e.g. +firstSeenTimestamp (default: -riskAssessment.riskScore)
      --status string     filter by status: OPEN or RESOLVED (muted ones: --selector 'muted("TRUE")')
      --to string         end time (default: now)
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

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

