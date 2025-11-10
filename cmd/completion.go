package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for work CLI.

To load completions:

Bash:
  $ source <(work completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ work completion bash > /etc/bash_completion.d/work
  # macOS:
  $ work completion bash > $(brew --prefix)/etc/bash_completion.d/work

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ work completion zsh > "${fpath[1]}/_work"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ work completion fish | source
  # To load completions for each session, execute once:
  $ work completion fish > ~/.config/fish/completions/work.fish

PowerShell:
  PS> work completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> work completion powershell > work.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
