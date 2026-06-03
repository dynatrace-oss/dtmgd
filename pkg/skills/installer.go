// Package skills provides installation and management of AI coding assistant
// skill files for dtmgd. It copies the embedded skill content (SKILL.md and,
// in the future, reference files) to the appropriate agent-specific location
// following the agentskills.io open standard.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	dtmgdskill "github.com/dynatrace-oss/dtmgd/skills/dtmgd"
)

// Agent represents a supported AI coding assistant.
type Agent struct {
	// Name is the canonical identifier (e.g. "claude", "copilot").
	Name string
	// DisplayName is the human-readable name (e.g. "Claude Code").
	DisplayName string
	// ProjectPath is the relative path from the project root for project-local
	// install (e.g. ".claude/skills/dtmgd").
	ProjectPath string
	// GlobalPath is the relative path from the user's home directory for
	// global install. Empty means global install is not supported.
	GlobalPath string
	// EnvVar is the environment variable used to detect this agent.
	EnvVar string
}

// CrossClientAgent is a pseudo-agent representing the cross-client shared
// directory defined by the agentskills.io convention. Skills installed here
// are automatically discovered by any agent that scans ~/.agents/skills/ or
// <project>/.agents/skills/.
var CrossClientAgent = Agent{
	Name:        "cross-client",
	DisplayName: "Cross-Client (agentskills.io)",
	ProjectPath: filepath.Join(".agents", "skills", "dtmgd"),
	GlobalPath:  filepath.Join(".agents", "skills", "dtmgd"),
}

// agents is the registry of all supported AI coding assistants.
// All agents follow the agentskills.io standard: <client>/skills/<name>/SKILL.md
var agents = []Agent{
	{
		Name:        "claude",
		DisplayName: "Claude Code",
		ProjectPath: filepath.Join(".claude", "skills", "dtmgd"),
		GlobalPath:  filepath.Join(".claude", "skills", "dtmgd"),
		EnvVar:      "CLAUDECODE",
	},
	{
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		ProjectPath: filepath.Join(".github", "skills", "dtmgd"),
		GlobalPath:  filepath.Join(".copilot", "skills", "dtmgd"),
		EnvVar:      "GITHUB_COPILOT",
	},
	{
		Name:        "cursor",
		DisplayName: "Cursor",
		ProjectPath: filepath.Join(".cursor", "skills", "dtmgd"),
		EnvVar:      "CURSOR_AGENT",
	},
	{
		Name:        "kiro",
		DisplayName: "Kiro",
		ProjectPath: filepath.Join(".kiro", "skills", "dtmgd"),
		EnvVar:      "KIRO",
	},
	{
		Name:        "junie",
		DisplayName: "Junie",
		ProjectPath: filepath.Join(".junie", "skills", "dtmgd"),
		GlobalPath:  filepath.Join(".junie", "skills", "dtmgd"),
		EnvVar:      "JUNIE",
	},
	{
		Name:        "opencode",
		DisplayName: "OpenCode",
		ProjectPath: filepath.Join(".opencode", "skills", "dtmgd"),
		GlobalPath:  filepath.Join(".config", "opencode", "skills", "dtmgd"),
		EnvVar:      "OPENCODE",
	},
}

// InstallResult describes the outcome of an install operation.
type InstallResult struct {
	Agent    Agent
	Path     string
	Global   bool
	Replaced bool
}

// StatusResult describes the current installation state for an agent.
type StatusResult struct {
	Agent     Agent
	Installed bool
	Path      string
	Global    bool
}

// SupportedAgents returns the list of all supported agent names
// (excluding the cross-client pseudo-agent).
func SupportedAgents() []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}

// AllAgents returns the full agent registry (excluding the cross-client
// pseudo-agent).
func AllAgents() []Agent {
	return agents
}

