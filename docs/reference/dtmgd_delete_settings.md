## dtmgd delete settings

Delete a Settings 2.0 object

### Synopsis

Delete one Settings 2.0 object. Deleting the object at a scope makes that
scope inherit its value again.

A person at a terminal confirms the delete; --yes skips the question, and is
required in agent mode or without a terminal.

An object that other settings still reference (a schema's "setting"
properties, such as an alerting profile a notification uses) is not
deleted: change or delete those settings first. Finding them reads every
schema's definition, which takes a few seconds. On a cluster context, an
update window that the ActiveGates' auto-update uses is not deleted either.
Only Settings 2.0 is checked; dashboards and other configuration are not.

--dry-run shows what would be deleted, with GETs only, at any write level.

Examples:
```
  dtmgd delete settings <object-id> --dry-run
  dtmgd delete settings <object-id> --yes
```

```
dtmgd delete settings <object-id> [flags]
```

### Options

```
      --dry-run[=true|false]   show what would be deleted, sending GETs only
  -h, --help[=true|false]      help for settings
  -y, --yes[=true|false]       delete without asking (required in agent mode and without a terminal)
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

* [dtmgd delete](dtmgd_delete.md)	 - Delete resources

