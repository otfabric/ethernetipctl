// SPDX-License-Identifier: MIT

package eip

import (
	"strings"
	"testing"
)

// The octets Encode sends at the bounds of every type: little-endian, two's complement,
// never a value narrowed from a wider one.
func TestEncodeBoundsExactOctets(t *testing.T) {
	for _, c := range []struct{ typ, value, hex string }{
		{"BOOL", "true", "01"},
		{"SINT", "-128", "80"}, {"SINT", "127", "7f"}, {"SINT", "-0x80", "80"},
		{"INT", "-32768", "0080"}, {"INT", "32767", "ff7f"},
		{"DINT", "-2147483648", "00000080"}, {"DINT", "2147483647", "ffffff7f"},
		{"LINT", "-9223372036854775808", "0000000000000080"}, {"LINT", "9223372036854775807", "ffffffffffffff7f"},
		{"USINT", "255", "ff"}, {"BYTE", "0b11111111", "ff"}, {"UINT", "65535", "ffff"}, {"WORD", "0o177777", "ffff"},
		{"UDINT", "4294967295", "ffffffff"}, {"DWORD", "0xffffffff", "ffffffff"},
		{"ULINT", "18446744073709551615", "ffffffffffffffff"}, {"LWORD", "0", "0000000000000000"},
		{"REAL", "3.4028234663852886e38", "ffff7f7f"}, {"REAL", "-0", "00000080"},
		{"LREAL", "1.7976931348623157e308", "ffffffffffffef7f"},
		{"SHORT_STRING", "", "00"}, {"STRING", "", "0000"}, {"STRING2", "", "0000"},
		{"USINT", "0,255", "00ff"},
	} {
		dt, err := ParseType(c.typ)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Encode(dt, c.value)
		if err != nil || Hex(b) != c.hex {
			t.Errorf("%s %s: %s %v, want %s", c.typ, c.value, Hex(b), err, c.hex)
		}
	}
	// One past every bound: refused.
	for _, c := range []struct{ typ, value string }{
		{"SINT", "128"}, {"SINT", "-129"}, {"INT", "32768"}, {"DINT", "2147483648"}, {"LINT", "9223372036854775808"},
		{"USINT", "256"}, {"USINT", "-1"}, {"UINT", "65536"}, {"UDINT", "4294967296"}, {"ULINT", "18446744073709551616"},
		{"REAL", "3.5e38"}, {"LREAL", "1.8e308"},
		{"SHORT_STRING", strings.Repeat("x", 256)}, {"STRING", strings.Repeat("x", 65536)}, {"STRING2", strings.Repeat("x", 65536)},
	} {
		dt, _ := ParseType(c.typ)
		if b, err := Encode(dt, c.value); err == nil {
			t.Errorf("%s %.20s: encoded as %s", c.typ, c.value, Hex(b))
		}
	}
}
