// Package cmd is the command tree of ethernetipctl.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/otfabric/ethernetipctl/cmd/adapter"
	"github.com/otfabric/ethernetipctl/cmd/device"
	"github.com/otfabric/ethernetipctl/cmd/eds"
	"github.com/otfabric/ethernetipctl/cmd/tag"
	clihelp "github.com/otfabric/ethernetipctl/internal/cli"
	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/runner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// buildMeta holds link-time metadata set from main via SetBuildMeta.
var buildMeta struct {
	version, tag, commit, buildDate string
}

// SetBuildMeta is called from main before Execute; values come from ldflags -X main.*.
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildMeta.version = version
	buildMeta.tag = tag
	buildMeta.commit = commit
	buildMeta.buildDate = buildDate
}

// NewRootCmd builds the command tree. Every call returns a fresh tree, with the flags of
// every command at the defaults the environment gives them.
func NewRootCmd() *cobra.Command {
	g := config.NewGlobal()
	config.MustLoadFromEnv(g)
	root := &cobra.Command{
		Use:   "ethernetipctl",
		Short: "EtherNet/IP and CIP command-line originator, diagnostics and adapter simulator",
		Long: `ethernetipctl speaks EtherNet/IP, the industrial Ethernet protocol of ODVA with the Common
Industrial Protocol (CIP) on top, used by programmable controllers, I/O adapters, drives
and sensors.

As an originator it finds devices (discover, one ListIdentity request; it never scans),
identifies them, reads and writes attributes with explicit messages (unconnected, routed
through a backplane, or over Class 3 connections), opens and closes connections, exchanges
I/O data, and diagnoses a device step by step. "tag" reads and writes the tags of Rockwell
Logix controllers (vendor specific). "eds" inspects, validates, generates and turns into Go
code Electronic Data Sheets. "adapter" simulates a device.

A device is named as host[:port]: an IPv4 address or a host name, port 44818 unless
--port or the host says otherwise. Commands that change a device (write, reset, assembly
write, tag write, io with output data) need --yes and send nothing without it.

Results go to stdout as text, a table or JSON (--format); progress and diagnostics go to
stderr. Exit codes are stable: see docs/exitcodes.md.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	config.RegisterFlagSet(root.PersistentFlags(), g)
	runner.RegisterStdoutFormatCompletion(root)
	root.AddCommand(device.Commands(g)...)
	root.AddCommand(
		eds.NewCmd(g),
		tag.NewCmd(g),
		adapter.NewCmd(g),
		newCompletionCmd(),
		newVersionCmd(),
	)
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of ethernetipctl",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			out := cmd.OutOrStdout()
			info, _ := debug.ReadBuildInfo()
			ver, commit := versionOf(buildMeta.version, buildMeta.commit, info)
			if ver == "" {
				ver = "(unknown)"
			}
			_, _ = fmt.Fprintf(out, "ethernetipctl version: %s\n", ver)
			if s := strings.TrimSpace(buildMeta.tag); s != "" {
				_, _ = fmt.Fprintf(out, "tag:         %s\n", s)
			}
			if commit != "" {
				_, _ = fmt.Fprintf(out, "commit:      %s\n", commit)
			}
			if s := strings.TrimSpace(buildMeta.buildDate); s != "" {
				_, _ = fmt.Fprintf(out, "build date:  %s\n", s)
			}
			_, _ = fmt.Fprintf(out, "go version:  %s\n", runtime.Version())
			_, _ = fmt.Fprintf(out, "platform:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

// devVersion is the version of a binary built without the ldflags of the Makefile.
const devVersion = "dev"

// versionOf returns the version and commit to report. The ldflags of the Makefile and of
// the release set them; a binary built without them (go install, go build) reports what the
// go toolchain records: the version of its module (v0.1.0 for go install …@v0.1.0, or one
// derived from the git tags for go build in a clone, such as v0.1.0+dirty) and the git
// revision, shortened as git rev-parse --short gives it.
func versionOf(version, commit string, info *debug.BuildInfo) (string, string) {
	version, commit = strings.TrimSpace(version), strings.TrimSpace(commit)
	if info == nil {
		return version, commit
	}
	if version == devVersion || version == "" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		}
	}
	if commit == "" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				commit = s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			}
		}
	}
	return version, commit
}

// Execute runs the root command and returns the process exit code (main should call os.Exit).
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// Run executes ethernetipctl with args, writing to stdout and stderr, and returns the exit
// code. It is Execute without the process: what the tests drive.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root, err := buildRoot()
	if err != nil {
		return runner.RenderFatal(nil, err, stdout, stderr, false)
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	inv := &runner.Invocation{}
	ctx = runner.WithInvocation(ctx, inv)
	cmd, err := root.ExecuteContextC(ctx)
	trace, _ := root.PersistentFlags().GetBool("debug")
	if err != nil {
		if !inv.HasOutputFormat {
			// Cobra failed before the command recorded its format: honour --format (or
			// ETHERNETIPCTL_FORMAT) all the same.
			if f, ok := runner.RequestedFormat(cmd, args); ok {
				inv.SetOutputFormat(f)
			}
		}
		return runner.RenderFatal(inv, usageError(unknownSubcommand(cmd, args, err)), stdout, stderr, trace)
	}
	if inv.SuccessExitSet {
		return inv.SuccessExitCode
	}
	return errs.ExitOK
}

// buildRoot is NewRootCmd with a malformed environment variable reported as invalid input
// instead of a panic: the configuration of every command is read when the tree is built.
func buildRoot() (root *cobra.Command, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := strings.TrimPrefix(fmt.Sprint(r), "ethernetipctl: ")
			err = errs.InvalidInput(errs.CodeInvalidInput, msg, nil)
		}
	}()
	return NewRootCmd(), nil
}

// unknownSubcommand returns the error for an unknown subcommand of a group command
// ("ethernetipctl tag nope --format json") in place of the error about the first flag:
// Cobra stops at the group, which has none of its subcommands' flags, and reports that
// flag. Any other error is returned as it is.
func unknownSubcommand(cmd *cobra.Command, args []string, err error) error {
	if cmd == nil || !cmd.HasSubCommands() || !strings.HasPrefix(err.Error(), "unknown flag") {
		return err
	}
	depth := len(strings.Fields(cmd.CommandPath())) - 1 // the words after "ethernetipctl"
	words := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			if takesValue(cmd, a) {
				i++ // its value is the next argument
			}
			continue
		}
		words++
		if words <= depth {
			continue
		}
		for _, sub := range cmd.Commands() {
			if sub.Name() == a || sub.HasAlias(a) {
				return err
			}
		}
		return clihelp.UnknownCommand(cmd, a)
	}
	return err
}

// takesValue reports whether the flag arg ("--format", "-o", not "--format=json") names a
// flag of cmd or of one of its subcommands that takes a value, so that the next argument
// is that value rather than a command.
func takesValue(cmd *cobra.Command, arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if !strings.HasPrefix(arg, "--") && len(name) != 1 {
		return false // a group of short flags, or a negative number
	}
	lookup := func(c *cobra.Command) *pflag.Flag {
		if strings.HasPrefix(arg, "--") {
			return c.Flags().Lookup(name)
		}
		return c.Flags().ShorthandLookup(name)
	}
	for _, c := range append([]*cobra.Command{cmd}, cmd.Commands()...) {
		if f := lookup(c); f != nil {
			return f.NoOptDefVal == ""
		}
	}
	return false
}

// usageError classifies what Cobra itself rejects (an unknown command or flag, a missing
// required flag, a value a flag cannot hold) as a usage error: exit code 2.
func usageError(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	return errs.New(errs.KindUsage, errs.CodeInvalidInput, err.Error(), err)
}
