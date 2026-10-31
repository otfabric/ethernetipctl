// SPDX-License-Identifier: MIT

package device

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/ethernetipctl/internal/cli"
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/eip"
	"github.com/otfabric/ethernetipctl/internal/runner"
)

func newDiscoverCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover --address <broadcast or host>",
		Short: "Find devices with one ListIdentity request over UDP",
		Long: `Send one ListIdentity request over UDP to --address and list the devices that answer
within --wait, with their identity.

--address is a broadcast address (255.255.255.255, or the broadcast address of a subnet such
as 192.168.1.255) to find the devices of a network segment, or the address of one device.
Exactly one datagram is sent: discover never scans address ranges, and it accepts no
prefixes or ranges. Devices delay their answer to a broadcast by up to --max-delay (the
request carries it; their default is 2 seconds), so --wait should be longer than that.
--local picks the interface to send from by its IPv4 address.

ListIdentity is read-only: it changes nothing on a device.`,
		Example: `
  # Every device on the local segment
  ethernetipctl discover --address 255.255.255.255

  # A directed broadcast from a given interface, as JSON
  ethernetipctl discover --address 192.168.1.255 --local 192.168.1.20 --format json

  # One device, over UDP
  ethernetipctl discover --address 192.168.1.10 --wait 500ms
`,
		Args: cobra.NoArgs,
	}
	cfg := config.DiscoverConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, func() error { _, err := e.CheckDiscover(cfg); return err },
			func(ctx context.Context) (any, error) { return e.Discover(ctx, cfg) })
	}
	return cmd
}

func newIdentifyCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identify " + hostUse,
		Short: "Read the identity of a device: ListIdentity and the Identity object",
		Long: `Open a session with the device and ask who it is twice: with ListIdentity over the TCP
session (the identity item with the socket address the device reports and its state), and
with Get_Attributes_All of the Identity object (class 1, instance 1): vendor, device type,
product code, revision, status word, serial number and product name.`,
		Example: `
  ethernetipctl identify 192.168.1.10
  ethernetipctl identify 192.168.1.10:44818 --format json
  # The controller in slot 2 behind an EtherNet/IP module
  ethernetipctl identify 192.168.1.10 --route 1/2
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.MessagingConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Identify(ctx, args[0], cfg) })
	}
	return cmd
}

func newStatusCmd(g *config.Global) *cobra.Command {
	return &cobra.Command{
		Use:   "status " + hostUse,
		Short: "Show the health of a device in one line",
		Long: `Ask the device for its identity with ListIdentity over the TCP session and judge it: the
device is healthy when its status word reports no fault and its state is operational.
Healthy or not, the exit code is 0 when the device answered; scripts read "healthy".`,
		Example: `
  ethernetipctl status 192.168.1.10
  ethernetipctl status 192.168.1.10 --format json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := Env(cmd, g)
			return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Status(ctx, args[0]) })
		},
	}
}

func newServicesCmd(g *config.Global) *cobra.Command {
	return &cobra.Command{
		Use:   "services " + hostUse,
		Short: "List the encapsulation services of a device (ListServices)",
		Long: `Send ListServices over the TCP session and list the services the device offers, usually
"Communications", with its capability flags: CIP over the TCP session and Class 0/1 I/O
over UDP.`,
		Example: `
  ethernetipctl services 192.168.1.10
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := Env(cmd, g)
			return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Services(ctx, args[0]) })
		},
	}
}

func newInterfacesCmd(g *config.Global) *cobra.Command {
	return &cobra.Command{
		Use:   "interfaces " + hostUse,
		Short: "List the non-CIP interfaces of a device (ListInterfaces)",
		Long: `Send ListInterfaces over the TCP session and list the items of the reply. Most devices
have none: the list is empty.`,
		Example: `
  ethernetipctl interfaces 192.168.1.10
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := Env(cmd, g)
			return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Interfaces(ctx, args[0]) })
		},
	}
}

func newReadCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "read " + hostUse + " <@class/instance/attribute>",
		Aliases: []string{"attribute", "get"},
		Short:   "Read an attribute (Get_Attribute_Single) or an instance (--all)",
		Long: `Read one attribute of an object instance with Get_Attribute_Single, or with --all every
attribute of the instance with Get_Attributes_All (the path is then @class/instance).

The path is @class/instance/attribute with decimal or 0x-prefixed hex numbers (@1/1/7 is the
product name of the Identity object, @0xF5/1/5 the interface configuration of the TCP/IP
Interface object), or the hex octets of a padded EPATH ("20 01 24 01 30 07").

The value is shown as hex octets as the device encodes them; --type also decodes it as a
CIP data type (UINT, DINT, REAL, SHORT_STRING, ...), as an array when the data holds
several values. "attribute" and "get" are aliases of read.

--route sends the request through the device to another one with Unconnected_Send (1/0 is
slot 0 of a ControlLogix backplane); --connected sends it over a Class 3 connection.`,
		Example: `
  # The product name of a device
  ethernetipctl read 192.168.1.10 @1/1/7 --type SHORT_STRING

  # Every attribute of the Identity object
  ethernetipctl read 192.168.1.10 @1/1 --all

  # The input assembly 100, connected, as JSON
  ethernetipctl read 192.168.1.10 @4/100/3 --connected --format json

  # The TCP/IP interface status of the module in slot 1, through the backplane
  ethernetipctl read 192.168.1.10 @0xF5/1/1 --type UDINT --route 1/1
`,
		Args: cobra.ExactArgs(2),
	}
	cfg := config.ReadConfig{}
	runner.RegisterCommandCfg(cmd, &cfg, typeCompletion)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Read(ctx, args[0], args[1], cfg) })
	}
	return cmd
}

func newWriteCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write " + hostUse + " <@class/instance/attribute> (--data HEX | --type T --value V) --yes",
		Short: "Write an attribute (Set_Attribute_Single); needs --yes",
		Long: `Write one attribute of an object instance with Set_Attribute_Single.

THIS CHANGES THE DEVICE. A write needs --yes: without it ethernetipctl validates the
invocation, sends nothing and exits 2 (code confirmation_required). Know what an attribute
does before you write it: attributes of the TCP/IP Interface object change the address of
the device, assembly data drives outputs.

The value is --data, hex octets as the device encodes them, or --value encoded as --type
(several comma-separated values for an array; a string type takes the whole text).`,
		Example: `
  # The TTL of multicast I/O (TCP/IP Interface attribute 8)
  ethernetipctl write 192.168.1.10 @0xF5/1/8 --type USINT --value 1 --yes

  # Raw octets
  ethernetipctl write 192.168.1.10 @4/150/3 --data "01 00 00 00" --yes
`,
		Args: cobra.ExactArgs(2),
	}
	cfg := config.WriteConfig{}
	runner.RegisterCommandCfg(cmd, &cfg, typeCompletion)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, func() error { _, _, _, err := eip.CheckWrite(args[1], cfg); return err },
			func(ctx context.Context) (any, error) { return e.Write(ctx, args[0], args[1], cfg) })
	}
	return cmd
}

func newObjectsCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objects " + hostUse,
		Short: "List the object classes of a device (Message Router attribute 1)",
		Long: `Read the object list of the Message Router (class 2, instance 1, attribute 1) and list
the classes the device implements, with their names. Vendor-specific classes (the Rockwell
Logix objects, for one) are marked.`,
		Example: `
  ethernetipctl objects 192.168.1.10
  ethernetipctl objects 192.168.1.10 --format table
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.ObjectConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Objects(ctx, args[0], cfg) })
	}
	return cmd
}

func newObjectCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "object " + hostUse + " <class> [instance]",
		Short: "Show an object class: its class attributes and an instance",
		Long: `Read the common class attributes of an object class (instance 0: revision, max instance,
number of instances, max class and instance attribute IDs) and Get_Attributes_All of one
instance (default 1). The class is a number (1, 0xF5) or a class name (identity,
"tcp/ip interface", ethernet-link; case and punctuation do not matter). An attribute the
device does not implement is listed with its error.`,
		Example: `
  ethernetipctl object 192.168.1.10 identity
  ethernetipctl object 192.168.1.10 0xF6 1 --format json
`,
		Args: cobra.RangeArgs(2, 3),
	}
	cfg := config.ObjectConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		instance := ""
		if len(args) == 3 {
			instance = args[2]
		}
		return Formatted(cmd, g, func() error { _, err := eip.ParseClass(args[1]); return err },
			func(ctx context.Context) (any, error) { return e.Object(ctx, args[0], args[1], instance, cfg) })
	}
	return cmd
}

func newResetCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset " + hostUse + " [--type 0|1] --yes",
		Short: "Reset a device (Identity object Reset service); needs --yes",
		Long: `Send the Reset service to the Identity object (class 1, instance 1).

