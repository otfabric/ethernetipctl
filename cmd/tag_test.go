package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/logix"
)

func TestTagRead(t *testing.T) {
	f := startFixture(t)
	f.run(t, "tag", "read", "ADDR", "Count").want(t, 0).has(t, "Count = DINT[1]", "[0] 42")
	f.run(t, "tag", "read", "ADDR", "count", "--route", "1/0", "--connected").want(t, 0).has(t, "(connected)", "[0] 42")
	f.run(t, "tag", "read", "ADDR", "Name").want(t, 0).has(t, `[0] "line 1"`)
	m := f.run(t, "tag", "read", "ADDR", "Values[2]", "--count", "3", "--format", "json").want(t, 0).json(t)
	if m["count"] != 3.0 || m["type"] != "INT" || len(m["values"].([]any)) != 3 {
		t.Errorf("tag read json: %v", m)
	}
	f.run(t, "tag", "read", "ADDR", "Values", "--count", "2", "--format", "table").want(t, 0).has(t, "TAG", "Values  1      INT")
	f.run(t, "tag", "read", "ADDR", "Program:Main.Local").want(t, 0).has(t, "SINT[1]")
	f.run(t, "tag", "read", "ADDR", "Speed").want(t, 0).has(t, "REAL[1]", "[0] 0")
	f.run(t, "tag", "read", "ADDR", "Big").want(t, 0).has(t, "ULINT[1]")
	f.run(t, "tag", "read", "ADDR", "Bits").want(t, 0).has(t, "DWORD[1]")
	f.run(t, "tag", "read", "ADDR", "On").want(t, 0).has(t, "BOOL[1]", "[0] false")

	// A structure is hex without --decode, its members with it.
	f.run(t, "tag", "read", "ADDR", "Motors[1]").want(t, 0).has(t, "struct handle 0xBEEF", "00 00 00")
	f.run(t, "tag", "read", "ADDR", "Motors", "--count", "2", "--format", "table").want(t, 0).has(t, "Motors  -      struct")
	// Motors[1]: Speed 2.5, Name "pump".
	data := make([]byte, 2*112)
	copy(data[112:], cip.AppendFloat32(nil, 2.5))
	copy(data[112+24:], append(cip.AppendUint32(nil, 4), "pump"...))
	must(t, f.tags.Set("Motors", logix.Value{Handle: motor.Handle, Count: 2, Data: data}))
	f.run(t, "tag", "decode", "ADDR", "Motors[1]").want(t, 0).has(t, "Speed                REAL     2.5", `Name                 STRING   "pump"`, "Count                DINT     [0 0 0 0]")
	f.run(t, "tag", "read", "ADDR", "Motors[1]", "--decode", "--format", "table").want(t, 0).has(t, "Motors[1].Speed", "Motors[1].Name")
	m = f.run(t, "tag", "decode", "ADDR", "Motors[1]", "--format", "json", "--connected").want(t, 0).json(t)
	if len(m["fields"].([]any)) != 4 || m["handle"] != float64(0xBEEF) {
		t.Errorf("decode json: %v", m)
	}
	// --decode is for structures: other values are read as usual.
	f.run(t, "tag", "decode", "ADDR", "Count").want(t, 0).has(t, "[0] 42")
	f.run(t, "tag", "decode", "ADDR", "Motors[1].Name").want(t, 0)

	for _, args := range [][]string{
		{"tag", "read", "ADDR", "Bad Name"},
		{"tag", "read", "ADDR", "Count", "--count", "0"},
		{"tag", "read", "ADDR", "Count", "--route", "x"},
		{"tag", "read", "x/y", "Count"},
	} {
		f.run(t, args...).want(t, 3)
	}
	r := f.run(t, "tag", "read", "ADDR", "Nope", "--format", "json").want(t, 6)
	if kind, code := r.errorCode(t); kind != "refused" || code != "path_destination_unknown" {
		t.Errorf("unknown tag: %s %s", kind, code)
	}
}

func TestTagDecodeErrors(t *testing.T) {
	f := startFixture(t)
	// The program's structure tags: none.
	f.run(t, "tag", "decode", "ADDR", "Program:Main.Local").want(t, 0)
	must(t, f.tags.AddStructTag("Program:Main.M", motor.ID))
	f.run(t, "tag", "decode", "ADDR", "Program:Main.M").want(t, 0).has(t, "Speed")
	must(t, f.tags.AddStructTag("Inner", motor.ID))
	f.run(t, "tag", "decode", "ADDR", "Motors[0].Name").want(t, 0)
}

