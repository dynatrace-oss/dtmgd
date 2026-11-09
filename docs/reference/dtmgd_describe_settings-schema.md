## dtmgd describe settings-schema

Show a Settings 2.0 schema: scopes, properties and allowed values

### Synopsis

Show a Settings 2.0 schema: its version, the scopes it can be set at, how
many objects a scope holds and whether they are ordered, its maturity and
key property, and its properties with their types, defaults and allowed
values. A reference to another settings object shows as
"setting → &lt;schema-id>", a secret as "secret".

-o json or -o yaml prints the API's definition as it is.

Examples:
```
  dtmgd describe settings-schema builtin:deployment.oneagent.updates
  dtmgd describe settings-schema builtin:deployment.management.update-windows -o json
```

```
dtmgd describe settings-schema <schema-id> [flags]
```

### Options

```
  -h, --help[=true|false]   help for settings-schema
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

* [dtmgd describe](dtmgd_describe.md)	 - Show detailed information about a specific resource

