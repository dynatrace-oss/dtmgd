## dtmgd edit

Edit resources in an editor

### Synopsis

Edit a resource's value in an editor, for a person at a terminal. Agents and
scripts use apply.

Supported resources:
```
  setting <object-id>        A Settings 2.0 object's value
  activegate-updates [id]    The cluster ActiveGates' auto-update (cluster contexts)
```

```
dtmgd edit [flags]
```

### Options

```
  -h, --help[=true|false]   help for edit
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
* [dtmgd edit activegate-updates](dtmgd_edit_activegate-updates.md)	 - Edit the cluster ActiveGates' auto-update in an editor
* [dtmgd edit setting](dtmgd_edit_setting.md)	 - Edit a Settings 2.0 object's value in an editor

