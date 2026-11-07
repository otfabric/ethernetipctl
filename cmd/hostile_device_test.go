// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/server"
)

// A hostile device: something on the EtherNet/IP port that answers RegisterSession and
// then garbage, oversize replies, stalls, half replies, a trickle or nothing at all. The
// CLI must end with a documented exit code within its timeout, never panic, keep its
// output bounded and its JSON valid.

// encapHeader is the 24-octet encapsulation header.
type encapHeader struct {
	command uint16
	length  uint16
	session uint32
	status  uint32
	context [8]byte
	options uint32
}

func (h encapHeader) encode(data []byte) []byte {
	b := make([]byte, 24, 24+len(data))
	binary.LittleEndian.PutUint16(b[0:], h.command)
	binary.LittleEndian.PutUint16(b[2:], h.length)
	binary.LittleEndian.PutUint32(b[4:], h.session)
	binary.LittleEndian.PutUint32(b[8:], h.status)
	copy(b[12:20], h.context[:])
	binary.LittleEndian.PutUint32(b[20:], h.options)
	return append(b, data...)
}

func decodeEncapHeader(b []byte) encapHeader {
	var h encapHeader
	h.command = binary.LittleEndian.Uint16(b[0:])
	h.length = binary.LittleEndian.Uint16(b[2:])
	h.session = binary.LittleEndian.Uint32(b[4:])
	h.status = binary.LittleEndian.Uint32(b[8:])
	copy(h.context[:], b[12:20])
	h.options = binary.LittleEndian.Uint32(b[20:])
	return h
}

const (
	cmdListIdentity    = 0x0063
	cmdRegisterSession = 0x0065
	cmdSendRRData      = 0x006F
	fakeSession        = 0x11223344
)

// reply returns a packet answering h with data.
func reply(h encapHeader, data []byte) []byte {
	h.length = uint16(len(data))
	if h.command != cmdRegisterSession || h.session == 0 {
		h.session = fakeSession
	}
	return h.encode(data)
}

// rrData wraps a CIP reply in the SendRRData data: interface handle, timeout and the
// common packet format with a null address and an unconnected data item.
func rrData(cipReply []byte) []byte {
	b := make([]byte, 0, 16+len(cipReply))
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 2)
	b = binary.LittleEndian.AppendUint16(b, 0x0000)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint16(b, 0x00B2)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(cipReply)))
	return append(b, cipReply...)
}

// cipReplyTo returns a CIP reply to the request in a SendRRData data.
func cipReplyTo(req []byte, status byte, data []byte) []byte {
	service := byte(0x0E)
	// interface handle (4), timeout (2), item count (2), null item (4), data item header (4)
	if len(req) > 16 {
		service = req[16]
	}
	return append([]byte{service | 0x80, 0, status, 0}, data...)
}

// behaviour answers one request after RegisterSession; it returns false to stop serving
// the connection (which is then closed).
type behaviour func(c net.Conn, h encapHeader, data []byte) bool

// fakeDevice is a TCP listener that speaks just enough EtherNet/IP to register a session
// and then does what its behaviour says.
type fakeDevice struct {
	addr     string
	requests atomic.Int64
}

func startFakeDevice(t *testing.T, b behaviour) *fakeDevice {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	d := &fakeDevice{addr: l.Addr().String()}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var conns []net.Conn
	wg.Go(func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
			wg.Go(func() {
				defer func() { _ = c.Close() }()
				hdr := make([]byte, 24)
				for {
					if _, err := io.ReadFull(c, hdr); err != nil {
						return
					}
					h := decodeEncapHeader(hdr)
					data := make([]byte, h.length)
					if _, err := io.ReadFull(c, data); err != nil {
						return
					}
					if h.command == cmdRegisterSession {
						if _, err := c.Write(reply(h, []byte{1, 0, 0, 0})); err != nil {
							return
						}
						continue
					}
					if h.command == 0x0066 { // UnRegisterSession: no reply
						return
					}
					d.requests.Add(1)
					if !b(c, h, data) {
						return
					}
				}
			})
		}
	})
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		for _, c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return d
}

