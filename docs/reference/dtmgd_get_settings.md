## dtmgd get settings

List Settings 2.0 objects, or the values in effect at a scope

### Synopsis

List the Settings 2.0 objects stored in the environment, or one object.
With --effective, list the values that apply at --scope instead: inherited
values and schema defaults included, each with its origin (the scope it is
stored at, or "default").

A list needs --schema or --scope, as the API does. --schema takes one or more
schema IDs (repeat it or separate them with commas); --scope takes
"environment" or an entity ID such as HOST-…, HOST_GROUP-… or
ENVIRONMENT_ACTIVE_GATE-…, and on a cluster context "cluster",
"environment-default" or another scope the schema lists. Find schema IDs
with get settings-schemas, and a schema's scopes with describe
settings-schema. An ordered schema's objects are listed in their order.

json and yaml print the API's own objects, with objectId, schemaId, scope,
externalId, summary, updateToken, schemaVersion and value; effective values
have schemaId, origin, summary and value.

Examples:
```
  dtmgd get settings --schema builtin:deployment.management.update-windows
  dtmgd get settings --schema builtin:deployment.oneagent.updates --scope HOST_GROUP-0000000000000001
  dtmgd get settings --scope HOST-0000000000000001          # every schema stored on a host
  dtmgd get settings <object-id> -o yaml
  dtmgd get settings --effective --scope HOST-0000000000000001 --schema builtin:deployment.oneagent.updates
```

```
dtmgd get settings [object-id] [flags]
```

### Options

```
      --effective[=true|false]   show the values that apply at --scope, inherited ones and schema defaults included
  -h, --help[=true|false]        help for settings
      --limit int                maximum number of results to show
      --schema strings           schema ID(s) to list; repeat or separate with commas
      --scope string             scope: "environment" or an entity ID (HOST-…, HOST_GROUP-…, ENVIRONMENT_ACTIVE_GATE-…)
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

