// SPDX-License-Identifier: MIT

package eip

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/otfabric/go-ethernetip/cip"
)

// FuzzDecode feeds Decode what a device answers. Data decoded as a named type
// must reach the output: JSON has no NaN, and a value that cannot be encoded
// would turn a good answer into a failure.
func FuzzDecode(f *testing.F) {
	for _, name := range ValueTypes {
		t, err := ParseType(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint16(t), []byte{1, 2, 3, 4, 5, 6, 7, 8})
		f.Add(uint16(t), []byte{0xFF, 0xFF, 0xFF, 0x7F})
		f.Add(uint16(t), []byte{})
	}
	f.Fuzz(func(t *testing.T, typ uint16, data []byte) {
		dt, err := ParseType(ValueTypes[int(typ)%len(ValueTypes)])
		if err != nil {
			t.Fatal(err)
		}
		v, err := Decode(dt, data)
		if err != nil {
			return
		}
		if _, err := json.Marshal(v); err != nil {
			t.Fatalf("%s % x: %v does not encode: %v", dt, data, v, err)
		}
	})
}

// FuzzDecodeAnyType feeds Decode a type a device named on the wire, which
// need not be one ethernetipctl knows.
func FuzzDecodeAnyType(f *testing.F) {
	f.Add(uint16(0xC3), []byte{1, 2})
	f.Add(uint16(0), []byte{1})
	f.Add(uint16(0xFFFF), []byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, typ uint16, data []byte) {
		dt := cip.DataType(typ)
		if _, err := Decode(dt, data); err == nil && dt.Size() == 0 && !dt.IsString() {
			t.Fatalf("type 0x%04X has no size and still decoded % x", typ, data)
		}
	})
}

// A type without a size used to divide by zero.
func TestDecodeTypeWithoutSize(t *testing.T) {
	if v, err := Decode(cip.DataType(0xFF), []byte{1, 2}); err == nil {
		t.Fatalf("decoded %v", v)
	}
}

// FuzzParseHex: the octets survive a round trip through Hex.
func FuzzParseHex(f *testing.F) {
	for _, s := range []string{"0a00", "0a 00", "0x0a00", "0a:00", "0", "zz", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, err := ParseHex(s)
		if err != nil {
			return
		}
		again, err := ParseHex(Hex(b))
		if err != nil || !bytes.Equal(b, again) {
			t.Fatalf("%q: % x, then % x, %v", s, b, again, err)
		}
	})
}

// FuzzParsePaths feeds the path, route and class syntaxes.
func FuzzParsePaths(f *testing.F) {
	for _, s := range []string{"@1/1/7", "@0x04/100/3", "20 04 24 64", "1,0", "backplane/0", "identity", "@1", "@/", "0x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseAttributePath(s)
		_, _ = ParseRoute(s)
		_, _ = ParseClass(s)
	})
}
