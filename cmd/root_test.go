// SPDX-License-Identifier: MIT

package cmd

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/otfabric/ethernetipctl/internal/runner"
)

func TestVersionHelpAndUsage(t *testing.T) {
	SetBuildMeta("1.2.3", "v1.2.3", "abc1234", "2026-01-31T12:00:00Z")
	t.Cleanup(func() { SetBuildMeta("dev", "", "", "") })
	run(t, "version").want(t, 0).has(t, "ethernetipctl version: 1.2.3", "tag:         v1.2.3", "commit:      abc1234", "build date:  2026-01-31T12:00:00Z", "go version:", "platform:")
	SetBuildMeta(" ", "", "", "")
	r := run(t, "version").want(t, 0).has(t, "ethernetipctl version: (unknown)")
	if strings.Contains(r.stdout, "commit:") {
		t.Errorf("empty build metadata is printed:\n%s", r.stdout)
	}

	run(t).want(t, 0).has(t, "Available Commands", "discover", "identify", "read", "write", "objects", "object", "reset",
		"session", "forward-open", "forward-close", "connections", "io", "assembly", "diagnose", "status", "eds", "tag", "adapter", "completion", "version")
	run(t, "eds").want(t, 0).has(t, "inspect", "validate", "generate", "codegen")
	run(t, "tag").want(t, 0).has(t, "read", "write", "list", "types", "decode", "Rockwell")
	run(t, "assembly").want(t, 0).has(t, "read", "write")
	run(t, "write", "--help").want(t, 0).has(t, "THIS CHANGES THE DEVICE", "--yes", "ETHERNETIPCTL_FORMAT", "--route")
	run(t, "tag", "read", "--help").want(t, 0).has(t, "Rockwell Automation Logix", "vendor specific")

	run(t, "nope").want(t, 2).errHas(t, `error: unknown command "nope"`)
	run(t, "tag", "nope").want(t, 2).errHas(t, `unknown command "nope" for "ethernetipctl tag"`)
	run(t, "eds", "inspec").want(t, 2).errHas(t, "Did you mean this?\n\tinspect")
	run(t, "identify", "--nope", "x").want(t, 2).errHas(t, "unknown flag")
	run(t, "identify").want(t, 2).errHas(t, "accepts 1 arg(s)")
	run(t, "read", "10.0.0.1").want(t, 2)
	run(t, "read", "10.0.0.1", "@1/1/1", "--port", "banana").want(t, 2)
	run(t, "read", "10.0.0.1", "@1/1/1", "--timeout", "soon").want(t, 2)
	run(t, "version", "extra").want(t, 2)
}

// A usage error (what Cobra rejects before the command runs) is the JSON error object on
// stdout when the invocation asks for JSON, by --format or ETHERNETIPCTL_FORMAT, and
// the text error line on stderr otherwise. Either way the exit code is 2.
func TestUsageErrorsHonourFormat(t *testing.T) {
	for _, c := range []struct {
		name string
		env  string
		args []string
		json bool
		msg  string
	}{
		{"unknown flag after --format", "", []string{"identify", "--format", "json", "--nope"}, true, "unknown flag: --nope"},
		{"unknown flag before --format", "", []string{"identify", "--nope", "--format", "json"}, true, "unknown flag: --nope"},
		{"--format=json", "", []string{"identify", "--nope", "--format=json"}, true, "unknown flag"},
		{"the last --format wins", "", []string{"identify", "--format", "json", "--nope", "--format", "text"}, false, "unknown flag"},
		{"missing argument", "", []string{"identify", "--format", "json"}, true, "accepts 1 arg(s)"},
		{"value a flag cannot hold", "", []string{"identify", "x", "--port", "banana", "--format", "json"}, true, `invalid argument "banana" for "--port"`},
		{"unknown command", "", []string{"nope", "--format", "json"}, true, `unknown command "nope"`},
		{"unknown subcommand", "", []string{"tag", "nope", "--format", "json"}, true, `unknown command "nope" for "ethernetipctl tag"`},
		{"unknown subcommand, flag first", "", []string{"eds", "--format", "json", "nope"}, true, `unknown command "nope" for "ethernetipctl eds"`},
		{"unknown subcommand, unknown flag", "", []string{"tag", "nope", "--yes", "--format", "json"}, true, `unknown command "nope" for "ethernetipctl tag"`},
		{"misspelt subcommand", "", []string{"tag", "lst", "--format", "json"}, true, "Did you mean this?\n\tlist"},
		{"alias of a command, unknown flag", "", []string{"attribute", "--nope", "--format", "json"}, true, "unknown flag: --nope"},
		{"unknown subcommand, grouped short flags", "", []string{"tag", "-xyz", "nope", "--format", "json"}, true, "unknown"},
		{"unknown subcommand, a known flag of a sub", "", []string{"tag", "--count", "2", "nope", "--format", "json"}, true, "unknown"},
		{"--format after --", "", []string{"identify", "--nope", "--", "--format", "json"}, false, "unknown flag"},
		{"invalid --format", "", []string{"identify", "--nope", "--format", "xml"}, false, "unknown flag"},
		{"environment", "json", []string{"identify"}, true, "accepts 1 arg(s)"},
		{"command line over environment", "json", []string{"identify", "--format", "text"}, false, "accepts 1 arg(s)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ETHERNETIPCTL_OUTPUT_FORMAT", c.env)
			r := run(t, c.args...).want(t, 2)
			if !c.json {
				r.errHas(t, "error: "+c.msg)
				if r.stdout != "" {
					t.Errorf("a text usage error writes to stdout:\n%s", r.stdout)
				}
				return
			}
			e := r.json(t)["error"].(map[string]any)
			if e["kind"] != "usage" || e["code"] != "invalid_input" || !strings.Contains(e["message"].(string), c.msg) {
				t.Errorf("error object: %s", r.stdout)
			}
			if r.stderr != "" {
				t.Errorf("a JSON usage error writes to stderr:\n%s", r.stderr)
			}
		})
	}
}

