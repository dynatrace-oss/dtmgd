## dtmgd commands

Print a machine-readable catalog of dtmgd commands

### Synopsis

Print every dtmgd command with its aliases, flags and the API token scopes it
needs, for AI agents and scripts to discover the CLI.

The catalog is JSON (or YAML with -o yaml) and is never wrapped in an agent
envelope. Global flags (--context, --env, -o and the rest) are listed once,
under global_flags; each command lists only its own flags. --brief lists only
the resources of get, describe and query.

Examples:
```
  dtmgd commands             # full catalog: paths, aliases, flags, token scopes
  dtmgd commands --brief     # the resources of get, describe and query
  dtmgd commands -o yaml     # the catalog as YAML
```

```
dtmgd commands [flags]
```

### Options

```
      --brief   list only the verbs and their resources
  -h, --help    help for commands
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
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

