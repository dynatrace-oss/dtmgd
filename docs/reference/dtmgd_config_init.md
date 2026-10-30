## dtmgd config init

Create a .dtmgd.yaml template in the current directory

### Synopsis

Create a project-local .dtmgd.yaml configuration template.

Environment variables can be used with ${VAR_NAME} syntax.

Examples:
```
  dtmgd config init
  dtmgd config init --context my-cluster
```

```
dtmgd config init [flags]
```

### Options

```
      --context string   context name to pre-fill in template
      --force            overwrite existing .dtmgd.yaml
  -h, --help             help for init
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -e, --env string        environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int     maximum number of pages to fetch (0 = all pages)
      --no-agent          disable auto-detected agent mode
  -o, --output string     output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd config](dtmgd_config.md)	 - Manage dtmgd configuration

