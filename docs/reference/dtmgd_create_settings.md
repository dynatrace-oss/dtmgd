## dtmgd create settings

Create a Settings 2.0 object from a value file

### Synopsis

Create one Settings 2.0 object. The file (YAML or JSON, - for stdin) holds the
bare value; --schema and --scope say where it goes. To change an existing
object, or to create objects from a full {schemaId, scope, value} file, use
apply.

For a schema that holds one object per scope, create refuses when the scope
already has one.

In an ordered schema a new object goes last, or where --position says: first,
last, after:&lt;ref> or before:&lt;ref>, where &lt;ref> is the objectId or externalId
of an object at the same scope.

--dry-run shows what would be created, with GETs only, at any write level.
--validate-only asks the cluster to check the value without saving it
(Dynatrace Managed 1.288 or later).

Examples:
```
  dtmgd create settings -f window.yaml --schema builtin:deployment.management.update-windows --scope environment --dry-run
  dtmgd create settings -f window.yaml --schema builtin:deployment.management.update-windows --scope environment
  dtmgd create settings -f window.yaml --schema builtin:deployment.management.update-windows --scope environment --env "stage;prod"
```

```
dtmgd create settings -f <value-file> --schema <schema-id> --scope <scope> [flags]
```

### Options

```
      --dry-run[=true|false]         show what would be created, sending GETs only
  -f, --filename string              file holding the value, YAML or JSON (- for stdin)
  -h, --help[=true|false]            help for settings
      --position string              place the object in an ordered schema: first, last, after:<ref> or before:<ref>
      --schema string                schema ID of the new object
      --scope string                 scope of the new object: "environment" or an entity ID
      --validate-only[=true|false]   have the cluster validate the value without saving it
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

* [dtmgd create](dtmgd_create.md)	 - Create resources