// hostileRun runs args with a short --timeout and checks what every hostile answer must
// give: an exit code among codes, within the timeout and a margin, and a valid JSON error
// object of the kind the code documents.
func hostileRun(t *testing.T, timeout time.Duration, codes []int, args ...string) result {
	t.Helper()
	start := time.Now()
	r := run(t, append(args, "--timeout", timeout.String(), "--format", "json")...)
	elapsed := time.Since(start)
	if limit := 4*timeout + 2*time.Second; elapsed > limit {
		t.Errorf("took %v with --timeout %v", elapsed, timeout)
	}
	okCode := false
	for _, c := range codes {
		okCode = okCode || r.code == c
	}
	if !okCode {
		t.Fatalf("exit code %d, want one of %v\nstdout:\n%s\nstderr:\n%s", r.code, codes, r.stdout, r.stderr)
	}
	if r.code != 0 {
		kind, code := r.errorCode(t)
		want := map[int][]string{4: {"transport"}, 5: {"timeout"}, 6: {"refused", "protocol"}, 8: {"unsupported"}, 3: {"invalid_input"}}[r.code]
		found := false
		for _, k := range want {
			found = found || k == kind
		}
		if !found {
			t.Errorf("exit %d with kind %s (%s)", r.code, kind, code)
		}
		if strings.Contains(r.stderr, "panic") || strings.Contains(r.stdout, "goroutine ") {
			t.Errorf("panic:\n%s%s", r.stdout, r.stderr)
		}
	}
	return r
}

func TestHostileDeviceReplies(t *testing.T) {
	const timeout = 300 * time.Millisecond
	rnd := rand.New(rand.NewPCG(1, 2))
	garbage := make([]byte, 512)
	for i := range garbage {
		garbage[i] = byte(rnd.UintN(256))
	}
	for _, c := range []struct {
		name  string
		b     behaviour
		codes []int
	}{
		{"garbage", func(c net.Conn, _ encapHeader, _ []byte) bool { _, _ = c.Write(garbage); return true }, []int{4, 5, 6}},
		{"zeros", func(c net.Conn, _ encapHeader, _ []byte) bool { _, _ = c.Write(make([]byte, 4096)); return true }, []int{4, 5, 6}},
		{"stall", func(net.Conn, encapHeader, []byte) bool { return true }, []int{5}},
		{"close", func(net.Conn, encapHeader, []byte) bool { return false }, []int{4}},
		{"half-header", func(c net.Conn, h encapHeader, _ []byte) bool {
			_, _ = c.Write(reply(h, nil)[:10])
			return false
		}, []int{4}},
		{"half-data", func(c net.Conn, h encapHeader, req []byte) bool {
			p := reply(h, rrData(cipReplyTo(req, 0, []byte{1, 2, 3, 4})))
			_, _ = c.Write(p[:len(p)-3])
			return false
		}, []int{4}},
		{"header-promises-64k", func(c net.Conn, h encapHeader, _ []byte) bool {
			h.length = 0xFFFF
			h.session = fakeSession
			_, _ = c.Write(h.encode(nil))
			return true
		}, []int{4, 5, 6}},
		{"oversize-64k", func(c net.Conn, h encapHeader, _ []byte) bool {
			h.length = 0xFFFF
			h.session = fakeSession
			_, _ = c.Write(h.encode(make([]byte, 0xFFFF)))
			return true
		}, []int{4, 5, 6}},
		{"trickle", func(c net.Conn, h encapHeader, req []byte) bool {
			// A full reply, one octet every 40 ms: it would take longer than the
			// timeout many times over.
			p := reply(h, rrData(cipReplyTo(req, 0, make([]byte, 200))))
			for _, b := range p {
				if _, err := c.Write([]byte{b}); err != nil {
					return false
				}
				time.Sleep(40 * time.Millisecond)
			}
			return true
		}, []int{4, 5}},
		{"wrong-context", func(c net.Conn, h encapHeader, req []byte) bool {
			h.context[0] ^= 0xFF
			_, _ = c.Write(reply(h, rrData(cipReplyTo(req, 0, []byte{1, 0}))))
			return true
		}, []int{4, 5, 6}},
		{"wrong-session", func(c net.Conn, h encapHeader, req []byte) bool {
			p := reply(h, rrData(cipReplyTo(req, 0, []byte{1, 0})))
			binary.LittleEndian.PutUint32(p[4:], 0xDEADBEEF)
			_, _ = c.Write(p)
			return true
		}, []int{4, 5, 6}},
		{"wrong-command", func(c net.Conn, h encapHeader, req []byte) bool {
			h.command = 0x0070
			_, _ = c.Write(reply(h, rrData(cipReplyTo(req, 0, []byte{1, 0}))))
			return true
		}, []int{4, 5, 6}},
		{"encap-status", func(c net.Conn, h encapHeader, _ []byte) bool {
			h.status = 0x64 // invalid session handle
			_, _ = c.Write(reply(h, nil))
			return true
		}, []int{6, 8}},
		{"encap-status-unknown", func(c net.Conn, h encapHeader, _ []byte) bool {
			h.status = 0xFFFFFFFF
			_, _ = c.Write(reply(h, nil))
			return true
		}, []int{6, 8}},
		{"wrong-service", func(c net.Conn, h encapHeader, _ []byte) bool {
			_, _ = c.Write(reply(h, rrData([]byte{0xCC, 0, 0, 0, 1, 0})))
			return true
		}, []int{4, 5, 6}},
		{"reply-bit-missing", func(c net.Conn, h encapHeader, req []byte) bool {
			p := cipReplyTo(req, 0, []byte{1, 0})
			p[0] &^= 0x80
			_, _ = c.Write(reply(h, rrData(p)))
			return true
		}, []int{4, 5, 6}},
		{"extended-status-overruns", func(c net.Conn, h encapHeader, req []byte) bool {
			p := cipReplyTo(req, 0x1F, nil)
			p[3] = 0xFF // 255 words of extended status, none sent
			_, _ = c.Write(reply(h, rrData(p)))
			return true
		}, []int{6}},
		{"cpf-item-count-lies", func(c net.Conn, h encapHeader, req []byte) bool {
			d := rrData(cipReplyTo(req, 0, []byte{1, 0}))
			binary.LittleEndian.PutUint16(d[6:], 0xFFFF)
			_, _ = c.Write(reply(h, d))
			return true
		}, []int{4, 5, 6}},
		{"cpf-item-length-lies", func(c net.Conn, h encapHeader, req []byte) bool {
			d := rrData(cipReplyTo(req, 0, []byte{1, 0}))
			binary.LittleEndian.PutUint16(d[14:], 0xFFF0)
			_, _ = c.Write(reply(h, d))
			return true
		}, []int{4, 5, 6}},
		{"cip-error", func(c net.Conn, h encapHeader, req []byte) bool {
			_, _ = c.Write(reply(h, rrData(cipReplyTo(req, 0x08, nil))))
			return true
		}, []int{6, 8}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := startFakeDevice(t, c.b)
			hostileRun(t, timeout, c.codes, "read", d.addr, "@1/1/7", "--type", "UINT")
		})
	}
}

