## dtmgd get

List or retrieve resources

### Synopsis

List resources from a Dynatrace Managed environment.

Supported resources:
```
  problems (prob)       Active and resolved problems
  entities (ent)        Monitored entities (requires --selector)
  entity-types (et)     All available entity types
  events (ev)           Events within a time range
  metrics (met)         Available metric descriptors
  slos                  Service Level Objectives
  security-problems (sp) Security vulnerabilities
```

Cluster resources (need a cluster context: one without env-id):
```
  environments (envs)   The cluster's environments
  nodes (node)          The cluster's nodes, their state, version and roles
  license-usage         Each environment's total billed license usage
```

Use 'dtmgd get &lt;resource> --help' for resource-specific options.

```
dtmgd get [flags]
```

### Options

```
  -h, --help   help for get
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

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI
* [dtmgd get entities](dtmgd_get_entities.md)	 - Discover entities using an entity selector
* [dtmgd get entity-types](dtmgd_get_entity-types.md)	 - List all available entity types
* [dtmgd get environments](dtmgd_get_environments.md)	 - List the environments of a Dynatrace Managed cluster (cluster context)
* [dtmgd get events](dtmgd_get_events.md)	 - List events from the Dynatrace Managed environment
* [dtmgd get license-usage](dtmgd_get_license-usage.md)	 - List each environment's total billed license usage (cluster context)
* [dtmgd get metrics](dtmgd_get_metrics.md)	 - List available metric descriptors
* [dtmgd get nodes](dtmgd_get_nodes.md)	 - List the nodes of a Dynatrace Managed cluster (cluster context)
* [dtmgd get problems](dtmgd_get_problems.md)	 - List problems from the Dynatrace Managed environment
* [dtmgd get security-problems](dtmgd_get_security-problems.md)	 - List security vulnerabilities
* [dtmgd get slos](dtmgd_get_slos.md)	 - List Service Level Objectives

