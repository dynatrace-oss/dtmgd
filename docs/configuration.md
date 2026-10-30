# Configuration and contexts

How dtmgd finds its config file, what a context holds, how environment and cluster contexts differ, and how to query several environments at once. The [command reference](reference/dtmgd_config.md) lists every `config` and `ctx` command.

## Configuration commands

```
dtmgd config set-context <name> --host <url> --env-id <id> --token-ref <ref>
dtmgd config set-credentials <name>              # prompts for the token
dtmgd config set-credentials <name> --token-stdin  # reads the token from stdin
dtmgd config get-contexts          # list all contexts
dtmgd config current-context       # show active context name
dtmgd config use-context <name>    # switch active context
dtmgd config delete-context <name> # remove a context
dtmgd config delete-context <name> --delete-credentials  # ... and its token, unless shared
dtmgd config delete-credentials <name>  # delete a token from the keyring and the config
dtmgd config migrate-tokens        # move plaintext tokens to OS keyring
dtmgd config view                  # dump full config (token values masked)
dtmgd config view --show-tokens    # ... including token values
dtmgd config init                  # create .dtmgd.yaml in current directory
dtmgd config import <file>         # create contexts from an MCP server config (see below)
dtmgd config import --from-env     # ... from DT_ENVIRONMENT_CONFIGS
dtmgd ctx                          # shortcut: list or switch contexts
dtmgd ctx [context-name]           # switch context
dtmgd ctx current                  # show current context
dtmgd ctx delete <name>            # delete context
```

With `-o json` or `-o yaml`, and in agent mode, these commands use the config
file's keys: `config view` prints the config as the file has it
(`current-context`, `contexts[].context.env-id`, `token-ref`, with token values
masked), `ctx` and `config get-contexts` print one object per context
(`current` true or false, `name`, `scope`, `host`, `env-id`, `token-ref`,
`description`), and `config current-context` and `ctx current` print
`{"current-context": "<name>"}`.

### Importing a Dynatrace Managed MCP server config

