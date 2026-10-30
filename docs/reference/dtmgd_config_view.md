## dtmgd config view

Display the current configuration

### Synopsis

Display the current configuration.

Token values are masked. Pass --show-tokens to print them verbatim.

Masking matters only where the OS keyring is unavailable — headless Linux,
containers, CI runners — because that is where SetToken falls back to writing
the token into the config file rather than leaving an empty placeholder. That
is also where the output of this command is most likely to be captured in a
build log.

```
dtmgd config view [flags]
```

### Options

```
  -h, --help          help for view
      --show-tokens   print token values instead of masking them
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

* [dtmgd config](dtmgd_config.md)	 - Manage dtmgd configuration

