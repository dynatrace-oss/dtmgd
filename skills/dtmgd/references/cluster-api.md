# dtmgd and the Cluster Management API

Read this when the user asks about a cluster's environments, nodes, license, license usage, backup configuration, the cluster's own settings, or how its ActiveGates update. These commands need a cluster context.

## Cluster Management API

A context **without `env-id`** is a cluster context: it reads the cluster's
own Cluster Management API: read-only, except the cluster's settings and its ActiveGates' auto-update, which follow the context's `safety-level`. `dtmgd ctx` shows each context's
`SCOPE` (`environment` or `cluster`); `dtmgd commands` marks the commands that
need a cluster context with `"cluster_scoped": true`.

```bash
dtmgd get environments --context <cluster-context>       # --name, --state, --filter, --sort, --consumption, --limit
dtmgd describe environment <environment-id> --context <cluster-context>   # properties, quotas, storage; --consumption
dtmgd get nodes --context <cluster-context>              # state, version, address, datacenter, roles
dtmgd describe cluster --context <cluster-context>       # name, UUID, version (lowest node's), node states
dtmgd describe license --context <cluster-context>       # license and billed usage (key masked)
dtmgd get license-usage --context <cluster-context>      # billed usage per environment
dtmgd describe backup --context <cluster-context>        # backup configuration only
```

- Run a cluster command against an environment context (or the reverse) and
  dtmgd exits 2 without sending anything, naming the contexts of the right
  kind in its suggestions. Retry with `--context <one of them>`.
- No `--env` fan-out for cluster commands.
- A cluster context's host is the cluster's own address (the Cluster
  Management Console's), not a `:9999` node address. A 404 from every cluster
  command means the host is wrong.
- Its token needs the `ServiceProviderAPI` scope, from the Cluster Management
  Console. An environment API token gets 401.
- `describe license`, `get license-usage` and `get environments --consumption`
  work only with Dynatrace classic licensing; on a DPS-licensed cluster they
  fail with a hint saying so.
- `describe backup` has no last-backup time or backup health: the API reports
  the configuration only.

## The cluster's own settings

The settings commands work on a cluster context too, through the Cluster
API's settings endpoints, with the same workflow as in an environment
([settings.md](settings.md)). `describe settings-schema` lists the scopes
each schema takes:

- `cluster`: the cluster itself;
- `environment-default`: defaults for every environment of the cluster;
- `realm`, `metric` and `CLUSTER_ACTIVE_GATE`, in a few schemas.

On a cluster context, `--scope environment` (or `tenant`) does not mean an
environment: the cluster takes it as its own scope and returns the
`cluster` objects. Environment and entity scopes (`HOST-…`) belong to an
environment context; a cluster answers them with 404 "Scope … not found".

```bash
dtmgd get settings-schemas --context <cluster-context>                     # the schemas a cluster has
dtmgd get settings --scope cluster --context <cluster-context>             # every schema stored at one scope
dtmgd get settings --scope environment-default --context <cluster-context>
dtmgd get settings --schema builtin:networkzones.zones --context <cluster-context>   # one schema, every scope
dtmgd get settings --schema builtin:deployment.management.update-windows --scope cluster --context <cluster-context>
dtmgd apply -f window.yaml --dry-run --context <cluster-context>           # writes need the context's safety-level
```

- **Network zones** (`builtin:networkzones.zones`) are stored at
  `environment-default`, although the schema lists `cluster` too: at
  `cluster` the cluster answers 404 "No schema with topic identifier".
- **Listing by scope alone works**, `--effective` included. The cluster
  itself answers such a listing with 404 "No schema with schemaId …", so
  dtmgd then asks each schema (8 at a time) and merges the results in
  schema ID order. That takes a few seconds; `--limit` and the note that
  more exist work as for any listing. Schemas that cannot answer at the scope
  are skipped; when none can (a mistyped scope, say), dtmgd reports an error.
- **A 404 with a specific message** ("No schema with …", "Settings objectId
  '…' not found") comes from the Cluster API, so the host is right. Only a
  404 without a message, or with the generic "HTTP 404 Not Found", adds the
  hint about a wrong host.
- The cluster token needs `settings.read` (and `settings.write` to write)
  besides `ServiceProviderAPI`, which creating, `--validate-only` (it reads
  the cluster version) and deleting an update window (it checks the
  ActiveGates' auto-update first) also use. A token made for the other
  cluster commands often lacks the settings scopes: a 403 says so.
- A cluster context runs alone: it is never part of `--env "a;b"` or
  `ALL_ENVIRONMENTS`, and no command takes `--context` together with
  `--env`.
- Cluster update windows are what cluster ActiveGates update in (where the
  cluster honours windows, below). The cluster's own self-update window
  (Cluster Management Console, Automatic update) has no public API.

## Cluster ActiveGate auto-update

```bash
dtmgd get activegate-updates --context <cluster-context>     # global setting, then each cluster ActiveGate
dtmgd describe activegate-updates [<activegate-id>] -o yaml --context <cluster-context>
dtmgd update activegate-updates [<activegate-id>] -f config.yaml --dry-run --context <cluster-context>
dtmgd update activegate-updates [<activegate-id>] -f config.yaml --context <cluster-context>
```

- The global configuration is `{globalSetting: ENABLED|DISABLED}`; one
  ActiveGate's is `{setting: ENABLED|DISABLED|INHERITED}` (any other field is
  refused, besides the two below), with the
  read-only `effectiveSetting` (what applies) and, in `get`, its update
  `STATUS` (`UP2DATE`, `OUTDATED`, `UPDATE_IN_PROGRESS`, …).
- `targetVersion` and `updateWindows` take effect only where the cluster's
  configuration returns them (a server feature flag, off by default).
  `get` shows them only then, and `update` refuses a file that sets them
  elsewhere instead of letting the cluster drop them silently. Where the
  cluster honours windows, a file must keep `updateWindows` (leaving it out
  would clear them); window names are read-only and not sent.
- `--validate-only` uses the cluster's validator, which never saves.
  Changing these settings starts or stops real updates: dry-run first.
- A cluster update window that the ActiveGate configurations still name is
  not deleted (`delete settings` exits 1, naming them).
- Token: `ServiceProviderAPI`.
