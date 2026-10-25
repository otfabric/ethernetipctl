package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/logix"
	"github.com/otfabric/go-ethernetip/server"

	"github.com/otfabric/ethernetipctl/internal/eip"
)

// syncBuffer is an output buffer that is safe for concurrent writers: streaming commands
// write from the goroutines of the connection.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// result is the outcome of one invocation of ethernetipctl.
type result struct {
	stdout, stderr string
	code           int
}

// run executes ethernetipctl with args, as the process would.
func run(t testing.TB, args ...string) result {
	t.Helper()
	return runCtx(t, context.Background(), args...)
}

func runCtx(t testing.TB, ctx context.Context, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var stdout, stderr syncBuffer
	code := Run(ctx, args, &stdout, &stderr)
	return result{stdout.String(), stderr.String(), code}
}

// want fails the test unless the exit code is code.
func (r result) want(t testing.TB, code int) result {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.stdout, r.stderr)
	}
	return r
}

// has fails the test unless stdout contains every one of subs.
func (r result) has(t testing.TB, subs ...string) result {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("stdout does not contain %q:\n%s", s, r.stdout)
		}
	}
	return r
}

// errHas fails the test unless stderr contains every one of subs.
func (r result) errHas(t testing.TB, subs ...string) result {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("stderr does not contain %q:\n%s", s, r.stderr)
		}
	}
	return r
}

// json parses stdout as one JSON object.
func (r result) json(t testing.TB) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, r.stdout)
	}
	return m
}

// lines parses stdout as JSON Lines.
func (r result) lines(t testing.TB) []map[string]any {
	t.Helper()
	return jsonLines(t, r.stdout)
}

func jsonLines(t testing.TB, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("stdout line is not JSON: %v\n%s", err, line)
		}
		out = append(out, m)
	}
	return out
}

