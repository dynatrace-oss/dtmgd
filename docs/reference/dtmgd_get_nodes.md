## dtmgd get nodes

List the nodes of a Dynatrace Managed cluster (cluster context)

### Synopsis

List a cluster's nodes from the Cluster Management API: each node's state and
version, and from the node configuration its address, datacenter and roles
(web UI, agent traffic). Needs a cluster context: one without env-id, whose
token has the ServiceProviderAPI scope.

json and yaml output is the API's node list, each node with its
configuration entry under "configuration".

Examples:
```
  dtmgd get nodes --context prod-cluster
  dtmgd get nodes -o wide
```

```
dtmgd get nodes [flags]
```

### Options

```
  -h, --help        help for nodes
      --limit int   maximum number of nodes to return
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

