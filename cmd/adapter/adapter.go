// SPDX-License-Identifier: MIT

// Package adapter holds `ethernetipctl adapter`: the simulated EtherNet/IP adapter.
package adapter

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/ethernetipctl/cmd/device"
	"github.com/otfabric/ethernetipctl/internal/adapter"
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/eip"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/format"
	"github.com/otfabric/ethernetipctl/internal/runner"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// NewCmd returns the `adapter` command.
func NewCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "adapter",
		Aliases: []string{"server"},
		Short:   "Run a simulated EtherNet/IP adapter (target) and stream its events",
		Long: `Serve a simulated EtherNet/IP adapter with go-ethernetip's server: sessions on TCP,
ListIdentity and ListServices on UDP (at the same port), the Identity, Message Router,
Assembly, Connection Manager, Port, TCP/IP Interface and Ethernet Link objects, explicit
messages unconnected and over Class 3 connections, and Class 0/1 I/O connections to its
assemblies. "server" is an alias.

The adapter is described by a model file (--config, YAML or JSON; see docs/adapter-config.md)
or by flags: the identity flags, --assembly instance:kind[:size] (default: input 100 and
output 150 of 32 octets, configuration 151 of 10, heartbeat points 152 input-only and 153
listen-only), --echo (output data copied to the input assembly of the same size) and
--logix (an emulated Rockwell Logix tag database with demo tags, for the "tag" commands).
Flags given with --config override the model's identity.

Every event (listening, connected, disconnected, connection-opened, connection-closed,
assembly-write, tag-write, reset, with --requests every request, stopping) is written to
stdout as it happens: a line of text, a JSON object per line, or a table row. It runs until
interrupted (Ctrl-C) or for --duration. It listens on every interface by default: use
--listen to restrict it. "connections" against the adapter shows its connection counters.`,
		Example: `
  # The default adapter on port 44818, I/O on UDP 2222, outputs echoed to inputs
  ethernetipctl adapter --echo

  # On localhost only, a free I/O port, with Logix tags, events as JSON Lines
  ethernetipctl adapter --listen 127.0.0.1:44818 --io-address 127.0.0.1:0 --logix --format json

  # From a model
  ethernetipctl adapter --config adapter.yaml --duration 60s
`,
		Args: cobra.NoArgs,
	}
	cfg := config.AdapterConfig{}
	runner.RegisterCommandCfg(cmd, &cfg, func(c *cobra.Command) { _ = c.MarkFlagFilename("config", "yaml", "yml", "json") })
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runner.RunStream(cmd, g.OutputFormat,
			func() error {
				if err := device.CheckGlobal(g); err != nil {
					return err
				}
				if _, err := adapter.ListenAddress(cfg.Listen, g.Port); err != nil {
					return err
				}
				_, err := adapter.Resolve(&cfg)
				return err
			},
			func(ctx context.Context, out *format.StreamWriter) (int, error) {
				var writeErr error
				log := func(e types.AdapterEvent) {
					if err := out.Write(&e); err != nil && writeErr == nil {
						writeErr = err
					}
				}
				env := eip.Env{Global: g, Stderr: cmd.ErrOrStderr()}
				if err := adapter.Run(ctx, &cfg, g, env.Logger(), log); err != nil {
					return 0, err
				}
				if writeErr != nil {
					return 0, errs.Output(errs.CodeOutputWriteFailed, writeErr)
				}
				return errs.ExitOK, nil
			})
	}
	return cmd
}
