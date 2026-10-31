// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Identity is the identity of a device: the Identity object (class 1, instance 1), as
// Get_Attributes_All and ListIdentity report it.
type Identity struct {
	VendorID       uint16 `json:"vendor_id"`
	VendorName     string `json:"vendor_name"`
	DeviceType     uint16 `json:"device_type"`
	DeviceTypeName string `json:"device_type_name"`
	ProductCode    uint16 `json:"product_code"`
	Revision       string `json:"revision"`
	// Status is the status word (attribute 5); StatusFlags its set flags joined with "|"
	// ("OWNED|CONFIGURED"), "" when none is set.
	Status               uint16 `json:"status"`
	StatusFlags          string `json:"status_flags"`
	ExtendedDeviceStatus uint8  `json:"extended_device_status"`
	SerialNumber         uint32 `json:"serial_number"`
	ProductName          string `json:"product_name"`
	// State is attribute 8, where it was reported (ListIdentity); StateName its meaning.
	State     *uint8 `json:"state,omitempty"`
	StateName string `json:"state_name,omitempty"`
}

// Summary renders the identity on one line.
func (id Identity) Summary() string {
	return fmt.Sprintf("%s (vendor %d %s, %s, product %d, rev %s, serial 0x%08X)",
		quoteName(id.ProductName), id.VendorID, orDash(id.VendorName), orDash(id.DeviceTypeName), id.ProductCode, id.Revision, id.SerialNumber)
}

// lines renders the identity as indented key-value lines.
func (id Identity) lines(b *strings.Builder, indent string) {
	kv(b, indent, "product name", quoteName(id.ProductName))
	kv(b, indent, "vendor", fmt.Sprintf("%d (%s)", id.VendorID, orDash(id.VendorName)))
	kv(b, indent, "device type", fmt.Sprintf("%d (%s)", id.DeviceType, orDash(id.DeviceTypeName)))
	kv(b, indent, "product code", strconv.Itoa(int(id.ProductCode)))
	kv(b, indent, "revision", id.Revision)
	kv(b, indent, "serial number", fmt.Sprintf("0x%08X", id.SerialNumber))
	kv(b, indent, "status", fmt.Sprintf("0x%04X %s", id.Status, statusText(id.StatusFlags, id.ExtendedDeviceStatus)))
	if id.State != nil {
		kv(b, indent, "state", fmt.Sprintf("%d (%s)", *id.State, orDash(id.StateName)))
	}
}

func statusText(flags string, ext uint8) string {
	s := orDash(flags)
	if ext != 0 {
		s += fmt.Sprintf(" extended %d", ext)
	}
	return s
}

// rows renders the identity as ATTRIBUTE/VALUE table rows.
func (id Identity) rows(prefix string) [][]string {
	rows := [][]string{
		{prefix + "product_name", id.ProductName},
		{prefix + "vendor", fmt.Sprintf("%d %s", id.VendorID, id.VendorName)},
		{prefix + "device_type", fmt.Sprintf("%d %s", id.DeviceType, id.DeviceTypeName)},
		{prefix + "product_code", strconv.Itoa(int(id.ProductCode))},
		{prefix + "revision", id.Revision},
		{prefix + "serial_number", fmt.Sprintf("0x%08X", id.SerialNumber)},
		{prefix + "status", fmt.Sprintf("0x%04X %s", id.Status, statusText(id.StatusFlags, id.ExtendedDeviceStatus))},
	}
	if id.State != nil {
		rows = append(rows, []string{prefix + "state", fmt.Sprintf("%d %s", *id.State, id.StateName)})
	}
	return rows
}

// ListIdentity is the identity item of a ListIdentity reply.
type ListIdentity struct {
	ProtocolVersion uint16 `json:"protocol_version"`
	// Socket is the address the device reports for its TCP port.
	Socket   string   `json:"socket"`
	Identity Identity `json:"identity"`
}

