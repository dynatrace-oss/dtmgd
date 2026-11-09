## dtmgd get settings-schemas

List Settings 2.0 schemas

### Synopsis

List the Settings 2.0 schemas the environment offers, or one schema.

--search keeps the schemas whose ID or display name contains the text,
ignoring case. describe settings-schema shows a schema's properties.

Examples:
```
  dtmgd get settings-schemas --search deployment
  dtmgd get settings-schemas builtin:deployment.oneagent.updates
  dtmgd describe settings-schema builtin:deployment.oneagent.updates
```

```
dtmgd get settings-schemas [schema-id] [flags]
```

### Options

```
  -h, --help[=true|false]   help for settings-schemas
      --limit int           maximum number of results to show
      --search string       keep schemas whose ID or display name contains this text (ignoring case)
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

* [dtmgd get](dtmgd_get.md)	 - List or retrieve resources

