// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// ioArgs are the flags of an I/O connection to the fixture, with a free I/O port.
func ioArgs(output, outputSize string, extra ...string) []string {
	args := []string{"io", "ADDR", "--config", "151", "--output", output, "--input", "100", "--output-size", outputSize,
		"--input-size", "32", "--rpi", "10ms", "--io-address", "127.0.0.1:0"}
	return append(args, extra...)
}

func TestIOInputOnly(t *testing.T) {
	f := startFixture(t)
	must(t, f.in.SetData(bytes.Repeat([]byte{7}, 32)))
	r := f.run(t, ioArgs("152", "0", "--count", "1")...).want(t, 0)
	r.has(t, "#1", "run ", strings.Repeat("07 ", 31)+"07").errHas(t, "I/O connection to", "1 sample(s) written")
	// Input only writes nothing: no --yes needed, and the output assembly is untouched.
	if f.out.Run() {
		t.Error("the output assembly was written")
	}
	f.run(t, ioArgs("152", "0", "--count", "1", "--format", "table")...).want(t, 0).has(t, "RECEIVED", "SEQ", "07070707")
	lines := f.run(t, ioArgs("152", "0", "--count", "1", "--format", "json", "--changes")...).want(t, 0).lines(t)
	if len(lines) != 1 || lines[0]["seq"] != 1.0 || lines[0]["run"] != true || lines[0]["size"] != 32.0 || lines[0]["data"] != strings.Repeat("07", 32) {
		t.Errorf("io json: %v", lines)
	}
	// --duration ends the stream normally.
	f.run(t, ioArgs("152", "0", "--duration", "50ms")...).want(t, 0).errHas(t, "sample(s) written")
}

func TestIOOutputNeedsYes(t *testing.T) {
	f := startFixture(t)
	r := f.run(t, append(ioArgs("150", "32"), "--format", "json")...).want(t, 2)
	if kind, code := r.errorCode(t); kind != "usage" || code != "confirmation_required" {
		t.Errorf("io without --yes: %s %s", kind, code)
	}
	f.run(t, ioArgs("152", "0", "--output-data", "00")...).want(t, 3)
	if f.metrics.connects.Load() != 0 {
		t.Fatal("io without --yes opened a session")
	}
	for _, extra := range [][]string{
		{"--yes", "--output-data", "0102"},
		{"--yes", "--output-data", "zz"},
		{"--yes", "--rpi", "0s"},
		{"--yes", "--timeout-multiplier", "9"},
	} {
		f.run(t, ioArgs("150", "32", extra...)...).want(t, 3)
	}
	f.run(t, ioArgs("150", "32", "--yes", "--io-address", "nonsense")...).want(t, 4).errHas(t, "I/O endpoint")
}

func TestIOOutputEchoed(t *testing.T) {
	f := startFixture(t)
	data := strings.Repeat("0102030405060708", 4)
	// The fixture copies the output to the input: a sample carries the output data.
	args := ioArgs("150", "32", "--yes", "--output-data", data, "--changes", "--format", "json")
	args[1] = f.addr
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() { done <- Run(ctx, args, &stdout, &stderr) }()
	eventually(t, "the echoed output", func() bool { return strings.Contains(stdout.String(), data) })
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("io: exit %d\n%s", code, stderr.String())
	}
	for _, l := range jsonLines(t, stdout.String()) {
		if l["size"] != 32.0 || l["run"] != true {
			t.Errorf("sample %v", l)
		}
	}
	if !bytes.Equal(f.out.Data()[:2], []byte{1, 2}) {
		t.Errorf("output assembly %x", f.out.Data())
	}
	// --idle sends the output in idle: the adapter's output is not in run.
	f.run(t, ioArgs("150", "32", "--yes", "--idle", "--duration", "50ms")...).want(t, 0)
	// A refused Forward_Open (wrong size) is refused.
	f.run(t, ioArgs("150", "31", "--yes", "--count", "1")...).want(t, 6)
	refused := ioArgs("150", "32", "--yes")
	refused[1] = refusedAddr(t)
	f.run(t, refused...).want(t, 4)
}

func TestIOTimeout(t *testing.T) {
	f := startFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan result, 1)
	var stdout, stderr syncBuffer
	args := ioArgs("152", "0", "--timeout", "5s")
	args[1] = f.addr
	go func() {
		code := Run(ctx, args, &stdout, &stderr)
		done <- result{stdout.String(), stderr.String(), code}
	}()
	eventually(t, "the first sample", func() bool { return strings.Contains(stdout.String(), "#1") })
	// The adapter goes away: its input stops, and the connection times out.
	_ = f.srv.Close()
	r := <-done
	r.want(t, 5).errHas(t, "error: timeout")

	// An interrupted stream ends normally.
	g := startFixture(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	args[1] = g.addr
	var out2, err2 syncBuffer
	done2 := make(chan int, 1)
	go func() { done2 <- Run(ctx2, args, &out2, &err2) }()
	eventually(t, "the first sample", func() bool { return strings.Contains(out2.String(), "#1") })
	cancel2()
	if code := <-done2; code != 0 {
		t.Errorf("interrupted io: exit %d\n%s", code, err2.String())
	}
}
