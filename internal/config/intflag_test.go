// SPDX-License-Identifier: MIT

package config

import (
	"math"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
)

// An int flag holds a value beyond the range of int as the nearest bound, on
// every platform, so that the field's range check refuses it; it never keeps
// the low bits of the value.
func TestIntFlagKeepsValueWithinInt(t *testing.T) {
	type cfg struct {
		N int `flag:"n" desc:"n"`
	}
	for _, c := range []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"-1", -1},
		{"0x10", 16},
		{"2147483647", 2147483647},
		{"-2147483648", -2147483648},
		{"4294967296", 4294967296},
		{"4294967297", 4294967297},
		{"9223372036854775807", math.MaxInt64},
		{"-9223372036854775808", math.MinInt64},
	} {
		var v cfg
		cmd := &cobra.Command{Use: "x"}
		RegisterFlags(cmd, &v)
		if err := cmd.Flags().Set("n", c.in); err != nil {
			t.Fatalf("--n %s: %v", c.in, err)
		}
		want := max(min(c.want, math.MaxInt), math.MinInt)
		if int64(v.N) != want {
			t.Errorf("--n %s on a %d-bit platform: %d, want %d", c.in, strconv.IntSize, v.N, want)
		}
	}
	var v cfg
	cmd := &cobra.Command{Use: "x"}
	RegisterFlags(cmd, &v)
	if err := cmd.Flags().Set("n", "9223372036854775808"); err == nil {
		t.Error("--n beyond int64: no error")
	}
	if got := cmd.Flags().Lookup("n").Value.Type(); got != "int" {
		t.Errorf("flag type %q, want int (the help shows it)", got)
	}
}
