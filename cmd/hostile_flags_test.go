// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Hostile flag and argument values: every numeric flag at the bound of its type and one
// past, negative values for unsigned flags, durations that are not durations, and values
// of every CIP type at their bounds. A value a flag cannot hold is a usage error (2); a
// value that does not make sense is invalid input (3). Neither may reach the device: the
// fixture counts every session.

// usageCases are invocations whose flag value cannot be parsed into its type: Cobra
// rejects them, exit 2, kind usage, code invalid_input.
var usageCases = [][]string{
	// The global flags.
	{"identify", "ADDR", "--port", "65536"},
	{"identify", "ADDR", "--port", "-1"},
	{"identify", "ADDR", "--port", "0x10000"},
	{"identify", "ADDR", "--timeout", "NaN"},
	{"identify", "ADDR", "--timeout", "Inf"},
	{"identify", "ADDR", "--timeout", "-Inf"},
	{"identify", "ADDR", "--timeout", "1e400s"},
	{"identify", "ADDR", "--timeout", "9223372036854775808ns"},
	{"identify", "ADDR", "--timeout", "2562048h"}, // above the 292 years a Duration holds
	{"identify", "ADDR", "--timeout", "5"},        // a duration needs a unit
	{"identify", "ADDR", "--timeout", ""},
	// session
	{"session", "ADDR", "--size", "65536"},
	{"session", "ADDR", "--size", "-1"},
	{"session", "ADDR", "--hold", "NaN"},
	// forward-open / forward-close
	{"forward-open", "ADDR", "--output", "4294967296", "--input", "100", "--rpi", "10ms"},
	{"forward-open", "ADDR", "--output", "-1", "--input", "100", "--rpi", "10ms"},
	{"forward-open", "ADDR", "--output", "150", "--input", "4294967296", "--rpi", "10ms"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--config", "4294967296", "--rpi", "10ms"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--output-size", "65536"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--input-size", "-1"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--timeout-multiplier", "256"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--timeout-multiplier", "-1"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--connection-serial", "65536"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--originator-vendor", "-1"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--originator-serial", "4294967296"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "NaN"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "Inf"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "9223372036854775808ns"},
	{"forward-close", "ADDR", "--connection-serial", "65536", "--originator-serial", "1", "--output", "1", "--input", "1"},
	// io
	{"io", "ADDR", "--output", "152", "--input", "100", "--rpi", "10ms", "--count", "4294967296"},
	{"io", "ADDR", "--output", "152", "--input", "100", "--rpi", "10ms", "--count", "-1"},
	{"io", "ADDR", "--output", "152", "--input", "100", "--rpi", "10ms", "--duration", "NaN"},
	// discover
	{"discover", "--address", "127.0.0.1", "--max-devices", "9223372036854775808"},
	{"discover", "--address", "127.0.0.1", "--wait", "Inf"},
	{"discover", "--address", "127.0.0.1", "--max-delay", "NaN"},
	// reset: --type is an int
	{"reset", "ADDR", "--type", "9223372036854775808", "--yes"},
	// tag read
	{"tag", "read", "ADDR", "Count", "--count", "9223372036854775808"},
	{"tag", "read", "ADDR", "Count", "--count", "1.5"},
	{"tag", "decode", "ADDR", "Motors", "--count", "2"}, // decode has no --count
	{"object", "ADDR", "1", "-1"},                       // -1 is a flag
	// eds generate
	{"eds", "generate", "--vendor-id", "65536", "--product-name", "x", "--revision", "1.1"},
	{"eds", "generate", "--vendor-id", "1", "--device-type", "-1", "--product-name", "x", "--revision", "1.1"},
	{"eds", "generate", "--vendor-id", "1", "--product-code", "65536", "--product-name", "x", "--revision", "1.1"},
	// adapter
	{"adapter", "--vendor-id", "65536"},
	{"adapter", "--product-code", "-1"},
	{"adapter", "--serial", "4294967296"},
	{"adapter", "--duration", "NaN"},
}

func TestHostileFlagValuesAreUsageErrors(t *testing.T) {
	f := startFixture(t)
	for _, args := range usageCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := f.run(t, append(args, "--format", "json")...).want(t, 2)
			if kind, code := r.errorCode(t); kind != "usage" || code != "invalid_input" {
				t.Errorf("%s %s", kind, code)
			}
		})
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Fatalf("an unparseable flag value opened %d session(s)", n)
	}
}

