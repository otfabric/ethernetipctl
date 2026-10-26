package eip

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/client"

	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// timingDialer dials TCP and records how long it took and how it ended, so that diagnose
// can tell the TCP connect from RegisterSession.
type timingDialer struct {
	d    net.Dialer
	mu   sync.Mutex
	dur  time.Duration
	err  error
	done bool
}

func (t *timingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	start := time.Now()
	c, err := t.d.DialContext(ctx, network, addr)
	t.mu.Lock()
	t.dur, t.err, t.done = time.Since(start), err, true
	t.mu.Unlock()
	return c, err
}

func (t *timingDialer) result() (time.Duration, error, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dur, t.err, t.done
}

// Diagnose runs `diagnose`: structured checks, each with its outcome and timing. A failed
// check does not stop the others, except that the checks after a failed TCP connect or
// RegisterSession are skipped. It returns the exit code: 0 when every check passed, 7
// (partial) otherwise.
func (e Env) Diagnose(ctx context.Context, host string) (*types.DiagnoseResult, int, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return nil, 0, err
	}
	start := time.Now()
	r := &types.DiagnoseResult{Target: t.String(), Checks: []types.Check{}}
	add := func(name string, d time.Duration, detail map[string]any, err error) {
		c := types.Check{Name: name, Status: "ok", DurationMs: ms(d), Detail: detail, Error: ErrorInfo(err)}
		if err != nil {
			c.Status, c.Detail = "error", nil
		}
		r.Checks = append(r.Checks, c)
	}
	td := &timingDialer{d: net.Dialer{Timeout: e.Timeout()}}
	c, err := client.New(t.Addr(), client.WithTimeout(e.Timeout()), client.WithLogger(e.Logger()), client.WithDialer(td))
	if err != nil {
		return nil, 0, WrapError(err)
	}
	defer func() { _ = c.Close() }()
	cstart := time.Now()
	cerr := c.Connect(ctx)
	total := time.Since(cstart)
	dialDur, dialErr, dialed := td.result()
	if !dialed {
		dialDur, dialErr = total, cerr
	}
	add("tcp-connect", dialDur, map[string]any{"address": t.Addr()}, dialErr)
	checks := []struct {
		name string
		run  func() (map[string]any, error)
	}{
		{"list-identity", func() (map[string]any, error) {
			li, err := c.ListIdentity(ctx)
			if err != nil {
				return nil, err
			}
			id := IdentityOf(li.Identity, true)
			return map[string]any{"product_name": id.ProductName, "vendor_id": id.VendorID, "revision": id.Revision, "state": id.StateName, "status": strings.TrimSpace(fmt.Sprintf("0x%04X %s", id.Status, id.StatusFlags))}, nil
		}},
		{"list-services", func() (map[string]any, error) {
			sv, err := c.ListServices(ctx)
			if err != nil {
				return nil, err
			}
			names := make([]string, len(sv))
			for i, s := range sv {
				names[i] = s.Name
			}
			return map[string]any{"services": strings.Join(names, ",")}, nil
		}},
		{"identity", func() (map[string]any, error) {
			id, err := c.Identity(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"product_name": id.ProductName, "serial_number": fmt.Sprintf("0x%08X", id.SerialNumber), "product_code": id.ProductCode}, nil
		}},
		{"tcpip-interface", func() (map[string]any, error) { return tcpipDetail(ctx, c) }},
		{"ethernet-link", func() (map[string]any, error) { return linkDetail(ctx, c) }},
		{"connection-manager", func() (map[string]any, error) {
			cs, err := e.counters(ctx, c)
			if err != nil {
				return nil, err
			}
			m := map[string]any{}
			for _, k := range cs {
				if k.Error != nil {
					// The device answered and refused the attribute (OpENer's
					// Connection Manager instance has none): a refusal, not an
					// internal error.
					return nil, errs.New(errs.KindRefused, k.Error.Code,
						fmt.Sprintf("attribute %d (%s): %s", k.Attribute, k.Name, k.Error.Message), nil)
				}
				m[k.Name] = *k.Value
			}
			return m, nil
		}},
	}
	if dialErr != nil {
		r.Checks = append(r.Checks, types.Check{Name: "register-session", Status: "skipped"})
	} else {
		add("register-session", total-dialDur, map[string]any{"session": fmt.Sprintf("0x%08X", c.Session())}, cerr)
	}
	for _, ch := range checks {
		if dialErr != nil || cerr != nil {
			r.Checks = append(r.Checks, types.Check{Name: ch.name, Status: "skipped"})
			continue
		}
		cs := time.Now()
		detail, err := ch.run()
		add(ch.name, time.Since(cs), detail, err)
	}
	for _, ch := range r.Checks {
		switch ch.Status {
		case "ok":
			r.Passed++
		case "error":
			r.Failed++
		default:
			r.Skipped++
		}
	}
	r.OK = r.Failed == 0 && r.Skipped == 0
	r.DurationMs = ms(time.Since(start))
	code := errs.ExitOK
	if !r.OK {
		code = errs.ExitPartial
	}
	return r, code, nil
}

