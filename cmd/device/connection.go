package device

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/eip"
	"github.com/otfabric/ethernetipctl/internal/format"
	"github.com/otfabric/ethernetipctl/internal/runner"
	"github.com/otfabric/ethernetipctl/internal/types"
)

func newSessionCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session " + hostUse,
		Aliases: []string{"connect"},
		Short:   "Open a session (and a Class 3 connection) and show it",
		Long: `Open a TCP connection and register an encapsulation session, and with --connected also
open a connected (Class 3) messaging connection to the Message Router with Forward_Open.
Show the session handle and the connection's IDs, triad and actual packet intervals, keep
them for --hold, then close them (Forward_Close, UnRegisterSession). "connect" is an alias.

Use it to check that a device accepts sessions and connections, and how many: run several
with --hold. A Class 3 connection ends with its session.`,
		Example: `
  ethernetipctl session 192.168.1.10
  ethernetipctl session 192.168.1.10 --connected --large --hold 30s
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.SessionConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Session(ctx, args[0], cfg) })
	}
	return cmd
}

const ioPathHelp = `The connection path names the assemblies: --config (the configuration instance; 0 leaves
it out), --output (the O->T connection point: the output assembly, or a heartbeat point for
input-only and listen-only connections) and --input (the T->O point: the input assembly).
--output-size and --input-size are the data sizes without sequence count and run/idle
header; the connection sizes of Forward_Open add them (O->T carries the 32-bit run/idle
header, T->O is modeless; an output size of 0 is a heartbeat). --rpi is the requested
packet interval of both directions.`

func newForwardOpenCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forward-open " + hostUse + " --output N --input N --rpi D [--config N] [--output-size N] [--input-size N]",
		Short: "Test a Forward_Open of an I/O connection, then close it again",
		Long: `Send a Forward_Open of a Class 1 cyclic I/O connection and show the target's answer: the
connection IDs, the actual packet intervals and the connection triad. No I/O data is
exchanged ("io" does that). Unless --keep, the connection is closed again with
Forward_Close at once.

With --keep the connection stays open on the target until it times out for lack of data
(RPI x 4 << --timeout-multiplier) or "forward-close" closes it with the triad shown and
the same path flags. Give the triad with --connection-serial and --originator-serial to
choose it.

` + ioPathHelp + `

A refusal is reported with the Connection Manager's extended status (for example 0x0106
ownership conflict, 0x0109 invalid connection size, 0x0127 invalid O->T size): exit code 6.`,
		Example: `
  # OpENer's sample application: exclusive owner 151/150/100 of 32 octets
  ethernetipctl forward-open 192.168.1.10 --config 151 --output 150 --input 100 --output-size 32 --input-size 32 --rpi 10ms

  # Input only (heartbeat 152), kept open for a later forward-close
  ethernetipctl forward-open 192.168.1.10 --config 151 --output 152 --input 100 --input-size 32 --rpi 1s --timeout-multiplier 7 --keep
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.ForwardOpenConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, func() error { return eip.CheckForwardOpen(cfg) },
			func(ctx context.Context) (any, error) { return e.ForwardOpen(ctx, args[0], cfg) })
	}
	return cmd
}

func newForwardCloseCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forward-close " + hostUse + " --connection-serial N --originator-serial N --output N --input N [--config N]",
		Short: "Close a connection with Forward_Close",
		Long: `Send a Forward_Close for the connection the triad names (--connection-serial,
--originator-vendor, --originator-serial, as "forward-open --keep" showed them) with its
connection path (--config, --output, --input). Targets accept the Forward_Close of a
connection from the host that opened it.`,
		Example: `
  ethernetipctl forward-close 192.168.1.10 --connection-serial 4711 --originator-serial 305419896 --config 151 --output 152 --input 100
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.ForwardCloseConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, func() error { _, err := eip.CheckForwardClose(cfg); return err },
			func(ctx context.Context) (any, error) { return e.ForwardClose(ctx, args[0], cfg) })
	}
	return cmd
}

func newIOCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "io " + hostUse + " --output N --input N --rpi D [--output-size N] [--input-size N] [--duration D]",
		Short: "Open an I/O connection and stream its input data",
		Long: `Open a Class 1 I/O connection with the scanner of go-ethernetip and write every input
(T->O) sample as it arrives: one line of text, one JSON object per line (JSON Lines) or one
table row. It runs until interrupted (Ctrl-C), --duration, --count samples, or the end of
the connection (a timeout of the target exits 5). Connection state and a summary go to
stderr.

` + ioPathHelp + `

A connection with output data (--output-size above 0) WRITES THE OUTPUTS OF THE DEVICE at
every interval: zeros, or --output-data. It needs --yes; without it nothing is sent and the
exit code is 2. --idle sends the output in idle mode, which most devices answer by holding
their outputs in their idle state. An input-only connection (--output the heartbeat point,
--output-size 0) writes nothing and needs no --yes.

The I/O endpoint listens on UDP --io-address (default :2222, the port targets send to); a
port other than 2222 is announced to the target in the Forward_Open.`,
		Example: `
  # Input only: watch input assembly 100 through heartbeat point 152
  ethernetipctl io 192.168.1.10 --config 151 --output 152 --input 100 --input-size 32 --rpi 50ms

  # Exclusive owner, writing outputs, for 10 seconds, only changes, as JSON Lines
  ethernetipctl io 192.168.1.10 --config 151 --output 150 --input 100 --output-size 32 --input-size 32 \
      --rpi 10ms --output-data 0102030405060708091011121314151617181920212223242526272829303132 --yes \
      --duration 10s --changes --format json
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.IOConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return runner.RunStream(cmd, g.OutputFormat,
			func() error {
				if err := CheckGlobal(g); err != nil {
					return err
				}
				_, err := eip.CheckIO(cfg)
				return err
			},
			func(ctx context.Context, out *format.StreamWriter) (int, error) {
				return e.IO(ctx, args[0], cfg, func(s *types.IOSample) error { return out.Write(s) }, cmd.ErrOrStderr())
			})
	}
	return cmd
}