// invalidCases parse into their type but make no sense: exit 3, kind invalid_input.
var invalidCases = [][]string{
	{"session", "ADDR", "--hold", "-1ns"},
	{"reset", "ADDR", "--type", "2", "--yes"},
	{"reset", "ADDR", "--type", "-1", "--yes"},
	{"reset", "ADDR", "--type", "9223372036854775807", "--yes"},
	{"reset", "ADDR", "--type", "-9223372036854775808", "--yes"},
	// --rpi: below a microsecond, negative, above the 32-bit microseconds of the wire.
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "999ns"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "-1ms"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "4294967296us"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "2562047h"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--timeout-multiplier", "8"},
	{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--timeout-multiplier", "255"},
	{"forward-open", "ADDR", "--output", "0", "--input", "100", "--rpi", "10ms"},
	{"forward-open", "ADDR", "--output", "150", "--input", "0", "--rpi", "10ms"},
	{"io", "ADDR", "--output", "152", "--input", "100", "--rpi", "-1ms"},
	{"io", "ADDR", "--output", "152", "--input", "100", "--rpi", "4294967296us"},
	{"io", "ADDR", "--output", "152", "--input", "100", "--rpi", "10ms", "--timeout-multiplier", "8"},
	{"discover", "--address", "127.0.0.1", "--wait", "-1ns"},
	{"discover", "--address", "127.0.0.1", "--max-delay", "-1ns"},
	{"discover", "--address", "127.0.0.1", "--max-devices", "-1"},
	{"discover", "--address", "127.0.0.1", "--max-devices", "-9223372036854775808"},
	{"discover", "--address", "127.0.0.1:0"},
	{"discover", "--address", "127.0.0.1:65536"},
	{"tag", "read", "ADDR", "Count", "--count", "0"},
	{"tag", "read", "ADDR", "Count", "--count", "-1"},
	{"tag", "read", "ADDR", "Count", "--count", "65536"},
	{"tag", "read", "ADDR", "Count", "--count", "9223372036854775807"},
	{"tag", "read", "ADDR", "Count", "--count", "-9223372036854775808"},
	// Instances and paths at the bound of their 32 and 16 bits and one past.
	{"read", "ADDR", "@1/4294967296/1"},
	{"read", "ADDR", "@1/1/65536"},
	{"read", "ADDR", "@65536/1/1"},
	{"read", "ADDR", "@-1/1/1"},
	{"object", "ADDR", "1", "4294967296"},
	{"object", "ADDR", "65536"},
	{"assembly", "read", "ADDR", "4294967296"},
	{"assembly", "read", "ADDR", "0"},
	{"assembly", "write", "ADDR", "4294967296", "--data", "00", "--yes"},
	// Targets.
	{"identify", "127.0.0.1:65536"},
	{"identify", "127.0.0.1:0"},
	{"identify", "127.0.0.1:-1"},
	{"identify", "256.0.0.1"},
	{"identify", strings.Repeat("a", 64) + ".example"},
	{"identify", "[::1]:44818"},
	// eds generate
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "128.0"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.256"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "-1.1"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--assembly", "1:65536"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--assembly", "4294967296:1"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--assembly", "0:1"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--assembly", "1:-1"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--connection", "4294967296:150:100:1:1"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--connection", "0:150:100:65536:1"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--connection", "0:150:100:1:65536"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--connection", "0:150:100:1:1:-1ms"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--connection", "0:150:100:1:1:0s"},
	{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--connection", "0:150:100:1:1:NaN"},
	// adapter
	{"adapter", "--revision", "128.1"},
	{"adapter", "--revision", "1.256"},
	{"adapter", "--assembly", "1:input:65536"},
	{"adapter", "--assembly", "1:input:65001"},
	{"adapter", "--assembly", "4294967296:input:1"},
	{"adapter", "--assembly", "-1:input:1"},
	{"adapter", "--product-name", strings.Repeat("x", 33)},
}

func TestHostileFlagValuesAreInvalidInput(t *testing.T) {
	f := startFixture(t)
	for _, args := range invalidCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := f.run(t, append(args, "--format", "json")...).want(t, 3)
			if kind, _ := r.errorCode(t); kind != "invalid_input" {
				t.Errorf("kind %s", kind)
			}
		})
	}
	// A negative --timeout: run, not f.run, which appends its own --timeout.
	for _, d := range []string{"-1ns", "-9223372036854775807ns"} {
		r := run(t, "identify", f.addr, "--timeout", d, "--format", "json").want(t, 3)
		if kind, code := r.errorCode(t); kind != "invalid_input" || code != "invalid_value" {
			t.Errorf("--timeout %s: %s %s", d, kind, code)
		}
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Fatalf("an invalid flag value opened %d session(s)", n)
	}
}

