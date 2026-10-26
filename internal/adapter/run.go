package adapter

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/server"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// Resolve returns the model `adapter` serves: the model file of --config, or the defaults,
// with the identity flags, --assembly, --echo and --logix applied.
func Resolve(cfg *config.AdapterConfig) (*Model, error) {
	m := &Model{Identity: DefaultIdentity}
	if cfg.Config != "" {
		loaded, err := Load(cfg.Config)
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidConfig, fmt.Sprintf("invalid adapter model: %v", err), err)
		}
		m = loaded
	}
	if cfg.ProductName != "" {
		m.Identity.ProductName = cfg.ProductName
	}
	if cfg.VendorID != 0 {
		m.Identity.VendorID = cfg.VendorID
	}
	if cfg.ProductCode != 0 {
		m.Identity.ProductCode = cfg.ProductCode
	}
	if cfg.Serial != 0 {
		m.Identity.SerialNumber = cfg.Serial
	}
	if cfg.Revision != "" {
		m.Identity.Revision = cfg.Revision
	}
	if len(cfg.Assemblies) > 0 {
		m.Assemblies = nil
		for _, s := range cfg.Assemblies {
			a, err := ParseAssembly(s)
			if err != nil {
				return nil, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
			}
			m.Assemblies = append(m.Assemblies, a)
		}
	} else if cfg.Config == "" {
		m.Assemblies = append([]Assembly(nil), DefaultAssemblies...)
	}
	if cfg.Echo {
		m.Echo = EchoPairs(m.Assemblies)
	}
	if cfg.Logix && m.Logix == nil {
		m.Logix = DemoLogix()
	}
	if err := m.Check(); err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidConfig, fmt.Sprintf("invalid adapter model: %v", err), err)
	}
	return m, nil
}

// ListenAddress returns the TCP address to listen on: --listen with the port of --port
// when it names none. Port 0 picks a free port.
func ListenAddress(listen string, port uint16) (string, error) {
	if listen == "" {
		listen = "0.0.0.0"
	}
	host, p, err := net.SplitHostPort(listen)
	if err == nil && p == "0" {
		listen = host
	}
	t, err := config.ParseHost(listen, port)
	if err != nil {
		return "", errs.InvalidInput(errs.CodeInvalidAddress, "invalid --listen: "+err.Error(), err)
	}
	if p == "0" {
		return net.JoinHostPort(t.Host, "0"), nil
	}
	return t.Addr(), nil
}

// emitter serializes the events of the server's goroutines.
type emitter struct {
	mu  sync.Mutex
	log func(types.AdapterEvent)
}

func (e *emitter) emit(ev types.AdapterEvent) {
	ev.Timestamp = time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	e.mu.Lock()
	defer e.mu.Unlock()
	e.log(ev)
}

// metrics turns the server's callbacks into events.
type metrics struct {
	ethernetip.NopMetrics
	e        *emitter
	requests bool
}

