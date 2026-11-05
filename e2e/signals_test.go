// SPDX-License-Identifier: MIT

//go:build e2e && !windows

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Signals sent to the built binary: Ctrl-C (SIGINT) and SIGTERM end a stream normally
// (0) and a one-shot command with 130, as docs/exitcodes.md says. The process must end
// promptly, and its stdout must stay valid JSON.

// proc is a running ethernetipctl.
type proc struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	stdout bytes.Buffer
	stderr bytes.Buffer
	lines  chan string
	done   chan struct{}
}

func start(t *testing.T, args ...string) *proc {
	t.Helper()
	bin := os.Getenv("ETHERNETIPCTL_E2E_BIN")
	if bin == "" {
		t.Fatal("ETHERNETIPCTL_E2E_BIN is not set; run make e2e")
	}
	p := &proc{cmd: exec.Command(bin, args...), lines: make(chan string, 1024), done: make(chan struct{})}
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.cmd.Stderr = &lockedWriter{mu: &p.mu, w: &p.stderr}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(p.done)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			p.mu.Lock()
			p.stdout.WriteString(sc.Text() + "\n")
			p.mu.Unlock()
			select {
			case p.lines <- sc.Text():
			default:
			}
		}
	}()
	t.Cleanup(func() { _ = p.cmd.Process.Kill(); _ = p.cmd.Wait() })
	return p
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

// waitLine waits for a stdout line containing sub.
func (p *proc) waitLine(t *testing.T, sub string) string {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case l := <-p.lines:
			if strings.Contains(l, sub) {
				return l
			}
		case <-timeout:
			t.Fatalf("no line with %q\nstdout:\n%s\nstderr:\n%s", sub, p.out(), p.errOut())
		}
	}
}

func (p *proc) out() string    { p.mu.Lock(); defer p.mu.Unlock(); return p.stdout.String() }
func (p *proc) errOut() string { p.mu.Lock(); defer p.mu.Unlock(); return p.stderr.String() }

// signal sends sig and returns the exit code, failing if the process does not end in 5 s.
func (p *proc) signal(t *testing.T, sig os.Signal) int {
	t.Helper()
	if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { <-p.done; exited <- p.cmd.Wait() }()
	select {
	case err := <-exited:
		var ee *exec.ExitError
		switch {
		case err == nil:
			return 0
		case errors.As(err, &ee):
			return ee.ExitCode()
		default:
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%v did not end within 5s of %v\nstderr:\n%s", p.cmd.Args, sig, p.errOut())
	}
	return -1
}

// startAdapter runs the binary's adapter on loopback and returns it and its address.
func startAdapter(t *testing.T, extra ...string) (*proc, string) {
	t.Helper()
	a := start(t, append([]string{"adapter", "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--format", "json"}, extra...)...)
	var ev struct{ Address string }
	if err := json.Unmarshal([]byte(a.waitLine(t, `"listening"`)), &ev); err != nil || ev.Address == "" {
		t.Fatalf("listening event: %v", err)
	}
	return a, ev.Address
}

func TestSignalAdapter(t *testing.T) {
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			a, _ := startAdapter(t)
			if code := a.signal(t, sig); code != 0 {
				t.Errorf("adapter after %v: exit %d, want 0\n%s", sig, code, a.errOut())
			}
			out := a.out()
			if !strings.Contains(out, `"stopping"`) {
				t.Errorf("no stopping event:\n%s", out)
			}
			jsonLines(t, out)
		})
	}
}

func TestSignalIO(t *testing.T) {
	_, addr := startAdapter(t)
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			io := start(t, "io", addr, "--config", "151", "--output", "152", "--input", "100", "--input-size", "32",
				"--rpi", "10ms", "--io-address", "127.0.0.1:0", "--format", "json")
			io.waitLine(t, `"seq":1`)
			if code := io.signal(t, sig); code != 0 {
				t.Errorf("io after %v: exit %d, want 0\n%s", sig, code, io.errOut())
			}
			jsonLines(t, io.out())
			if !strings.Contains(io.errOut(), "sample(s) written") {
				t.Errorf("no summary on stderr:\n%s", io.errOut())
			}
		})
	}
}

