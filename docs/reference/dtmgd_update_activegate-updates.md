## dtmgd update activegate-updates

Change how the cluster's ActiveGates update

### Synopsis

Change the cluster ActiveGates' global auto-update configuration, or, with an
ID, one ActiveGate's. The file (YAML or JSON, - for stdin) holds the object
describe activegate-updates prints: {globalSetting: ENABLED|DISABLED} for the
global one, {setting: ENABLED|DISABLED|INHERITED} for one ActiveGate.

targetVersion and updateWindows are accepted only where the cluster's
configuration carries them (they take effect behind a server feature flag);
elsewhere the file is refused rather than the fields dropped.

--dry-run shows the change with GETs only. --validate-only has the cluster's
validator check it without saving. Needs a cluster context whose
safety-level allows updates, and a token with ServiceProviderAPI.

Examples:
```
  dtmgd describe activegate-updates -o yaml --context stage-cluster > global.yaml
  dtmgd update activegate-updates -f global.yaml --dry-run --context stage-cluster
  dtmgd update activegate-updates <activegate-id> -f gate.yaml --context stage-cluster
```

```
dtmgd update activegate-updates [activegate-id] -f <file> [flags]
```

### Options

```
      --dry-run[=true|false]         show the change, sending GETs only
  -f, --filename string              file holding the configuration, YAML or JSON (- for stdin)
  -h, --help[=true|false]            help for activegate-updates
      --validate-only[=true|false]   have the cluster's validator check the change without saving it
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

* [dtmgd update](dtmgd_update.md)	 - Update resources from a file

