package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
	"github.com/dynatrace-oss/dtmgd/pkg/skills"
)

// skillsCmd is the parent command for AI assistant skill management.
var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage AI coding assistant skill files",
	Long: `Manage dtmgd skill files for AI coding assistants.

Skill files teach your AI assistant how to use dtmgd effectively. They follow
the agentskills.io open standard for skill installation.

Supported agents: claude, copilot, cursor, junie, kiro, opencode.

Use --cross-client to install to the shared .agents/skills/ directory, which
is automatically discovered by any agentskills.io-compatible agent.`,
	Example: `  # Auto-detect agent and install
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
  dtmgd skills install --list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install skill file for an AI coding assistant",
	Long: `Install the dtmgd skill directory for the specified AI coding assistant.

Skills are installed as directories following the agentskills.io standard:
  <agent-config>/skills/dtmgd/SKILL.md

If no agent is specified with --for, the command auto-detects the current
agent from environment variables. Use --global to install to the user-wide
location instead of the project directory.

Use --cross-client to install to the shared .agents/skills/ directory defined
by the agentskills.io convention. Skills installed there are automatically
discovered by any compatible agent without per-agent installation.`,
	RunE: runSkillsInstall,
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed skill files",
	Long: `Remove dtmgd skill files installed for an AI coding assistant.

If no agent is specified with --for, the command auto-detects the current
agent. Removes skill directories from both project-local and global locations.`,
	RunE: runSkillsUninstall,
}

var skillsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show installation status of skill files",
	Long: `Show the current installation status of dtmgd skill files.

