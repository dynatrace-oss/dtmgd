## dtmgd config import

Import contexts from a Dynatrace Managed MCP server config

### Synopsis

Import contexts from a Dynatrace Managed MCP server configuration: the file
DT_CONFIG_FILE names (a YAML or JSON array; the extension decides which), or,
with --from-env, the JSON in DT_ENVIRONMENT_CONFIGS.

Each environment becomes a context named after its alias:
```
  alias           context name
  apiEndpointUrl  host (trailing / trimmed)
  environmentId   env-id (trailing / trimmed)
  apiToken        a token named <alias>-token, the context's token-ref
  httpProxyUrl    http-proxy
  httpsProxyUrl   https-proxy
```
Other fields, such as dynatraceUrl, are ignored.

A literal apiToken is stored as set-credentials stores one: in the OS keyring
when available, otherwise in the config file. An apiToken that is or contains
a ${VAR} reference is written to the config as the reference. It is never
expanded, so the secret it names reaches neither the file nor the keyring.

An alias that names an existing context is skipped unless --overwrite is
given. Nothing is written if any environment is invalid.

Examples:
```
  dtmgd config import dt-config.yaml --dry-run
  dtmgd config import dt-config.yaml
  dtmgd config import --from-env --overwrite
```

```
dtmgd config import (<file> | --from-env) [flags]
```

### Options

```
      --dry-run     show what would be imported without writing anything
      --from-env    read the environments from DT_ENVIRONMENT_CONFIGS
  -h, --help        help for import
      --overwrite   replace existing contexts with the same name
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

* [dtmgd config](dtmgd_config.md)	 - Manage dtmgd configuration