// A large well-formed reply is shown, bounded by what the encapsulation carries.
func TestHostileDeviceLargeValidReply(t *testing.T) {
	data := make([]byte, 0xFFFF-16-4)
	for i := range data {
		data[i] = byte(i)
	}
	d := startFakeDevice(t, func(c net.Conn, h encapHeader, req []byte) bool {
		_, _ = c.Write(reply(h, rrData(cipReplyTo(req, 0, data))))
		return true
	})
	r := hostileRun(t, time.Second, []int{0, 4, 6}, "read", d.addr, "@1/1/7")
	if r.code == 0 {
		m := r.json(t)
		if m["length"] != float64(len(data)) {
			t.Errorf("length %v", m["length"])
		}
	}
	if len(r.stdout) > 4*len(data)+4096 {
		t.Errorf("%d octets of output for %d octets of data", len(r.stdout), len(data))
	}
}

// Every command that talks to one device survives garbage and a dropped connection.
func TestHostileDeviceEveryCommand(t *testing.T) {
	rnd := rand.New(rand.NewPCG(3, 4))
	garbage := make([]byte, 300)
	for i := range garbage {
		garbage[i] = byte(rnd.UintN(256))
	}
	behaviours := map[string]behaviour{
		"garbage": func(c net.Conn, _ encapHeader, _ []byte) bool { _, _ = c.Write(garbage); return true },
		"close":   func(net.Conn, encapHeader, []byte) bool { return false },
		"stall":   func(net.Conn, encapHeader, []byte) bool { return true },
	}
	commands := [][]string{
		{"identify", "ADDR"},
		{"status", "ADDR"},
		{"services", "ADDR"},
		{"interfaces", "ADDR"},
		{"objects", "ADDR"},
		{"object", "ADDR", "identity"},
		{"connections", "ADDR"},
		{"read", "ADDR", "@1/1", "--all"},
		{"read", "ADDR", "@1/1/7", "--connected"},
		{"write", "ADDR", "@0xF5/1/8", "--data", "01", "--yes"},
		{"reset", "ADDR", "--yes"},
		{"session", "ADDR", "--connected"},
		{"forward-open", "ADDR", "--output", "152", "--input", "100", "--input-size", "4", "--rpi", "10ms"},
		{"forward-close", "ADDR", "--connection-serial", "1", "--originator-serial", "1", "--output", "152", "--input", "100"},
		{"assembly", "read", "ADDR", "100"},
		{"tag", "read", "ADDR", "Count"},
		{"tag", "decode", "ADDR", "Motors"},
		{"tag", "list", "ADDR"},
		{"tag", "types", "ADDR", "0x123"},
		{"tag", "write", "ADDR", "Count", "--type", "DINT", "--value", "1", "--yes"},
		{"eds", "generate", "--from-device", "ADDR"},
	}
	for name, b := range behaviours {
		d := startFakeDevice(t, b)
		for _, args := range commands {
			full := make([]string, len(args))
			for i, a := range args {
				full[i] = strings.ReplaceAll(a, "ADDR", d.addr)
			}
			t.Run(name+"/"+strings.Join(args[:min(len(args), 2)], " "), func(t *testing.T) {
				t.Parallel()
				hostileRun(t, 200*time.Millisecond, []int{4, 5, 6, 8}, full...)
			})
		}
	}
	// diagnose writes its report whatever the device does.
	for name, b := range behaviours {
		d := startFakeDevice(t, b)
		r := run(t, "diagnose", d.addr, "--timeout", "200ms", "--format", "json")
		if r.code != 1 {
			t.Errorf("diagnose against %s: exit %d, want 1\n%s", name, r.code, r.stdout)
		}
		_ = r.json(t)
	}
}

