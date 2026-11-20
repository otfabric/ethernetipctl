// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
		for line := range strings.SplitSeq(a.stdout.String(), "\n") {
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
	// The tag of an I/O module: one controller-scoped symbol with colons.
	run(t, "tag", "list", a.addr).want(t, 0).has(t, "Local:2:O[2]")
	run(t, "tag", "read", a.addr, "Local:2:O[1]").want(t, 0).has(t, "[0] 20")
	run(t, "tag", "write", a.addr, "Local:2:O[0]", "--type", "DINT", "--value", "-3", "--yes").want(t, 0)
	run(t, "tag", "write", a.addr, "Local:2:O[1].3", "--type", "DINT", "--value", "1", "--yes").want(t, 0)
	run(t, "tag", "read", a.addr, "Local:2:O", "--count", "2").want(t, 0).has(t, "[0] -3", "[1] 28")
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

// stalledWriter takes the first write and holds every later one until release is closed:
// a stdout that stops reading.
type stalledWriter struct {
	mu      sync.Mutex
	writes  int
	first   chan string
	release chan struct{}
	buf     syncBuffer
}

func (w *stalledWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes++
	n := w.writes
	w.mu.Unlock()
	if n == 1 {
		w.first <- string(p)
	} else {
		<-w.release
	}
	return w.buf.Write(p)
}

// While stdout takes no events the adapter goes on serving, and it stops when it is
// interrupted; the events wait for stdout, which gets them once it reads again.
func TestAdapterServesWhileStdoutStalls(t *testing.T) {
	w := &stalledWriter{first: make(chan string, 1), release: make(chan struct{})}
	released := false
	defer func() {
		if !released {
			close(w.release)
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan int, 1)
	var stderr syncBuffer
	go func() {
		done <- Run(ctx, []string{"adapter", "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--format", "json", "--requests"}, w, &stderr)
	}()
	var ev map[string]any
	if err := json.Unmarshal([]byte(<-w.first), &ev); err != nil || ev["event"] != "listening" {
		t.Fatalf("first event %v (%v)", ev, err)
	}
	addr := ev["address"].(string)

	// Stdout now holds every write: the adapter still answers.
	for range 3 {
		run(t, "identify", addr, "--timeout", "2s").want(t, 0).has(t, "ethernetipctl adapter")
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d\n%s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the adapter did not stop while stdout stalled")
	}

	close(w.release)
	released = true
	eventually(t, "the events written once stdout reads again", func() bool { return strings.Contains(w.buf.String(), `"event":"stopping"`) })
	evs := map[string]int{}
	for _, e := range jsonLines(t, w.buf.String()) {
		evs[e["event"].(string)]++
	}
	if evs["connected"] != 3 || evs["request"] == 0 {
		t.Errorf("events %v", evs)
	}
}

// TestAdapterTagAccess: a read-only tag is read but not written, a tag without access is
// neither, both are listed; each refusal is answered with the model's status and logged
// as a tag-refused event, and calls no tag-write.
func TestAdapterTagAccess(t *testing.T) {
	model := filepath.Join(t.TempDir(), "access.yaml")
	must(t, os.WriteFile(model, []byte(`identity: {vendor_id: 1, product_code: 1, revision: "1.1", serial_number: 1, product_name: access}
logix:
  tags:
    - {name: Open, type: DINT, value: "1"}
    - {name: Setpoint, type: DINT, value: "5", access: read-only}
    - {name: Secret, type: DINT, value: "9", access: NONE}
    - {name: "Program:Main.Step", type: INT, value: "3", access: read-only}
`), 0o600))
	a := startAdapter(t, "--config", model)
	run(t, "tag", "list", a.addr).want(t, 0).has(t, "Open", "Setpoint", "Secret")
	run(t, "tag", "list", a.addr, "--program", "Main").want(t, 0).has(t, "Step")
	run(t, "tag", "read", a.addr, "Setpoint").want(t, 0).has(t, "[0] 5")
	run(t, "tag", "write", a.addr, "Setpoint", "--type", "DINT", "--value", "7", "--yes").want(t, 6).errHas(t, "0x0F")
	run(t, "tag", "write", a.addr, "program:main.step", "--type", "INT", "--value", "4", "--yes").want(t, 6)
	run(t, "tag", "read", a.addr, "Setpoint").want(t, 0).has(t, "[0] 5")
	run(t, "tag", "read", a.addr, "Secret").want(t, 6).errHas(t, "0x0F")
	run(t, "tag", "write", a.addr, "Open", "--type", "DINT", "--value", "2", "--yes").want(t, 0)
	eventually(t, "the events", func() bool { ev := a.events(t); return ev["tag-refused"] == 3 && ev["tag-write"] == 1 })
	var refused []string
	for _, ev := range jsonLines(t, a.stdout.String()) {
		if ev["event"] == "tag-refused" {
			refused = append(refused, fmt.Sprintf("%v %v %v", ev["service"], ev["tag"], ev["status"]))
		}
	}
	want := []string{"write Setpoint 0x0F privilege violation", "write Program:Main.Step 0x0F privilege violation", "read Secret 0x0F privilege violation"}
	if strings.Join(refused, "|") != strings.Join(want, "|") {
		t.Errorf("refused %v, want %v", refused, want)
	}

	// The model's status, and an access that does not parse.
	id := "identity: {vendor_id: 1, product_code: 1, revision: \"1.1\", serial_number: 1, product_name: access}\n"
	must(t, os.WriteFile(model, []byte(id+"logix:\n  access_status: 0x10\n  tags: [{name: R, type: DINT, access: read-only}]\n"), 0o600))
	b := startAdapter(t, "--config", model)
	run(t, "tag", "write", b.addr, "R", "--type", "DINT", "--value", "1", "--yes").want(t, 6).errHas(t, "0x10")
	must(t, os.WriteFile(model, []byte(id+"logix:\n  tags: [{name: R, type: DINT, access: write-only}]\n"), 0o600))
	run(t, "adapter", "--config", model, "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "10ms").want(t, 3).errHas(t, "access")
}
