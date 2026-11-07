// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	clihelp "github.com/otfabric/ethernetipctl/internal/cli"
)

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

Flags with a fixed set of values (--format, --type) complete to those values;
EDS files, the adapter model and the --output of eds generate and codegen complete files
of their type.`,
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

// registerCompletions completes what no command completes itself: the flags and arguments
// that take a file complete files of its type, every other flag that takes a value (an
// address, a number, a path) and every other argument (a host, a path) nothing.
func registerCompletions(root *cobra.Command) {
	eds := []string{"eds"}
	clihelp.CompleteTree(root, clihelp.Completion{
		Files: map[string][]string{
			"adapter --config":      {"yaml", "yml", "json"},
			"eds generate --output": {"eds"},
			"eds codegen --output":  {"go"},
		},
		Args: map[string][]string{"eds inspect": eds, "eds validate": eds, "eds codegen": eds},
	})
}
