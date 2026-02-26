package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for datadog-cli.

To load completions:

  Bash:
    $ source <(datadog-cli completion bash)
    # To load completions for each session, execute once:
    $ datadog-cli completion bash > /etc/bash_completion.d/datadog-cli

  Zsh:
    # If shell completion is not already enabled in your environment,
    # you will need to enable it.  You can execute the following once:
    $ echo "autoload -U compinit; compinit" >> ~/.zshrc
    # To load completions for each session, execute once:
    $ datadog-cli completion zsh > "${fpath[1]}/_datadog-cli"

  Fish:
    $ datadog-cli completion fish | source
    # To load completions for each session, execute once:
    $ datadog-cli completion fish > ~/.config/fish/completions/datadog-cli.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