// IdentifyResult is the result of `identify`.
type IdentifyResult struct {
	Target string `json:"target"`
	// ListIdentity is the reply to ListIdentity over the TCP session.
	ListIdentity ListIdentity `json:"list_identity"`
	// Identity is the Identity object read with Get_Attributes_All; omitted when the read
	// failed, with IdentityError set.
	Identity      *Identity  `json:"identity,omitempty"`
	IdentityError *ErrorInfo `json:"identity_error,omitempty"`
	DurationMs    float64    `json:"duration_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *IdentifyResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Identity of %s (in %.0f ms)\n", r.Target, r.DurationMs)
	b.WriteString("ListIdentity:\n")
	kv(&b, "  ", "protocol version", strconv.Itoa(int(r.ListIdentity.ProtocolVersion)))
	kv(&b, "  ", "socket", r.ListIdentity.Socket)
	r.ListIdentity.Identity.lines(&b, "  ")
	b.WriteString("Identity object (Get_Attributes_All @1/1):\n")
	if r.Identity != nil {
		r.Identity.lines(&b, "  ")
	} else {
		_, _ = fmt.Fprintf(&b, "  error: %s\n", ErrorMessage(r.IdentityError))
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *IdentifyResult) TableHeaders() []string { return []string{"ATTRIBUTE", "VALUE"} }

// TableRows implements format.TableMarshaler.
func (r *IdentifyResult) TableRows() [][]string {
	rows := [][]string{{"socket", r.ListIdentity.Socket}}
	rows = append(rows, r.ListIdentity.Identity.rows("list_identity.")...)
	if r.Identity != nil {
		rows = append(rows, r.Identity.rows("identity.")...)
	} else {
		rows = append(rows, []string{"identity.error", ErrorMessage(r.IdentityError)})
	}
	return rows
}

// StatusResult is the result of `status`: the health of a device in one line.
type StatusResult struct {
	Target string `json:"target"`
	// Healthy is false when the status word reports a fault or the state is not
	// operational.
	Healthy  bool     `json:"healthy"`
	Faults   []string `json:"faults"`
	Identity Identity `json:"identity"`
	RTTMs    float64  `json:"rtt_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *StatusResult) MarshalTextOutput() (string, error) {
	health := "ok"
	if !r.Healthy {
		health = "FAULT: " + strings.Join(r.Faults, ", ")
	}
	state := "-"
	if r.Identity.State != nil {
		state = fmt.Sprintf("%d %s", *r.Identity.State, r.Identity.StateName)
	}
	return fmt.Sprintf("%s: %s  state %s  status 0x%04X %s  %s  (%.1f ms)", r.Target, health, state,
		r.Identity.Status, statusText(r.Identity.StatusFlags, r.Identity.ExtendedDeviceStatus), r.Identity.Summary(), r.RTTMs), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *StatusResult) TableHeaders() []string {
	return []string{"TARGET", "HEALTHY", "STATE", "STATUS", "PRODUCT", "VENDOR", "REVISION", "RTT_MS"}
}

// TableRows implements format.TableMarshaler.
func (r *StatusResult) TableRows() [][]string {
	state := "-"
	if r.Identity.State != nil {
		state = r.Identity.StateName
	}
	return [][]string{{r.Target, strconv.FormatBool(r.Healthy), state, fmt.Sprintf("0x%04X", r.Identity.Status),
		r.Identity.ProductName, strconv.Itoa(int(r.Identity.VendorID)), r.Identity.Revision, fmt.Sprintf("%.1f", r.RTTMs)}}
}

// Device is a device that answered discover.
type Device struct {
	// Address is where the reply came from (ip:port).
	Address         string   `json:"address"`
	Socket          string   `json:"socket"`
	ProtocolVersion uint16   `json:"protocol_version"`
	Identity        Identity `json:"identity"`
}

