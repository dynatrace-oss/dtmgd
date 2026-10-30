## dtmgd skills install

Install skill file for an AI coding assistant

### Synopsis

Install the dtmgd skill directory for the specified AI coding assistant.

Skills are installed as directories following the agentskills.io standard:
```
  <agent-config>/skills/dtmgd/SKILL.md
```

If no agent is specified with --for, the command auto-detects the current
agent from environment variables. When stdin is not a terminal (scripts, CI,
an agent's shell tool), --for is required. Use --global to install to the user-wide
location instead of the project directory.

Use --cross-client to install to the shared .agents/skills/ directory defined
by the agentskills.io convention. Skills installed there are automatically
discovered by any compatible agent without per-agent installation.

```
dtmgd skills install [flags]
```

### Options

```
      --cross-client   install to the shared .agents/skills/ directory (agentskills.io convention)
      --for string     install for a specific agent (claude, copilot, cursor, junie, kiro, opencode)
      --force          overwrite the skill's own files; anything else in the directory is left alone
      --global         install to user-wide location instead of project directory
  -h, --help           help for install
      --list           list all supported agents and exit
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table, csv and tsv output
      --concurrency int   maximum environments queried at once with --env (default 10)
      --config string     config file (default: ~/.config/dtmgd/config on Linux, ~/Library/Application Support/dtmgd/config on macOS, %LOCALAPPDATA%\dtmgd\config on Windows, or $XDG_CONFIG_HOME/dtmgd/config when set)
  -c, --context string    context to use (overrides current-context)
  -e, --env string        environment(s): context name, "prod;staging", or ALL_ENVIRONMENTS
      --max-pages int     maximum number of pages to fetch (0 = all pages)
      --no-agent          disable auto-detected agent mode
  -o, --output string     output format: table|wide|json|yaml|csv|tsv (default: table)
      --plain             plain output: no symbols or screen control (automatic when stdout is not a terminal or NO_COLOR is set)
  -v, --verbose count     verbosity level (-v for summary, -vv for full request/response)
```

### SEE ALSO

* [dtmgd skills](dtmgd_skills.md)	 - Manage AI coding assistant skill files