Checks both project-local and global locations for all supported agents,
including the cross-client shared directory (.agents/skills/).`,
	RunE: runSkillsStatus,
}

func init() {
	rootCmd.AddCommand(skillsCmd)
	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsUninstallCmd)
	skillsCmd.AddCommand(skillsStatusCmd)

	skillsInstallCmd.Flags().String("for", "", "install for a specific agent (claude, copilot, cursor, junie, kiro, opencode)")
	skillsInstallCmd.Flags().Bool("cross-client", false, "install to the shared .agents/skills/ directory (agentskills.io convention)")
	skillsInstallCmd.Flags().Bool("global", false, "install to user-wide location instead of project directory")
	skillsInstallCmd.Flags().Bool("force", false, "overwrite the skill's own files; anything else in the directory is left alone")
	skillsInstallCmd.Flags().Bool("list", false, "list all supported agents and exit")

	skillsUninstallCmd.Flags().String("for", "", "uninstall for a specific agent")
	skillsUninstallCmd.Flags().Bool("cross-client", false, "uninstall from the shared .agents/skills/ directory")

	skillsStatusCmd.Flags().String("for", "", "check status for a specific agent (or \"cross-client\")")
}

// skillsAgentEntry is a single agent entry for agent-mode list/status output.
type skillsAgentEntry struct {
	Name           string `json:"name" yaml:"name" table:"NAME"`
	DisplayName    string `json:"display_name" yaml:"display_name" table:"DISPLAY-NAME"`
	ProjectPath    string `json:"project_path" yaml:"project_path" table:"PROJECT-PATH,wide"`
	SupportsGlobal bool   `json:"supports_global" yaml:"supports_global" table:"GLOBAL,wide"`
}

// skillsStatusEntry is a single status row for list/status output.
type skillsStatusEntry struct {
	Agent     string `json:"agent" yaml:"agent" table:"AGENT"`
	Installed bool   `json:"installed" yaml:"installed" table:"INSTALLED"`
	Scope     string `json:"scope,omitempty" yaml:"scope,omitempty" table:"SCOPE"`
	Path      string `json:"path,omitempty" yaml:"path,omitempty" table:"PATH,wide"`
}

func runSkillsInstall(cmd *cobra.Command, _ []string) error {
	if listFlag, _ := cmd.Flags().GetBool("list"); listFlag {
		return runSkillsList()
	}

	forFlag, _ := cmd.Flags().GetString("for")
	crossClient, _ := cmd.Flags().GetBool("cross-client")
	global, _ := cmd.Flags().GetBool("global")
	force, _ := cmd.Flags().GetBool("force")

	if crossClient && forFlag != "" {
		return fmt.Errorf("--cross-client and --for cannot be used together")
	}

	agent, err := chooseAgent(forFlag, crossClient)
	if err != nil {
		return err
	}

	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	result, err := skills.Install(agent, baseDir, global, force)
	if err != nil {
		return err
	}

	scope := "project"
	if result.Global {
		scope = "global"
	}

	if agentMode() {
		return output.NewAgentPrinter("skills").Print(map[string]interface{}{
			"action":    map[bool]string{true: "updated", false: "installed"}[result.Replaced],
			"agent":     result.Agent.Name,
			"path":      result.Path,
			"scope":     scope,
			"preserved": result.Preserved,
		})
	}

	verb := "Installed"
	if result.Replaced {
		verb = "Updated"
	}
	fmt.Printf("✓ %s %s skill: %s (%s)\n", verb, result.Agent.DisplayName, result.Path, scope)
	// Say what was left alone. --force used to delete the whole directory, so
	// anyone who kept notes or extra reference files beside SKILL.md lost them
	// and only found out later.
	if len(result.Preserved) > 0 {
		output.PrintInfo("Left %d file(s) in place that are not part of the skill: %s",
			len(result.Preserved), strings.Join(result.Preserved, ", "))
	}
	return nil
}

func runSkillsUninstall(cmd *cobra.Command, _ []string) error {
	forFlag, _ := cmd.Flags().GetString("for")
	crossClient, _ := cmd.Flags().GetBool("cross-client")

	if crossClient && forFlag != "" {
		return fmt.Errorf("--cross-client and --for cannot be used together")
	}

	agent, err := chooseAgent(forFlag, crossClient)
	if err != nil {
		return err
	}

	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	removed, err := skills.Uninstall(agent, baseDir)
	if err != nil {
		return err
	}

	if agentMode() {
		return output.NewAgentPrinter("skills").Print(map[string]interface{}{
			"agent":   agent.Name,
			"removed": removed,
		})
	}

	if len(removed) == 0 {
		fmt.Printf("No %s skill files found to remove.\n", agent.DisplayName)
		return nil
	}
	for _, p := range removed {
		fmt.Printf("✓ Removed: %s\n", p)
	}
	return nil
}

func runSkillsStatus(cmd *cobra.Command, _ []string) error {
	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	forFlag, _ := cmd.Flags().GetString("for")

	if forFlag != "" {
		agent, ok := skills.FindAgent(forFlag)
		if !ok {
			return unknownAgentError(forFlag)
		}
		result := skills.Status(agent, baseDir)
		entries := []skillsStatusEntry{statusToEntry(result)}
		return printStatusEntries(entries)
	}

	results := skills.StatusAll(baseDir)
	entries := make([]skillsStatusEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, statusToEntry(r))
	}
	return printStatusEntries(entries)
}

func printStatusEntries(entries []skillsStatusEntry) error {
	printer := NewPrinterForResource("skills")
	return printer.PrintList(entries)
}

func statusToEntry(r *skills.StatusResult) skillsStatusEntry {
	entry := skillsStatusEntry{Agent: r.Agent.Name, Installed: r.Installed}
	if r.Installed {
		entry.Path = r.Path
		entry.Scope = "project"
		if r.Global {
			entry.Scope = "global"
		}
	}
	return entry
}

func runSkillsList() error {
	all := skills.AllAgents()
	entries := make([]skillsAgentEntry, 0, len(all)+1)
	entries = append(entries, skillsAgentEntry{
		Name:           skills.CrossClientAgent.Name,
		DisplayName:    skills.CrossClientAgent.DisplayName,
		ProjectPath:    skills.CrossClientAgent.ProjectPath,
		SupportsGlobal: skills.CrossClientAgent.GlobalPath != "",
	})
	for _, a := range all {
		entries = append(entries, skillsAgentEntry{
			Name:           a.Name,
			DisplayName:    a.DisplayName,
			ProjectPath:    a.ProjectPath,
			SupportsGlobal: a.GlobalPath != "",
		})
	}
	printer := NewPrinterForResource("skills")
	return printer.PrintList(entries)
}

// chooseAgent picks the target agent based on flags. If crossClient is true,
// returns the cross-client pseudo-agent. If forFlag is set, looks up by name.
// Otherwise auto-detects from environment variables.
func chooseAgent(forFlag string, crossClient bool) (skills.Agent, error) {
	if crossClient {
		return skills.CrossClientAgent, nil
	}
	if forFlag != "" {
		agent, ok := skills.FindAgent(forFlag)
		if !ok {
			return skills.Agent{}, unknownAgentError(forFlag)
		}
		return agent, nil
	}
	agent, ok := skills.DetectAgent()
	if !ok {
		return skills.Agent{}, fmt.Errorf(
			"no AI agent detected — use --for to specify one: %s",
			strings.Join(skills.SupportedAgents(), ", "),
		)
	}
	return agent, nil
}

func unknownAgentError(name string) error {
	return fmt.Errorf(
		"unknown agent %q\nSupported agents: %s, cross-client",
		name, strings.Join(skills.SupportedAgents(), ", "),
	)
}