// errorCode returns the kind and code of the JSON error object on stdout.
func (r result) errorCode(t testing.TB) (kind, code string) {
	t.Helper()
	var env struct {
		Error struct{ Kind, Code, Message string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil || env.Error.Code == "" {
		t.Fatalf("stdout is not a JSON error object: %v\n%s", err, r.stdout)
	}
	return env.Error.Kind, env.Error.Code
}

// eventually polls cond for up to ten seconds.
func eventually(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// The identity of the test adapter.
var testIdentity = cip.Identity{
	VendorID:     1,
	DeviceType:   cip.DeviceTypeCommunicationsAdapter,
	ProductCode:  42,
	Revision:     cip.Revision{Major: 3, Minor: 7},
	SerialNumber: 0x12345678,
	ProductName:  "test adapter",
	State:        3,
}

// counters counts what the test adapter saw: sessions and requests.
type counters struct {
	ethernetip.NopMetrics
	connects atomic.Int64
	requests atomic.Int64
	sets     atomic.Int64
}

func (c *counters) OnConnect(net.Addr) { c.connects.Add(1) }

func (c *counters) OnHandled(_ net.Addr, s cip.Service, _ cip.GeneralStatus, _ time.Duration) {
	c.requests.Add(1)
	if s == cip.ServiceSetAttributeSingle || s == cip.ServiceReset {
		c.sets.Add(1)
	}
}

func (c *counters) OnConnectionOpened(net.Addr, ethernetip.ConnectionInfo)        {}
func (c *counters) OnConnectionClosed(net.Addr, ethernetip.ConnectionInfo, error) {}

// fixture is a go-ethernetip server on loopback: OpENer's sample assemblies (input 100,
// output 150 echoed to input, configuration 151, heartbeats 152 and 153) and a Logix tag
// database.
type fixture struct {
	srv     *server.Server
	addr    string
	in, out *server.Assembly
	tags    *logix.TagServer
	metrics *counters
	resets  atomic.Int64
}

// motor is the UDT of the fixture's tag database.
var motor = logix.Template{
	ID: 0x123, Name: "Motor", Handle: 0xBEEF, Size: 112,
	Members: []logix.Member{
		{Name: "Speed", Type: logix.NewAtomicType(cip.TypeREAL, 0, 0), Offset: 0},
		{Name: "Count", Type: logix.NewAtomicType(cip.TypeDINT, 1, 0), Info: 4, Offset: 4},
		{Name: "ZZZZZZZZZZMotor0", Type: logix.NewAtomicType(cip.TypeSINT, 0, 0), Offset: 20},
		{Name: "Running", Type: logix.NewAtomicType(cip.TypeBOOL, 0, 0), Info: 0, Offset: 20},
		{Name: "Name", Type: logix.NewStructType(logix.StringTemplate.ID, 0), Offset: 24},
	},
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// startFixture starts the test adapter.
func startFixture(t testing.TB, opts ...server.Option) *fixture {
	t.Helper()
	f := &fixture{metrics: &counters{}}
	opts = append([]server.Option{
		server.WithIdentity(testIdentity),
		server.WithIOAddress("127.0.0.1:0"),
		server.WithMetrics(f.metrics),
		server.WithResetHandler(func(uint8) cip.GeneralStatus { f.resets.Add(1); return cip.StatusSuccess }),
	}, opts...)
	srv, err := server.New(opts...)
	must(t, err)
	f.srv = srv
	f.in, err = srv.AddAssembly(100, server.AssemblyInput, 32)
	must(t, err)
	f.out, err = srv.AddAssembly(150, server.AssemblyOutput, 32)
	must(t, err)
	_, err = srv.AddAssembly(151, server.AssemblyConfig, 10)
	must(t, err)
	_, err = srv.AddAssembly(152, server.AssemblyInputOnly, 0)
	must(t, err)
	_, err = srv.AddAssembly(153, server.AssemblyListenOnly, 0)
	must(t, err)
	f.out.OnWrite(func(data []byte, _ bool) { _ = f.in.SetData(data) })

	ts := logix.NewTagServer(logix.WithProgramName("test-project"))
	must(t, ts.AddTemplate(motor))
	must(t, ts.AddTag("Count", cip.TypeDINT))
	must(t, ts.AddTag("Flags", cip.TypeDINT))
	must(t, ts.AddTag("Speed", cip.TypeREAL))
	must(t, ts.AddTag("Big", cip.TypeULINT))
	must(t, ts.AddTag("Values", cip.TypeINT, 10))
	must(t, ts.AddTag("Bits", cip.TypeBOOL, 32))
	must(t, ts.AddTag("On", cip.TypeBOOL))
	must(t, ts.AddStructTag("Name", logix.StringTemplate.ID))
	must(t, ts.AddStructTag("Motors", motor.ID, 2))
	must(t, ts.AddTag("Program:Main.Local", cip.TypeSINT))
	v, _ := logix.Ints(cip.TypeDINT, 42)
	must(t, ts.Set("Count", v))
	s, _ := logix.Strings("line 1")
	must(t, ts.Set("Name", s))
	ts.Attach(srv)
	f.tags = ts

	must(t, srv.Listen("127.0.0.1:0"))
	f.addr = srv.Addr().String()
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.Close()
		select {
		case err := <-done:
			if !errors.Is(err, ethernetip.ErrClosed) {
				t.Errorf("Serve returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after Close")
		}
	})
	return f
}

// cli runs ethernetipctl with the fixture's address after the command words: the words
// are those of args up to the first that starts with "-" or "@", or is "ADDR".
func (f *fixture) run(t testing.TB, args ...string) result {
	t.Helper()
	full := make([]string, 0, len(args)+3)
	for _, a := range args {
		if a == "ADDR" {
			a = f.addr
		}
		full = append(full, a)
	}
	return run(t, append(full, "--timeout", "5s")...)
}

// silentListener accepts TCP connections and never answers.
func silentListener(t testing.TB) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return l.Addr().String()
}

// refusedAddr returns an address nobody listens on.
func refusedAddr(t testing.TB) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	a := l.Addr().String()
	_ = l.Close()
	return a
}

// encodeForTest encodes values as the write command does, as hex.
func encodeForTest(typ, value string) (string, error) {
	dt, err := eip.ParseType(typ)
	if err != nil {
		return "", err
	}
	b, err := eip.Encode(dt, value)
	return eip.Hex(b), err
}

// udpFreePort returns a UDP port nobody listens on.
func udpFreePort(t testing.TB) int {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	must(t, err)
	p := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return p
}
