package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Messaging describes how the requests of a result travelled.
type Messaging struct {
	Target string `json:"target"`
	// Route is the --route the requests were sent along; omitted when direct.
	Route string `json:"route,omitempty"`
	// Connected is true for requests over a Class 3 connection.
	Connected bool `json:"connected"`
}

func (m Messaging) via() string {
	s := m.Target
	if m.Route != "" {
		s += " route " + m.Route
	}
	if m.Connected {
		s += " (connected)"
	}
	return s
}

// AttributeResult is the result of `read`, `write`, `assembly read` and `assembly write`.
type AttributeResult struct {
	Messaging
	// Service is the service sent: "Get_Attribute_Single", "Get_Attributes_All" or
	// "Set_Attribute_Single".
	Service   string  `json:"service"`
	Path      string  `json:"path"`
	Class     uint16  `json:"class"`
	ClassName string  `json:"class_name"`
	Instance  uint32  `json:"instance"`
	Attribute *uint16 `json:"attribute,omitempty"`
	// Length and Data are the value read or written: its size and its octets as hex.
	Length int    `json:"length"`
	Data   string `json:"data"`
	// Type and Value are the value decoded as --type; omitted without it.
	Type  string  `json:"type,omitempty"`
	Value any     `json:"value,omitempty"`
	RTTMs float64 `json:"rtt_ms"`
	// valueText is the text form of Value.
	ValueText string `json:"-"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *AttributeResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	verb := "Read"
	if strings.HasPrefix(r.Service, "Set") {
		verb = "Wrote"
	}
	_, _ = fmt.Fprintf(&b, "%s %s (%s) %s at %s: %d octets in %.1f ms\n", verb, r.Path, r.ClassName, r.Service, r.via(), r.Length, r.RTTMs)
	kv(&b, "  ", "data", hexDump(r.Data))
	if r.Type != "" {
		kv(&b, "  ", "value", r.Type+" "+r.ValueText)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *AttributeResult) TableHeaders() []string {
	return []string{"PATH", "SERVICE", "LENGTH", "DATA", "TYPE", "VALUE"}
}

// TableRows implements format.TableMarshaler.
func (r *AttributeResult) TableRows() [][]string {
	return [][]string{{r.Path, r.Service, strconv.Itoa(r.Length), orDash(r.Data), orDash(r.Type), orDash(r.ValueText)}}
}

// hexDump renders hex with a space between octets, "-" for none.
func hexDump(h string) string {
	if h == "" {
		return "-"
	}
	var b strings.Builder
	for i := 0; i+1 < len(h); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(h[i : i+2])
	}
	return b.String()
}

// ClassEntry is an object class a device implements.
type ClassEntry struct {
	Class uint16 `json:"class"`
	Hex   string `json:"hex"`
	Name  string `json:"name"`
	// VendorSpecific marks a vendor extension rather than a CIP object.
	VendorSpecific bool `json:"vendor_specific"`
}

// ObjectsResult is the result of `objects`: the class list of the Message Router.
type ObjectsResult struct {
	Messaging
	Count   int          `json:"count"`
	Objects []ClassEntry `json:"objects"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ObjectsResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Objects of %s (Message Router @2/1/1): %d classes\n", r.via(), r.Count)
	for _, o := range r.Objects {
		_, _ = fmt.Fprintf(&b, "  %-6s %5d  %s\n", o.Hex, o.Class, o.Name)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ObjectsResult) TableHeaders() []string {
	return []string{"CLASS", "HEX", "NAME", "VENDOR_SPECIFIC"}
}

// TableRows implements format.TableMarshaler.
func (r *ObjectsResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Objects))
	for _, o := range r.Objects {
		rows = append(rows, []string{strconv.Itoa(int(o.Class)), o.Hex, o.Name, strconv.FormatBool(o.VendorSpecific)})
	}
	return rows
}

// AttributeValue is one attribute of an object: its value as hex, decoded where its type is
// known, or the error the target answered.
type AttributeValue struct {
	Attribute uint16     `json:"attribute"`
	Name      string     `json:"name"`
	Data      string     `json:"data,omitempty"`
	Value     any        `json:"value,omitempty"`
	Error     *ErrorInfo `json:"error,omitempty"`
}

func (a AttributeValue) text() string {
	if a.Error != nil {
		return "error: " + a.Error.Message
	}
	if a.Value != nil {
		return fmt.Sprintf("%v (%s)", a.Value, hexDump(a.Data))
	}
	return hexDump(a.Data)
}