// valueBound is a value of a CIP type: ok when the type holds it.
type valueBound struct {
	typ, value string
	ok         bool
}

// valueBounds are the values of every type of write at the bounds and one past.
var valueBounds = []valueBound{
	{"BOOL", "1", true}, {"BOOL", "0", true}, {"BOOL", "2", false}, {"BOOL", "-1", false}, {"BOOL", "yes", false},
	{"SINT", "127", true}, {"SINT", "-128", true}, {"SINT", "128", false}, {"SINT", "-129", false},
	{"SINT", "0x7f", true}, {"SINT", "0x80", false}, {"SINT", "-0x80", true}, {"SINT", "-0x81", false},
	{"INT", "32767", true}, {"INT", "-32768", true}, {"INT", "32768", false}, {"INT", "-32769", false},
	{"DINT", "2147483647", true}, {"DINT", "-2147483648", true}, {"DINT", "2147483648", false}, {"DINT", "-2147483649", false},
	{"LINT", "9223372036854775807", true}, {"LINT", "-9223372036854775808", true},
	{"LINT", "9223372036854775808", false}, {"LINT", "-9223372036854775809", false},
	{"USINT", "255", true}, {"USINT", "0", true}, {"USINT", "256", false}, {"USINT", "-1", false}, {"USINT", "-0", false},
	{"BYTE", "0xff", true}, {"BYTE", "0x100", false}, {"BYTE", "-1", false},
	{"UINT", "65535", true}, {"UINT", "65536", false}, {"UINT", "-1", false},
	{"WORD", "65535", true}, {"WORD", "65536", false},
	{"UDINT", "4294967295", true}, {"UDINT", "4294967296", false}, {"UDINT", "-1", false},
	{"DWORD", "0xffffffff", true}, {"DWORD", "0x100000000", false},
	{"ULINT", "18446744073709551615", true}, {"ULINT", "18446744073709551616", false}, {"ULINT", "-1", false},
	{"LWORD", "0xffffffffffffffff", true}, {"LWORD", "0x10000000000000000", false},
	{"REAL", "3.4028234663852886e38", true}, {"REAL", "-3.4028234663852886e38", true},
	{"REAL", "3.5e38", false}, {"REAL", "-3.5e38", false}, {"REAL", "1e39", false},
	{"LREAL", "1.7976931348623157e308", true}, {"LREAL", "1.8e308", false}, {"LREAL", "-1e309", false},
	{"SHORT_STRING", strings.Repeat("x", 255), true}, {"SHORT_STRING", strings.Repeat("x", 256), false},
	{"STRING", strings.Repeat("x", 65535), true}, {"STRING", strings.Repeat("x", 65536), false},
	{"STRING2", strings.Repeat("x", 65535), true}, {"STRING2", strings.Repeat("x", 65536), false},
	// An array with one element past the bound fails as a whole.
	{"SINT", "1,2,3,128", false}, {"UINT", "1,65536,3", false},
	// A type that is not one.
	{"FLOAT", "1", false}, {"", "1", false}, {"SINT ", "1", true},
}

// Without --yes a write validates and sends nothing: a value the type holds is refused
// with confirmation_required (2), one it does not hold with invalid_value (3), never with
// a wrapped value.
func TestWriteValueBoundsDryRun(t *testing.T) {
	f := startFixture(t)
	for _, c := range valueBounds {
		name := c.typ + "=" + c.value
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			r := f.run(t, "write", "ADDR", "@0xF5/1/8", "--type", c.typ, "--value", c.value, "--format", "json")
			_, code := r.errorCode(t)
			switch {
			case c.ok && (r.code != 2 || code != "confirmation_required"):
				t.Errorf("a value %s holds: exit %d, %s", c.typ, r.code, code)
			case !c.ok && r.code != 3:
				t.Errorf("a value %s does not hold: exit %d, %s", c.typ, r.code, code)
			}
		})
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Fatalf("a dry-run write opened %d session(s)", n)
	}
}

// With --yes a value the type does not hold is still refused before any connection.
func TestWriteOutOfRangeValuesNeverReachTheDevice(t *testing.T) {
	f := startFixture(t)
	for _, c := range valueBounds {
		if c.ok {
			continue
		}
		f.run(t, "write", "ADDR", "@0xF5/1/8", "--type", c.typ, "--value", c.value, "--yes").want(t, 3)
	}
	if n, s := f.metrics.connects.Load(), f.metrics.sets.Load(); n != 0 || s != 0 {
		t.Fatalf("out-of-range values reached the adapter: %d sessions, %d sets", n, s)
	}
	// The bound itself is written as it is: the TTL of the TCP/IP Interface is a USINT.
	f.run(t, "write", "ADDR", "@0xF5/1/8", "--type", "USINT", "--value", "255", "--yes").want(t, 0)
	m := f.run(t, "read", "ADDR", "@0xF5/1/8", "--type", "USINT", "--format", "json").want(t, 0).json(t)
	if m["data"] != "ff" || m["value"] != 255.0 {
		t.Errorf("USINT 255 read back as %v", m)
	}
}

