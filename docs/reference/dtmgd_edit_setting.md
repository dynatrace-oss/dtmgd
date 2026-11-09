## dtmgd edit setting

Edit a Settings 2.0 object's value in an editor

### Synopsis

Open a Settings 2.0 object's value, as YAML, in $EDITOR (else $VISUAL, else
vi), and update the object when the file changed. Leaving it unchanged
cancels.

It needs a person at a terminal: in agent mode or without one it refuses;
use describe settings -o yaml and apply instead.

--validate-only asks the cluster to check the edited value without saving it
(Dynatrace Managed 1.288 or later).

Examples:
```
  dtmgd edit setting <object-id>
  EDITOR="code --wait" dtmgd edit setting <object-id> --validate-only
```

```
dtmgd edit setting <object-id> [flags]
```

### Options

```
  -h, --help[=true|false]            help for setting
      --validate-only[=true|false]   have the cluster validate the edited value without saving it
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