If you already run the [Dynatrace Managed MCP server](https://github.com/dynatrace-oss/dynatrace-managed-mcp),
`config import` turns its environments into dtmgd contexts. It reads the file
`DT_CONFIG_FILE` points at (a YAML or JSON array; the extension decides which), or,
with `--from-env`, the JSON in `DT_ENVIRONMENT_CONFIGS`:

```bash
dtmgd config import dt-config.yaml --dry-run   # show what would be created; writes nothing
dtmgd config import dt-config.yaml
dtmgd config import --from-env --overwrite     # replace contexts that already exist
```

| MCP field | dtmgd setting |
|---|---|
| `alias` | context name |
| `apiEndpointUrl` | `host` (trailing `/` trimmed) |
| `environmentId` | `env-id` (trailing `/` trimmed) |
| `apiToken` | token `<alias>-token`, the context's `token-ref` |
| `httpProxyUrl` / `httpsProxyUrl` | `http-proxy` / `https-proxy` |

Other fields, such as `dynatraceUrl`, are ignored with a note. A literal `apiToken` is
stored as `set-credentials` stores one: in the OS keyring, or in the config file when no
keyring is available. An `apiToken` such as `${DT_PROD_TOKEN}` is written to the config as
that reference, never expanded, so the secret stays in the environment variable. An alias
that names an existing context is skipped unless you pass `--overwrite`. If any environment
is invalid (a missing `alias`, `apiEndpointUrl` or `environmentId`, a duplicate alias, or
an alias containing `;`), nothing is written and the command exits 2.

## Cluster Contexts

A context without an `env-id` is a cluster context. It reaches the cluster's
Cluster Management API, read-only, for what belongs to the cluster rather than
to one environment: its environments, nodes, license and backup configuration.

```bash
# Create one. --host is the cluster's own address, the one the Cluster
# Management Console uses: not a node address on port 9999, which
# environment contexts often use, and without /e/<env-id>.
dtmgd ctx set prod-cluster --host https://managed.company.com --token-ref prod-cluster-token

# Store a Cluster API token: created in the Cluster Management Console
# (Settings > API tokens), with the ServiceProviderAPI scope. An
# environment API token does not work here.
dtmgd config set-credentials prod-cluster-token

dtmgd get environments --context prod-cluster
dtmgd get environments --state enabled --consumption --context prod-cluster
dtmgd describe environment <environment-id> --context prod-cluster
dtmgd get nodes --context prod-cluster
dtmgd describe cluster --context prod-cluster
dtmgd describe license --context prod-cluster
dtmgd get license-usage --context prod-cluster
dtmgd describe backup --context prod-cluster
dtmgd doctor -c prod-cluster
```

- **The two kinds don't mix.** A cluster command run against an environment
  context, or an environment command run against a cluster context, fails
  with exit code 2 before sending anything, and names the configured contexts
  of the right kind. dtmgd never pairs contexts by host.
- **No fan-out.** Cluster commands take one context; `--env` with several
  names or `ALL_ENVIRONMENTS` is an error. `ALL_ENVIRONMENTS` never includes
  cluster contexts.
- **Read-only.** dtmgd sends only `GET` requests to the Cluster API.
- **Backups:** `describe backup` shows the configuration only. The API has no
  last-backup time and no backup health.
- **License:** `describe license` and `get license-usage` need Dynatrace
  Managed 1.326+ and work only with Dynatrace classic licensing. On a cluster
  licensed with the Dynatrace Platform Subscription (DPS) they say so, and so
  does `get environments --consumption`. The license key is masked in every
  output format.
- `dtmgd commands` marks cluster commands with `"cluster_scoped": true`.

## Multi-Environment Queries

Query one, several, or all configured environments in a single command.
Requests fan out in parallel and results are merged.

Context names (`prod`, `staging` below) are just user-defined aliases —
substitute whatever names you used when configuring your contexts.

```bash
# Query all configured environments
dtmgd get problems --env ALL_ENVIRONMENTS -o json

# Query specific environments (semicolon-separated list of your context names)
dtmgd get problems --env "prod;staging" -o json

# You can list any number of contexts:
dtmgd get problems --env "eu-prod;us-prod;ap-prod;dev" -o json

# One context (-c, or --env with one name) → unwrapped data
# ALL_ENVIRONMENTS or a ';' list            → { "<context-1>": {...}, "<context-2>": {...} }
```

With `--env ALL_ENVIRONMENTS` or a `;`-separated list, `-o json`, `-o yaml` and
agent output are one map keyed by context name, even when it selects only one
context, so a script can parse it the same way whatever the config holds. An
environment that failed holds
`{"error": "..."}` in place of its result. A table prints a `== <name> ==`
section per environment, with a failed environment's error on stderr. Either
way every environment that answered is printed, and the call still exits
non-zero if any failed (see [Exit Codes](output.md#exit-codes)).

The `--env` flag works with every `get`, `describe` and `query` command except
the cluster commands, which run against one cluster context. `ALL_ENVIRONMENTS`
selects environment contexts only, and naming a cluster context in a `;` list
is an error. `config`,
`ctx`, `skills`, `doctor` and `commands` never fan out over it. A single
`--env` name still acts like `-c` where they show the current context
(`config view`, `config current-context`, `ctx`), for that run only; `doctor`
takes `-c` to check a single context.

Environments are queried in parallel, up to 10 at a time. Since a Managed
cluster commonly hosts 20-50 environment IDs, firing them all at once draws
`429 Too Many Requests`, and every request then retries on the same schedule —
so each retry wave is as large as the original burst, and no output appears
until the slowest one finishes. Raise the cap with `--concurrency` if your
cluster is dedicated or you have only a few environments:

```bash
dtmgd get problems --env ALL_ENVIRONMENTS --concurrency 25
```

## Configuration File Format

Context names (`production`, `staging`, etc.) and token names (`prod-token`)
are user-chosen aliases. There are no reserved names — pick whatever fits
your environments. You can register as many contexts as you need.

```yaml
apiVersion: dtmgd.io/v1
kind: Config
current-context: production       # any context name from the list below
contexts:
  - name: production              # ← user-defined alias
    context:
      host: https://managed.company.com   # Managed cluster base URL
      env-id: abc12345                     # Environment ID
      token-ref: prod-token                # reference into tokens list
      description: Production environment
      http-proxy: http://proxy.corp:8080   # optional HTTP proxy
      https-proxy: http://proxy.corp:8080  # optional HTTPS proxy
  - name: staging                 # ← another user-defined alias; add as many as needed
    context:
      host: https://managed-staging.company.com
      env-id: def67890
      token-ref: staging-token
  - name: eu-prod                 # ← any name you want
    context:
      host: https://managed-eu.company.com
      env-id: xyz12345
      token-ref: eu-prod-token
  - name: prod-cluster            # ← no env-id: a cluster context
    context:
      host: https://managed.company.com    # the cluster's own address
      token-ref: prod-cluster-token        # a ServiceProviderAPI token
tokens:
  - name: prod-token              # ← matches token-ref above
    token: ""   # empty when stored in OS keyring
  - name: staging-token
    token: ""
  - name: eu-prod-token
    token: ""
  - name: prod-cluster-token
    token: ""
preferences:
  output: table
```

### Environment variables in the config

Values can reference environment variables with `${VAR_NAME}`:

```yaml
host: ${DT_MANAGED_HOST}
env-id: ${DT_ENV_ID}
token: ${DT_API_TOKEN}
```

Only the braced `${VAR_NAME}` form is expanded. A bare `$VAR_NAME` is treated as
literal text, so dollar signs in values — a proxy password, for example — are left
alone.

References are resolved when a value is used, not when the file is read. Config
commands (`use-context`, `set-context`, `set-credentials`, `delete-context`) preserve
them, so `${DT_API_TOKEN}` stays in the file rather than being replaced by the token
it currently resolves to. `dtmgd config view` shows the reference for the same reason —
a `${VAR}` reference names a secret without being one, so it is printed as written,
while a literal token value is masked unless you pass `--show-tokens`.

If a referenced variable is unset, the command that needs it fails and names it. Other
contexts are unaffected, so a multi-context config still works when you hold
credentials for only one of them.

### Which config file is used

`dtmgd` picks the first of these that exists:

1. the path given to `--config`
2. the path in the `DTMGD_CONFIG` environment variable
3. a `.dtmgd.yaml` found by searching the working directory and then each
   parent directory in turn, up to the filesystem root
4. the global config: `~/.config/dtmgd/config` on Linux,
   `~/Library/Application Support/dtmgd/config` on macOS,
   `%LOCALAPPDATA%\dtmgd\config` on Windows, or
   `$XDG_CONFIG_HOME/dtmgd/config` when that variable is set

Step 3 is worth knowing about on shared machines. Anyone who can write to a
directory above the one you work in — `/tmp`, a group-writable project root, a
CI workspace — can leave a `.dtmgd.yaml` there, and it will take precedence
over your global config on your next `dtmgd` run. Because such a file sets
`host`, it also chooses where your API token is sent.

When the config in use comes from a parent directory rather than the one you
are in, `dtmgd` says so on stderr:

```
⚠ Using config /tmp/.dtmgd.yaml — found in a parent directory, not
  /home/you/.config/dtmgd/config. Check it before trusting this run.
```

A `.dtmgd.yaml` in your working directory is the intended project-local
workflow (`dtmgd config init` creates one) and is not announced; run with `-v`
to print the active config path whatever its source. Use `--config` or
`DTMGD_CONFIG` to pin the file explicitly and bypass the search.
