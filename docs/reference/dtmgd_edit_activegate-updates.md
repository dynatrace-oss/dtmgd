## dtmgd edit activegate-updates

Edit the cluster ActiveGates' auto-update in an editor

### Synopsis

Open the cluster ActiveGates' global auto-update configuration (or one
ActiveGate's) in $EDITOR, and update it when the file changed. For people at a
terminal; agents use update activegate-updates -f.

Examples:
```
  dtmgd edit activegate-updates --context stage-cluster
```

```
dtmgd edit activegate-updates [activegate-id] [flags]
```

### Options

```
  -h, --help[=true|false]            help for activegate-updates
      --validate-only[=true|false]   have the cluster's validator check the edited configuration without saving it
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

* [dtmgd edit](dtmgd_edit.md)	 - Edit resources in an editor

