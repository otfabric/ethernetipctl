// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-ethernetip/implicit"
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
	ctx := t.Context()
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

// forward-open, forward-close and io send the same connection path for the same flags:
// the configuration instance is in it also when --config is 0 (instance 0), so a
// forward-open test says what io will get.
func TestForwardOpenSendsThePathOfIO(t *testing.T) {
	f := startFixture(t)
	const want = "[class 0x04] [instance 0] [point 152] [point 100]"
	r := f.run(t, "io", "ADDR", "--output", "152", "--input", "100", "--output-size", "0", "--input-size", "32",
		"--rpi", "10ms", "--timeout-multiplier", "7", "--io-address", "127.0.0.1:0", "--count", "1").want(t, 0).errHas(t, "path "+want+"\n")

	conn := f.run(t, "forward-open", "ADDR", "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "1s",
		"--timeout-multiplier", "7", "--keep", "--connection-serial", "78", "--originator-serial", "4321", "--format", "json").
		want(t, 0).json(t)["connection"].(map[string]any)
	if conn["path"] != want {
		t.Errorf("forward-open path %v, io path %s", conn["path"], r.stderr)
	}
	m := f.run(t, "forward-close", "ADDR", "--connection-serial", "78", "--originator-serial", "4321",
		"--output", "152", "--input", "100", "--format", "json").want(t, 0).json(t)
	if m["path"] != want {
		t.Errorf("forward-close path %v, want %s", m["path"], want)
	}
	if n := len(f.srv.Connections()); n != 0 {
		t.Errorf("%d connections left open", n)
	}
}

// --config-data is written to the configuration assembly: forward-open and io send it only
// with --yes. Without it they exit 2 (confirmation_required) before any connection.
func TestConfigDataNeedsYes(t *testing.T) {
	f := startFixture(t)
	cfg := f.srv.Assembly(151)
	zeros := strings.Repeat("00", 10)
	open := []string{"forward-open", "ADDR", "--config", "151", "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "100ms",
		"--config-data", "0102030405060708090a"}
	io := []string{"io", "ADDR", "--config", "151", "--output", "152", "--input", "100", "--output-size", "0", "--input-size", "32",
		"--rpi", "10ms", "--timeout-multiplier", "7", "--io-address", "127.0.0.1:0", "--count", "1", "--config-data", "a1a2a3a4a5a6a7a8a9aa"}

	for _, args := range [][]string{open, io} {
		connects := f.metrics.connects.Load()
		r := f.run(t, append(args, "--format", "json")...).want(t, 2)
		if kind, code := r.errorCode(t); kind != "usage" || code != "confirmation_required" || !strings.Contains(r.stdout, "--config-data changes the target") {
			t.Errorf("%s without --yes: %s", args[0], r.stdout)
		}
		if n := f.metrics.connects.Load() - connects; n != 0 || hex.EncodeToString(cfg.Data()) != zeros {
			t.Errorf("%s without --yes: %d connection(s), configuration %x", args[0], n, cfg.Data())
		}
	}

	f.run(t, append(open, "--yes")...).want(t, 0).has(t, "closed again")
	if got := hex.EncodeToString(cfg.Data()); got != "0102030405060708090a" {
		t.Errorf("forward-open --yes: configuration %s", got)
	}
	f.run(t, append(io, "--yes")...).want(t, 0)
	if got := hex.EncodeToString(cfg.Data()); got != "a1a2a3a4a5a6a7a8a9aa" {
		t.Errorf("io --yes: configuration %s", got)
	}
	// Without --config-data nothing changes: no --yes needed, and --yes is harmless.
	noData := open[: len(open)-2 : len(open)-2]
	f.run(t, noData...).want(t, 0)
	f.run(t, append(noData, "--yes")...).want(t, 0)
	run(t, "forward-open", "--help").want(t, 0).has(t, "--yes", "needs --yes")
}

