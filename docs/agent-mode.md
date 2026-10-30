# AI agent mode

How dtmgd behaves when an AI agent runs it, and how to install the dtmgd skill for your agent.

## AI Agent Mode

When running inside an AI agent (Claude Code, Cursor, GitHub Copilot, Codex, Gemini
CLI, Kiro, and others), dtmgd auto-detects the environment and wraps all output in a
structured JSON envelope. The two exceptions are `commands`, whose catalog is
JSON already, and `completion`, which prints a shell script. A command that
prints one line for a person (`version`, `config current-context`,
`ctx current`) puts an object in `result`, for example
`{"version": "0.5.0", "commit": "…", "date": "…"}`. The envelope is the one dtctl uses (its `sdk/agentmode`
package), so an agent that drives both CLIs parses one shape:

```json
{
  "ok": true,
  "result": { "problems": [...], "totalCount": 5 },
  "context": { "verb": "get", "resource": "problems", "total": 5 }
}
```

`result` is the API's own response. In agent mode a list returns at most 50
items when `--limit` is not given; `context.total` says how many exist (when
the API reports it) and `context.has_more` is `true` when there are more than
`result` holds. Pass `--limit` to get more or fewer.

Errors are also wrapped, with a classified `code`, the HTTP `status_code` when
there is one, and what to do next in `suggestions`:

```json
{
  "ok": false,
  "result": null,
  "error": {
    "code": "auth_required",
    "message": "API error 401: Token Authentication failed",
    "status_code": 401,
    "suggestions": ["Authentication failed. Check your API token and ensure it hasn't expired. …"]
  }
}
```

Error codes: `auth_required` (401), `permission_denied` (403), `not_found` (404),
`bad_request` (400), `rate_limited` (429), `server_error` (5xx), `config_error`,
`timeout`, `partial_failure` (some environments of an `--env` call failed),
`validation_error` and `unknown_command` (usage errors), and `error` for
anything else.

API error messages carry the status code and, when the cluster returns a
recognised error envelope, its human-readable message — but not the rest of the
response body. Run with `-vv` to see the full response.

Force it on with `-A` / `--agent`, or disable auto-detection with `--no-agent`.
When agent mode was switched on by auto-detection, dtmgd prints one line to
stderr naming the agent it detected (`Agent mode is on (detected claude-code). …`). An explicit `-A` or `--no-agent` suppresses
that line, so keep stderr separate (no `2>&1`) when piping envelopes to `jq`,
or pass `-A`.

### Install the dtmgd Skill for Your AI Agent

`dtmgd` ships with an embedded skill (`SKILL.md` and the `references/` it
points to) that teaches your AI coding assistant how to use the CLI
effectively — including command patterns, output formats, and gotchas
specific to Dynatrace Managed Classic.

To update an installed skill after upgrading dtmgd, run the install again
with `--force` (for example `dtmgd skills install --for claude --force`). It
rewrites the skill's own files, adds new ones such as `references/`, and keeps
any files of your own in the skill's directory.

Install it with one command — the target agent is auto-detected from your
environment, or you can pick it explicitly. Outside an interactive terminal
(CI, scripts, an agent's own shell tool) auto-detection is not used and
`--for` is required, so an environment variable alone never decides where
files are written:

```bash
# Auto-detect the current AI agent and install (interactive terminal only)
dtmgd skills install

# Install for a specific agent (claude, copilot, cursor, junie, kiro, opencode)
dtmgd skills install --for claude

# Install user-wide (instead of project-local)
dtmgd skills install --for claude --global

# Install to the cross-client shared directory (agentskills.io convention)
# — picked up automatically by any compatible agent
dtmgd skills install --cross-client

# List all supported agents and their install paths
dtmgd skills install --list

# Check what's installed across all agents
dtmgd skills status

# Remove the skill
dtmgd skills uninstall --for claude
```

Skill files are installed as a directory following the
[agentskills.io](https://agentskills.io) standard:
`<agent-config>/skills/dtmgd/SKILL.md`. Use `--force` to overwrite an
existing installation.
