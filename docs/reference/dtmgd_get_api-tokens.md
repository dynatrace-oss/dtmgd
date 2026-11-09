## dtmgd get api-tokens

List API tokens and find the unused ones

### Synopsis

List the environment's API tokens (never their values), with owner, creation,
last use, expiry and scope count.

Filters run on the client, after all tokens are fetched. With several set, a
token matching any one is kept:

```
  --unused-days N   last used more than N days ago, or never used and older than N days
  --never-used      never used (tokens younger than --grace-days are skipped)
  --expired         expiration date in the past
  --disabled        disabled
```

--selector is the apiTokenSelector DSL, applied by the API:
```
  name("..."), owner("..."), enabled(true|false), personalAccessToken(true|false)
```

lastUsedDate is refreshed by Dynatrace about once an hour.

Needs the token scope apiTokens.read.

Examples:
```
  dtmgd get api-tokens
  dtmgd get api-tokens --unused-days 90
  dtmgd get api-tokens --never-used --grace-days 14 -o json
  dtmgd get api-tokens --unused-days 90 --expired --disabled
  dtmgd get api-tokens --selector 'owner("jane@example.com")' --sort=-lastUsedDate
```

```
dtmgd get api-tokens [flags]
```

### Options

```
      --disabled[=true|false]     keep disabled tokens
      --expired[=true|false]      keep tokens whose expiration date has passed
      --grace-days int            with --never-used, skip tokens created less than N days ago
  -h, --help[=true|false]         help for api-tokens
      --limit int                 maximum number of results to show
      --never-used[=true|false]   keep tokens that were never used
      --selector string           apiTokenSelector DSL to filter tokens (e.g. 'owner("jane@example.com")')
      --sort string               sort by name, owner, creationDate, expirationDate or lastUsedDate; prefix with - to descend
      --unused-days int           keep tokens last used more than N days ago, or never used and older than N days
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

