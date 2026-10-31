// SPDX-License-Identifier: MIT

package cmd

import "github.com/spf13/cobra"

const (
	completionBash       = "bash"
	completionZsh        = "zsh"
	completionFish       = "fish"
	completionPowerShell = "powershell"
)

var completionShells = []string{completionBash, completionZsh, completionFish, completionPowerShell}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Output a completion script for the given shell.

  Bash:        source <(ethernetipctl completion bash)
  Zsh:         source <(ethernetipctl completion zsh)
  Fish:        ethernetipctl completion fish | source
  PowerShell:  ethernetipctl completion powershell | Out-String | Invoke-Expression

Flags with a fixed set of values (--format, --type, …) complete to those
values.`,
		ValidArgs: completionShells,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, out := cmd.Root(), cmd.OutOrStdout()
			switch args[0] {
			case completionBash:
				return root.GenBashCompletionV2(out, true)
			case completionZsh:
				return root.GenZshCompletion(out)
			case completionFish:
				return root.GenFishCompletion(out, true)
			default:
				return root.GenPowerShellCompletion(out)
			}
		},
	}
}
