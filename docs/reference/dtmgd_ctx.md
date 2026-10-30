## dtmgd ctx

Manage contexts (shortcut for config context commands)

### Synopsis

Quick context management without the "config" prefix.

When called without arguments, lists all contexts.
When called with a context name, switches to that context.

Examples:
```
  dtmgd ctx                   # list all contexts
  dtmgd ctx production        # switch to context
  dtmgd ctx current           # show current context name
```

```
dtmgd ctx [context-name] [flags]
```

### Options

```
  -h, --help   help for ctx
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
* [dtmgd ctx current](dtmgd_ctx_current.md)	 - Display the current context name
* [dtmgd ctx delete](dtmgd_ctx_delete.md)	 - Delete a context
* [dtmgd ctx set](dtmgd_ctx_set.md)	 - Create or update a context

