// SPDX-License-Identifier: MIT

package cmd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// complete runs the hidden __complete command and returns the candidates (descriptions
// cut off) followed by the directive line (":4").
func complete(t *testing.T, args ...string) string {
	t.Helper()
	r := run(t, append([]string{cobra.ShellCompRequestCmd}, args...)...).want(t, 0)
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(r.stdout), "\n") {
		out = append(out, strings.SplitN(line, "\t", 2)[0])
	}
	return strings.Join(out, " ")
}

func TestCompletionCandidates(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"completion", ""}, "bash zsh fish powershell :4"},
		{[]string{"identify", "--format", ""}, "json table text :4"},
		{[]string{"version", "--format", "j"}, "json :4"},
		{[]string{"reset", "--type", ""}, "0 1 :4"},
		{[]string{"tag", "write", "--type", "DI"}, "DINT :4"},
		{[]string{"adapter", "--config", ""}, "yaml yml json :8"},
		{[]string{"eds", "inspect", ""}, "eds :8"},
		{[]string{"eds", "generate", "--output", ""}, "eds :8"},
		{[]string{"eds", "codegen", "x.eds", "--output", ""}, "go :8"},
		{[]string{"forward-open", "10.0.0.1", "--config", ""}, ":4"},
		{[]string{"identify", ""}, ":4"},
		{[]string{"discover", "--address", ""}, ":4"},
		{[]string{"read", "10.0.0.1", "@1/1/1", "--timeout", ""}, ":4"},
	} {
		if got := complete(t, c.args...); got != c.want {
			t.Errorf("completion of %v: %q, want %q", c.args, got, c.want)
		}
	}
}

// valueSet matches a usage that lists the values of a flag: "text, json, or table",
// "data type: BOOL, SINT, …", "one of …", "a|b".
var valueSet = regexp.MustCompile(`(^|: )([a-z0-9-]+|[A-Z0-9_]+)( \([^)]*\))?(, ([a-z0-9-]+|[A-Z0-9_]+)( \([^)]*\))?)*,? or [a-zA-Z0-9_-]+|\bone of\b|\b[a-z0-9-]+\|[a-z0-9-]+\b`)

// Every flag that takes a value completes, and one whose usage lists its values
// completes to them, not to file names.
func TestEveryFlagCompletes(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.NoOptDefVal != "" || f.Name == "help" {
				return
			}
			fn, ok := c.GetFlagCompletionFunc(f.Name)
			if !ok {
				t.Errorf("%s --%s has no completion", c.CommandPath(), f.Name)
				return
			}
			if valueSet.MatchString(f.Usage) {
				values, dir := fn(c, nil, "")
				if len(values) < 2 || dir != cobra.ShellCompDirectiveNoFileComp {
					t.Errorf("%s --%s (%q) completes %q, directive %d", c.CommandPath(), f.Name, f.Usage, values, dir)
				}
			}
		})
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(NewRootCmd())
}
