# dtmgd and the Cluster Management API

Read this when the user asks about a cluster's environments, nodes, license, license usage or backup configuration. These commands need a cluster context.

## Cluster Management API

A context **without `env-id`** is a cluster context: it reads the cluster's
own Cluster Management API, read-only. `dtmgd ctx` shows each context's
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
