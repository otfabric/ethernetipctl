// SPDX-License-Identifier: MIT

// Package device holds the commands that talk to one EtherNet/IP device with explicit
// messages: identify, read, write, objects, sessions, connections and diagnostics.
package device

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/ethernetipctl/internal/cli"
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/eip"
	"github.com/otfabric/ethernetipctl/internal/runner"
)

// Commands returns the device commands, all at the top level of the tree.
func Commands(g *config.Global) []*cobra.Command {
	return []*cobra.Command{
		newDiscoverCmd(g),
		newIdentifyCmd(g),
		newStatusCmd(g),
		newServicesCmd(g),
		newInterfacesCmd(g),
		newReadCmd(g),
		newWriteCmd(g),
		newObjectsCmd(g),
		newObjectCmd(g),
		newResetCmd(g),
		newSessionCmd(g),
		newForwardOpenCmd(g),
		newForwardCloseCmd(g),
		newConnectionsCmd(g),
		newIOCmd(g),
		newAssemblyCmd(g),
		newDiagnoseCmd(g),
	}
}

// Env returns the environment of an operation of cmd.
func Env(cmd *cobra.Command, g *config.Global) eip.Env {
	return eip.Env{Global: g, Stderr: cmd.ErrOrStderr()}
}

// noCheck is the validation of a command whose arguments are checked as it runs; the
// global flags are checked by every command.
func noCheck() error { return nil }

// Formatted runs a command with one result: validate, then collect, then write it.
func Formatted(cmd *cobra.Command, g *config.Global, validate func() error, collect func(context.Context) (any, error), opts ...runner.FormatRunOption) error {
	return runner.RunFormattedChecked(cmd, g.OutputFormat, func() error {
		if err := CheckGlobal(g); err != nil {
			return err
		}
		return validate()
	}, collect, opts...)
}

// CheckGlobal validates the global flags.
func CheckGlobal(g *config.Global) error {
	if g.Timeout < 0 {
		return eip.InvalidTimeout(g.Timeout)
	}
	return nil
}

// enum registers shell completion for a flag with a fixed set of values.
func enum(flag string, values ...string) func(*cobra.Command) {
	return func(cmd *cobra.Command) { _ = cli.RegisterEnumFlagCompletion(cmd, flag, values) }
}

// typeCompletion completes --type with the data types.
func typeCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "type", eip.ValueTypes)
}

// hostUse names the device argument in the usage line of a command.
const hostUse = "<host[:port]>"