// ObjectResult is the result of `object`: the class attributes and an instance.
type ObjectResult struct {
	Messaging
	Class           uint16           `json:"class"`
	ClassName       string           `json:"class_name"`
	ClassAttributes []AttributeValue `json:"class_attributes"`
	// Instance is the instance read with Get_Attributes_All, InstanceData its data;
	// InstanceError is set when the read failed.
	Instance      uint32     `json:"instance"`
	InstanceData  string     `json:"instance_data,omitempty"`
	InstanceError *ErrorInfo `json:"instance_error,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ObjectResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Class 0x%02X %s at %s\n", r.Class, r.ClassName, r.via())
	b.WriteString("Class attributes (instance 0):\n")
	for _, a := range r.ClassAttributes {
		_, _ = fmt.Fprintf(&b, "  %2d %-22s %s\n", a.Attribute, a.Name, a.text())
	}
	_, _ = fmt.Fprintf(&b, "Instance %d (Get_Attributes_All):\n", r.Instance)
	if r.InstanceError != nil {
		_, _ = fmt.Fprintf(&b, "  error: %s\n", r.InstanceError.Message)
	} else {
		_, _ = fmt.Fprintf(&b, "  %s\n", hexDump(r.InstanceData))
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ObjectResult) TableHeaders() []string {
	return []string{"INSTANCE", "ATTRIBUTE", "NAME", "VALUE"}
}

// TableRows implements format.TableMarshaler.
func (r *ObjectResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.ClassAttributes)+1)
	for _, a := range r.ClassAttributes {
		rows = append(rows, []string{"0", strconv.Itoa(int(a.Attribute)), a.Name, a.text()})
	}
	v := hexDump(r.InstanceData)
	if r.InstanceError != nil {
		v = "error: " + r.InstanceError.Message
	}
	return append(rows, []string{strconv.Itoa(int(r.Instance)), "all", "Get_Attributes_All", v})
}

// ResetResult is the result of `reset`.
type ResetResult struct {
	Target string `json:"target"`
	Route  string `json:"route,omitempty"`
	// Type is the reset type sent: 0 power cycle, 1 factory defaults.
	Type  int     `json:"type"`
	RTTMs float64 `json:"rtt_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ResetResult) MarshalTextOutput() (string, error) {
	what := "power cycle"
	if r.Type == 1 {
		what = "return to factory defaults"
	}
	return fmt.Sprintf("Reset type %d (%s) of %s accepted in %.1f ms", r.Type, what, r.Target, r.RTTMs), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ResetResult) TableHeaders() []string { return []string{"TARGET", "TYPE", "RTT_MS"} }

// TableRows implements format.TableMarshaler.
func (r *ResetResult) TableRows() [][]string {
	return [][]string{{r.Target, strconv.Itoa(r.Type), fmt.Sprintf("%.1f", r.RTTMs)}}
}

// Counter is a named counter.
type Counter struct {
	Attribute uint16     `json:"attribute"`
	Name      string     `json:"name"`
	Value     *uint64    `json:"value,omitempty"`
	Error     *ErrorInfo `json:"error,omitempty"`
}

func (c Counter) text() string {
	if c.Error != nil {
		return "error: " + c.Error.Message
	}
	return strconv.FormatUint(*c.Value, 10)
}

// ConnectionsResult is the result of `connections`: what a device says about its CIP
// connections.
type ConnectionsResult struct {
	Target string `json:"target"`
	// MaxConnections and ActiveConnections are the Message Router's attributes 2 and 3,
	// where the device has them.
	MaxConnections    *uint64 `json:"max_connections,omitempty"`
	ActiveConnections *uint64 `json:"active_connections,omitempty"`
	// Counters are the Connection Manager's attributes 1 to 8.
	Counters []Counter `json:"counters"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ConnectionsResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Connections of %s\n", r.Target)
	kv(&b, "  ", "active", optUint(r.ActiveConnections))
	kv(&b, "  ", "maximum", optUint(r.MaxConnections))
	b.WriteString("Connection Manager counters:\n")
	for _, c := range r.Counters {
		_, _ = fmt.Fprintf(&b, "  %-24s %s\n", c.Name, c.text())
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ConnectionsResult) TableHeaders() []string { return []string{"NAME", "VALUE"} }

// TableRows implements format.TableMarshaler.
func (r *ConnectionsResult) TableRows() [][]string {
	rows := [][]string{{"active_connections", optUint(r.ActiveConnections)}, {"max_connections", optUint(r.MaxConnections)}}
	for _, c := range r.Counters {
		rows = append(rows, []string{c.Name, c.text()})
	}
	return rows
}

func optUint(v *uint64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatUint(*v, 10)
}
