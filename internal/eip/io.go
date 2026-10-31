// SPDX-License-Identifier: MIT

package eip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/otfabric/go-ethernetip/scanner"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// DefaultIOAddress is the local address of the I/O endpoint: UDP 2222 on every interface,
// the port targets send to.
const DefaultIOAddress = ":2222"

// CheckIO validates `io` before anything is sent, and returns the output data.
func CheckIO(cfg config.IOConfig) ([]byte, error) {
	if _, err := checkIO(cfg.IOConnConfig); err != nil {
		return nil, err
	}
	out := make([]byte, cfg.OutputSize)
	if cfg.OutputData != "" {
		data, err := ParseHex(cfg.OutputData)
		if err != nil {
			return nil, err
		}
		if len(data) != int(cfg.OutputSize) {
			return nil, invalid(errs.CodeInvalidValue, "--output-data has %d octets, --output-size is %d", len(data), cfg.OutputSize)
		}
		out = data
	}
	if cfg.OutputSize > 0 || cfg.OutputData != "" {
		if err := confirm(cfg.Yes, "an I/O connection with output data (--output-size above 0)"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// sampler turns the inputs of an I/O connection into records, from the endpoint's
// goroutine to the command's.
type sampler struct {
	mu      sync.Mutex
	changes bool
	last    []byte
	lastRun bool
	seen    bool
	queue   []types.IOSample
	seq     uint64
	notify  chan struct{}
}

func (s *sampler) input(data []byte, run bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changes && s.seen && run == s.lastRun && bytes.Equal(data, s.last) {
		return
	}
	s.seen, s.lastRun, s.last = true, run, append(s.last[:0], data...)
	s.seq++
	s.queue = append(s.queue, types.IOSample{
		ReceivedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Seq:        s.seq, Run: run, Size: len(data), Data: Hex(data),
	})
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *sampler) take() []types.IOSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.queue
	s.queue = nil
	return q
}

// IO runs `io`: opens an I/O connection with the scanner and writes every input sample
// until --duration, --count, a signal or the end of the connection. It returns the exit
// code of a completed stream.
func (e Env) IO(ctx context.Context, host string, cfg config.IOConfig, write func(*types.IOSample) error, stderr io.Writer) (int, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return 0, err
	}
	out, err := CheckIO(cfg)
	if err != nil {
		return 0, err
	}
	configData, _ := checkIO(cfg.IOConnConfig)
	c, err := e.dial(ctx, t, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = c.Close() }()
	ioAddr := cfg.IOAddress
	if ioAddr == "" {
		ioAddr = DefaultIOAddress
	}
	sc, err := scanner.New(c, scanner.WithLocalAddr(ioAddr), scanner.WithLogger(e.Logger()))
	if err != nil {
		return 0, errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot open the I/O endpoint %s: %v", ioAddr, err), err)
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), e.Timeout())
		_ = sc.Close(cctx)
		cancel()
	}()
	smp := &sampler{changes: cfg.Changes, notify: make(chan struct{}, 1)}
	ioc, err := sc.Open(ctx, scanner.IOConfig{
		ConfigInstance: cfg.Config, ConfigData: configData,
		OutputInstance: cfg.Output, InputInstance: cfg.Input,
		OutputSize: int(cfg.OutputSize), InputSize: int(cfg.InputSize),
		RPI: cfg.RPI, Multicast: cfg.Multicast, Large: cfg.Large, TimeoutMultiplier: cfg.TimeoutMultiplier,
		OnInput: smp.input,
	})
	if err != nil {
		return 0, WrapError(err)
	}
	if cfg.OutputSize > 0 {
		if err := ioc.SetOutput(out); err != nil {
			return 0, WrapError(err)
		}
	}
	ioc.SetRun(!cfg.Idle)
	info := ioc.Info()
	_, _ = fmt.Fprintf(stderr, "I/O connection to %s open: O->T 0x%08X, T->O 0x%08X, API %v/%v, path %s\n",
		t, info.OTConnID, info.TOConnID, info.OTRPI, info.TORPI, info.Path)
	var stop <-chan time.Time
	if cfg.Duration > 0 {
		timer := time.NewTimer(cfg.Duration)
		defer timer.Stop()
		stop = timer.C
	}
	var written uint64
	flush := func() (bool, error) {
		for _, s := range smp.take() {
			if err := write(&s); err != nil {
				return true, errs.Output(errs.CodeOutputWriteFailed, err)
			}
			written++
			if cfg.Count > 0 && written >= uint64(cfg.Count) {
				return true, nil
			}
		}
		return false, nil
	}
	for {
		select {
		case <-smp.notify:
			if done, err := flush(); done || err != nil {
				e.ioSummary(stderr, ioc, written)
				return errs.ExitOK, err
			}
		case <-ioc.Done():
			if _, err := flush(); err != nil {
				return 0, err
			}
			e.ioSummary(stderr, ioc, written)
			if err := ioc.Err(); err != nil {
				return 0, WrapError(err)
			}
			return errs.ExitOK, nil
		case <-stop:
			_, err := flush()
			e.ioSummary(stderr, ioc, written)
			return errs.ExitOK, err
		case <-ctx.Done():
			_, err := flush()
			e.ioSummary(stderr, ioc, written)
			return errs.ExitOK, err
		}
	}
}

func (e Env) ioSummary(w io.Writer, ioc *scanner.IO, written uint64) {
	st := ioc.Stats()
	state := ioc.State()
	_, _ = fmt.Fprintf(w, "%d sample(s) written; sent %d, received %d, dropped %d, duplicates %d; state %s\n",
		written, st.Sent, st.Received, st.Dropped, st.Duplicates, state)
}
