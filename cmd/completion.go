package cmd

import "github.com/spf13/cobra"

func newCompletionCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion script for dtmgd.

Examples:
  # Bash
  dtmgd completion bash > /etc/bash_completion.d/dtmgd

  # Zsh
  dtmgd completion zsh > "${fpath[1]}/_dtmgd"

  # Fish
  dtmgd completion fish > ~/.config/fish/completions/dtmgd.fish

  # PowerShell
  dtmgd completion powershell | Out-String | Invoke-Expression`,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(a.Out)
			case "zsh":
				return root.GenZshCompletion(a.Out)
			case "fish":
				return root.GenFishCompletion(a.Out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(a.Out)
			}
			return nil
		},
	}
}
