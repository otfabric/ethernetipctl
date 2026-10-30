package cmd

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files of the JSON output")

// dynamicKeys are the JSON fields whose values change from run to run: timings, session
// handles, connection IDs and serials, local addresses.
var dynamicKeys = map[string]bool{
	"duration_ms": true, "rtt_ms": true, "held_ms": true, "session": true, "local": true,
	"ot_connection_id": true, "to_connection_id": true, "connection_serial": true, "originator_serial": true,
	"received_at": true, "timestamp": true,
}

// normalize makes a JSON document deterministic: dynamic fields become "<dynamic>" and the
// address of the fixture becomes ADDR.
func normalize(t *testing.T, doc, addr string) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, doc)
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if dynamicKeys[k] {
					x[k] = "<dynamic>"
					continue
				}
				x[k] = walk(e)
			}
		case []any:
			for i, e := range x {
				x[i] = walk(e)
			}
		}
		return v
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(walk(v)); err != nil {
		t.Fatal(err)
	}
	if addr == "" {
		return b.Bytes()
	}
	return []byte(strings.ReplaceAll(b.String(), addr, "ADDR"))
}

// golden compares the normalized JSON of r with testdata/golden/name.json.
func golden(t *testing.T, name string, r result, addr string) {
	t.Helper()
	got := normalize(t, r.stdout, addr)
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd -run TestGolden -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s changed:\n%s\nwant:\n%s", name, got, want)
	}
}

// TestGoldenJSON pins the JSON output of representative commands: a change to a field name
// or shape shows up here.
func TestGoldenJSON(t *testing.T) {
	f := startFixture(t)
	j := func(args ...string) result {
		t.Helper()
		return f.run(t, append(args, "--format", "json")...)
	}
	golden(t, "identify", j("identify", "ADDR").want(t, 0), f.addr)
	golden(t, "status", j("status", "ADDR").want(t, 0), f.addr)
	golden(t, "services", j("services", "ADDR").want(t, 0), f.addr)
	golden(t, "read", j("read", "ADDR", "@1/1/7", "--type", "SHORT_STRING").want(t, 0), f.addr)
	golden(t, "read-all", j("read", "ADDR", "@1/1", "--all", "--route", "1/0").want(t, 0), f.addr)
	golden(t, "write", j("write", "ADDR", "@0xF5/1/13", "--type", "UINT", "--value", "60", "--yes").want(t, 0), f.addr)
	golden(t, "objects", j("objects", "ADDR").want(t, 0), f.addr)
	golden(t, "object", j("object", "ADDR", "identity").want(t, 0), f.addr)
	golden(t, "connections", j("connections", "ADDR").want(t, 0), f.addr)
	golden(t, "session", j("session", "ADDR", "--connected").want(t, 0), f.addr)
	golden(t, "forward-open", j("forward-open", "ADDR", "--config", "151", "--output", "150", "--input", "100", "--output-size", "32", "--input-size", "32", "--rpi", "10ms").want(t, 0), f.addr)
	golden(t, "diagnose", j("diagnose", "ADDR").want(t, 0), f.addr)
	golden(t, "tag-list", j("tag", "list", "ADDR").want(t, 0), f.addr)
	golden(t, "tag-read", j("tag", "read", "ADDR", "Values", "--count", "3").want(t, 0), f.addr)
	golden(t, "tag-decode", j("tag", "decode", "ADDR", "Motors[0]").want(t, 0), f.addr)
	golden(t, "tag-types", j("tag", "types", "ADDR", "0x123").want(t, 0), f.addr)
	golden(t, "eds-inspect", run(t, "eds", "inspect", adapterEDS, "--format", "json").want(t, 0), "")
	golden(t, "eds-validate-invalid", run(t, "eds", "validate", "../testdata/invalid.eds", "--format", "json").want(t, 3), "")
	golden(t, "error-refused", j("read", "ADDR", "@1/1/99").want(t, 6), f.addr)
	golden(t, "error-confirmation", j("write", "ADDR", "@1/1/1", "--data", "00").want(t, 2), f.addr)
	golden(t, "error-usage", run(t, "read", "--format", "json").want(t, 2), "")
}
