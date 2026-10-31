// SPDX-License-Identifier: MIT

package eip

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/otfabric/go-ethernetip/client"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// DefaultDiscoverWait is how long discover collects replies.
const DefaultDiscoverWait = 2 * time.Second

// CheckDiscover validates `discover` and returns the address the request goes to.
func (e Env) CheckDiscover(cfg config.DiscoverConfig) (string, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return "", invalid(errs.CodeInvalidAddress, "--address is required: a broadcast address (255.255.255.255, 192.168.1.255) or one device")
	}
	t, err := config.ParseHost(cfg.Address, e.Port())
	if err != nil {
		return "", errs.InvalidInput(errs.CodeInvalidAddress, "invalid --address: "+err.Error(), err)
	}
	if cfg.Local != "" {
		if a, err := netip.ParseAddr(strings.TrimSpace(cfg.Local)); err != nil || !a.Is4() {
			return "", invalid(errs.CodeInvalidAddress, "invalid --local %q: an IPv4 address of this host", cfg.Local)
		}
	}
	if cfg.Wait < 0 || cfg.MaxDelay < 0 || cfg.MaxDevices < 0 {
		return "", invalid(errs.CodeInvalidValue, "--wait, --max-delay and --max-devices cannot be negative")
	}
	return t.Addr(), nil
}

// Discover runs `discover`: exactly one ListIdentity datagram, and the replies that arrive
// within --wait.
func (e Env) Discover(ctx context.Context, cfg config.DiscoverConfig) (*types.DiscoverResult, error) {
	addr, err := e.CheckDiscover(cfg)
	if err != nil {
		return nil, err
	}
	wait := cfg.Wait
	if wait == 0 {
		wait = DefaultDiscoverWait
	}
	local := strings.TrimSpace(cfg.Local)
	if local != "" {
		local = net.JoinHostPort(local, "0")
	}
	start := time.Now()
	devs, err := client.Discover(ctx, client.DiscoverOptions{Address: addr, Local: local, Wait: wait, MaxDelay: cfg.MaxDelay, MaxDevices: cfg.MaxDevices})
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.DiscoverResult{Address: addr, WaitMs: ms(wait), Devices: []types.Device{}}
	for _, d := range devs {
		r.Devices = append(r.Devices, types.Device{Address: d.Addr.String(), Socket: d.Identity.Socket.AddrPort().String(),
			ProtocolVersion: d.Identity.ProtocolVersion, Identity: IdentityOf(d.Identity.Identity, true)})
	}
	r.Found = len(r.Devices)
	r.DurationMs = ms(time.Since(start))
	return r, nil
}
