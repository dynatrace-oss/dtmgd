# dtmgd contexts and multi-environment queries

Read this when setting up or switching contexts, importing a Dynatrace Managed MCP server config, or querying several environments at once.

## Context Management

Contexts are named environments. **Names are arbitrary, user-defined aliases**
— `prod`, `staging`, `dev`, `acme-tenant1`, anything. The examples below use
`prod` purely as a placeholder.

```bash
# List all configured environments (shows the user's actual context names)
dtmgd ctx
dtmgd config get-contexts

# Show or switch context
dtmgd ctx current
dtmgd ctx <context-name>
```

In agent mode (and with `-o json`), `dtmgd ctx` returns one object per context,
keyed as in the config file, with `current` as a boolean:
`{"current": true, "name": "prod", "scope": "environment", "host": "…", "env-id": "…", "token-ref": "prod-token"}`
(a cluster context has no `env-id`). `dtmgd ctx current` returns
`{"current-context": "prod"}`, and `dtmgd config view` the whole config with the
file's keys (`current-context`, `contexts[].context.env-id`, …) and token values
masked.

```bash
# Configure a new environment (pick any name you like for <context-name>)
dtmgd config set-context <context-name> \
  --host https://managed.company.com \
  --env-id abc12345 \
  --token-ref <context-name>-token

# Store the token. Prompts for it with echo disabled; use --token-stdin when
# scripting. Avoid --token: it leaves the token in the process list and shell
# history.
dtmgd config set-credentials <context-name>-token
echo "$DT_API_TOKEN" | dtmgd config set-credentials <context-name>-token --token-stdin

# Create contexts from a Dynatrace Managed MCP server config (DT_CONFIG_FILE's
# YAML/JSON file, or DT_ENVIRONMENT_CONFIGS with --from-env). Each alias becomes
# a context; ${VAR} tokens stay references. Preview with --dry-run; existing
# contexts are skipped unless --overwrite.
dtmgd config import dt-config.yaml --dry-run
dtmgd config import dt-config.yaml

# Delete context
dtmgd ctx delete <name>
```

Proxy support: add `http-proxy` and/or `https-proxy` to the context in the config file for environments behind corporate proxies.

## Multi-Environment Queries

Query multiple environments in parallel with `--env`. Environment names are
**user-defined aliases** (any names the user chose during setup) — `prod` and
`staging` below are illustrative only.

```bash
# All configured environments
dtmgd get problems --env ALL_ENVIRONMENTS -o json

# Specific environments (semicolon-separated; substitute the user's actual context names)
dtmgd get problems --env "prod;staging" -o json

# One context (-c, or --env with one name) → unwrapped data
# ALL_ENVIRONMENTS or a ';' list → { "<context-name-1>": {...}, "<context-name-2>": {...} }
```

With `--env ALL_ENVIRONMENTS` or a `;` list, json/yaml/agent output is one map
keyed by context name, even when only one context is selected; a failed environment holds `{"error": "..."}` instead of its result. A
table prints a `== <name> ==` section per environment. Every environment that
answered is printed, and the exit code is non-zero if any failed.

Works with every `get`, `describe` and `query` command except the cluster
commands (see [cluster-api.md](cluster-api.md)), which run
against one cluster context. `ALL_ENVIRONMENTS` selects environment contexts
only. `config`, `ctx`, `skills`,
`doctor` and `commands` never fan out over `--env`; a single `--env` name acts
like `-c` where they show the current context. Use `dtmgd ctx` first to
discover which context names are available.
