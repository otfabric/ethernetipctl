package eip

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/client"
	"github.com/otfabric/go-ethernetip/implicit"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// ConnectionOf converts the description of a connection.
func ConnectionOf(info ethernetip.ConnectionInfo) types.Connection {
	return types.Connection{
		Class: info.Class, OTConnID: info.OTConnID, TOConnID: info.TOConnID,
		ConnectionSerial: info.Serial, OriginatorVendor: info.VendorID, OriginatorSerial: info.OriginatorSerial,
		OTRPIMs: ms(info.OTRPI), TORPIMs: ms(info.TORPI), Path: info.Path.String(),
	}
}

// Session runs `session`: RegisterSession and, with --connected, a Class 3 connection,
// held for --hold.
func (e Env) Session(ctx context.Context, host string, cfg config.SessionConfig) (*types.SessionResult, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return nil, err
	}
	route, err := ParseRoute(cfg.Route)
	if err != nil {
		return nil, err
	}
	if cfg.Hold < 0 {
		return nil, invalid(errs.CodeInvalidValue, "invalid --hold %v", cfg.Hold)
	}
	start := time.Now()
	s, err := e.openTarget(ctx, t, route, cfg.Connected, client.ConnectionOptions{Large: cfg.Large, Size: cfg.Size})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	r := &types.SessionResult{Target: t.String(), Route: strings.TrimSpace(cfg.Route), Session: s.Client.Session(), DurationMs: ms(time.Since(start))}
	if a := s.Client.LocalAddr(); a != nil {
		r.Local = a.String()
	}
	if s.Conn != nil {
		c := ConnectionOf(s.Conn.Info())
		r.Connection = &c
	}
	if cfg.Hold > 0 {
		held := time.Now()
		select {
		case <-ctx.Done():
		case <-time.After(cfg.Hold):
		}
		r.HeldMs = ms(time.Since(held))
	}
	return r, nil
}

// connectionPath returns the connection path of an I/O connection: the Assembly class, the
// configuration instance, the consumed and produced connection points and the
// configuration data.
func connectionPath(p config.ConnectionPathConfig, configData []byte) (cip.Path, error) {
	if p.Output == 0 || p.Input == 0 {
		return nil, invalid(errs.CodeInvalidInput, "--output and --input (the assembly instances of the connection) are required")
	}
	path := cip.Path{cip.ClassSegment(cip.ClassAssembly)}
	if p.Config != 0 {
		path = append(path, cip.InstanceSegment(p.Config))
	}
	path = append(path, cip.ConnectionPointSegment(p.Output), cip.ConnectionPointSegment(p.Input))
	if len(configData) > 0 {
		if len(configData)%2 != 0 {
			return nil, invalid(errs.CodeInvalidValue, "--config-data has %d octets: it must be an even number", len(configData))
		}
		path = append(path, cip.DataSegment{Data: configData})
	}
	return path, nil
}

// checkIO validates the description of an I/O connection.
func checkIO(c config.IOConnConfig) ([]byte, error) {
	var data []byte
	if c.ConfigData != "" {
		var err error
		if data, err = ParseHex(c.ConfigData); err != nil {
			return nil, err
		}
	}
	switch {
	case c.RPI <= 0:
		return nil, invalid(errs.CodeInvalidInput, "--rpi is required (e.g. 10ms)")
	case c.RPI < time.Microsecond || c.RPI > time.Duration(1<<32-1)*time.Microsecond:
		return nil, invalid(errs.CodeInvalidValue, "invalid --rpi %v", c.RPI)
	case c.TimeoutMultiplier > 7:
		return nil, invalid(errs.CodeInvalidValue, "invalid --timeout-multiplier %d: 0 to 7", c.TimeoutMultiplier)
	}
	if _, err := connectionPath(c.ConnectionPathConfig, data); err != nil {
		return nil, err
	}
	return data, nil
}

// randomUint32 returns a random number for connection serials.
func randomUint32() uint32 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}

// triad returns the connection triad of the flags, random where they are 0.
func triad(t config.TriadConfig) cip.Triad {
	out := cip.Triad{Serial: t.ConnectionSerial, VendorID: t.OriginatorVendor, OriginatorSerial: t.OriginatorSerial}
	if out.Serial == 0 {
		out.Serial = uint16(randomUint32()) | 1
	}
	if out.VendorID == 0 {
		out.VendorID = client.DefaultVendorID
	}
	if out.OriginatorSerial == 0 {
		out.OriginatorSerial = randomUint32() | 1
	}
	return out
}

// CheckForwardOpen validates `forward-open` before anything is sent.
func CheckForwardOpen(cfg config.ForwardOpenConfig) error {
	if _, err := ParseRoute(cfg.Route); err != nil {
		return err
	}
	_, err := checkIO(cfg.IOConnConfig)
	return err
}

// ForwardOpen runs `forward-open`: a Forward_Open of a Class 1 cyclic I/O connection, which
// is closed again with Forward_Close unless --keep. No I/O data is exchanged.
func (e Env) ForwardOpen(ctx context.Context, host string, cfg config.ForwardOpenConfig) (*types.ForwardOpenResult, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return nil, err
	}
	if err := CheckForwardOpen(cfg); err != nil {
		return nil, err
	}
	route, _ := ParseRoute(cfg.Route)
	configData, _ := checkIO(cfg.IOConnConfig)
	path, _ := connectionPath(cfg.ConnectionPathConfig, configData)
	tr := triad(cfg.TriadConfig)
	c, err := e.dial(ctx, t, route)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	// The connection sizes: the data, plus the sequence count, plus the 32-bit run/idle
	// header O->T (or nothing more for a heartbeat); T->O is modeless.
	otFmt := implicit.FormatHeader32
	if cfg.OutputSize == 0 {
		otFmt = implicit.FormatHeartbeat
	}
	otSize := uint16(otFmt.Overhead(1))
	if otFmt == implicit.FormatHeader32 {
		otSize += cfg.OutputSize
	}
	toSize := cfg.InputSize + uint16(implicit.FormatModeless.Overhead(1))
	toType := cip.ConnectionPointToPoint
	if cfg.Multicast {
		toType = cip.ConnectionMulticast
	}
	rpi := uint32(cfg.RPI / time.Microsecond)
	fo := cip.ForwardOpen{
		Large:             cfg.Large || otSize > 0x1FF || toSize > 0x1FF,
		TOConnID:          client.NewConnectionID(),
		Triad:             tr,
		TimeoutMultiplier: cfg.TimeoutMultiplier,
		OTRPI:             rpi,
		OTParams:          cip.NetworkParams{Type: cip.ConnectionPointToPoint, Priority: cip.PriorityScheduled, Size: otSize},
		TORPI:             rpi,
		TOParams:          cip.NetworkParams{Type: toType, Priority: cip.PriorityScheduled, Size: toSize},
		Transport:         cip.NewTransportTrigger(1, cip.TriggerCyclic, false),
		Path:              path,
	}
	fo.PriorityTick, fo.TimeoutTicks = cip.Ticks(e.Timeout())
	start := time.Now()
	reply, _, err := c.ForwardOpen(ctx, fo)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.ForwardOpenResult{
		Target: t.String(), Route: strings.TrimSpace(cfg.Route), Service: fo.Service().String(), Transport: fo.Transport.String(),
		OTSize: otSize, TOSize: toSize, RTTMs: ms(time.Since(start)),
		Connection: ConnectionOf(ethernetip.ConnectionInfo{
			Class: 1, OTConnID: reply.OTConnID, TOConnID: reply.TOConnID,
			Serial: reply.Serial, VendorID: reply.VendorID, OriginatorSerial: reply.OriginatorSerial,
			OTRPI: time.Duration(reply.OTAPI) * time.Microsecond, TORPI: time.Duration(reply.TOAPI) * time.Microsecond, Path: path,
		}),
	}
	if !cfg.Keep {
		fc := cip.ForwardClose{Triad: tr, Path: path, PriorityTick: fo.PriorityTick, TimeoutTicks: fo.TimeoutTicks}
		if err := c.ForwardClose(ctx, fc); err != nil {
			return nil, WrapError(fmt.Errorf("the connection was opened, but Forward_Close failed: %w", err))
		}
		r.Closed = true
	}
	return r, nil
}

// CheckForwardClose validates `forward-close` before anything is sent.
func CheckForwardClose(cfg config.ForwardCloseConfig) (cip.Path, error) {
	if _, err := ParseRoute(cfg.Route); err != nil {
		return nil, err
	}
	if cfg.ConnectionSerial == 0 || cfg.OriginatorSerial == 0 {
		return nil, invalid(errs.CodeInvalidInput, "--connection-serial and --originator-serial (the triad forward-open printed) are required")
	}
	return connectionPath(cfg.ConnectionPathConfig, nil)
}

// ForwardClose runs `forward-close`.
func (e Env) ForwardClose(ctx context.Context, host string, cfg config.ForwardCloseConfig) (*types.ForwardCloseResult, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return nil, err
	}
	path, err := CheckForwardClose(cfg)
	if err != nil {
		return nil, err
	}
	route, _ := ParseRoute(cfg.Route)
	tr := triad(cfg.TriadConfig)
	c, err := e.dial(ctx, t, route)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	fc := cip.ForwardClose{Triad: tr, Path: path}
	fc.PriorityTick, fc.TimeoutTicks = cip.Ticks(e.Timeout())
	start := time.Now()
	if err := c.ForwardClose(ctx, fc); err != nil {
		return nil, WrapError(err)
	}
	return &types.ForwardCloseResult{Target: t.String(), Route: strings.TrimSpace(cfg.Route), ConnectionSerial: tr.Serial,
		OriginatorVendor: tr.VendorID, OriginatorSerial: tr.OriginatorSerial, Path: path.String(), RTTMs: ms(time.Since(start))}, nil
}
