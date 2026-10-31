// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adapterRun is an `adapter` command running in a goroutine.
type adapterRun struct {
	stdout, stderr syncBuffer
	cancel         context.CancelFunc
	done           chan int
	addr           string
}

// startAdapter runs `ethernetipctl adapter` with args and waits for its listening event,
// which names the address. The events must be JSON Lines (--format json).
func startAdapter(t *testing.T, args ...string) *adapterRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	a := &adapterRun{cancel: cancel, done: make(chan int, 1)}
	go func() {
		a.done <- Run(ctx, append([]string{"adapter", "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--format", "json"}, args...), &a.stdout, &a.stderr)
	}()
	eventually(t, "the listening event", func() bool {
		for _, line := range strings.Split(a.stdout.String(), "\n") {
			var ev map[string]any
			if json.Unmarshal([]byte(line), &ev) == nil && ev["event"] == "listening" {
				a.addr = ev["address"].(string)
				return true
			}
		}
		return false
	})
	t.Cleanup(func() { a.stop(t) })
	return a
}

// stop interrupts the adapter and returns its exit code.
func (a *adapterRun) stop(t *testing.T) int {
	t.Helper()
	a.cancel()
	select {
	case code := <-a.done:
		a.done <- code
		return code
	case <-t.Context().Done():
		return -1
	}
}

// events returns the names of the events written so far.
func (a *adapterRun) events(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, ev := range jsonLines(t, a.stdout.String()) {
		out[ev["event"].(string)]++
	}
	return out
}

func TestAdapterFromFlags(t *testing.T) {
	a := startAdapter(t, "--logix", "--echo", "--requests", "--product-name", "flag adapter", "--vendor-id", "1", "--product-code", "9",
		"--serial", "77", "--revision", "4.2")
	run(t, "identify", a.addr).want(t, 0).has(t, `"flag adapter"`, "revision:       4.2", "serial number:  0x0000004D")
	run(t, "assembly", "write", a.addr, "150", "--data", strings.Repeat("11", 32), "--yes").want(t, 0)
	run(t, "assembly", "read", a.addr, "100").want(t, 0).has(t, "11 11 11")
	run(t, "tag", "write", a.addr, "Count", "--type", "DINT", "--value", "5", "--yes").want(t, 0)
	run(t, "tag", "read", a.addr, "Count").want(t, 0).has(t, "[0] 5")
	run(t, "tag", "decode", a.addr, "Motors[0]").want(t, 0).has(t, "Speed")
	run(t, "tag", "read", a.addr, "Values", "--count", "10").want(t, 0).has(t, "[9] 9")
	run(t, "tag", "read", a.addr, "Name").want(t, 0).has(t, `"line 1"`)
	run(t, "forward-open", a.addr, "--config", "151", "--output", "150", "--input", "100", "--output-size", "32", "--input-size", "32", "--rpi", "10ms").want(t, 0)
	run(t, "reset", a.addr, "--yes").want(t, 0)
	run(t, "connections", a.addr).want(t, 0).has(t, "open_requests            1", "close_requests           1")
	if code := a.stop(t); code != 0 {
		t.Fatalf("adapter exit %d\n%s", code, a.stderr.String())
	}
	ev := a.events(t)
	for _, name := range []string{"listening", "connected", "disconnected", "request", "assembly-write", "tag-write", "connection-opened", "connection-closed", "reset", "stopping"} {
		if ev[name] == 0 {
			t.Errorf("no %s event in\n%s", name, a.stdout.String())
		}
	}
}