// DiscoverResult is the result of `discover`.
type DiscoverResult struct {
	Address    string   `json:"address"`
	WaitMs     float64  `json:"wait_ms"`
	Found      int      `json:"found"`
	DurationMs float64  `json:"duration_ms"`
	Devices    []Device `json:"devices"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *DiscoverResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "ListIdentity to %s: %d device(s) in %.0f ms\n", r.Address, r.Found, r.DurationMs)
	for _, d := range r.Devices {
		_, _ = fmt.Fprintf(&b, "  %-21s %s\n", d.Address, d.Identity.Summary())
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *DiscoverResult) TableHeaders() []string {
	return []string{"ADDRESS", "PRODUCT", "VENDOR", "DEVICE_TYPE", "PRODUCT_CODE", "REVISION", "SERIAL", "STATE"}
}

// TableRows implements format.TableMarshaler.
func (r *DiscoverResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Devices))
	for _, d := range r.Devices {
		id := d.Identity
		state := "-"
		if id.State != nil {
			state = id.StateName
		}
		rows = append(rows, []string{d.Address, id.ProductName, fmt.Sprintf("%d %s", id.VendorID, id.VendorName), id.DeviceTypeName,
			strconv.Itoa(int(id.ProductCode)), id.Revision, fmt.Sprintf("0x%08X", id.SerialNumber), state})
	}
	return rows
}

// Service is an item of a ListServices reply.
type Service struct {
	Name         string `json:"name"`
	Version      uint16 `json:"version"`
	Capabilities uint16 `json:"capabilities"`
	// TCP and UDP are the capability flags: CIP over the TCP session, and Class 0/1 I/O
	// over UDP.
	TCP bool `json:"tcp"`
	UDP bool `json:"udp"`
}

// ServicesResult is the result of `services`.
type ServicesResult struct {
	Target   string    `json:"target"`
	Count    int       `json:"count"`
	Services []Service `json:"services"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ServicesResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "ListServices of %s: %d service(s)\n", r.Target, r.Count)
	for _, s := range r.Services {
		_, _ = fmt.Fprintf(&b, "  %-16s version %d  capabilities 0x%04X  tcp=%t udp=%t\n", s.Name, s.Version, s.Capabilities, s.TCP, s.UDP)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ServicesResult) TableHeaders() []string {
	return []string{"NAME", "VERSION", "CAPABILITIES", "TCP", "UDP"}
}

// TableRows implements format.TableMarshaler.
func (r *ServicesResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Services))
	for _, s := range r.Services {
		rows = append(rows, []string{s.Name, strconv.Itoa(int(s.Version)), fmt.Sprintf("0x%04X", s.Capabilities), strconv.FormatBool(s.TCP), strconv.FormatBool(s.UDP)})
	}
	return rows
}

// Interface is an item of a ListInterfaces reply.
type Interface struct {
	Type     uint16 `json:"type"`
	TypeName string `json:"type_name"`
	Data     string `json:"data"`
}

// InterfacesResult is the result of `interfaces`.
type InterfacesResult struct {
	Target     string      `json:"target"`
	Count      int         `json:"count"`
	Interfaces []Interface `json:"interfaces"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *InterfacesResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "ListInterfaces of %s: %d interface(s)\n", r.Target, r.Count)
	for _, it := range r.Interfaces {
		_, _ = fmt.Fprintf(&b, "  0x%04X %s  %s\n", it.Type, it.TypeName, orDash(it.Data))
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *InterfacesResult) TableHeaders() []string { return []string{"TYPE", "NAME", "DATA"} }

// TableRows implements format.TableMarshaler.
func (r *InterfacesResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Interfaces))
	for _, it := range r.Interfaces {
		rows = append(rows, []string{fmt.Sprintf("0x%04X", it.Type), it.TypeName, orDash(it.Data)})
	}
	return rows
}

// kv writes one "key: value" line.
func kv(b *strings.Builder, indent, key, value string) {
	_, _ = fmt.Fprintf(b, "%s%-15s %s\n", indent, key+":", value)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func quoteName(s string) string { return strconv.Quote(s) }
