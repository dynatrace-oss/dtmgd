## dtmgd doctor

Check connectivity, token and cluster version of each context

### Synopsis

Check every configured context, or only the one named with --context: that it
resolves, that its API token is stored, that the cluster answers, and that the
cluster version is at least 1.328.0.

When the token cannot read the cluster version, doctor tries a metrics call
instead, as the Dynatrace Managed MCP server does, and reports a warning if
that works. A cluster context (no env-id) is checked against the Cluster API:
its token must have the ServiceProviderAPI scope, and its version is its
lowest node's, with no metrics fallback.

It prints one row per context (CONTEXT, SCOPE, HOST, ENV-ID, STATUS, VERSION,
DETAIL) with status ok, warn or fail, checking up to --concurrency contexts at
once. Exits 1 if any context fails.

Examples:
```
  dtmgd doctor               # every configured context
  dtmgd doctor -c prod       # one context
  dtmgd doctor -o json       # rows as JSON (also -o yaml, or -A for an agent envelope)
```

```
dtmgd doctor [flags]
```

### Options

```
  -h, --help   help for doctor
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

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI

