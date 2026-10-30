## dtmgd describe problem

Show detailed information about a specific problem

### Synopsis

Show full problem details including evidence, affected entities, and root cause.
Use the problemId (UUID format) from 'dtmgd get problems', not the display ID (P-XXXXX).
Negative-integer problem IDs (e.g. -6546711275898328738_1776193140000V2) are handled
automatically — no special syntax or "--" separator is required.

```
dtmgd describe problem <problem-id> [flags]
```

### Options

```
  -h, --help   help for problem
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

* [dtmgd describe](dtmgd_describe.md)	 - Show detailed information about a specific resource

