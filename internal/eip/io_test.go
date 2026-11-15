// SPDX-License-Identifier: MIT

package eip

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/server"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// The samples waiting for the output are bounded in number and in data: the oldest are
// dropped and counted, the latest is kept.
func TestSamplerQueueIsBounded(t *testing.T) {
	s := &sampler{notify: make(chan struct{}, 1)}
	for i := range maxQueuedSamples + 10 {
		s.input([]byte{byte(i)}, true)
	}
	q := s.take()
	if len(q) != maxQueuedSamples || q[0].Seq != 11 || q[len(q)-1].Seq != uint64(maxQueuedSamples+10) || s.droppedSamples() != 10 {
		t.Fatalf("%d queued, seq %d to %d, %d dropped", len(q), q[0].Seq, q[len(q)-1].Seq, s.droppedSamples())
	}

	big := bytes.Repeat([]byte{0xAB}, maxScannerSize) // 130000 octets of hex each
	for range 40 {
		s.input(big, true)
	}
	if q := s.take(); len(q) != maxQueuedData/(2*maxScannerSize) || s.droppedSamples() != 10+40-uint64(len(q)) {
		t.Errorf("%d samples of %d octets queued, %d dropped", len(q), len(big), s.droppedSamples())
	}

	// A sample larger than the data bound is still written: the latest is always kept.
	old := maxQueuedData
	t.Cleanup(func() { maxQueuedData = old })
	maxQueuedData = 10
	for range 3 {
		s.input([]byte{1, 2, 3, 4, 5, 6, 7, 8}, true)
	}
	if q := s.take(); len(q) != 1 || q[0].Data != "0102030405060708" {
		t.Errorf("queue %+v", q)
	}
}

// startIOServer starts a go-ethernetip adapter on loopback with input assembly 100 and the
// input-only heartbeat point 152.
func startIOServer(t *testing.T) (string, *server.Assembly) {
	t.Helper()
	srv, err := server.New(server.WithIOAddress("127.0.0.1:0"))
	if err != nil {
		t.Fatal(err)
	}
	in, err := srv.AddAssembly(100, server.AssemblyInput, 32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.AddAssembly(152, server.AssemblyInputOnly, 0); err != nil {
		t.Fatal(err)
	}
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.Close()
		if err := <-done; !errors.Is(err, ethernetip.ErrClosed) {
			t.Errorf("Serve returned %v", err)
		}
	})
	return srv.Addr().String(), in
}

// While the output takes no samples the queue stays within its bound, and an interrupt
// ends the stream with the samples already waiting and the count of those dropped.
func TestIOWithAStalledOutput(t *testing.T) {
	addr, in := startIOServer(t)
	// The input changes every millisecond: every packet the target sends is a new sample.
	stopInput := make(chan struct{})
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		data := make([]byte, 32)
		for {
			select {
			case <-stopInput:
				return
			case <-tick.C:
				data[0]++
				_ = in.SetData(data)
			}
		}
	}()
	defer func() { close(stopInput); <-inputDone }()
	oldMax, oldHook := maxQueuedSamples, sampleQueued
	t.Cleanup(func() { maxQueuedSamples, sampleQueued = oldMax, oldHook })
	maxQueuedSamples = 8
	const pushes = 40
	var longest, n atomic.Int64
	full := make(chan struct{})
	var once sync.Once
	sampleQueued = func(l int) { // called by one goroutine at a time, under the sampler's lock
		if int64(l) > longest.Load() {
			longest.Store(int64(l))
		}
		if n.Add(1) == pushes {
			once.Do(func() { close(full) })
		}
	}

	release := make(chan struct{})
	var writes atomic.Int64
	write := func(*types.IOSample) error {
		if writes.Add(1) > 1 {
			<-release // the output stalls after the first sample
		}
		return nil
	}
	g := config.NewGlobal()
	g.Timeout = 5 * time.Second
	var stderr bytes.Buffer
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cfg := config.IOConfig{IOAddress: "127.0.0.1:0", IOConnConfig: config.IOConnConfig{
		ConnectionPathConfig: config.ConnectionPathConfig{Output: 152, Input: 100}, InputSize: 32, RPI: 2 * time.Millisecond,
		TimeoutMultiplier: 7}} // a timeout of about a second: a loaded machine keeps the connection
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := Env{Global: g, Stderr: &stderr}.IO(ctx, addr, cfg, write, &stderr)
		done <- result{code, err}
	}()

	select {
	case <-full:
	case <-time.After(10 * time.Second):
		cancel()
		close(release)
		<-done
		t.Fatalf("%d samples in 10 s, %d written: %s", n.Load(), writes.Load(), stderr.String())
	}
	cancel()
	close(release)
	select {
	case r := <-done:
		if r.err != nil || r.code != 0 {
			t.Fatalf("exit %d: %v", r.code, r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("io did not stop after the interrupt")
	}
	if l := longest.Load(); l > int64(maxQueuedSamples) {
		t.Errorf("%d samples queued, at most %d", l, maxQueuedSamples)
	}
	if w := writes.Load(); w > int64(1+3*maxQueuedSamples) {
		t.Errorf("%d samples written after %d arrived with a stalled output", w, pushes)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("sample(s) not written: the output did not keep up\n")) {
		t.Errorf("summary: %s", stderr.String())
	}
}
