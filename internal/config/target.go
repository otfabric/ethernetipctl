package config

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Target is the address of a device: an IPv4 address or host name, and a port.
type Target struct {
	Host string
	Port uint16
}

// Addr returns host:port.
func (t Target) Addr() string { return net.JoinHostPort(t.Host, strconv.Itoa(int(t.Port))) }

// String returns host:port, the form used in output.
func (t Target) String() string { return t.Addr() }

// ParseHost parses host[:port]: an IPv4 address or a host name, with the port defPort
// when it names none. It refuses IPv6 addresses, ranges, prefixes and wildcards: a command
// addresses one device, it never scans.
func ParseHost(s string, defPort uint16) (Target, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Target{}, fmt.Errorf("no target given: name the device as host[:port]")
	}
	host, port := raw, defPort
	if h, p, err := net.SplitHostPort(raw); err == nil {
		n, perr := strconv.ParseUint(p, 10, 16)
		if perr != nil || n == 0 {
			return Target{}, fmt.Errorf("invalid target %q: port must be 1-65535", raw)
		}
		host, port = h, uint16(n)
	} else if strings.Count(raw, ":") > 0 {
		return Target{}, fmt.Errorf("invalid target %q: use host[:port] with an IPv4 address or a host name (IPv6 is not supported)", raw)
	}
	if port == 0 {
		port = DefaultPort
	}
	if err := checkHost(host); err != nil {
		return Target{}, fmt.Errorf("invalid target %q: %w", raw, err)
	}
	return Target{Host: host, Port: port}, nil
}

// checkHost accepts an IPv4 address or a host name.
func checkHost(host string) error {
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if strings.ContainsAny(host, "/*, ") || strings.Contains(host, "-") && strings.Trim(host, "0123456789.-") == "" {
		return fmt.Errorf("one address only: prefixes, ranges, lists and wildcards are not accepted (ethernetipctl does not scan)")
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if !a.Is4() {
			return fmt.Errorf("IPv6 is not supported: use an IPv4 address")
		}
		return nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("%q is neither an IPv4 address nor a host name", host)
		}
		for _, r := range label {
			if !hostRune(r) {
				return fmt.Errorf("%q is neither an IPv4 address nor a host name", host)
			}
		}
	}
	if allDigitsAndDots(host) {
		return fmt.Errorf("%q is not a valid IPv4 address", host)
	}
	return nil
}

func allDigitsAndDots(s string) bool {
	for _, r := range s {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// hostRune reports whether r may appear in a host name label.
func hostRune(r rune) bool {
	return r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}