func TestSignalDiscover(t *testing.T) {
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			silent, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = silent.Close() }()
			d := start(t, "discover", "--address", silent.LocalAddr().String(), "--wait", "30s", "--format", "json")
			// The request arrived: discover is collecting.
			_ = silent.SetReadDeadline(time.Now().Add(10 * time.Second))
			if _, _, err := silent.ReadFrom(make([]byte, 64)); err != nil {
				t.Fatalf("no ListIdentity request: %v", err)
			}
			if code := d.signal(t, sig); code != 130 {
				t.Errorf("discover after %v: exit %d, want 130\n%s", sig, code, d.errOut())
			}
			var env struct{ Error struct{ Kind, Code string } }
			if err := json.Unmarshal([]byte(d.out()), &env); err != nil || env.Error.Kind != "interrupted" || env.Error.Code != "context_canceled" {
				t.Errorf("stdout is not the interrupted error object (%v):\n%s", err, d.out())
			}
		})
	}
}

// A one-shot command waiting for a device that never answers ends with 130 at Ctrl-C,
// well before its --timeout.
func TestSignalOneShotWaitingForDevice(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := l.Accept(); err == nil {
			accepted <- c
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
	p := start(t, "identify", l.Addr().String(), "--timeout", "60s", "--format", "json")
	select {
	case c := <-accepted:
		defer func() { _ = c.Close() }()
	case <-time.After(10 * time.Second):
		t.Fatal("identify did not connect")
	}
	if code := p.signal(t, syscall.SIGINT); code != 130 {
		t.Errorf("identify after SIGINT: exit %d, want 130\n%s", code, p.errOut())
	}
	var env struct{ Error struct{ Kind string } }
	if err := json.Unmarshal([]byte(p.out()), &env); err != nil || env.Error.Kind != "interrupted" {
		t.Errorf("stdout:\n%s", p.out())
	}
}

// Bound violations reach the process exit code as documented: 2 for a value a flag cannot
// hold, 3 for one that makes no sense, with the JSON error object on stdout.
func TestBoundsExitCodes(t *testing.T) {
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"identify", "127.0.0.1", "--port", "65536"}, 2},
		{[]string{"identify", "127.0.0.1", "--timeout", "NaN"}, 2},
		{[]string{"write", "127.0.0.1", "@1/1/1", "--type", "SINT", "--value", "128", "--yes"}, 3},
		{[]string{"write", "127.0.0.1", "@1/1/1", "--type", "UINT", "--value", "-1", "--yes"}, 3},
		{[]string{"tag", "read", "127.0.0.1", "X", "--count", "65536"}, 3},
		{[]string{"forward-open", "127.0.0.1", "--output", "1", "--input", "1", "--rpi", "4294967296us"}, 3},
	} {
		output, code := runStdout(t, append(c.args, "--format", "json")...)
		if code != c.code {
			t.Errorf("%v: exit %d, want %d\n%s", c.args, code, c.code, output)
			continue
		}
		var env struct{ Error struct{ Kind, Code string } }
		if err := json.Unmarshal([]byte(output), &env); err != nil || env.Error.Code == "" {
			t.Errorf("%v: stdout is not a JSON error object: %v\n%s", c.args, err, output)
		}
	}
}

// runStdout runs the binary and returns its stdout alone.
func runStdout(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	cmd := exec.Command(os.Getenv("ETHERNETIPCTL_E2E_BIN"), args...)
	cmd.Stdout = &out
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &ee):
		return out.String(), ee.ExitCode()
	}
	t.Fatal(err)
	return "", -1
}

func jsonLines(t *testing.T, s string) {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("stdout line is not JSON: %v\n%s", err, line)
		}
	}
}
