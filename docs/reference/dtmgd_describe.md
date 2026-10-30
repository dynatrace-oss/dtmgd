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
```

Cluster resources (need a cluster context: one without env-id):
```
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
  -h, --help   help for describe
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

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI
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
* [dtmgd describe slo](dtmgd_describe_slo.md)	 - Show detailed information about a specific SLO

