## dtmgd describe

Show detailed information about a specific resource

### Synopsis

Show detailed information about a resource by its ID.

Supported resources:
```
  problem <id>              Detailed problem with evidence and affected entities
  entity <id>               Entity details with properties and tags
  entity-type <type>        Entity type schema and properties
  entity-relations <id>     Relationships of an entity
  event <id>                Event details
  metric <id>               Metric descriptor details
  slo <id>                  SLO details with evaluation data
  security-problem <id>     Security vulnerability details with CVE info
  settings <object-id>      A Settings 2.0 object and its value
  settings-schema <id>      A Settings 2.0 schema: scopes, properties, allowed values
```

Cluster resources (need a cluster context: one without env-id):
```
  activegate-updates [id]   The cluster ActiveGates' auto-update configuration
  backup                    The cluster's backup configuration
  cluster                   The cluster's name, version and node states
  environment <id>          An environment's properties, quotas and storage
  license                   The cluster license and its billed usage
```

Detail commands print a readable summary (or YAML) by default; use -o json for the raw API object.

```
dtmgd describe [flags]
```

### Options

```
  -h, --help[=true|false]   help for describe
```

### Options inherited from parent commands

```
  -A, --agent[=true|false]      force agent envelope output mode
      --columns string          comma-separated list of columns to show in table, csv and tsv output
      --concurrency int         maximum environments queried at once with --env (default 10)
      --config string           config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -c, --context string          context to use (overrides current-context)
  -e, --env string              environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int           maximum number of pages to fetch (0 = all pages)
      --no-agent[=true|false]   disable auto-detected agent mode
  -o, --output string           output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain[=true|false]      plain output: no colors, symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count           verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI
* [dtmgd describe activegate-updates](dtmgd_describe_activegate-updates.md)	 - Show the cluster ActiveGates' auto-update configuration
* [dtmgd describe backup](dtmgd_describe_backup.md)	 - Show a Dynatrace Managed cluster's backup configuration (cluster context)
* [dtmgd describe cluster](dtmgd_describe_cluster.md)	 - Show a Dynatrace Managed cluster's name, version and nodes (cluster context)
* [dtmgd describe entity](dtmgd_describe_entity.md)	 - Show detailed information about a specific entity
* [dtmgd describe entity-relations](dtmgd_describe_entity-relations.md)	 - Show entity relationships
* [dtmgd describe entity-type](dtmgd_describe_entity-type.md)	 - Show schema details of an entity type
* [dtmgd describe environment](dtmgd_describe_environment.md)	 - Show one environment of a Dynatrace Managed cluster (cluster context)
* [dtmgd describe event](dtmgd_describe_event.md)	 - Show detailed information about a specific event
* [dtmgd describe license](dtmgd_describe_license.md)	 - Show a Dynatrace Managed cluster's license and billed usage (cluster context)
* [dtmgd describe metric](dtmgd_describe_metric.md)	 - Show detailed information about a specific metric descriptor
* [dtmgd describe problem](dtmgd_describe_problem.md)	 - Show detailed information about a specific problem
* [dtmgd describe security-problem](dtmgd_describe_security-problem.md)	 - Show detailed information about a security vulnerability
* [dtmgd describe settings](dtmgd_describe_settings.md)	 - Show a Settings 2.0 object
* [dtmgd describe settings-schema](dtmgd_describe_settings-schema.md)	 - Show a Settings 2.0 schema: scopes, properties and allowed values
* [dtmgd describe slo](dtmgd_describe_slo.md)	 - Show detailed information about a specific SLO