func TestTagWrite(t *testing.T) {
	f := startFixture(t)
	f.run(t, "tag", "write", "ADDR", "Count", "--type", "dint", "--value", "-7", "--yes").want(t, 0).has(t, "Wrote Count = DINT[1] {-7}")
	if v, _ := f.tags.Get("Count"); v.Data[0] != 0xF9 {
		t.Errorf("Count %v", v)
	}
	f.run(t, "tag", "write", "ADDR", "Values[1]", "--type", "INT", "--value", "1,2,3", "--yes", "--connected").want(t, 0)
	f.run(t, "tag", "read", "ADDR", "Values", "--count", "4").want(t, 0).has(t, "[1] 1", "[2] 2", "[3] 3")
	f.run(t, "tag", "write", "ADDR", "Speed", "--type", "REAL", "--value", "1.25", "--yes", "--format", "table").want(t, 0).has(t, "Speed  REAL  1      1.25")
	f.run(t, "tag", "write", "ADDR", "Name", "--type", "STRING", "--value", "hello", "--yes").want(t, 0).has(t, `{"hello"}`)
	f.run(t, "tag", "write", "ADDR", "Name", "--type", "STRING", "--value", "", "--yes").want(t, 0)
	f.run(t, "tag", "write", "ADDR", "On", "--type", "BOOL", "--value", "true", "--yes").want(t, 0)
	f.run(t, "tag", "read", "ADDR", "On").want(t, 0).has(t, "[0] true")
	m := f.run(t, "tag", "write", "ADDR", "Flags.5", "--type", "DINT", "--value", "1", "--yes", "--format", "json").want(t, 0).json(t)
	if m["bit"] != 5.0 {
		t.Errorf("bit write: %v", m)
	}
	f.run(t, "tag", "read", "ADDR", "Flags").want(t, 0).has(t, "[0] 32")
	f.run(t, "tag", "write", "ADDR", "Flags.5", "--type", "DINT", "--value", "0", "--yes").want(t, 0)
	f.run(t, "tag", "read", "ADDR", "Flags").want(t, 0).has(t, "[0] 0")

	for _, args := range [][]string{
		{"tag", "write", "ADDR", "Count", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "DINT", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "FOO", "--value", "1", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "SHORT_STRING", "--value", "1", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "DINT", "--value", "x", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "SINT", "--value", "300", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "REAL", "--value", "x", "--yes"},
		{"tag", "write", "ADDR", "Count", "--type", "BOOL", "--value", "maybe", "--yes"},
		{"tag", "write", "ADDR", "Name", "--type", "STRING", "--value", strings.Repeat("x", 83), "--yes"},
		{"tag", "write", "ADDR", "Flags.5", "--type", "REAL", "--value", "1", "--yes"},
		{"tag", "write", "ADDR", "Flags.5", "--type", "DINT", "--value", "2", "--yes"},
		{"tag", "write", "ADDR", "Flags.5", "--type", "DINT", "--value", "1,0", "--yes"},
		{"tag", "write", "ADDR", "Bad Name", "--type", "DINT", "--value", "1", "--yes"},
	} {
		f.run(t, args...).want(t, 3)
	}
	// A value of another type than the tag's is refused by the controller.
	f.run(t, "tag", "write", "ADDR", "Count", "--type", "INT", "--value", "1", "--yes").want(t, 6)
}

func TestTagListAndTypes(t *testing.T) {
	f := startFixture(t)
	f.run(t, "tag", "list", "ADDR").want(t, 0).has(t, `(project "test-project")`, "Count", "Values[10]", "Motors[2]                        Motor", "Name                             STRING")
	m := f.run(t, "tag", "list", "ADDR", "--program", "Main", "--format", "json").want(t, 0).json(t)
	if m["program"] != "Program:Main" || m["count"] != 1.0 {
		t.Errorf("program tags: %v", m)
	}
	f.run(t, "tag", "list", "ADDR", "--program", "Program:Main", "--format", "table", "--system", "--connected").want(t, 0).has(t, "Program:Main.Local")
	f.run(t, "tag", "list", "ADDR", "--program", "Nope").want(t, 6)

	f.run(t, "tag", "types", "ADDR", "0x123").want(t, 0).has(t, "Template 0x0123 Motor", "Count                    DINT[4]", "(hidden)")
	f.run(t, "tag", "types", "ADDR", "291", "--format", "table").want(t, 0).has(t, "MEMBER", "Running")
	f.run(t, "tag", "types", "ADDR", "0x0FCE", "--format", "json").want(t, 0).has(t, `"name": "ASCIISTRING82"`)
	f.run(t, "tag", "types", "ADDR", "0").want(t, 3)
	f.run(t, "tag", "types", "ADDR", "0x7").want(t, 6)
	f.run(t, "tag", "types", "-", "1").want(t, 3)
	f.run(t, "tag", "list", "-").want(t, 3)
}