// FindAgent looks up an agent by canonical name. Also matches "cross-client".
func FindAgent(name string) (Agent, bool) {
	if name == CrossClientAgent.Name {
		return CrossClientAgent, true
	}
	for _, a := range agents {
		if a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}

// DetectAgent reads agent environment variables and returns the first
// matching agent.
func DetectAgent() (Agent, bool) {
	for _, a := range agents {
		if a.EnvVar != "" && os.Getenv(a.EnvVar) != "" {
			return a, true
		}
	}
	return Agent{}, false
}

// skillFileName is the SKILL.md filename used for installation checks.
const skillFileName = "SKILL.md"

// Install copies the embedded skill content into the agent's skill directory.
// If global is true, it installs to the user's home directory; otherwise to
// the project root (baseDir). It returns an error if SKILL.md already exists
// at the destination and overwrite is false.
func Install(agent Agent, baseDir string, global bool, overwrite bool) (*InstallResult, error) {
	skillDir, err := resolvePath(agent, baseDir, global)
	if err != nil {
		return nil, err
	}

	skillFile := filepath.Join(skillDir, skillFileName)
	replaced := false
	if _, err := os.Stat(skillFile); err == nil {
		if !overwrite {
			return nil, fmt.Errorf("skill file already exists at %s (use --force to overwrite)", skillFile)
		}
		replaced = true
	}

	if replaced {
		if err := os.RemoveAll(skillDir); err != nil {
			return nil, fmt.Errorf("failed to remove existing skill directory %s: %w", skillDir, err)
		}
	}

	if err := copyEmbeddedFS(dtmgdskill.Content, skillDir); err != nil {
		return nil, fmt.Errorf("failed to install skill files: %w", err)
	}

	return &InstallResult{
		Agent:    agent,
		Path:     skillDir,
		Global:   global,
		Replaced: replaced,
	}, nil
}

// copyEmbeddedFS copies all files from the embedded filesystem to destDir,
// preserving the directory structure.
func copyEmbeddedFS(embeddedFS fs.FS, destDir string) error {
	return fs.WalkDir(embeddedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip Go source files (embed.go etc.) — they're not skill content.
		if filepath.Ext(path) == ".go" {
			return nil
		}

		destPath := filepath.Join(destDir, path)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		data, err := fs.ReadFile(embeddedFS, path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		dir := filepath.Dir(destPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}

		return os.WriteFile(destPath, data, 0o644)
	})
}

// Uninstall removes installed skill directories from both project-local and
// global locations. Returns the paths that were successfully removed.
func Uninstall(agent Agent, baseDir string) ([]string, error) {
	var removed []string
	var errs []string

	projectDir := filepath.Join(baseDir, agent.ProjectPath)
	if _, err := os.Stat(projectDir); err == nil {
		if err := os.RemoveAll(projectDir); err != nil {
			errs = append(errs, fmt.Sprintf("failed to remove %s: %v", projectDir, err))
		} else {
			removed = append(removed, projectDir)
		}
	}

	if agent.GlobalPath != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to determine home directory: %v", err))
		} else {
			globalDir := filepath.Join(home, agent.GlobalPath)
			if _, err := os.Stat(globalDir); err == nil {
				if err := os.RemoveAll(globalDir); err != nil {
					errs = append(errs, fmt.Sprintf("failed to remove %s: %v", globalDir, err))
				} else {
					removed = append(removed, globalDir)
				}
			}
		}
	}

	if len(errs) > 0 {
		return removed, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return removed, nil
}

// Status checks the installation state for a given agent.
func Status(agent Agent, baseDir string) *StatusResult {
	projectDir := filepath.Join(baseDir, agent.ProjectPath)
	if _, err := os.Stat(filepath.Join(projectDir, skillFileName)); err == nil {
		return &StatusResult{Agent: agent, Installed: true, Path: projectDir, Global: false}
	}

	if agent.GlobalPath != "" {
		if home, err := os.UserHomeDir(); err == nil {
			globalDir := filepath.Join(home, agent.GlobalPath)
			if _, err := os.Stat(filepath.Join(globalDir, skillFileName)); err == nil {
				return &StatusResult{Agent: agent, Installed: true, Path: globalDir, Global: true}
			}
		}
	}

	return &StatusResult{Agent: agent, Installed: false}
}

// StatusAll checks installation state for all supported agents and the
// cross-client location.
func StatusAll(baseDir string) []*StatusResult {
	results := make([]*StatusResult, 0, len(agents)+1)
	results = append(results, Status(CrossClientAgent, baseDir))
	for _, a := range agents {
		results = append(results, Status(a, baseDir))
	}
	return results
}

// resolvePath determines the absolute path for the skill directory.
func resolvePath(agent Agent, baseDir string, global bool) (string, error) {
	if global {
		if agent.GlobalPath == "" {
			return "", fmt.Errorf("%s does not support global installation", agent.DisplayName)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine home directory: %w", err)
		}
		return filepath.Join(home, agent.GlobalPath), nil
	}
	return filepath.Join(baseDir, agent.ProjectPath), nil
}
