## dtmgd config

Manage dtmgd configuration

### Synopsis

View and modify dtmgd configuration including contexts (Managed environments) and credentials.

```
dtmgd config [flags]
```

### Options

```
  -h, --help   help for config
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
* [dtmgd config current-context](dtmgd_config_current-context.md)	 - Display the current context
* [dtmgd config delete-context](dtmgd_config_delete-context.md)	 - Delete a context from the config
* [dtmgd config delete-credentials](dtmgd_config_delete-credentials.md)	 - Delete a stored API token from the keyring and the config
* [dtmgd config get-contexts](dtmgd_config_get-contexts.md)	 - List all available contexts
* [dtmgd config import](dtmgd_config_import.md)	 - Import contexts from a Dynatrace Managed MCP server config
* [dtmgd config init](dtmgd_config_init.md)	 - Create a .dtmgd.yaml template in the current directory
* [dtmgd config migrate-tokens](dtmgd_config_migrate-tokens.md)	 - Migrate tokens from config file to OS keyring
* [dtmgd config set-context](dtmgd_config_set-context.md)	 - Create or update a context entry
* [dtmgd config set-credentials](dtmgd_config_set-credentials.md)	 - Store an API token credential
* [dtmgd config use-context](dtmgd_config_use-context.md)	 - Switch to a different context
* [dtmgd config view](dtmgd_config_view.md)	 - Display the current configuration