// tagBounds are the values of `tag write` at the bounds of the type of a fixture tag.
var tagBounds = []struct {
	tag, typ, value string
	ok              bool
}{
	{"Program:Main.Local", "SINT", "127", true},
	{"Program:Main.Local", "SINT", "-128", true},
	{"Program:Main.Local", "SINT", "128", false},
	{"Program:Main.Local", "SINT", "-129", false},
	{"Count", "DINT", "2147483647", true},
	{"Count", "DINT", "-2147483648", true},
	{"Count", "DINT", "2147483648", false},
	{"Count", "DINT", "-2147483649", false},
	{"Count", "DINT", "9223372036854775808", false},
	{"Values", "INT", "32768", false},
	{"Values", "INT", "1,2,32768", false},
	{"Speed", "REAL", "3.4028234663852886e38", true},
	{"Name", "STRING", strings.Repeat("x", 82), true},
	{"Name", "STRING", strings.Repeat("x", 83), false},
	{"Big", "ULINT", "-1", false},
	{"Big", "ULINT", "18446744073709551616", false},
	{"On", "BOOL", "2", false},
	{"Count", "SHORT_STRING", "x", false},
	{"Count", "STRING2", "x", false},
}

// tag write refuses a value its type does not hold before any connection and writes the
// bounds as they are.
func TestTagWriteValueBounds(t *testing.T) {
	f := startFixture(t)
	for _, c := range tagBounds {
		if !c.ok {
			f.run(t, "tag", "write", "ADDR", c.tag, "--type", c.typ, "--value", c.value, "--yes").want(t, 3)
		}
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Fatalf("out-of-range tag values opened %d session(s)", n)
	}
	for _, c := range tagBounds {
		if !c.ok {
			continue
		}
		f.run(t, "tag", "write", "ADDR", c.tag, "--type", c.typ, "--value", c.value, "--yes").want(t, 0)
		r := f.run(t, "tag", "read", "ADDR", c.tag, "--format", "json").want(t, 0).json(t)
		vals, _ := r["values"].([]any)
		if len(vals) == 0 {
			t.Errorf("%s read back as %v", c.tag, r)
		}
	}
}

// A REAL tag written with a value above the largest float32 was sent as +Inf: the explicit
// write refuses it (ParseFloat with 32 bits) but tag write parses 64 bits and narrows.
func TestTagWriteREALOverflowIsRefused(t *testing.T) {
	f := startFixture(t)
	for _, v := range []string{"3.5e38", "-3.5e38", "1e39", "1.7976931348623157e308"} {
		r := f.run(t, "tag", "write", "ADDR", "Speed", "--type", "REAL", "--value", v, "--yes")
		if r.code != 3 {
			got := f.run(t, "tag", "read", "ADDR", "Speed", "--format", "json").json(t)
			t.Errorf("REAL %s: exit %d, want 3 (invalid_value); the tag now holds %v", v, r.code, got["values"])
		}
	}
}

// ULINT and LWORD hold up to 2^64-1, which write accepts; tag write parses the values of
// every integer type as int64 and refuses the upper half of the range.
func TestTagWriteULINTUpperHalfIsAccepted(t *testing.T) {
	f := startFixture(t)
	for _, v := range []string{"9223372036854775808", "18446744073709551615", "0xffffffffffffffff"} {
		r := f.run(t, "tag", "write", "ADDR", "Big", "--type", "ULINT", "--value", v, "--yes")
		if r.code != 0 {
			t.Errorf("ULINT %s: exit %d, want 0\n%s", v, r.code, r.stderr)
			continue
		}
		m := f.run(t, "tag", "read", "ADDR", "Big", "--format", "json").want(t, 0).json(t)
		if vals, _ := m["values"].([]any); len(vals) != 1 {
			t.Errorf("ULINT %s read back as %v", v, m)
		}
	}
}