// TestIOStartsWithOutputAndRunState: the first datagram the target
// consumes already carries --output-data, in idle with --idle; no datagram
// with zeros in run comes first.
func TestIOStartsWithOutputAndRunState(t *testing.T) {
	data := strings.Repeat("A5", 32)
	want, _ := hex.DecodeString(data)
	for _, idle := range []bool{true, false} {
		f := startFixture(t)
		var mu sync.Mutex
		var first []byte
		var firstRun, seen bool
		f.out.OnWrite(func(d []byte, run bool) {
			mu.Lock()
			defer mu.Unlock()
			if !seen {
				first, firstRun, seen = append([]byte(nil), d...), run, true
			}
		})
		extra := []string{"--yes", "--output-data", data, "--duration", "50ms"}
		if idle {
			extra = append(extra, "--idle")
		}
		f.run(t, ioArgs("150", "32", extra...)...).want(t, 0)
		mu.Lock()
		if !seen || !bytes.Equal(first, want) || firstRun == idle {
			t.Errorf("idle %v: first datagram %x run %v (seen %v)", idle, first, firstRun, seen)
		}
		mu.Unlock()
	}
}

// TestIOMulticastInterfaceRefused: --multicast-interface needs --multicast and an
// interface of this host, by name or IPv4 address; refused before anything is sent.
func TestIOMulticastInterfaceRefused(t *testing.T) {
	f := startFixture(t)
	for _, extra := range [][]string{
		{"--multicast-interface", "lo0"},
		{"--multicast", "--multicast-interface", "no-such-interface-x"},
		{"--multicast", "--multicast-interface", "::1"},
		{"--multicast", "--multicast-interface", "192.0.2.254"},
	} {
		f.run(t, ioArgs("152", "0", extra...)...).want(t, 3).errHas(t, "--multicast")
	}
	if n := len(f.srv.Connections()); n != 0 {
		t.Errorf("%d connections opened", n)
	}
}

// TestIOMulticastInterface: inputs by multicast arrive on the interface the flag names,
// the loopback one here, by name and by address, where the system's choice is another.
func TestIOMulticastInterface(t *testing.T) {
	lo := loopbackMulticast(t)
	f := startFixture(t)
	must(t, f.in.SetData(bytes.Repeat([]byte{0x5C}, 32)))
	for _, ifname := range []string{lo.Name, "127.0.0.1"} {
		args := ioArgs("152", "0", "--multicast", "--multicast-interface", ifname, "--duration", "300ms", "--format", "json")
		args = append(args, "--io-address", "0.0.0.0:0") // the last one wins
		r := f.run(t, args...).want(t, 0)
		lines := jsonLines(t, r.stdout)
		if len(lines) == 0 {
			t.Errorf("%s: no sample\n%s", ifname, r.stderr)
		}
		for _, l := range lines {
			if l["data"] != strings.Repeat("5c", 32) {
				t.Errorf("%s: sample %v", ifname, l)
			}
		}
	}
}

// loopbackMulticast returns the loopback interface where multicast works on it: a group
// joined there receives what an endpoint bound to 127.0.0.1 sends to it. Skips the test
// elsewhere (Linux runners often have no multicast on lo).
func loopbackMulticast(t *testing.T) *net.Interface {
	t.Helper()
	var lo *net.Interface
	ifs, _ := net.Interfaces()
	for i := range ifs {
		if ifs[i].Flags&net.FlagLoopback != 0 && ifs[i].Flags&net.FlagUp != 0 {
			lo = &ifs[i]
			break
		}
	}
	if lo == nil {
		t.Skip("no loopback interface")
	}
	group := netip.MustParseAddr("239.192.254.77")
	rx, err := implicit.Listen("0.0.0.0:0", implicit.WithMulticastInterface(lo))
	if err != nil {
		t.Skipf("loopback multicast: %v", err)
	}
	defer func() { _ = rx.Close() }()
	tx, err := implicit.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Close() }()
	got := make(chan struct{}, 1)
	in, err := rx.Open(implicit.Config{ConsumedID: 1, Group: group, OnData: func([]byte, bool) {
		select {
		case got <- struct{}{}:
		default:
		}
	}})
	if err != nil {
		t.Skipf("loopback multicast: %v", err)
	}
	defer in.Close()
	out, err := tx.Open(implicit.Config{ProducedID: 1, ProduceRPI: 10 * time.Millisecond,
		Destination: netip.AddrPortFrom(group, rx.LocalAddr().Port())})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	select {
	case <-got:
		return lo
	case <-time.After(time.Second):
		t.Skip("multicast is not delivered on the loopback interface here")
		return nil
	}
}

