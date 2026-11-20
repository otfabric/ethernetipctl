// SPDX-License-Identifier: MIT

package eip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
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
	if _, err := multicastInterface(cfg); err != nil {
		return nil, err
	}
	if _, err := ParseRoute(cfg.Route); err != nil {
		return nil, err
	}
	switch {
	case cfg.OutputSize > maxScannerSize || cfg.InputSize > maxScannerSize:
		// The scanner's own limit, checked here so that nothing is sent.
		return nil, invalid(errs.CodeInvalidValue, "invalid --output-size %d or --input-size %d: at most %d for io", cfg.OutputSize, cfg.InputSize, maxScannerSize)
	case cfg.Duration < 0:
		return nil, invalid(errs.CodeInvalidValue, "invalid --duration %v: 0 (until interrupted) or more", cfg.Duration)
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
	if cfg.ConfigData != "" {
		// The target writes the configuration data to its configuration assembly.
		if err := confirm(cfg.Yes, "an I/O connection with --config-data"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// maxScannerSize is the largest I/O data size go-ethernetip's scanner
// accepts.
const maxScannerSize = 65000

// The most input samples, and octets of sample data as hex, that wait for the output:
// when the output is slower than the target, the oldest samples are dropped and counted.
// The latest sample is always kept.
var (
	maxQueuedSamples = 1024
	maxQueuedData    = 4 << 20
)

// sampleQueued is called with the length of the queue after each sample; tests set it.
var sampleQueued = func(int) {}

// sampler turns the inputs of an I/O connection into records, from the endpoint's
// goroutine to the command's.
type sampler struct {
	mu      sync.Mutex
	changes bool
	last    []byte
	lastRun bool
	seen    bool
	queue   []types.IOSample
	data    int // octets of Data in queue
	dropped uint64
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
	smp := types.IOSample{
		ReceivedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Seq:        s.seq, Run: run, Size: len(data), Data: Hex(data),
	}
	s.queue = append(s.queue, smp)
	s.data += len(smp.Data)
	for len(s.queue) > 1 && (len(s.queue) > maxQueuedSamples || s.data > maxQueuedData) {
		s.data -= len(s.queue[0].Data)
		s.queue[0] = types.IOSample{}
		s.queue = s.queue[1:]
		s.dropped++
	}
	sampleQueued(len(s.queue))
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *sampler) take() []types.IOSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.queue
	s.queue, s.data = nil, 0
	return q
}

// droppedSamples returns the number of samples dropped because the output was slower.
func (s *sampler) droppedSamples() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
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
	route, _ := ParseRoute(cfg.Route) // CheckIO checked it
	c, err := e.dial(ctx, t, route)
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
	sCfg := scannerConfig(cfg, out, configData, smp.input)
	sCfg.MulticastInterface, _ = multicastInterface(cfg)
	ioc, err := sc.Open(ctx, sCfg)
	if err != nil {
		return 0, WrapError(err)
	}
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
	// stopped ends the stream at a signal: the samples queued until then are written, and
	// no later ones.
	stopped := func() (int, error) {
		_, err := flush()
		e.ioSummary(stderr, ioc, smp, written)
		return errs.ExitOK, err
	}
	for {
		select {
		case <-smp.notify:
			if ctx.Err() != nil {
				return stopped()
			}
			if done, err := flush(); done || err != nil {
				e.ioSummary(stderr, ioc, smp, written)
				return errs.ExitOK, err
			}
		case <-ioc.Done():
			if _, err := flush(); err != nil {
				return 0, err
			}
			e.ioSummary(stderr, ioc, smp, written)
			if err := ioc.Err(); err != nil {
				return 0, WrapError(err)
			}
			return errs.ExitOK, nil
		case <-stop:
			_, err := flush()
			e.ioSummary(stderr, ioc, smp, written)
			return errs.ExitOK, err
		case <-ctx.Done():
			return stopped()
		}
	}
}

func (e Env) ioSummary(w io.Writer, ioc *scanner.IO, smp *sampler, written uint64) {
	st := ioc.Stats()
	state := ioc.State()
	_, _ = fmt.Fprintf(w, "%d sample(s) written; sent %d, received %d, dropped %d, duplicates %d; state %s",
		written, st.Sent, st.Received, st.Dropped, st.Duplicates, state)
	if n := smp.droppedSamples(); n > 0 {
		_, _ = fmt.Fprintf(w, "; %d sample(s) not written: the output did not keep up", n)
	}
	_, _ = fmt.Fprintln(w)
}

// scannerConfig is the scanner's configuration of an I/O connection. The
// output data and the run/idle state are part of it, not set after Open:
// the connection produces as soon as it is open, and its first datagrams
// would otherwise carry zeros in run, whatever --output-data and --idle say.
func scannerConfig(cfg config.IOConfig, out, configData []byte, onInput func([]byte, bool)) scanner.IOConfig {
	sc := scanner.IOConfig{
		ConfigInstance: cfg.Config, ConfigData: configData,
		OutputInstance: cfg.Output, InputInstance: cfg.Input,
		OutputSize: int(cfg.OutputSize), InputSize: int(cfg.InputSize),
		RPI: cfg.RPI, Multicast: cfg.Multicast, Large: cfg.Large, TimeoutMultiplier: cfg.TimeoutMultiplier,
		Idle:    cfg.Idle,
		OnInput: onInput,
	}
	sc.Key, _ = electronicKey(cfg.IOConnConfig)
	if cfg.OutputSize > 0 {
		sc.Output = out
	}
	return sc
}

// multicastInterface resolves --multicast-interface: an interface name, or an IPv4
// address of one. nil without the flag.
func multicastInterface(cfg config.IOConfig) (*net.Interface, error) {
	name := strings.TrimSpace(cfg.MulticastInterface)
	if name == "" {
		return nil, nil
	}
	if !cfg.Multicast {
		return nil, invalid(errs.CodeInvalidInput, "--multicast-interface needs --multicast")
	}
	if ip, err := netip.ParseAddr(name); err == nil {
		if !ip.Is4() {
			return nil, invalid(errs.CodeInvalidValue, "invalid --multicast-interface %s: an IPv4 address or an interface name", name)
		}
		// An interface's addresses read as prefixes, 192.168.1.20/24.
		ifs, _ := net.Interfaces()
		for i := range ifs {
			addrs, _ := ifs[i].Addrs()
			for _, a := range addrs {
				if strings.HasPrefix(a.String(), ip.String()+"/") {
					return &ifs[i], nil
				}
			}
		}
		return nil, invalid(errs.CodeInvalidValue, "invalid --multicast-interface %s: no interface has this address", name)
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return nil, invalid(errs.CodeInvalidValue, "invalid --multicast-interface %s: no such interface", name)
	}
	return ifi, nil
}