// partialSymbols answers every Logix read with a partial transfer of a structure that
// never ends: the client must give up, not loop or grow without bound.
type partialSymbols struct{ calls atomic.Int64 }

func (p *partialSymbols) ServeSymbol(_ context.Context, r *server.Request) cip.Response {
	p.calls.Add(1)
	data := []byte{0xA0, 0x02, 0xEF, 0xBE} // a structure, handle 0xBEEF
	data = append(data, make([]byte, 400)...)
	return cip.Response{Service: r.Service, Status: cip.StatusPartialTransfer, Data: data}
}

func TestHostileDeviceEndlessFragmentedRead(t *testing.T) {
	if testing.Short() {
		t.Skip("the client reads up to its fragment limit")
	}
	srv, err := server.New(server.WithIdentity(testIdentity))
	must(t, err)
	p := &partialSymbols{}
	srv.HandleSymbols(p)
	must(t, srv.Listen("127.0.0.1:0"))
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })
	start := time.Now()
	r := run(t, "tag", "read", srv.Addr().String(), "Endless", "--timeout", "2s", "--format", "json")
	t.Logf("%d requests, %v", p.calls.Load(), time.Since(start))
	if r.code != 6 {
		t.Fatalf("exit %d, want 6 (protocol)\n%s", r.code, r.stdout)
	}
	if kind, code := r.errorCode(t); kind != "protocol" || code != "protocol_violation" {
		t.Errorf("%s %s", kind, code)
	}
}

// A partial transfer of an atomic tag that sends one octet at a time ends when the reply
// passes the size of the elements.
type trickleSymbols struct{ calls atomic.Int64 }

func (p *trickleSymbols) ServeSymbol(_ context.Context, r *server.Request) cip.Response {
	p.calls.Add(1)
	return cip.Response{Service: r.Service, Status: cip.StatusPartialTransfer, Data: []byte{0xC4, 0x00, 0x01}}
}

func TestHostileDeviceTrickledAtomicRead(t *testing.T) {
	srv, err := server.New(server.WithIdentity(testIdentity))
	must(t, err)
	p := &trickleSymbols{}
	srv.HandleSymbols(p)
	must(t, srv.Listen("127.0.0.1:0"))
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })
	r := hostileRun(t, time.Second, []int{6}, "tag", "read", srv.Addr().String(), "Count", "--count", "2")
	_ = r
	if n := p.calls.Load(); n > 16 {
		t.Errorf("%d requests for 8 octets", n)
	}
}

// listIdentityReply captures the fixture's answer to ListIdentity over UDP: a real
// identity item to replay with another sender context.
func listIdentityReply(t *testing.T, f *fixture) []byte {
	t.Helper()
	c, err := net.Dial("udp4", f.srv.UDPAddr().String())
	must(t, err)
	defer func() { _ = c.Close() }()
	_, err = c.Write(encapHeader{command: cmdListIdentity}.encode(nil))
	must(t, err)
	must(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1500)
	n, err := c.Read(buf)
	must(t, err)
	return append([]byte(nil), buf[:n]...)
}

