// SPDX-License-Identifier: MIT

package types

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The escapes printable writes; built from parts so the source stays ASCII.
const (
	escNEL = `\u` + "0085"
	escLS  = `\u` + "2028"
	escBOM = `\u` + "feff"
)

func TestPrintable(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"Motor 1", "Motor 1"},
		{"caf\xc3\xa9 \xe6\xbc\xa2", "caf\xc3\xa9 \xe6\xbc\xa2"}, // e-acute and a CJK rune print
		{"\xef\xbf\xbd", "\xef\xbf\xbd"},                         // U+FFFD as such prints
		{"a\x1b[2Jb", `a\x1b[2Jb`},
		{"\r\n\t\a\b\f\v\x00", `\r\n\t\a\b\f\v\x00`},
		{"\x7f", `\x7f`},
		{"\xc2\x85", escNEL},                  // C1 control
		{"x\xe2\x80\xa8y", "x" + escLS + "y"}, // line separator
		{"\xef\xbb\xbf", escBOM},              // byte order mark
		{"a\xffb", `a\xffb`},                  // not UTF-8
		{"\xe2\x80", `\xe2\x80`},              // a cut rune
	} {
		if got := printable(c.in); got != c.want {
			t.Errorf("printable(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// bad returns what in s is neither printable nor a newline or tab.
func bad(s string) []string {
	var out []string
	for i, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case r == utf8.RuneError:
			out = append(out, strconv.Quote(s[i:i+1]))
		case !strconv.IsPrint(r):
			out = append(out, strconv.QuoteRuneToASCII(r))
		}
	}
	return out
}

type textTable interface {
	MarshalTextOutput() (string, error)
	TableRows() [][]string
}

// Every string a device sends is escaped in text and table output.
func TestDeviceStringsEscaped(t *testing.T) {
	const odd = "x\x1b]0;t\x07\r\n\x7f\xc2\x85\xe2\x80\xa8\xff"
	id := Identity{ProductName: odd}
	h := uint16(0x0BEE)
	for name, v := range map[string]textTable{
		"identify":  &IdentifyResult{Target: "t", Identity: &id},
		"status":    &StatusResult{Target: "t", Identity: id},
		"discover":  &DiscoverResult{Found: 1, Devices: []Device{{Address: "a", Identity: id}}},
		"services":  &ServicesResult{Target: "t", Count: 1, Services: []Service{{Name: odd}}},
		"tag list":  &TagListResult{Count: 1, Tags: []TagEntry{{Name: odd, Type: odd}}},
		"tag types": &TemplateResult{Name: odd, Members: []TemplateMember{{Name: odd, Type: "DINT"}}},
		"tag decode": &TagReadResult{Tag: "T", Type: "struct", Struct: odd, Handle: &h, Count: 1,
			Fields: []TagField{{Name: odd, Type: "STRUCT", Fields: []TagField{{Name: odd, Type: "STRING", Value: odd}}}}},
		"tag read": &TagReadResult{Tag: "T", Type: "STRING", Count: 1, Values: []any{odd}},
		"diagnose": &DiagnoseResult{Target: "t", Checks: []Check{{Name: "identity", Detail: map[string]any{"product": odd}}}},
	} {
		text, err := v.MarshalTextOutput()
		if err != nil {
			t.Fatal(err)
		}
		if b := bad(text); len(b) > 0 {
			t.Errorf("%s text: not escaped %v in %q", name, b, text)
		}
		for _, row := range v.TableRows() {
			for _, cell := range row {
				if b := bad(cell); len(b) > 0 || strings.ContainsAny(cell, "\n\t") {
					t.Errorf("%s table: not escaped %v in %q", name, b, cell)
				}
			}
		}
	}
}

// reset names the route it went through, as the other messaging commands do.
func TestResetNamesTheRoute(t *testing.T) {
	r := &ResetResult{Target: "10.0.0.1:44818", Route: "1/0", Type: 0, RTTMs: 1.5}
	text, err := r.MarshalTextOutput()
	if err != nil {
		t.Fatal(err)
	}
	if want := "Reset type 0 (power cycle) of 10.0.0.1:44818 route 1/0 accepted in 1.5 ms"; text != want {
		t.Errorf("text %q, want %q", text, want)
	}
	if got := r.TableRows()[0][0]; got != "10.0.0.1:44818 route 1/0" {
		t.Errorf("table TARGET %q", got)
	}
	r.Route = ""
	if got := r.TableRows()[0][0]; got != "10.0.0.1:44818" {
		t.Errorf("table TARGET without a route %q", got)
	}
}
