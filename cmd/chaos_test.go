// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// Chaos between the CLI and an adapter: a TCP proxy that delays or cuts the traffic, and
// an adapter that goes away while a stream runs.

// proxyFilter sees every chunk read in one direction ("up" from the CLI, "down" from the
// device) with the number of octets already forwarded in that direction; it returns what
// to forward and whether to keep the connection.
type proxyFilter func(dir string, b []byte, sent int) ([]byte, bool)

type proxy struct{ addr string }

// startProxy forwards TCP connections to target through filter.
func startProxy(t *testing.T, target string, filter proxyFilter) *proxy {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var conns []net.Conn
	track := func(c net.Conn) {
		mu.Lock()
		conns = append(conns, c)
		mu.Unlock()
	}
	wg.Go(func() {
		for {
			in, err := l.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp", target)
			if err != nil {
				_ = in.Close()
				continue
			}
			track(in)
			track(out)
			pipe := func(dir string, src, dst net.Conn) {
				defer func() { _ = src.Close(); _ = dst.Close() }()
				buf := make([]byte, 4096)
				sent := 0
				for {
					n, err := src.Read(buf)
					if n > 0 {
						b, keep := filter(dir, buf[:n], sent)
						if len(b) > 0 {
							if _, werr := dst.Write(b); werr != nil {
								return
							}
							sent += len(b)
						}
						if !keep {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}
			wg.Go(func() { pipe("up", in, out) })
			wg.Go(func() { pipe("down", out, in) })
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
	return &proxy{addr: l.Addr().String()}
}

// A delay below the timeout changes nothing; a delay above it is a timeout (5).
func TestChaosProxyDelay(t *testing.T) {
	a := startAdapter(t, "--logix")
	slow := startProxy(t, a.addr, func(dir string, b []byte, _ int) ([]byte, bool) {
		if dir == "down" {
			time.Sleep(20 * time.Millisecond)
		}
		return b, true
	})
	run(t, "identify", slow.addr, "--timeout", "2s").want(t, 0)
	run(t, "tag", "read", slow.addr, "Values", "--count", "10", "--timeout", "2s").want(t, 0).has(t, "[9] 9")

	var once sync.Once
	stuck := startProxy(t, a.addr, func(dir string, b []byte, sent int) ([]byte, bool) {
		// The RegisterSession reply goes through; the next reply comes too late.
		if dir == "down" && sent >= 28 {
			once.Do(func() { time.Sleep(600 * time.Millisecond) })
		}
		return b, true
	})
	start := time.Now()
	r := run(t, "read", stuck.addr, "@1/1/7", "--timeout", "200ms", "--format", "json").want(t, 5)
	if kind, code := r.errorCode(t); kind != "timeout" || code != "request_timeout" {
		t.Errorf("%s %s", kind, code)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("a 200ms timeout took %v", d)
	}
}

// A connection cut in the middle of a command of many requests is a lost connection (4),
// with a valid JSON error.
func TestChaosProxyCutsMidCommand(t *testing.T) {
	a := startAdapter(t, "--logix")
	for _, cmd := range [][]string{
		{"tag", "list", "ADDR"},
		{"tag", "types", "ADDR", "0x123"},
		{"connections", "ADDR"},
		{"object", "ADDR", "identity"},
		{"diagnose", "ADDR"},
	} {
		t.Run(strings.Join(cmd, " "), func(t *testing.T) {
			p := startProxy(t, a.addr, func(dir string, b []byte, sent int) ([]byte, bool) {
				// RegisterSession (28 octets) and the first reply pass, then the connection breaks.
				if dir == "down" && sent > 28 {
					return nil, false
				}
				return b, true
			})
			args := make([]string, len(cmd))
			for i, a := range cmd {
				args[i] = strings.ReplaceAll(a, "ADDR", p.addr)
			}
			r := run(t, append(args, "--timeout", "1s", "--format", "json")...)
			switch {
			case cmd[0] == "diagnose":
				if r.code != 1 {
					t.Errorf("diagnose: exit %d, want 1", r.code)
				}
				_ = r.json(t)
			case r.code == 0:
				// The command finished before the cut.
				_ = r.json(t)
			case r.code == 4 || r.code == 5:
				r.errorCode(t)
			default:
				t.Errorf("exit %d\n%s\n%s", r.code, r.stdout, r.stderr)
			}
		})
	}
}

// An I/O connection whose adapter goes away: the input stops arriving (5) or the session
// is lost (4), and io ends with the samples it has, never hanging.
func TestChaosAdapterVanishesDuringIO(t *testing.T) {
	code, stdout, stderr := ioUntilAdapterVanishes(t)
	if code != 4 && code != 5 {
		t.Errorf("io after the adapter stopped: exit %d, want 4 or 5\n%s", code, stderr)
	}
	if !strings.Contains(stdout, `"seq":1`) {
		t.Errorf("the sample before the adapter stopped is missing:\n%s", stdout)
	}
}

// A stream that ends with a fatal error must stay JSON Lines: the error object is one
// compact line after the records. RenderFatal writes it indented over several lines, so
// a consumer reading line by line gets "{" and fails.
func TestStreamFatalErrorKeepsJSONLines(t *testing.T) {
	_, stdout, _ := ioUntilAdapterVanishes(t)
	for i, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("stdout line %d is not a JSON object (%v): %q\nstdout:\n%s", i+1, err, line, stdout)
		}
	}
}

// ioUntilAdapterVanishes runs io against an adapter, stops the adapter after the first
// sample, and returns how io ended.
func ioUntilAdapterVanishes(t *testing.T) (int, string, string) {
	t.Helper()
	a := startAdapter(t)
	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- Run(t.Context(), []string{"io", a.addr, "--config", "151", "--output", "152", "--input", "100", "--input-size", "32",
			"--rpi", "10ms", "--timeout-multiplier", "0", "--io-address", "127.0.0.1:0", "--format", "json"}, &stdout, &stderr)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(stdout.String(), "\n") { // a new sample only when the data change
		if time.Now().After(deadline) {
			t.Fatalf("no samples\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if code := a.stop(t); code != 0 {
		t.Errorf("adapter: exit %d", code)
	}
	select {
	case code := <-done:
		return code, stdout.String(), stderr.String()
	case <-time.After(10 * time.Second):
		t.Fatal("io did not end after the adapter stopped")
		return -1, "", ""
	}
}

// Interrupting (the context of Ctrl-C) ends a stream normally (0) and a one-shot command
// with 130, as docs/exitcodes.md says.
func TestChaosInterrupt(t *testing.T) {
	f := startFixture(t)
	f.requireUDP(t)
	interruptAfter := func(args []string, ready func(*syncBuffer) bool) (int, string) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var stdout, stderr syncBuffer
		done := make(chan int, 1)
		go func() { done <- Run(ctx, args, &stdout, &stderr) }()
		eventually(t, "the command to run", func() bool { return ready(&stdout) })
		cancel()
		select {
		case code := <-done:
			return code, stdout.String()
		case <-time.After(10 * time.Second):
			t.Fatalf("%v did not end after the interrupt", args)
			return -1, ""
		}
	}
	// io: a stream, exit 0, its JSON lines valid.
	code, out := interruptAfter([]string{"io", f.addr, "--config", "151", "--output", "152", "--input", "100", "--input-size", "32",
		"--rpi", "10ms", "--io-address", "127.0.0.1:0", "--format", "json"}, func(b *syncBuffer) bool { return strings.Contains(b.String(), "\n") })
	if code != 0 {
		t.Errorf("io interrupted: exit %d, want 0", code)
	}
	jsonLines(t, out)
	// adapter: a stream, exit 0, ends with its stopping event.
	code, out = interruptAfter([]string{"adapter", "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--format", "json"},
		func(b *syncBuffer) bool { return strings.Contains(b.String(), "listening") })
	if code != 0 || !strings.Contains(out, "stopping") {
		t.Errorf("adapter interrupted: exit %d\n%s", code, out)
	}
	jsonLines(t, out)
	// discover: one-shot, 130, with a JSON error object.
	silent, err := net.ListenPacket("udp4", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { _ = silent.Close() })
	got := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 64)
		if _, _, err := silent.ReadFrom(buf); err == nil {
			got <- struct{}{}
		}
	}()
	code, out = interruptAfter([]string{"discover", "--address", silent.LocalAddr().String(), "--wait", "20s", "--format", "json"},
		func(*syncBuffer) bool {
			select {
			case <-got:
				return true
			default:
				return false
			}
		})
	if code != 130 {
		t.Errorf("discover interrupted: exit %d, want 130\n%s", code, out)
	}
	r := result{stdout: out, code: code}
	if kind, c := r.errorCode(t); kind != "interrupted" || c != "context_canceled" {
		t.Errorf("discover interrupted: %s %s", kind, c)
	}
	// session --hold: one-shot that already has its session; the interrupt ends the hold.
	code, _ = interruptAfter([]string{"session", f.addr, "--hold", "30s", "--format", "json"},
		func(*syncBuffer) bool { return f.metrics.connects.Load() > 0 })
	if code != 0 && code != 130 {
		t.Errorf("session --hold interrupted: exit %d", code)
	}
}