// A bit beyond the width of the integer type is invalid input found before any
// connection: tag write checks the bit against 0 and 1 but not against the type.
func TestTagWriteBitBeyondTypeWidthIsRefusedBeforeConnecting(t *testing.T) {
	f := startFixture(t)
	for _, c := range []struct{ tag, typ string }{
		{"Program:Main.Local.8", "SINT"},
		{"Values[0].16", "INT"},
		{"Count.32", "DINT"},
		{"Count.63", "DINT"},
	} {
		r := f.run(t, "tag", "write", "ADDR", c.tag, "--type", c.typ, "--value", "1", "--yes", "--format", "json")
		if r.code != 3 {
			t.Errorf("%s as %s: exit %d, want 3\n%s", c.tag, c.typ, r.code, r.stdout)
		}
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Errorf("bits beyond the type opened %d session(s) before being refused", n)
	}
}

// forward-open adds the sequence count and run/idle header to --output-size and
// --input-size in 16 bits: a size near 65535 wrapped to a few octets and the wrapped size
// went out in the Forward_Open. It must be refused before connecting.
func TestForwardOpenSizeNearMaxIsRefusedNotWrapped(t *testing.T) {
	f := startFixture(t)
	base := []string{"forward-open", "ADDR", "--config", "151", "--output", "150", "--input", "100", "--rpi", "10ms", "--format", "json"}
	for _, extra := range [][]string{
		{"--output-size", "65535", "--input-size", "32"},
		{"--output-size", "65530", "--input-size", "32"},
		{"--output-size", "32", "--input-size", "65535"},
		{"--output-size", "32", "--input-size", "65534"},
	} {
		r := f.run(t, append(base, extra...)...)
		if r.code != 3 {
			t.Errorf("%v: exit %d, want 3 (invalid_value) before any connection\n%s", extra, r.code, r.stdout)
		}
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Errorf("sizes that overflow the 16-bit connection size opened %d session(s)", n)
	}
}

// Configuration data longer than a data segment holds (255 words) is invalid input found
// before connecting, not a protocol error after it.
func TestForwardOpenConfigDataTooLongIsInvalidInput(t *testing.T) {
	f := startFixture(t)
	r := f.run(t, "forward-open", "ADDR", "--config", "151", "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "10ms",
		"--config-data", strings.Repeat("00", 512), "--format", "json")
	kind, code := r.errorCode(t)
	if r.code != 3 || kind != "invalid_input" {
		t.Errorf("512 octets of configuration data: exit %d, %s %s; want 3 invalid_input", r.code, kind, code)
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Errorf("configuration data too long for its segment opened %d session(s)", n)
	}
}

// io checks its sizes against the library's limit (65000) only after connecting.
func TestIOSizeAboveLimitIsRefusedBeforeConnecting(t *testing.T) {
	f := startFixture(t)
	r := f.run(t, "io", "ADDR", "--output", "152", "--input", "100", "--input-size", "65001", "--rpi", "10ms", "--io-address", "127.0.0.1:0", "--format", "json")
	if r.code != 3 {
		t.Errorf("io --input-size 65001: exit %d, want 3\n%s", r.code, r.stdout)
	}
	if n := f.metrics.connects.Load(); n != 0 {
		t.Errorf("io with a size above the limit opened %d session(s) before being refused", n)
	}
}

// A negative --duration is not "until interrupted": it is invalid input. io and adapter
// test only for a positive duration and run until the context ends.
func TestNegativeStreamDurationIsInvalidInput(t *testing.T) {
	f := startFixture(t)
	for _, args := range [][]string{
		{"io", f.addr, "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "10ms", "--io-address", "127.0.0.1:0", "--duration", "-1s"},
		{"adapter", "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "-1s"},
	} {
		t.Run(args[0], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			start := time.Now()
			r := runCtx(t, ctx, append(args, "--format", "json")...)
			if r.code != 3 {
				t.Errorf("%s --duration -1s: exit %d after %v, want 3 at once", args[0], r.code, time.Since(start).Round(time.Millisecond))
			}
		})
	}
}

// The largest valid --rpi and --timeout-multiplier pass validation and reach the device,
// whose answer decides.
func TestForwardOpenAtTheBoundsOfItsFlags(t *testing.T) {
	f := startFixture(t)
	r := f.run(t, "forward-open", "ADDR", "--config", "151", "--output", "152", "--input", "100", "--input-size", "32",
		"--rpi", "4294967295us", "--timeout-multiplier", "7", "--connection-serial", "65535", "--originator-vendor", "65535",
		"--originator-serial", "4294967295", "--format", "json")
	if r.code != 0 && r.code != 6 {
		t.Fatalf("exit %d, want 0 or a refusal (6)\n%s", r.code, r.stdout)
	}
	if f.metrics.connects.Load() == 0 {
		t.Fatal("valid bounds were refused before connecting")
	}
	_ = r.json(t)
}
