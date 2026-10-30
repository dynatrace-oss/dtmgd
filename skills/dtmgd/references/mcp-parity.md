# Feature parity with the Dynatrace Managed MCP Server

dtmgd covers every tool of the [Dynatrace Managed MCP Server](https://github.com/dynatrace-oss/dynatrace-managed-mcp)
(compared with its 2.0.0 release, which prefixes every tool name with
`dynatrace_managed_`; the table leaves the prefix out). The two send mostly the
same requests; the table lists where they differ. "Undocumented" means the parameter is not in the
endpoint's published API specification; whether a cluster honours it has not
been verified.

| MCP tool | dtmgd command | Differences |
|---|---|---|
| `get_environments_info` | `dtmgd doctor` | `doctor` checks each context live (token, cluster version ≥ 1.328.0, falling back to a metrics call) and exits 1 on failure. The MCP server reports the check it ran at startup and a fixed scope list. |
| `list_available_metrics` | `dtmgd get metrics [--search] [--entity]` | Both send `--entity` as `entitySelector`, which `/metrics` does not document. |
| `get_metric_details` | `dtmgd describe metric <id>` | Same request. |
| `query_metrics_data` | `dtmgd query metrics --metric <sel> --from <t> [--to] [--resolution] [--entity]` | The MCP server defaults to `resolution=Inf` (one value per series) and requires `to`. dtmgd sends no resolution unless `--resolution` is given (the API default: a time series), and `--to` defaults to now. |
| `query_logs` | `dtmgd query logs --query <text> --from <t> [--to] [--sort] [--limit] [--entity]` | Both sort newest first (`-timestamp`) and return one slice of at most 1000 records (100 by default; 50 for dtmgd in agent mode). dtmgd clamps a larger `--limit` to 1000 where the MCP server rejects it, and `--to` defaults to now. dtmgd's `--entity` sends `entitySelector`, which `/logs/search` does not document. |
| `list_events` | `dtmgd get events --from <t> [--to] [--type] [--entity] [--watch]` | Both send `--type` as `eventType`, which `/events` does not document (it documents `eventSelector`). dtmgd adds `--watch`. |
| `get_event_details` | `dtmgd describe event <id>` | Same request. |
| `list_entity_types` | `dtmgd get entity-types` | dtmgd's json output has the full type schemas; the MCP server lists type names. |
| `get_entity_type_details` | `dtmgd describe entity-type <type>` | Same request. |
| `discover_entities` | `dtmgd get entities --selector <sel> [--mz] [--from] [--to] [--sort]` | The MCP server returns 10 entities unless asked for more (at most 50). Both send `--mz` as `mzSelector`, which `/entities` does not document. |
| `get_entity_details` | `dtmgd describe entity <id>` | dtmgd asks for properties, relationships, tags and management zones explicitly (`fields`); the MCP server relies on the API's default fields. |
| `get_entity_relationships` | `dtmgd describe entity-relations <id>` | dtmgd asks for the relationship fields explicitly. |
| `list_problems` | `dtmgd get problems [--status] [--impact] [--selector] [--entity] [--sort] [--watch]` | The MCP server sends the impact level as a top-level `impactLevel`, which `/problems` does not document; dtmgd puts it in `problemSelector`. dtmgd adds `--selector` and `--watch`, and sends `fields=+affectedEntities,+managementZones`, which `/problems` does not document. Both default to the last 24 hours. |
| `get_problem_details` | `dtmgd describe problem <id>` | Same request. A negative problem ID goes after `--`. |
| `list_security_problems` | `dtmgd get security-problems [--risk] [--status] [--selector] [--from] [--to] [--sort]` | The MCP server sends `riskLevel` and `entitySelector` as top-level parameters, which `/securityProblems` does not document. dtmgd puts the risk level in `securityProblemSelector`, and scopes to entities with selector criteria (`--selector 'relatedHostIds("HOST-…")'`). dtmgd requests the risk assessment and sorts by risk score by default. |
| `get_security_problem_details` | `dtmgd describe security-problem <id \| S-NNN>` | dtmgd also accepts the display ID (`S-1234`), and requests the details fields: risk assessment, CVE description, vulnerable components, code-level details, affected and exposed entities. The MCP server requests none. |
| `list_slos` | `dtmgd get slos [--selector] [--timeframe] [--from] [--to] [--evaluate] [--sort] [--enabled] [--show-global] [--demo]` | The MCP server never sends `showGlobalSlos=false` or `demo=false`; dtmgd sends either flag whenever it is given. dtmgd turns `--from`/`--to` into `timeFrame=GTF`, without which the API ignores them, and asks for 25 SLOs per page with `--evaluate`, the API's limit. |
| `get_slo_details` | `dtmgd describe slo <id> [--timeframe] [--from] [--to]` | Same request, including the GTF rule. |

Differences across all tools:

- **Result counts.** The MCP server returns one page per call, capped per tool
  (for example 50 problems, 100 security problems). dtmgd fetches every page
  unless `--limit` or `--max-pages` stops it; in agent mode lists default to 50,
  and `context.has_more` says when there are more.
- **Several environments.** Both accept `"a;b"` and `ALL_ENVIRONMENTS`. The MCP
  server queries the environments one after another and fails the whole call
  if one fails. dtmgd queries them in parallel, prints every environment that
  answered (keyed by context name), and exits non-zero if any failed.
- **UI links.** The MCP server adds a Dynatrace UI link when one environment
  answered. dtmgd puts the links in agent output's `context.links`, keyed by
  context name for several environments, and `describe` commands print
  `Open in Dynatrace:` on stderr.
- **Only in the MCP server:** HTTP transport with per-request tokens, rate
  limiting, usage telemetry, and output trimmed for model context (at most 11
  tags or dimensions, 50 data points per series).
- **Only in dtmgd:** the Cluster Management API commands (`get environments`,
  `describe environment`, `get nodes`, `describe cluster`, `describe license`,
  `get license-usage`, `describe backup`; the MCP server has no cluster
  tools), `query log-counts` (log counts per process group and
  level), the `--selector` DSL on problems and security problems, `--watch`,
  display IDs for security problems, paging past the first page (with
  `--max-pages` to stop it), table, wide, json and yaml output, `commands`,
  `config import` (creates contexts from an MCP server config), and shell
  completion.