// ipText renders an IP address the TCP/IP Interface object carries as a UDINT.
func ipText(v uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// tcpipDetail reads the TCP/IP Interface object (0xF5): status, interface configuration,
// host name.
func tcpipDetail(ctx context.Context, c *client.Client) (map[string]any, error) {
	st, err := c.GetAttributeSingle(ctx, cip.ClassTCPIPInterface, 1, 1)
	if err != nil {
		return nil, err
	}
	status, ok := uintValue(st).(uint64)
	if !ok {
		return nil, fmt.Errorf("%w: interface status of %d octets", cip.ErrMalformed, len(st))
	}
	m := map[string]any{"status": status}
	cfg, err := c.GetAttributeSingle(ctx, cip.ClassTCPIPInterface, 1, 5)
	if err != nil {
		return nil, err
	}
	r := cip.NewReader(cfg)
	ip, mask, gw := r.Uint32(), r.Uint32(), r.Uint32()
	if r.Err() != nil {
		return nil, fmt.Errorf("%w: interface configuration of %d octets", cip.ErrMalformed, len(cfg))
	}
	m["ip"], m["mask"], m["gateway"] = ipText(ip), ipText(mask), ipText(gw)
	if hn, err := c.GetAttributeSingle(ctx, cip.ClassTCPIPInterface, 1, 6); err == nil {
		hr := cip.NewReader(hn)
		if name := hr.String(); hr.Err() == nil && name != "" {
			m["host_name"] = name
		}
	} else if fatal(err) {
		return nil, err
	}
	return m, nil
}

// linkDetail reads the Ethernet Link object (0xF6): speed, flags, MAC address.
func linkDetail(ctx context.Context, c *client.Client) (map[string]any, error) {
	speed, err := c.GetAttributeSingle(ctx, cip.ClassEthernetLink, 1, 1)
	if err != nil {
		return nil, err
	}
	flags, err := c.GetAttributeSingle(ctx, cip.ClassEthernetLink, 1, 2)
	if err != nil {
		return nil, err
	}
	mac, err := c.GetAttributeSingle(ctx, cip.ClassEthernetLink, 1, 3)
	if err != nil {
		return nil, err
	}
	sv, ok1 := uintValue(speed).(uint64)
	fv, ok2 := uintValue(flags).(uint64)
	if !ok1 || !ok2 || len(mac) != 6 {
		return nil, fmt.Errorf("%w: Ethernet Link attributes of %d, %d and %d octets", cip.ErrMalformed, len(speed), len(flags), len(mac))
	}
	return map[string]any{
		"speed_mbps": sv, "link_active": fv&1 != 0, "full_duplex": fv&2 != 0,
		"mac": net.HardwareAddr(mac).String(),
	}, nil
}