// A discover answered by a flood: garbage, replies to another request, and many devices.
// The result keeps to --max-devices, ends within --wait, and stays valid JSON.
func TestHostileDiscoverFlood(t *testing.T) {
	f := startFixture(t)
	f.requireUDP(t)
	template := listIdentityReply(t, f)
	dev, err := net.ListenPacket("udp4", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { _ = dev.Close() })
	const responders = 40
	var socks []net.PacketConn
	for range responders {
		s, err := net.ListenPacket("udp4", "127.0.0.1:0")
		must(t, err)
		socks = append(socks, s)
	}
	t.Cleanup(func() {
		for _, s := range socks {
			_ = s.Close()
		}
	})
	rnd := rand.New(rand.NewPCG(5, 6))
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1500)
		n, from, err := dev.ReadFrom(buf)
		if err != nil || n < 24 {
			return
		}
		ctx := buf[12:20]
		junk := make([]byte, 1400)
		for i := 0; i < 500; i++ {
			for j := range junk[:rnd.IntN(len(junk))] {
				junk[j] = byte(rnd.UintN(256))
			}
			_, _ = dev.WriteTo(junk[:rnd.IntN(len(junk))], from)
			if i%50 == 0 {
				// A reply to another request: wrong sender context.
				other := append([]byte(nil), template...)
				other[12] ^= 0xFF
				_, _ = dev.WriteTo(other, from)
				// A reply whose header promises more than the datagram holds.
				lie := append([]byte(nil), template...)
				binary.LittleEndian.PutUint16(lie[2:], 0xFFFF)
				copy(lie[12:20], ctx)
				_, _ = dev.WriteTo(lie, from)
			}
		}
		good := append([]byte(nil), template...)
		copy(good[12:20], ctx)
		for _, s := range socks {
			for range 5 { // duplicates of each device
				_, _ = s.WriteTo(good, from)
			}
		}
	}()
	start := time.Now()
	r := run(t, "discover", "--address", dev.LocalAddr().String(), "--wait", "2s", "--max-devices", "10", "--format", "json").want(t, 0)
	m := r.json(t)
	if m["found"] != 10.0 {
		t.Errorf("found %v, want 10 (--max-devices)", m["found"])
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("discover took %v", d)
	}
	<-done

	// Without --max-devices the duplicates are counted once.
	dev2, err := net.ListenPacket("udp4", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { _ = dev2.Close() })
	go func() {
		buf := make([]byte, 1500)
		n, from, err := dev2.ReadFrom(buf)
		if err != nil || n < 24 {
			return
		}
		good := append([]byte(nil), template...)
		copy(good[12:20], buf[12:20])
		for range 200 {
			_, _ = dev2.WriteTo(good, from)
		}
	}()
	m = run(t, "discover", "--address", dev2.LocalAddr().String(), "--wait", "300ms", "--format", "json").want(t, 0).json(t)
	if m["found"] != 1.0 {
		t.Errorf("200 replies of one device: found %v", m["found"])
	}
}

// A ListIdentity over TCP that is not one: identify and status fail with a protocol
// error, never a panic.
func TestHostileListIdentity(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":           nil,
		"no-items":        {0, 0},
		"count-lies":      {0xFF, 0xFF},
		"short-item":      {1, 0, 0x0C, 0, 0xFF, 0x00, 1, 2},
		"item-length-lie": {1, 0, 0x0C, 0, 0xF0, 0xFF, 1, 0},
		"wrong-item-type": {1, 0, 0x34, 0x12, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			d := startFakeDevice(t, func(c net.Conn, h encapHeader, _ []byte) bool {
				_, _ = c.Write(reply(h, data))
				return true
			})
			for _, cmd := range []string{"identify", "status"} {
				hostileRun(t, 300*time.Millisecond, []int{4, 5, 6}, cmd, d.addr)
			}
		})
	}
}

// An adapter that speaks the protocol but drops each connection after N octets: the CLI
// ends with transport (4) or protocol (6), and the JSON error object stays valid.
func TestHostileDeviceDropsAfterNOctets(t *testing.T) {
	f := startFixture(t)
	for _, n := range []int{1, 23, 24, 27, 28, 40, 60} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			p := startProxy(t, f.addr, func(dir string, b []byte, sent int) ([]byte, bool) {
				if dir == "down" && sent+len(b) >= n {
					return b[:n-sent], false
				}
				return b, true
			})
			hostileRun(t, 500*time.Millisecond, []int{4, 5, 6}, "read", p.addr, "@1/1/7")
		})
	}
}
