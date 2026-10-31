// SPDX-License-Identifier: MIT

package adapter

import "testing"

// FuzzParseAssembly feeds --assembly: a parsed assembly has a usable size.
func FuzzParseAssembly(f *testing.F) {
	for _, s := range []string{"100:input:8", "150:output", "0x10:Config:0x20", ":::", "100:input:-1", "100:input:99999"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		a, err := ParseAssembly(s)
		if err == nil && (a.Size < 0 || a.Size > 0xFFFF) {
			t.Fatalf("%q: size %d", s, a.Size)
		}
	})
}
