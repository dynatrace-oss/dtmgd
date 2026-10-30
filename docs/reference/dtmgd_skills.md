## dtmgd skills

Manage AI coding assistant skill files

### Synopsis

Manage dtmgd skill files for AI coding assistants.

Skill files teach your AI assistant how to use dtmgd effectively. They follow
the agentskills.io open standard for skill installation.

Supported agents: claude, copilot, cursor, junie, kiro, opencode.

Use --cross-client to install to the shared .agents/skills/ directory, which
is automatically discovered by any agentskills.io-compatible agent.

```
dtmgd skills [flags]
```

### Examples

```
  # Auto-detect agent and install
  dtmgd skills install

  # Install for a specific agent
  dtmgd skills install --for claude

  # Install to cross-client shared directory
  dtmgd skills install --cross-client

  # Install user-wide
  dtmgd skills install --for claude --global

  # Check what's installed
  dtmgd skills status

  # List all supported agents
  dtmgd skills install --list
```

### Options

```
  -h, --help   help for skills
```

### Options inherited from parent commands

```
  -A, --agent             force agent envelope output mode
      --columns string    comma-separated list of columns to show in table output
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

* [dtmgd](dtmgd.md)	 - Dynatrace Managed CLI
* [dtmgd skills install](dtmgd_skills_install.md)	 - Install skill file for an AI coding assistant
* [dtmgd skills status](dtmgd_skills_status.md)	 - Show installation status of skill files
* [dtmgd skills uninstall](dtmgd_skills_uninstall.md)	 - Remove installed skill files

