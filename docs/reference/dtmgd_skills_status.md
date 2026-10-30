## dtmgd skills status

Show installation status of skill files

### Synopsis

Show the current installation status of dtmgd skill files.

Checks both project-local and global locations for all supported agents,
including the cross-client shared directory (.agents/skills/).

```
dtmgd skills status [flags]
```

### Options

```
      --for string   check status for a specific agent (or "cross-client")
  -h, --help         help for status
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
      --plain             plain output: no colors, symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd skills](dtmgd_skills.md)	 - Manage AI coding assistant skill files