func addrText(a net.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

func (m *metrics) OnConnect(remote net.Addr) {
	m.e.emit(types.AdapterEvent{Event: "connected", Remote: addrText(remote)})
}

func (m *metrics) OnDisconnect(remote net.Addr, err error) {
	ev := types.AdapterEvent{Event: "disconnected", Remote: addrText(remote)}
	if err != nil {
		ev.Detail = err.Error()
	}
	m.e.emit(ev)
}

func (m *metrics) OnHandled(remote net.Addr, service cip.Service, status cip.GeneralStatus, _ time.Duration) {
	if !m.requests {
		return
	}
	m.e.emit(types.AdapterEvent{Event: "request", Remote: addrText(remote), Service: service.String(), Status: fmt.Sprintf("0x%02X %s", uint8(status), status)})
}

func connection(info ethernetip.ConnectionInfo) *types.Connection {
	return &types.Connection{Class: info.Class, OTConnID: info.OTConnID, TOConnID: info.TOConnID,
		ConnectionSerial: info.Serial, OriginatorVendor: info.VendorID, OriginatorSerial: info.OriginatorSerial,
		OTRPIMs: float64(info.OTRPI.Microseconds()) / 1000, TORPIMs: float64(info.TORPI.Microseconds()) / 1000, Path: info.Path.String()}
}

func (m *metrics) OnConnectionOpened(remote net.Addr, info ethernetip.ConnectionInfo) {
	m.e.emit(types.AdapterEvent{Event: "connection-opened", Remote: addrText(remote), Conn: connection(info)})
}

func (m *metrics) OnConnectionClosed(remote net.Addr, info ethernetip.ConnectionInfo, err error) {
	ev := types.AdapterEvent{Event: "connection-closed", Remote: addrText(remote), Conn: connection(info)}
	if err != nil {
		ev.Detail = err.Error()
	}
	m.e.emit(ev)
}

// build builds the server of a model.
func build(m *Model, cfg *config.AdapterConfig, logger ethernetip.Logger, e *emitter) (*server.Server, error) {
	id, err := m.Identity.identity()
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	opts := []server.Option{
		server.WithIdentity(id),
		server.WithLogger(logger),
		server.WithMetrics(&metrics{e: e, requests: cfg.Requests}),
		server.WithResetHandler(func(t uint8) cip.GeneralStatus {
			e.emit(types.AdapterEvent{Event: "reset", Detail: "type " + strconv.Itoa(int(t))})
			return cip.StatusSuccess
		}),
	}
	if cfg.IOAddress != "" {
		opts = append(opts, server.WithIOAddress(cfg.IOAddress))
	}
	srv, err := server.New(opts...)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidConfig, fmt.Sprintf("invalid adapter: %v", err), err)
	}
	asm := map[uint32]*server.Assembly{}
	for _, a := range m.Assemblies {
		x, err := srv.AddAssembly(a.Instance, assemblyKinds[a.Kind], a.Size)
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidConfig, fmt.Sprintf("assembly %d: %v", a.Instance, err), err)
		}
		if a.Data != "" {
			d, _ := parseHex(a.Data)
			_ = x.SetData(d)
		}
		asm[a.Instance] = x
	}
	targets := map[uint32]*server.Assembly{}
	for _, ec := range m.Echo {
		targets[ec.Output] = asm[ec.Input]
	}
	for _, a := range m.Assemblies {
		if a.Kind != "output" && a.Kind != "config" {
			continue
		}
		in, x, echo := a.Instance, asm[a.Instance], targets[a.Instance]
		x.OnWrite(func(data []byte, run bool) {
			if echo != nil {
				_ = echo.SetData(data)
			}
			r := run
			e.emit(types.AdapterEvent{Event: "assembly-write", Instance: in, Size: len(data), Data: hex.EncodeToString(data), Run: &r})
		})
	}
	if m.Logix != nil {
		ts, err := m.Logix.buildTagServer()
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidConfig, fmt.Sprintf("invalid logix model: %v", err), err)
		}
		ts.OnWrite(func(name string) { e.emit(types.AdapterEvent{Event: "tag-write", Tag: name}) })
		ts.Attach(srv)
	}
	return srv, nil
}

// Run serves the adapter until ctx ends or --duration elapses. log receives every event;
// it is called from one goroutine at a time.
func Run(ctx context.Context, cfg *config.AdapterConfig, global *config.Global, logger ethernetip.Logger, log func(types.AdapterEvent)) error {
	m, err := Resolve(cfg)
	if err != nil {
		return err
	}
	addr, err := ListenAddress(cfg.Listen, global.Port)
	if err != nil {
		return err
	}
	e := &emitter{log: log}
	srv, err := build(m, cfg, logger, e)
	if err != nil {
		return err
	}
	if err := srv.Listen(addr); err != nil {
		return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot listen on %s: %v", addr, err), err)
	}
	detail := "io " + srv.IOAddr().String()
	if u := srv.UDPAddr(); u != nil {
		detail += " udp " + u.String()
	} else {
		detail += " (ListIdentity over UDP unavailable)"
	}
	e.emit(types.AdapterEvent{Event: "listening", Address: srv.Addr().String(), Detail: detail})
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	var stop <-chan time.Time
	if cfg.Duration > 0 {
		t := time.NewTimer(cfg.Duration)
		defer t.Stop()
		stop = t.C
	}
	select {
	case <-ctx.Done():
	case <-stop:
	case err := <-done:
		_ = srv.Close()
		return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("the server stopped: %v", err), err)
	}
	e.emit(types.AdapterEvent{Event: "stopping"})
	_ = srv.Close()
	if err := <-done; err != nil && !errors.Is(err, ethernetip.ErrClosed) {
		return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("the server stopped: %v", err), err)
	}
	return nil
}
