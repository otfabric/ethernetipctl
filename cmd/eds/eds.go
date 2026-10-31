// SPDX-License-Identifier: MIT

// Package eds holds `ethernetipctl eds …`: Electronic Data Sheets.
package eds

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/ethernetipctl/cmd/device"
	"github.com/otfabric/ethernetipctl/internal/cli"
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/eip"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/runner"
)

// NewCmd returns the `eds` command with its subcommands.
func NewCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eds",
		Args:  cli.GroupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Inspect, validate, generate and turn into Go code Electronic Data Sheets",
		Long: `Commands for Electronic Data Sheets (.eds): the text files in which a vendor describes an
EtherNet/IP device's identity, parameters, assemblies and I/O connections.

"eds inspect" shows what a file describes, "eds validate" checks it against the rules of
the format, "eds generate" writes an EDS for a device (from the device itself or from
flags) and "eds codegen" turns an EDS into Go constants, types and scanner configurations.
None of them sends anything to a device, except "eds generate --from-device", which reads
its identity.`,
	}
	cmd.AddCommand(newInspectCmd(g), newValidateCmd(g), newGenerateCmd(g), newCodegenCmd(g))
	return cmd
}

func newInspectCmd(g *config.Global) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <file.eds>",
		Short: "Show the device, parameters, assemblies and connections of an EDS",
		Example: `
  ethernetipctl eds inspect adapter.eds
  ethernetipctl eds inspect adapter.eds --format json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return device.Formatted(cmd, g, func() error { return nil }, func(context.Context) (any, error) { return eip.EDSInspect(args[0]) })
		},
	}
}

func newValidateCmd(g *config.Global) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file.eds>",
		Short: "Check an EDS against the rules of the format",
		Long: `Check an EDS and list its issues, errors first: File and Device sections with their
required entries, dates, times and revisions, numbers, parameter data types and sizes,
minimum, maximum and default, ParamN and AssemN references, EPATHs, assembly sizes and
repeated keywords.

The report is always written. The exit code is 0 when the file has no error (warnings are
allowed) and 3 when it has errors, so that a pipeline fails on an invalid file.`,
		Example: `
  ethernetipctl eds validate adapter.eds
  ethernetipctl eds validate adapter.eds --format json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := errs.ExitOK
			return device.Formatted(cmd, g, func() error { return nil }, func(context.Context) (any, error) {
				r, c, err := eip.EDSValidate(args[0])
				code = c
				return r, err
			}, runner.WithSuccessExit(func(any) int { return code }))
		},
	}
}

// writeOutput writes generated content to stdout, or to the file of output with a result
// in the --format of the invocation.
func writeOutput(cmd *cobra.Command, g *config.Global, output, kind string, force bool, data []byte) error {
	if output == "" || output == "-" {
		if _, err := cmd.OutOrStdout().Write(data); err != nil {
			return errs.Output(errs.CodeOutputWriteFailed, err)
		}
		return nil
	}
	return device.Formatted(cmd, g, func() error { return nil }, func(context.Context) (any, error) {
		return eip.WriteOutput(output, kind, data, force)
	})
}

func newGenerateCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate (--from-device <host> | --vendor-id N --product-name NAME ...) [--assembly ...] [--connection ...]",
		Short: "Write an EDS for a device",
		Long: `Write an EDS for a device: the File, Device, Device Classification, Params (the RPI
parameter), Assembly and Connection Manager sections.

The identity comes from the device itself with --from-device (Get_Attributes_All of its
Identity object), or from the identity flags (--vendor-id, --product-name, --device-type,
--product-code, --revision). The assemblies (--assembly instance:size[:name]) and I/O
connections (--connection config:output:input:output-size:input-size[:rpi[:name]]) come
from flags: CIP has no way to list a device's assemblies. The connections are Class 1
cyclic point-to-point with fixed sizes: review them against the device.

Without --output, or with --output -, the EDS text goes to stdout whatever --format says.
With a file, a short result (file, size, SHA-256) is written in --format. --date fixes the
creation date, for reproducible files.`,
		Example: `
  ethernetipctl eds generate --from-device 192.168.1.10 --assembly 100:32:Input --assembly 150:32:Output \
      --assembly 151:10:Config --connection 151:150:100:32:32:10ms --output device.eds

  ethernetipctl eds generate --vendor-id 65535 --product-name "My adapter" --revision 1.1 \
      --device-type 12 --product-code 7 --assembly 100:8 --assembly 150:8 --connection 0:150:100:8:8
`,
		Args: cobra.NoArgs,
	}
	cfg := config.EDSGenerateConfig{Revision: "1.1", DeviceType: 12, Output: "-"}
	runner.RegisterCommandCfg(cmd, &cfg, func(c *cobra.Command) { _ = c.MarkFlagFilename("output", "eds") })
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := device.CheckGlobal(g); err != nil {
			return errs.WrapValidation(err)
		}
		if _, err := runner.AttachOutputFormat(cmd, g.OutputFormat); err != nil {
			return err
		}
		if _, err := eip.CheckEDSGenerate(cfg); err != nil {
			return err
		}
		data, err := device.Env(cmd, g).EDSGenerate(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		return writeOutput(cmd, g, cfg.Output, "eds", cfg.Force, data)
	}
	return cmd
}

func newCodegenCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegen <file.eds> --package NAME",
		Short: "Generate Go code for the device an EDS describes",
		Long: `Generate Go source for the device an EDS describes, with go-ethernetip's generator: its
identity as constants and its electronic key; a constant and a descriptor per parameter,
with a range check, and a type with named constants per enumerated parameter; per
assembly its instance and size and, where its members are whole octets of numeric
parameters, a struct with Encode and Decode methods; per I/O connection a function that
returns its scanner.IOConfig. The output is gofmt-formatted and deterministic.

Validate the file first ("eds validate"): generation fails on the errors that prevent it.
Without --output the source goes to stdout.`,
		Example: `
  ethernetipctl eds codegen adapter.eds --package adapter --output adapter/device.go
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.EDSCodegenConfig{Package: "device", Output: "-"}
	runner.RegisterCommandCfg(cmd, &cfg, func(c *cobra.Command) { _ = c.MarkFlagFilename("output", "go") })
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if _, err := runner.AttachOutputFormat(cmd, g.OutputFormat); err != nil {
			return err
		}
		src, err := eip.EDSCodegen(args[0], cfg)
		if err != nil {
			return err
		}
		return writeOutput(cmd, g, cfg.Output, "go", cfg.Force, src)
	}
	return cmd
}
