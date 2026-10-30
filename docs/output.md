# Output, exit codes, paging and time formats

What dtmgd prints in each format, its exit codes, how it pages through results, `--watch`, and the time formats it accepts. The [command reference](reference/dtmgd.md) is the authoritative list of flags.

## Output

stdout carries only data. Everything meant for a person — "No problems found",
"Showing 10 of 15 problems", warnings, errors and hints — goes to stderr, so
`dtmgd get problems -o json | jq` never sees it.

For every `get`, `describe` and `query` command except `query log-counts`,
`-o json`, `-o yaml` and the agent envelope's `result` are the API's own
response (with three additions: `get nodes` puts each node's configuration
under `configuration`, `describe cluster` puts the node list under `nodes`,
and `describe license` masks `licenseKey`), with its camelCase keys (`problemId`, `totalCount`, `nextPageKey`),
not the table's columns. An empty result prints that object with an empty list
(`"problems": []`) rather than nothing.

`--plain` replaces the stderr symbols (`⚠ ✗ 💡`) with words (`Warning:`,
`Error:`, `Hint:`) and stops `--watch` from clearing the screen. It is on
automatically when stdout is not a terminal or `NO_COLOR` is set, unless
`FORCE_COLOR` is set.

## Global Flags

| Flag | Description |
|---|---|
| `--config <file>` | Config file path (default: the global config, see [Which config file is used](configuration.md#which-config-file-is-used)) |
| `-c, --context <name>` | Override current context |
| `-e, --env <spec>` | Target environment(s): context name, `"prod;staging"`, or `ALL_ENVIRONMENTS` |
| `-o, --output <format>` | Output format: `table` (default), `wide`, `json`, `yaml` (or `yml`); any other value is a usage error (exit 2) before any request is made |
| `-A, --agent` | Force AI agent envelope output (`{ok, result, context}`) |
| `--no-agent` | Disable auto-detected agent mode |
| `--plain` | Words instead of symbols on stderr, no screen control (automatic when stdout is not a terminal) |
| `--max-pages <n>` | Maximum pages to fetch (0 = all, default). Pagination is automatic. |
| `--concurrency <n>` | Maximum environments queried at once with `--env` (default: `10`) |
| `--columns <cols>` | Comma-separated columns to show in table output |
| `-v` | Verbose: show HTTP request/response summaries |
| `-vv` | Extra verbose: full headers and body |

`--watch` and `--watch-interval` are flags of `get problems` and `get events`
only; see [Watch Mode](#watch-mode).

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Any other error |
| `2` | Usage error: unknown command, resource or flag, a missing argument or required flag |
| `3` | Authentication failed (HTTP 401) |
| `4` | Not found (HTTP 404) |
| `5` | Permission denied (HTTP 403) |

The codes are the ones `dtctl` uses. A multi-environment (`--env`) call exits
non-zero if any environment fails: with the shared code when every environment
failed the same way (all 403 → `5`), otherwise `1`. The environments that
answered are still printed.

## Pagination

All list commands (`get problems`, `get entities`, etc.) automatically follow
`nextPageKey` to fetch all pages. `--limit <n>` returns at most `n` items in
every output format and stops paging as soon as it has them, so a small limit
costs one or two requests. Use `--max-pages` to limit the number of pages
fetched. When the result is truncated, `-o json` and `-o yaml` keep the API's
`nextPageKey`, and a table is followed by a `Showing n of m` note on stderr.
In agent mode, lists return at most 50 items unless you pass `--limit` (see
[AI Agent Mode](agent-mode.md#ai-agent-mode)).

## Watch Mode

Monitor resources in real-time with `--watch`, a flag of `get problems` and
`get events` only:

```bash
dtmgd get problems --status OPEN --watch
dtmgd get problems --watch --watch-interval 30s
dtmgd get events --from now-1h --watch --watch-interval 10s
```

Press `Ctrl+C` to stop. Default interval is 5 seconds.

## Time Formats

Both relative and absolute times are accepted:

| Format | Example |
|---|---|
| Relative | `now-1h`, `now-24h`, `now-7d`, `now-30m` |
| ISO 8601 | `2024-01-01T10:00:00Z` |
| Unix ms | `1640995200000` |

### Environment variables

| Variable | Effect |
|---|---|
| `DTMGD_CONFIG` | Config file to use when `--config` is not given. |
| `DTMGD_CONTEXT` | Context to use when neither `--context` nor a single `--env` name is given; overrides the file's `current-context`. |
| `NO_COLOR` | Plain output: no symbols or screen control (same as `--plain`). |
| `FORCE_COLOR` | Keep symbols even when stdout is not a terminal. |

Flags always win over environment variables, which win over the config file.