// TestIOAndForwardOpenElectronicKey: --key goes first in the connection path; the target
// accepts its own key, a key with fields at 0, and a compatible one with a lower minor,
// and refuses another device's (exit 6, the connection manager's status).
func TestIOAndForwardOpenElectronicKey(t *testing.T) {
	f := startFixture(t) // vendor 1, device type 12, product 42, revision 3.7
	open := func(extra ...string) []string {
		return append([]string{"forward-open", "ADDR", "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "100ms", "--format", "json"}, extra...)
	}
	m := f.run(t, open("--key", "1/12/42/3.7")...).want(t, 0).json(t)
	if p := m["connection"].(map[string]any)["path"].(string); !strings.HasPrefix(p, "[key 1/12/42 3.7") {
		t.Errorf("path %s", p)
	}
	f.run(t, open("--key", "0/0/0/0.0")...).want(t, 0)
	f.run(t, open("--key", "0x1/0xC/42/3.5", "--key-compatible")...).want(t, 0)
	f.run(t, open("--key", "1/12/42/3.5")...).want(t, 6)
	f.run(t, open("--key", "7/12/42/3.7")...).want(t, 6).has(t, "0x0114 (vendor ID or product code mismatch)")
	f.run(t, open("--key", "1/13/42/3.7")...).want(t, 6).has(t, "0x0115")
	f.run(t, ioArgs("152", "0", "--key", "1/12/42/3.7", "--count", "1")...).want(t, 0).errHas(t, "[key 1/12/42 3.7")
	f.run(t, ioArgs("152", "0", "--key", "1/12/43/3.7", "--count", "1")...).want(t, 6)
	for _, bad := range [][]string{
		{"--key", "1/12/42"},
		{"--key", "1/12/42/3"},
		{"--key", "1/12/42/128.0"},
		{"--key", "1/12/65536/3.7"},
		{"--key", "a/12/42/3.7"},
		{"--key-compatible"},
	} {
		f.run(t, open(bad...)...).want(t, 3)
		f.run(t, ioArgs("152", "0", bad...)...).want(t, 3)
	}
	if n := len(f.srv.Connections()); n != 0 {
		t.Errorf("%d connections left open", n)
	}
}

// TestIORoute: io takes --route, as forward-open does, and only from the command line:
// ETHERNETIPCTL_ROUTE is not used, and one line on stderr says so.
func TestIORoute(t *testing.T) {
	f := startFixture(t)
	f.run(t, ioArgs("152", "0", "--route", "1/0", "--count", "1")...).want(t, 0)
	f.run(t, ioArgs("152", "0", "--route", "1/2", "--count", "1")...).want(t, 6)
	f.run(t, ioArgs("152", "0", "--route", "x/y", "--count", "1")...).want(t, 3).errHas(t, "--route")
	// The fixture has no port 2: the variable's route would be refused.
	t.Setenv("ETHERNETIPCTL_ROUTE", "1/2")
	f.run(t, ioArgs("152", "0", "--count", "1")...).want(t, 0).errHas(t, "ETHERNETIPCTL_ROUTE=1/2 is not used")
}
