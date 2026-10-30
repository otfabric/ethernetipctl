// Package tag holds `ethernetipctl tag …`: the Rockwell Logix tag services.
package tag

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/ethernetipctl/cmd/device"
	"github.com/otfabric/ethernetipctl/internal/cli"
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/eip"
	"github.com/otfabric/ethernetipctl/internal/runner"
)

const vendorNote = `These are Rockwell Automation Logix services (ControlLogix, CompactLogix and compatible
controllers): vendor specific, not part of CIP. Other EtherNet/IP devices refuse them.`

// NewCmd returns the `tag` command with its subcommands.
func NewCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Args:  cli.GroupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Rockwell Logix (vendor-specific): read, write and list controller tags",
		Long: `Read, write and list the tags of a Logix controller by name, and read its structure
templates (UDTs).

` + vendorNote + `

The controller is usually reached through the EtherNet/IP module of its chassis: --route
1/0 for the controller in slot 0. --connected uses a Class 3 connection (Large_Forward_Open,
4000 octets), as Logix programming tools do; requests are otherwise unconnected. Tag names
are controller-scoped (Count), program-scoped (Program:Main.Count), array elements
(Values[3], Grid[1,2]), structure members (Motor.Speed) or bits of an integer (Flags.5).`,
	}
	cmd.AddCommand(newReadCmd(g), newWriteCmd(g), newListCmd(g), newTypesCmd(g), newDecodeCmd(g))
	return cmd
}

func newReadCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read " + "<host[:port]> <tag>",
		Short: "Read a tag (Read Tag; Rockwell Logix)",
		Long: `Read --count elements of a tag with Read Tag, continued with Read Tag Fragmented for
large values. Atomic values and STRINGs are decoded; a structure is shown as hex (use
"tag decode" or --decode to decode it with its templates).

` + vendorNote,
		Example: `
  ethernetipctl tag read 192.168.1.10 Count --route 1/0
  ethernetipctl tag read 192.168.1.10 Values[2] --count 5 --route 1/0 --format json
  ethernetipctl tag read 192.168.1.10 Program:Main.Speed --route 1/0 --connected
`,
		Args: cobra.ExactArgs(2),
	}
	cfg := config.TagReadConfig{Count: 1}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := device.Env(cmd, g)
		return device.Formatted(cmd, g, func() error { return eip.CheckTagRead(args[1], cfg) },
			func(ctx context.Context) (any, error) { return e.TagRead(ctx, args[0], args[1], cfg) })
	}
	return cmd
}

func newDecodeCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decode <host[:port]> <tag>",
		Short: "Read a structure tag and decode it with its templates (Rockwell Logix)",
		Long: `Read a structure tag (a UDT, or an element of an array of them) and decode its members
with the templates the controller reports: "tag read --decode".

` + vendorNote,
		Example: `
  ethernetipctl tag decode 192.168.1.10 Motors[1] --route 1/0
`,
		Args: cobra.ExactArgs(2),
	}
	cfg := config.TagReadConfig{Count: 1, Decode: true}
	runner.RegisterCommandCfg(cmd, &cfg.MessagingConfig)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := device.Env(cmd, g)
		return device.Formatted(cmd, g, func() error { return eip.CheckTagRead(args[1], cfg) },
			func(ctx context.Context) (any, error) { return e.TagRead(ctx, args[0], args[1], cfg) })
	}
	return cmd
}

func newWriteCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write <host[:port]> <tag> --type T --value V[,V...] --yes",
		Short: "Write a tag (Write Tag; Rockwell Logix); needs --yes",
		Long: `Write a tag with Write Tag (Write Tag Fragmented for large values): --value encoded as
--type, which must be the tag's type; several comma-separated values write consecutive
array elements from the one named. A name that ends in a bit number (Flags.5) sets or
clears that bit with Read-Modify-Write: --type is then the integer type of the tag and
--value 0 or 1.

THIS CHANGES THE CONTROLLER'S DATA, which its program acts on. It needs --yes: without it
nothing is sent and the exit code is 2.

` + vendorNote,
		Example: `
  ethernetipctl tag write 192.168.1.10 Count --type DINT --value 42 --route 1/0 --yes
  ethernetipctl tag write 192.168.1.10 Values[0] --type INT --value 1,2,3 --route 1/0 --yes
  ethernetipctl tag write 192.168.1.10 Flags.5 --type DINT --value 1 --route 1/0 --yes
  ethernetipctl tag write 192.168.1.10 Name --type STRING --value "line 2" --route 1/0 --yes
`,
		Args: cobra.ExactArgs(2),
	}
	cfg := config.TagWriteConfig{}
	runner.RegisterCommandCfg(cmd, &cfg, func(c *cobra.Command) { _ = cli.RegisterEnumFlagCompletion(c, "type", eip.TagTypes) })
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := device.Env(cmd, g)
		return device.Formatted(cmd, g, func() error { _, _, _, err := eip.CheckTagWrite(args[1], cfg); return err },
			func(ctx context.Context) (any, error) { return e.TagWrite(ctx, args[0], args[1], cfg) })
	}
	return cmd
}

func newListCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <host[:port]>",
		Short: "List the tags of a controller (Rockwell Logix)",
		Long: `List the controller-scoped tags, or with --program the tags of a program, from the
Symbol object, with their types, array dimensions and instances, and the controller's
project name. System tags and the names of programs are left out unless --system.

` + vendorNote,
		Example: `
  ethernetipctl tag list 192.168.1.10 --route 1/0
  ethernetipctl tag list 192.168.1.10 --route 1/0 --program Main --format table
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.TagListConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := device.Env(cmd, g)
		return device.Formatted(cmd, g, func() error { return nil },
			func(ctx context.Context) (any, error) { return e.TagList(ctx, args[0], cfg) })
	}
	return cmd
}

func newTypesCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "types <host[:port]> <template-id>",
		Short: "Show a structure template and its members (Rockwell Logix)",
		Long: `Read a structure template (Template object instance, the template ID "tag list" shows
for a structure tag) and list its members with their types, offsets and array lengths.
Members the compiler generates (the hosts of BOOL members) are marked hidden.

` + vendorNote,
		Example: `
  ethernetipctl tag types 192.168.1.10 0x123 --route 1/0
`,
		Args: cobra.ExactArgs(2),
	}
	cfg := config.MessagingConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := device.Env(cmd, g)
		return device.Formatted(cmd, g, func() error { return nil },
			func(ctx context.Context) (any, error) { return e.TagTemplate(ctx, args[0], args[1], cfg) })
	}
	return cmd
}
