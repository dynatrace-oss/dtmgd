## dtmgd get entities

Discover entities using an entity selector

### Synopsis

Discover monitored entities using the Dynatrace entity selector syntax.
--selector is required and must specify exactly ONE entity type.

Examples:
```
  dtmgd get entities --selector 'type(SERVICE)'
  dtmgd get entities --selector 'type(HOST),healthState("HEALTHY")'
  dtmgd get entities --env ALL_ENVIRONMENTS --selector 'type(SERVICE)'
```

```
dtmgd get entities [flags]
```

### Options

```
      --from string       start time for entity observation
  -h, --help              help for entities
      --limit int         maximum number of entities
      --mz string         management zone selector
      --selector string   entity selector (required), e.g. 'type(SERVICE)'
      --sort string       sort order (name or -name)
      --to string         end time for entity observation
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

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

