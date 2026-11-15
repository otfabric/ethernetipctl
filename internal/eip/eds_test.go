// SPDX-License-Identifier: MIT

package eip

import "testing"

// The file name in the header comment of generated code stays on its one line.
func TestCommentName(t *testing.T) {
	for in, want := range map[string]string{
		"adapter.eds":               "adapter.eds",
		"caf\xc3\xa9 `x` */.eds":    "caf\xc3\xa9 `x` */.eds",
		"a\nb\rc\td.eds":            "a_b_c_d.eds",
		"\x1b[2J\x07\x7f.eds":       "_[2J__.eds",
		"\xc2\x85.eds":              "_.eds", // C1 control
		"x\xe2\x80\xa8\xe2\x80\xa9": "x__",   // line and paragraph separators
		"\xef\xbb\xbfa":             "_a",    // byte order mark
		"a\xffb\xe2\x80":            "a_b_",  // not UTF-8
	} {
		if got := commentName(in); got != want {
			t.Errorf("commentName(%q) = %q, want %q", in, got, want)
		}
	}
}
