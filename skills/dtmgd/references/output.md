# dtmgd output, paging and time formats

Read this when parsing dtmgd output in detail, paging through large results, watching for changes, choosing table columns, or writing time ranges.

## Output Formats

Use `-o json` for AI/script consumption. Default is human-friendly `table`.

```bash
-o table    # default; human-readable
-o wide     # table with extra columns (timestamps, root cause, etc.)
-o json     # indented JSON — best for parsing and piping to jq
-o yaml     # YAML output (-o yml also works)
```

Any other `-o` value is a usage error (exit 2) before any request is made.

- **stdout is data only.** Notes for a person ("No problems found", "Showing
  10 of 15 problems", warnings, errors, hints) go to stderr, so piping stdout to
  `jq` is safe.
- **json, yaml and the envelope's `result` are the API's own response**, with
  its camelCase keys (`problemId`, `totalCount`, `nextPageKey`), not the table
  columns. This holds for every `get`, `describe` and `query` command except
  `query log-counts` (see [Logs](recipes.md#logs)). Cluster commands add three things:
  `get nodes` puts each node's configuration under `configuration`,
  `describe cluster` puts the node list under `nodes`, and `describe license`
  masks `licenseKey`.
- **An empty result prints the object with an empty list** (`"problems": []`),
  not nothing.
- **Tables**: `describe problem`, `describe slo`, `describe security-problem`,
  `describe cluster`, `describe license` and `describe backup` print a summary;
  the other describe commands print the API object as YAML.
- `--plain` swaps stderr symbols for words (`Warning:`, `Error:`, `Hint:`). It
  is automatic when stdout is not a terminal or `NO_COLOR` is set (unless
  `FORCE_COLOR` is set).

## AI Agent Envelope Mode

dtmgd auto-detects AI agent environments (Claude Code, Cursor, GitHub Copilot,
Codex, Gemini CLI, Amazon Q, Kiro, Junie, OpenCode, and others) and wraps all
output in dtctl's structured envelope:

```json
{
  "ok": true,
  "result": { "problems": [...], "totalCount": 5 },
  "context": { "verb": "get", "resource": "problems", "total": 5 }
}
```

**In agent mode a list returns at most 50 items unless you pass `--limit`.**
`context.total` is how many exist (when the API reports it) and
`context.has_more` is `true` when `result` holds fewer — pass a larger
`--limit` or narrow the filters.

Errors are also wrapped, with a classified code and next steps:
`{"ok": false, "result": null, "error": {"code": "permission_denied", "message": "...", "status_code": 403, "suggestions": ["..."]}}`.
Codes: `auth_required`, `permission_denied`, `not_found`, `bad_request`,
`rate_limited`, `server_error`, `config_error`, `timeout`, `partial_failure`,
`validation_error` and `unknown_command` (usage errors), `error`.

Force with `-A`/`--agent`, disable with `--no-agent`.

When agent mode is auto-detected, dtmgd also prints one line to **stderr**
(`Agent mode is on (detected claude-code). …`). Don't merge stderr into
stdout (`2>&1`) before parsing JSON — or pass `-A` explicitly, which
suppresses the line.

To install this skill from an agent's shell (no terminal), name the agent:
`dtmgd skills install --for claude`.

**Tip for AI agents**: prefer `-o json` or `-A` when parsing results:

```bash
dtmgd get problems --status OPEN -o json
dtmgd describe problem <uuid> -A
```

## Pagination

All list commands automatically follow `nextPageKey` to fetch complete results.
`--limit <n>` returns at most `n` items in every format and stops paging once
it has them; `--max-pages` limits page count. A truncated json/yaml result
keeps the API's `nextPageKey`. In agent mode, lists default to 50 items when
`--limit` is not given.

## Watch Mode

Monitor resources in real-time (`get problems` and `get events` only):

```bash
dtmgd get problems --status OPEN --watch
dtmgd get events --from now-1h --watch --watch-interval 10s
```

## Column Filtering

Show only specific columns in table output:

```bash
dtmgd get problems --columns "PROBLEM-ID,TITLE,STATUS"
dtmgd get entities --selector 'type(HOST)' --columns "ENTITY-ID,DISPLAY-NAME"
```

## Time Format Reference

| Format   | Example                    |
|----------|----------------------------|
| Relative | `now-1h`, `now-24h`, `now-7d`, `now-30m` |
| ISO 8601 | `2024-01-01T10:00:00Z`     |
| Unix ms  | `1640995200000`            |
