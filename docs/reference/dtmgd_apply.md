## dtmgd apply

Create or update the Settings 2.0 objects in a file

### Synopsis

Create or update the Settings 2.0 objects in a file: YAML or JSON (- for
stdin), holding one {objectId?, externalId?, schemaId, scope, value,
updateToken?, position?} object, an array of them, or a list as
get settings -o yaml prints it, in one or several "---" documents. An
exported object applies back as it is, an agent-mode export (the envelope)
included.

For each object apply first finds out whether it exists, with GETs only:
```
  - with objectId: that object, which must have the file's schemaId and
    scope (if it is not found, apply creates it, which moves a file from one
    environment to another);
  - without: for a schema holding one object per scope, the object at the
    scope; otherwise the object with the same externalId, or with the same
    value of the schema's key property.
```
Two objects of one file may not write the same object.
An existing object is updated, with the updateToken from the file (which
catches changes since the export) or from the GET; one whose value is the
same is left unchanged. Anything else is created. Give objects of
multi-object schemas an externalId to make repeated applies update them
instead of creating duplicates.

In an ordered schema a new object goes last and an updated one keeps its
place, unless the object's position key says: first, last, after:&lt;ref> or
before:&lt;ref>, where &lt;ref> is the objectId or externalId of an object at the
same scope, or the externalId of an object created earlier in the file.

A secret property holding the cluster's mask, as exports do, keeps the
stored secret on an update, but cannot create an object. Reports show
secrets as &lt;secret>.

--dry-run shows the planned changes, with GETs only, at any write level.
--validate-only asks the cluster to check every change without saving it
(Dynatrace Managed 1.288 or later). Every selected context is checked before
any of them is written.

Examples:
```
  dtmgd describe settings <object-id> -o yaml > object.yaml   # export, edit, then:
  dtmgd apply -f object.yaml --dry-run
  dtmgd apply -f object.yaml
  dtmgd apply -f windows.yaml --env ALL_ENVIRONMENTS
```

```
dtmgd apply -f <file> [flags]
```

### Options

```
      --dry-run[=true|false]         show the planned changes, sending GETs only
  -f, --filename string              file holding the objects, YAML or JSON (- for stdin)
  -h, --help[=true|false]            help for apply
      --validate-only[=true|false]   have the cluster validate every change without saving it
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

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI

