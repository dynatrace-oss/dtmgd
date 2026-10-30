## dtmgd completion

Generate shell completion script

### Synopsis

Generate shell completion script for dtmgd.

Examples:
```
  # Bash
  dtmgd completion bash > /etc/bash_completion.d/dtmgd

  # Zsh
  dtmgd completion zsh > "${fpath[1]}/_dtmgd"

  # Fish
  dtmgd completion fish > ~/.config/fish/completions/dtmgd.fish

  # PowerShell
  dtmgd completion powershell | Out-String | Invoke-Expression
```

```
dtmgd completion [bash|zsh|fish|powershell] [flags]
```

### Options

```
  -h, --help   help for completion
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

