// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Connection describes a CIP connection: its IDs, triad and actual intervals.
type Connection struct {
	// Class is the transport class: 0, 1 (I/O) or 3 (explicit messaging).
	Class    uint8  `json:"class"`
	OTConnID uint32 `json:"ot_connection_id"`
	TOConnID uint32 `json:"to_connection_id"`
	// ConnectionSerial, OriginatorVendor and OriginatorSerial are the connection triad,
	// which forward-close needs.
	ConnectionSerial uint16 `json:"connection_serial"`
	OriginatorVendor uint16 `json:"originator_vendor"`
	OriginatorSerial uint32 `json:"originator_serial"`
	// OTRPIMs and TORPIMs are the actual packet intervals.
	OTRPIMs float64 `json:"ot_rpi_ms"`
	TORPIMs float64 `json:"to_rpi_ms"`
	Path    string  `json:"path"`
}

func (c Connection) lines(b *strings.Builder, indent string) {
	kv(b, indent, "class", strconv.Itoa(int(c.Class)))
	kv(b, indent, "O->T conn ID", fmt.Sprintf("0x%08X", c.OTConnID))
	kv(b, indent, "T->O conn ID", fmt.Sprintf("0x%08X", c.TOConnID))
	kv(b, indent, "triad", fmt.Sprintf("serial %d vendor %d originator %d", c.ConnectionSerial, c.OriginatorVendor, c.OriginatorSerial))
	kv(b, indent, "API", fmt.Sprintf("O->T %g ms, T->O %g ms", c.OTRPIMs, c.TORPIMs))
	kv(b, indent, "path", c.Path)
}

func (c Connection) rows(prefix string) [][]string {
	return [][]string{
		{prefix + "class", strconv.Itoa(int(c.Class))},
		{prefix + "ot_connection_id", fmt.Sprintf("0x%08X", c.OTConnID)},
		{prefix + "to_connection_id", fmt.Sprintf("0x%08X", c.TOConnID)},
		{prefix + "connection_serial", strconv.Itoa(int(c.ConnectionSerial))},
		{prefix + "originator_vendor", strconv.Itoa(int(c.OriginatorVendor))},
		{prefix + "originator_serial", strconv.FormatUint(uint64(c.OriginatorSerial), 10)},
		{prefix + "ot_rpi_ms", fmt.Sprintf("%g", c.OTRPIMs)},
		{prefix + "to_rpi_ms", fmt.Sprintf("%g", c.TORPIMs)},
		{prefix + "path", c.Path},
	}
}

// SessionResult is the result of `session`.
type SessionResult struct {
	Target string `json:"target"`
	Route  string `json:"route,omitempty"`
	Local  string `json:"local"`
	// Session is the session handle the target assigned.
	Session uint32 `json:"session"`
	// Connection is the Class 3 connection opened with --connected.
	Connection *Connection `json:"connection,omitempty"`
	HeldMs     float64     `json:"held_ms"`
	DurationMs float64     `json:"duration_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *SessionResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Session 0x%08X with %s from %s (registered in %.1f ms)\n", r.Session, r.Target, r.Local, r.DurationMs)
	if r.Route != "" {
		kv(&b, "  ", "route", r.Route)
	}
	if r.Connection != nil {
		b.WriteString("Class 3 connection:\n")
		r.Connection.lines(&b, "  ")
	}
	if r.HeldMs > 0 {
		_, _ = fmt.Fprintf(&b, "held for %.0f ms, then closed\n", r.HeldMs)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *SessionResult) TableHeaders() []string { return []string{"ATTRIBUTE", "VALUE"} }

// TableRows implements format.TableMarshaler.
func (r *SessionResult) TableRows() [][]string {
	rows := [][]string{{"target", r.Target}, {"local", r.Local}, {"session", fmt.Sprintf("0x%08X", r.Session)}}
	if r.Connection != nil {
		rows = append(rows, r.Connection.rows("connection.")...)
	}
	return rows
}

// ForwardOpenResult is the result of `forward-open`.
type ForwardOpenResult struct {
	Target string `json:"target"`
	Route  string `json:"route,omitempty"`
	// Service is "Forward_Open" or "Large_Forward_Open".
	Service    string     `json:"service"`
	Transport  string     `json:"transport"`
	OTSize     uint16     `json:"ot_size"`
	TOSize     uint16     `json:"to_size"`
	Connection Connection `json:"connection"`
	// Closed is true when the connection was closed again with Forward_Close; with --keep
	// it is false and the target keeps it until it times out or forward-close closes it.
	Closed bool    `json:"closed"`
	RTTMs  float64 `json:"rtt_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ForwardOpenResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s to %s accepted in %.1f ms (%s, O->T %d octets, T->O %d octets)\n", r.Service, r.Target, r.RTTMs, r.Transport, r.OTSize, r.TOSize)
	r.Connection.lines(&b, "  ")
	if r.Closed {
		b.WriteString("closed again with Forward_Close\n")
	} else {
		_, _ = fmt.Fprintf(&b, "kept open: close it with forward-close %s --connection-serial %d --originator-vendor %d --originator-serial %d and the same path flags\n",
			r.Target, r.Connection.ConnectionSerial, r.Connection.OriginatorVendor, r.Connection.OriginatorSerial)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ForwardOpenResult) TableHeaders() []string { return []string{"ATTRIBUTE", "VALUE"} }