THIS RESTARTS THE DEVICE, and with --type 1 returns it to its factory defaults (its IP
configuration included). It needs --yes: without it nothing is sent and the exit code is 2.
Type 0 (the default) emulates a power cycle.`,
		Example: `
  ethernetipctl reset 192.168.1.10 --yes
  ethernetipctl reset 192.168.1.10 --type 1 --yes
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.ResetConfig{}
	runner.RegisterCommandCfg(cmd, &cfg, enum("type", "0", "1"))
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, func() error { return eip.CheckReset(cfg) },
			func(ctx context.Context) (any, error) { return e.Reset(ctx, args[0], cfg) })
	}
	return cmd
}

func newConnectionsCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connections " + hostUse,
		Short: "Show the connection counters of a device",
		Long: `Show what a device says about its CIP connections: the number of active connections and
the maximum (Message Router attributes 3 and 2, where the device has them), and the
Connection Manager's counters (attributes 1 to 8: open requests and rejects, close
requests and rejects, connection timeouts).

CIP has no service that lists another device's connections. ethernetipctl itself keeps no
connection open between commands: "forward-open --keep" leaves one for the target to time
out, and "forward-close" closes it. The simulated adapter ("ethernetipctl adapter") logs
every connection it opens and closes as an event.`,
		Example: `
  ethernetipctl connections 192.168.1.10
`,
		Args: cobra.ExactArgs(1),
	}
	cfg := config.ObjectConfig{}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.Connections(ctx, args[0], cfg) })
	}
	return cmd
}

func newAssemblyCmd(g *config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assembly",
		Args:  cli.GroupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Read and write the data of an assembly (class 4, attribute 3)",
		Long: `Read or write the data of an Assembly object instance (class 4, attribute 3) with explicit
messages: the input, output or configuration data that I/O connections exchange.
"assembly write" changes the outputs of the device and needs --yes.`,
	}
	read := &cobra.Command{
		Use:   "read " + hostUse + " <instance>",
		Short: "Read the data of an assembly",
		Example: `
  ethernetipctl assembly read 192.168.1.10 100
`,
		Args: cobra.ExactArgs(2),
	}
	rcfg := config.AssemblyConfig{}
	runner.RegisterCommandCfg(read, &rcfg.MessagingConfig)
	read.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) { return e.AssemblyRead(ctx, args[0], args[1], rcfg) })
	}
	write := &cobra.Command{
		Use:   "write " + hostUse + " <instance> --data HEX --yes",
		Short: "Write the data of an assembly; needs --yes",
		Long: `Write the data of an assembly with Set_Attribute_Single (attribute 3).

THIS CHANGES THE DEVICE: an output assembly drives its outputs. It needs --yes: without it
nothing is sent and the exit code is 2. Most devices refuse the write while an I/O
connection owns the assembly.`,
		Example: `
  ethernetipctl assembly write 192.168.1.10 150 --data 0102030405060708 --yes
`,
		Args: cobra.ExactArgs(2),
	}
	wcfg := config.AssemblyConfig{}
	runner.RegisterCommandCfg(write, &wcfg)
	write.RunE = func(cmd *cobra.Command, args []string) error {
		e := Env(cmd, g)
		return Formatted(cmd, g, func() error { _, err := eip.CheckAssemblyWrite(args[1], wcfg); return err },
			func(ctx context.Context) (any, error) { return e.AssemblyWrite(ctx, args[0], args[1], wcfg) })
	}
	cmd.AddCommand(read, write)
	return cmd
}

func newDiagnoseCmd(g *config.Global) *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose " + hostUse,
		Short: "Run structured checks against a device, each with its outcome and timing",
		Long: `Check a device step by step and report each check with ok, error or skipped and its
duration:

  tcp-connect         the TCP connection to the port
  register-session    RegisterSession, and the session handle
  list-identity       ListIdentity over the session
  list-services       ListServices
  identity            Get_Attributes_All of the Identity object
  tcpip-interface     TCP/IP Interface object: status, IP, mask, gateway, host name
  ethernet-link       Ethernet Link object: speed, link and duplex flags, MAC address
  connection-manager  Connection Manager counters

Nothing is changed on the device. A failed check does not stop the others; after a failed
TCP connect or RegisterSession the rest are skipped. The report is always written: the exit
code is 0 when every check passed and 7 (partial) when one failed or was skipped.`,
		Example: `
  ethernetipctl diagnose 192.168.1.10
  ethernetipctl diagnose 192.168.1.10 --format json --timeout 2s
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := Env(cmd, g)
			code := 0
			return Formatted(cmd, g, noCheck, func(ctx context.Context) (any, error) {
				r, c, err := e.Diagnose(ctx, args[0])
				code = c
				return r, err
			}, runner.WithSuccessExit(func(any) int { return code }))
		},
	}
}