func TestAdapterFromModel(t *testing.T) {
	a := startAdapter(t, "--config", "../testdata/adapter.yaml", "--product-name", "override")
	run(t, "identify", a.addr).want(t, 0).has(t, `"override"`, "revision:       2.1", "serial number:  0x00C0FFEE")
	run(t, "assembly", "read", a.addr, "100").want(t, 0).has(t, "01 02 03 04 05 06 07 08")
	run(t, "assembly", "write", a.addr, "150", "--data", "0909090909090909", "--yes").want(t, 0)
	run(t, "assembly", "read", a.addr, "100").want(t, 0).has(t, "09 09 09")
	run(t, "tag", "list", a.addr).want(t, 0).has(t, `(project "Line1")`, "Levels[3]", "Valves[2]", "Label")
	run(t, "tag", "read", a.addr, "Levels", "--count", "3").want(t, 0).has(t, "[2] 3.5")
	run(t, "tag", "read", a.addr, "Label").want(t, 0).has(t, `"tank 1"`)
	run(t, "tag", "read", a.addr, "Program:Main.Step").want(t, 0).has(t, "[0] 3")
	run(t, "tag", "types", a.addr, "0x200").want(t, 0).has(t, "Position", "Open")

	// The same model as JSON, with text events and a duration.
	jsonModel := filepath.Join(t.TempDir(), "adapter.json")
	must(t, os.WriteFile(jsonModel, []byte(`{"identity": {"vendor_id": 1, "device_type": 12, "product_code": 1, "revision": "1.1", "serial_number": 5, "product_name": "json adapter"},
		"assemblies": [{"instance": 100, "kind": "input", "size": 4}, {"instance": 150, "kind": "output", "size": 4}]}`), 0o600))
	r := run(t, "adapter", "--config", jsonModel, "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "20ms").want(t, 0)
	r.has(t, "listening", "stopping")
	run(t, "adapter", "--config", jsonModel, "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "20ms", "--format", "table", "--debug").want(t, 0).has(t, "TIMESTAMP", "listening")
}

func TestAdapterInvalid(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		must(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}
	id := `identity: {vendor_id: 1, product_code: 1, revision: "1.1", serial_number: 1, product_name: x}` + "\n"
	for name, content := range map[string]string{
		"unknown-field.yaml": id + "nope: 1\n",
		"revision.yaml":      `identity: {revision: "x", product_name: x}`,
		"name.yaml":          `identity: {revision: "1.1", product_name: ""}`,
		"kind.yaml":          id + "assemblies: [{instance: 1, kind: sideways, size: 1}]\n",
		"zero.yaml":          id + "assemblies: [{instance: 0, kind: input, size: 1}]\n",
		"twice.yaml":         id + "assemblies: [{instance: 1, kind: input, size: 1}, {instance: 1, kind: input, size: 1}]\n",
		"heartbeat.yaml":     id + "assemblies: [{instance: 1, kind: input-only, size: 1}]\n",
		"size.yaml":          id + "assemblies: [{instance: 1, kind: input, size: 70000}]\n",
		"data.yaml":          id + "assemblies: [{instance: 1, kind: input, size: 2, data: \"01\"}]\n",
		"echo.yaml":          id + "assemblies: [{instance: 1, kind: input, size: 2}, {instance: 2, kind: output, size: 4}]\necho: [{output: 2, input: 1}]\n",
		"tagtype.yaml":       id + "logix: {tags: [{name: X, type: FLOAT}]}\n",
		"tagvalue.yaml":      id + "logix: {tags: [{name: X, type: DINT, value: x}]}\n",
		"tagreal.yaml":       id + "logix: {tags: [{name: X, type: REAL, value: x}]}\n",
		"tagstruct.yaml":     id + "logix: {tags: [{name: X, type: struct, template: 9}]}\n",
		"structvalue.yaml":   id + "logix: {templates: [{id: 1, name: T, handle: 1, size: 4, members: [{name: A, type: DINT, offset: 0}]}], tags: [{name: X, type: struct, template: 1, value: \"1\"}]}\n",
		"member.yaml":        id + "logix: {templates: [{id: 1, name: T, handle: 1, size: 4, members: [{name: A, type: FLOAT, offset: 0}]}]}\n",
		"template.yaml":      id + "logix: {templates: [{id: 0, name: T, handle: 1, size: 4}]}\n",
		"tagname.yaml":       id + "logix: {tags: [{name: \"bad name\", type: DINT}]}\n",
		"stringlen.yaml":     id + "logix: {tags: [{name: S, type: STRING, value: \"" + strings.Repeat("x", 90) + "\"}]}\n",
		"bad.json":           `{"identity": 1}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := run(t, "adapter", "--config", write(name, content), "--listen", "127.0.0.1:0", "--duration", "1ms", "--format", "json").want(t, 3)
			if kind, code := r.errorCode(t); kind != "invalid_input" || code != "invalid_config" {
				t.Errorf("%s: %s %s", name, kind, code)
			}
		})
	}
	run(t, "adapter", "--config", filepath.Join(dir, "missing.yaml")).want(t, 3)
	run(t, "adapter", "--assembly", "1").want(t, 3)
	run(t, "adapter", "--assembly", "x:input").want(t, 3)
	run(t, "adapter", "--assembly", "1:input:x").want(t, 3)
	run(t, "adapter", "--listen", "::1").want(t, 3)
	run(t, "adapter", "--timeout", "-1s").want(t, 3)
	run(t, "adapter", "--io-address", "nonsense", "--listen", "127.0.0.1:0").want(t, 3)

	// A port that is taken cannot be listened on.
	f := startFixture(t)
	run(t, "adapter", "--listen", f.addr, "--io-address", "127.0.0.1:0", "--duration", "1ms").want(t, 4).errHas(t, "cannot listen")
	// The port of --port when --listen names none.
	run(t, "adapter", "--listen", "127.0.0.1", "--port", strings.Split(f.addr, ":")[1], "--io-address", "127.0.0.1:0", "--duration", "1ms").want(t, 4)
	// Assemblies from flags, without echo: the input stays as it was.
	a := startAdapter(t, "--assembly", "100:input:4", "--assembly", "150:output:4", "--assembly", "152:input-only")
	run(t, "assembly", "write", a.addr, "150", "--data", "01020304", "--yes").want(t, 0)
	run(t, "assembly", "read", a.addr, "100").want(t, 0).has(t, "00 00 00 00")
	// An unwritable stdout is an output error.
	if code := Run(t.Context(), []string{"adapter", "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "1ms"}, failingWriter{}, &syncBuffer{}); code != 10 {
		t.Errorf("unwritable stdout: %d", code)
	}
}