// TableRows implements format.TableMarshaler.
func (r *ForwardOpenResult) TableRows() [][]string {
	rows := [][]string{{"service", r.Service}, {"transport", r.Transport}}
	rows = append(rows, r.Connection.rows("")...)
	return append(rows, []string{"closed", strconv.FormatBool(r.Closed)})
}

// ForwardCloseResult is the result of `forward-close`.
type ForwardCloseResult struct {
	Target           string  `json:"target"`
	Route            string  `json:"route,omitempty"`
	ConnectionSerial uint16  `json:"connection_serial"`
	OriginatorVendor uint16  `json:"originator_vendor"`
	OriginatorSerial uint32  `json:"originator_serial"`
	Path             string  `json:"path"`
	RTTMs            float64 `json:"rtt_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ForwardCloseResult) MarshalTextOutput() (string, error) {
	return fmt.Sprintf("Forward_Close of connection serial %d vendor %d originator %d (%s) at %s accepted in %.1f ms",
		r.ConnectionSerial, r.OriginatorVendor, r.OriginatorSerial, r.Path, r.Target, r.RTTMs), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ForwardCloseResult) TableHeaders() []string {
	return []string{"TARGET", "CONNECTION_SERIAL", "ORIGINATOR_VENDOR", "ORIGINATOR_SERIAL", "PATH"}
}

// TableRows implements format.TableMarshaler.
func (r *ForwardCloseResult) TableRows() [][]string {
	return [][]string{{r.Target, strconv.Itoa(int(r.ConnectionSerial)), strconv.Itoa(int(r.OriginatorVendor)), strconv.FormatUint(uint64(r.OriginatorSerial), 10), r.Path}}
}

// IOSample is one record of the `io` stream: input data as it arrived.
type IOSample struct {
	// ReceivedAt is when the data arrived (RFC 3339, UTC, milliseconds).
	ReceivedAt string `json:"received_at"`
	// Seq counts the samples written, from 1.
	Seq  uint64 `json:"seq"`
	Run  bool   `json:"run"`
	Size int    `json:"size"`
	Data string `json:"data"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (s *IOSample) MarshalTextOutput() (string, error) {
	mode := "run "
	if !s.Run {
		mode = "idle"
	}
	return fmt.Sprintf("%s  #%-6d %s  %s", s.ReceivedAt, s.Seq, mode, hexDump(s.Data)), nil
}

// TableHeaders implements format.TableMarshaler.
func (s *IOSample) TableHeaders() []string { return []string{"RECEIVED", "SEQ", "RUN", "SIZE", "DATA"} }

// TableRows implements format.TableMarshaler.
func (s *IOSample) TableRows() [][]string {
	return [][]string{{s.ReceivedAt, strconv.FormatUint(s.Seq, 10), strconv.FormatBool(s.Run), strconv.Itoa(s.Size), orDash(s.Data)}}
}

// Check is one check of `diagnose`.
type Check struct {
	Name string `json:"name"`
	// Status is "ok", "error" or "skipped" (a check that needs one that failed).
	Status     string         `json:"status"`
	DurationMs float64        `json:"duration_ms"`
	Detail     map[string]any `json:"detail,omitempty"`
	Error      *ErrorInfo     `json:"error,omitempty"`
}

// DiagnoseResult is the result of `diagnose`.
type DiagnoseResult struct {
	Target string `json:"target"`
	// OK is true when every check passed.
	OK         bool    `json:"ok"`
	Passed     int     `json:"passed"`
	Failed     int     `json:"failed"`
	Skipped    int     `json:"skipped"`
	DurationMs float64 `json:"duration_ms"`
	Checks     []Check `json:"checks"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *DiagnoseResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Diagnosis of %s: %d passed, %d failed, %d skipped in %.0f ms\n", r.Target, r.Passed, r.Failed, r.Skipped, r.DurationMs)
	for _, c := range r.Checks {
		_, _ = fmt.Fprintf(&b, "  %-7s %-20s %8.1f ms  %s\n", strings.ToUpper(c.Status), c.Name, c.DurationMs, c.detailText())
	}
	return b.String(), nil
}

// detailText renders the detail of a check, or its error.
func (c Check) detailText() string {
	if c.Error != nil {
		return c.Error.Message
	}
	return DetailText(c.Detail)
}

// DetailText renders a map as key=value pairs in key order.
func DetailText(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return strings.Join(parts, " ")
}

// TableHeaders implements format.TableMarshaler.
func (r *DiagnoseResult) TableHeaders() []string { return []string{"CHECK", "STATUS", "MS", "DETAIL"} }

// TableRows implements format.TableMarshaler.
func (r *DiagnoseResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Checks))
	for _, c := range r.Checks {
		rows = append(rows, []string{c.Name, c.Status, fmt.Sprintf("%.1f", c.DurationMs), orDash(c.detailText())})
	}
	return rows
}
