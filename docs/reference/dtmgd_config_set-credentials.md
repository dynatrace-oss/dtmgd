## dtmgd config set-credentials

Store an API token credential

### Synopsis

Store an API token under a name that contexts refer to with --token-ref.

The token is read from the terminal by default, with echo disabled, so it
reaches neither the screen nor your shell history:

```
  dtmgd config set-credentials prod-token
```

For scripts and CI, pipe it in on stdin:

```
  echo "$DT_API_TOKEN" | dtmgd config set-credentials prod-token --token-stdin
  dtmgd config set-credentials prod-token --token-stdin < /run/secrets/dt-token
```

--token is still accepted, but a value passed there is copied into the process
argument vector by the kernel and is readable from /proc/<pid>/cmdline by any
process running as you, as well as being recorded in your shell history.

```
dtmgd config set-credentials <name> [flags]
```

### Options

```
  -h, --help           help for set-credentials
      --token string   API token value (exposed in the process list — prefer --token-stdin)
      --token-stdin    read the API token from stdin
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config)
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

