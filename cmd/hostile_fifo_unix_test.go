// SPDX-License-Identifier: MIT

//go:build unix

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// An input file that never ends (a FIFO fed without end, like /dev/zero) must be read
// only up to a limit: the CLI stops reading, refuses it as invalid input, and stays
// bounded in memory. The feeder gives up at feedLimit octets so that a reader without a
// limit still finishes; how much it read is the evidence.

const feedLimit = 64 << 20

// feedFIFO creates a FIFO and feeds it with line until the reader closes it or feedLimit
// octets went through. It returns the FIFO's path and a function that waits for the
// feeder and returns the octets the reader took.
func feedFIFO(t *testing.T, name string, head, line []byte) (string, func() int) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	fed := make(chan int, 1)
	go func() {
		f, err := os.OpenFile(p, os.O_WRONLY, 0) // blocks until the CLI opens it
		if err != nil {
			fed <- 0
			return
		}
		defer func() { _ = f.Close() }()
		chunk := bytes.Repeat(line, 64<<10/len(line))
		n, _ := f.Write(head)
		for n < feedLimit {
			w, err := f.Write(chunk)
			n += w
			if err != nil { // EPIPE: the reader stopped
				break
			}
		}
		fed <- n
	}()
	return p, func() int {
		select {
		case n := <-fed:
			return n
		case <-time.After(2 * time.Second):
			// The CLI never opened it: unblock the feeder.
			if r, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
				_ = r.Close()
			}
			return <-fed
		}
	}
}

func TestEndlessEDSInputIsBounded(t *testing.T) {
	p, wait := feedFIFO(t, "endless.eds", []byte("[File]\n"), []byte("$ comment\n"))
	r := run(t, "eds", "validate", p, "--format", "json")
	n := wait()
	if r.code != 3 {
		t.Errorf("exit %d, want 3", r.code)
	}
	if n >= feedLimit {
		t.Errorf("eds validate read all %d MiB the FIFO offered: no read limit", n>>20)
	}
}

// A model that never ends: a reader without a limit (os.ReadFile) would take it until
// memory runs out. The feeder stops at 64 MiB; a bounded reader stops at its limit.
func TestEndlessAdapterModelIsBounded(t *testing.T) {
	p, wait := feedFIFO(t, "endless.yaml", []byte("# model\n"), []byte("#\n"))
	r := run(t, "adapter", "--config", p, "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "1ms", "--format", "json")
	n := wait()
	if r.code != 3 {
		t.Errorf("exit %d, want 3 (invalid_config)", r.code)
	}
	if n >= feedLimit {
		t.Errorf("adapter --config read all %d MiB the FIFO offered: the model file has no size limit", n>>20)
	}
}
