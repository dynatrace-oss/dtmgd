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

`-o csv` and `-o tsv` print the table's rows as comma- or tab-separated
values with a header line, for spreadsheets and scripts:

- **Columns:** every column, wide ones included and never shortened;
  `--columns` narrows them. A `--columns` value that matches no column is a
  usage error listing the columns there are.
- **Empty results:** the header line alone.
- **Placeholders:** the table's `—` and `-` (none, unknown) are empty cells.
- **`--env` with several environments:** one table led by a `CONTEXT` column.
  A truncated environment gets its `Showing n of m` note on stderr.
- **Quoting:** fields are quoted as in RFC 4180, in tsv too, so a field with
  the separator, a quote, a line break or a leading or trailing space is in
  `"…"` with quotes doubled. In csv a field with `;` is quoted as well, for
  spreadsheets whose list separator is `;`.
- **Spreadsheet formulas:** a cell starting with `=`, `+`, `@`, a tab or a
  carriage return, or with `-` unless it is a plain number, gets a leading `'`.
  Log content and entity names can carry text an attacker chose, and a
  negative problem ID would otherwise show as `#NAME?`. Scripts that need the
  exact values should use `-o json`.
- **Commands without rows:** the ones that print a summary or the API object
  (`describe` commands, `query metrics`) refuse csv and tsv with exit 2; use
  `-o json` or `-o yaml`. `query logs` gives a timestamp, status and content
  per record, and `query log-counts` a row per process group.
- **`--watch`** prints the header line again with every refresh.
- **Agent mode:** the agent envelope wins over any `-o`; pass `--no-agent`.

On a terminal, tables have bold headers and a colored status column (`get
problems`, `get slos`, `doctor`: green for ok, success and resolved, yellow
for warnings, red for open and failed), and the stderr symbols are colored
(`✗` red, `⚠` yellow, `✓` green). Stderr is colored only when it is a
terminal too. csv, tsv, json, yaml and the agent envelope never contain
color codes.

`--plain` turns colors off, replaces the stderr symbols (`⚠ ✗ 💡`) with words
(`Warning:`, `Error:`, `Hint:`) and stops `--watch` from clearing the screen.
It is on automatically when stdout is not a terminal or `NO_COLOR` is set,
unless `FORCE_COLOR` is set.

## Global Flags

| Flag | Description |
|---|---|
| `--config <file>` | Config file path (default: the global config, see [Which config file is used](configuration.md#which-config-file-is-used)) |
| `-c, --context <name>` | Override current context |
| `-e, --env <spec>` | Target environment(s): context name, `"prod;staging"`, or `ALL_ENVIRONMENTS` |
| `-o, --output <format>` | Output format: `table` (default), `wide`, `json`, `yaml` (or `yml`), `csv`, `tsv`; any other value is a usage error (exit 2) before any request is made |
| `-A, --agent` | Force AI agent envelope output (`{ok, result, context}`) |
| `--no-agent` | Disable auto-detected agent mode |
| `--plain` | No colors, words instead of symbols on stderr, no screen control (automatic when stdout is not a terminal) |
| `--max-pages <n>` | Maximum pages to fetch (0 = all, default). Pagination is automatic. |
| `--concurrency <n>` | Maximum environments queried at once with `--env` (default: `10`) |
| `--columns <cols>` | Comma-separated columns to show in table, csv and tsv output |
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
`nextPageKey`, and a table, csv or tsv is followed by a `Showing n of m` note
on stderr.
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
| `NO_COLOR` | Plain output: no colors, symbols or screen control (same as `--plain`). |
| `FORCE_COLOR` | Keep colors and symbols even when stdout or stderr is not a terminal. |

Flags always win over environment variables, which win over the config file.
