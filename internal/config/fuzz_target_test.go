// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

// FuzzParseHost feeds the target syntax: what parses is one IPv4 address or host name
// with a port of 1 to 65535, and parses again to itself from its own rendering.
func FuzzParseHost(f *testing.F) {
	for _, s := range []string{
		"192.168.1.10", "192.168.1.10:44818", "plc-1.local", "plc_1:2222", "127.0.0.1:0", "127.0.0.1:65536", "[::1]:44818", "::1",
		"10.0.0.0/8", "10.0.0.1-5", "*", "a,b", " 1.2.3.4 ", "1.2.3", "256.1.1.1", "01.2.3.4", "host:", ":44818", "host:0x10",
		strings.Repeat("a", 64), strings.Repeat("a.", 200), "\x00", "host:+1", "host:-1", "1.2.3.4%eth0",
	} {
		f.Add(s, uint16(44818))
	}
	f.Add("host", uint16(0))
	f.Fuzz(func(t *testing.T, s string, port uint16) {
		tg, err := ParseHost(s, port)
		if err != nil {
			return
		}
		if tg.Port == 0 || tg.Host == "" || strings.ContainsAny(tg.Host, ":/*, ") {
			t.Fatalf("%q: %+v", s, tg)
		}
		again, err := ParseHost(tg.String(), 1)
		if err != nil || again != tg {
			t.Fatalf("%q: %+v renders as %q, which parses as %+v %v", s, tg, tg.String(), again, err)
		}
	})
}