// A binary built without the ldflags of the Makefile (go install, go build) reports what the
// go toolchain recorded: the version of the module and the git revision.
func TestVersionFromBuildInfo(t *testing.T) {
	rev := []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"}}
	for _, c := range []struct {
		name, version, commit   string
		info                    *debug.BuildInfo
		wantVersion, wantCommit string
	}{
		{"ldflags", "v0.1.0", "abc1234", &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}, Settings: rev}, "v0.1.0", "abc1234"},
		{"go install", "dev", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}, Settings: rev}, "v0.1.0", "0123456"},
		{"devel", "dev", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev", ""},
		{"no build info", "dev", "", nil, "dev", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, commit := versionOf(c.version, c.commit, c.info)
			if v != c.wantVersion || commit != c.wantCommit {
				t.Errorf("versionOf(%q, %q) = %q, %q; want %q, %q", c.version, c.commit, v, commit, c.wantVersion, c.wantCommit)
			}
		})
	}
	SetBuildMeta("dev", "", "", "")
	run(t, "version").want(t, 0).has(t, "ethernetipctl version: ")
}

func TestCompletion(t *testing.T) {
	for shell, marker := range map[string]string{
		"bash":       "__start_ethernetipctl",
		"zsh":        "#compdef ethernetipctl",
		"fish":       "complete -c ethernetipctl",
		"powershell": "Register-ArgumentCompleter",
	} {
		run(t, "completion", shell).want(t, 0).has(t, marker)
	}
	run(t, "completion").want(t, 2)
	run(t, "completion", "tcsh").want(t, 2)

	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"identify", "--format", ""}, []string{"json", "table", "text"}},
		{[]string{"read", "--type", "SHORT"}, []string{"SHORT_STRING"}},
		{[]string{"write", "--type", "UD"}, []string{"UDINT"}},
		{[]string{"reset", "--type", ""}, []string{"0", "1"}},
		{[]string{"tag", "write", "--type", "STR"}, []string{"STRING"}},
		{[]string{"completion", ""}, []string{"bash", "zsh", "fish", "powershell"}},
	} {
		r := run(t, append([]string{cobra.ShellCompNoDescRequestCmd}, c.args...)...).want(t, 0)
		for _, w := range c.want {
			if !strings.Contains("\n"+r.stdout, "\n"+w+"\n") {
				t.Errorf("completion of %v lacks %q:\n%s", c.args, w, r.stdout)
			}
		}
	}
}

// Every command has a long description and examples; environment variables are
// ETHERNETIPCTL_*; the flags that confirm a change (--yes) have no environment variable.
func TestCommandsAreDocumented(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == "help" || c.Name() == "completion" || strings.HasPrefix(c.Name(), "__") {
			return
		}
		if c.Long == "" && c.Name() != "version" && c.Name() != "read" && c.Name() != "inspect" {
			t.Errorf("%s has no long description", c.CommandPath())
		}
		if c.Runnable() && !c.HasSubCommands() && c.Example == "" && c.Name() != "version" {
			t.Errorf("%s has no examples", c.CommandPath())
		}
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			i := strings.Index(f.Usage, "[env: ")
			if f.Name == "yes" && i >= 0 {
				t.Errorf("%s --yes can be set from the environment", c.CommandPath())
			}
			if i >= 0 && !strings.HasPrefix(f.Usage[i+6:], "ETHERNETIPCTL_") {
				t.Errorf("%s --%s: environment variable is not ETHERNETIPCTL_*", c.CommandPath(), f.Name)
			}
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	root := NewRootCmd()
	walk(root)
	if _, ok := runner.RequestedFormat(root, nil); !ok {
		t.Error("the global --format is not the format of stdout")
	}

	// A malformed value is an error before anything runs, whatever the command.
	t.Setenv("ETHERNETIPCTL_PORT", "banana")
	run(t, "version").want(t, 3).errHas(t, `error: invalid environment: ETHERNETIPCTL_PORT="banana"`)
	t.Setenv("ETHERNETIPCTL_PORT", "")
	t.Setenv("ETHERNETIPCTL_TIMEOUT", "soon")
	run(t, "version", "--format", "json").want(t, 3)
}
