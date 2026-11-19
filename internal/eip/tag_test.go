// SPDX-License-Identifier: MIT

package eip

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/logix"
)

// The room valuesOf takes follows the data the device sent, not the count it declares: a
// reply of 4 octets with a count of 65535 costs what 4 octets cost.
func TestValuesOfSizedByItsData(t *testing.T) {
	small := []byte{1, 0, 0, 0}
	for name, v := range map[string]logix.Value{
		"DINT":   {Type: cip.TypeDINT, Count: 65535, Data: small},
		"STRING": {Handle: logix.StringHandle, Count: 65535, Data: small},
		"BOOL":   {Type: cip.TypeBOOL, Count: 65535, Data: small},
	} {
		if got := valuesOf(v); got != nil {
			t.Errorf("%s: %d values of 4 octets", name, len(got))
		}
		const calls = 64
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for range calls {
			_ = valuesOf(v)
		}
		runtime.ReadMemStats(&after)
		if perCall := (after.TotalAlloc - before.TotalAlloc) / calls; perCall > 4<<10 {
			t.Errorf("%s: %d octets allocated per call for 4 octets of data", name, perCall)
		}
	}
	// What the data holds is still decoded.
	v := logix.Value{Type: cip.TypeDINT, Count: 2, Data: []byte{1, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF}}
	if got := valuesOf(v); len(got) != 2 || got[0] != int64(1) || got[1] != int64(-1) {
		t.Errorf("DINT[2]: %v", got)
	}
	if got := valuesOf(logix.Value{Type: cip.TypeDINT, Count: -1}); len(got) != 0 {
		t.Errorf("count -1: %v", got)
	}
}

// TestCheckTagMessage: a refused tag name is named once, with what is wrong
// with it, without the library's prefix.
func TestCheckTagMessage(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"1x", `invalid tag name "1x": symbol "1x"`},
		{"Program:Main", `invalid tag name "Program:Main": program without a tag`},
		{"A[1", `invalid tag name "A[1": unclosed index`},
		{"", `invalid tag name "": empty`},
	} {
		_, err := checkTag(c.name)
		if err == nil || err.Error() != c.want {
			t.Errorf("%q: %v, want %s", c.name, err, c.want)
		}
	}
}

// TestTagNameErrText: what is left of the library's error after its prefix
// and the name; an error with nothing more to say is given whole.
func TestTagNameErrText(t *testing.T) {
	quoted := fmt.Sprintf("%v: %q", logix.ErrTagName, "A")
	for _, c := range []struct {
		err        error
		name, want string
	}{
		{fmt.Errorf("%w: %q: symbol %q", logix.ErrTagName, "1x", "1x"), "1x", `symbol "1x"`},
		{fmt.Errorf("%w: empty", logix.ErrTagName), "", "empty"},
		{logix.ErrTagName, "A", logix.ErrTagName.Error()},
		{fmt.Errorf("%w: %q", logix.ErrTagName, "A"), "A", quoted},
	} {
		if got := tagNameErrText(c.err, c.name); got != c.want {
			t.Errorf("%v: %q, want %q", c.err, got, c.want)
		}
	}
}
