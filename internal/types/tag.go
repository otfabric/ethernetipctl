package types

import (
	"fmt"
	"strconv"
	"strings"
)

// TagField is a decoded member of a structure value.
type TagField struct {
	Name   string     `json:"name"`
	Type   string     `json:"type"`
	Value  any        `json:"value,omitempty"`
	Data   string     `json:"data,omitempty"`
	Fields []TagField `json:"fields,omitempty"`
}

func fieldLines(b *strings.Builder, fs []TagField, indent string) {
	for _, f := range fs {
		if len(f.Fields) > 0 {
			_, _ = fmt.Fprintf(b, "%s%s (%s):\n", indent, f.Name, f.Type)
			fieldLines(b, f.Fields, indent+"  ")
			continue
		}
		v := valueText(f.Value)
		if f.Value == nil {
			v = hexDump(f.Data)
		}
		_, _ = fmt.Fprintf(b, "%s%-20s %-8s %s\n", indent, f.Name, f.Type, v)
	}
}

// TagReadResult is the result of `tag read` and `tag decode`.
type TagReadResult struct {
	Messaging
	Tag string `json:"tag"`
	// Type is the atomic type ("DINT"), "STRING" for a Logix string, or "struct" with
	// Handle the structure handle.
	Type   string  `json:"type"`
	Handle *uint16 `json:"handle,omitempty"`
	// Struct is the structure's name (its template), with --decode.
	Struct string `json:"struct,omitempty"`
	Count  int    `json:"count"`
	Data   string `json:"data"`
	// Values are the elements, for atomic types and strings.
	Values []any `json:"values,omitempty"`
	// Fields is the decoded structure, with --decode.
	Fields []TagField `json:"fields,omitempty"`
	RTTMs  float64    `json:"rtt_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *TagReadResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	t := fmt.Sprintf("%s[%d]", r.Type, r.Count)
	switch {
	case r.Struct != "" && r.Handle != nil:
		t = fmt.Sprintf("%s[%d] (handle 0x%04X)", r.Struct, r.Count, *r.Handle)
	case r.Handle != nil:
		t = fmt.Sprintf("%s handle 0x%04X[%d]", r.Type, *r.Handle, r.Count)
	}
	_, _ = fmt.Fprintf(&b, "%s = %s at %s in %.1f ms\n", r.Tag, t, r.via(), r.RTTMs)
	for i, v := range r.Values {
		_, _ = fmt.Fprintf(&b, "  [%d] %v\n", i, valueText(v))
	}
	if len(r.Fields) > 0 {
		fieldLines(&b, r.Fields, "  ")
	}
	if len(r.Values) == 0 && len(r.Fields) == 0 {
		_, _ = fmt.Fprintf(&b, "  %s\n", hexDump(r.Data))
	}
	return b.String(), nil
}

func valueText(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	return fmt.Sprint(v)
}

// TableHeaders implements format.TableMarshaler.
func (r *TagReadResult) TableHeaders() []string { return []string{"TAG", "INDEX", "TYPE", "VALUE"} }

// TableRows implements format.TableMarshaler.
func (r *TagReadResult) TableRows() [][]string {
	var rows [][]string
	for i, v := range r.Values {
		rows = append(rows, []string{r.Tag, strconv.Itoa(i), r.Type, valueText(v)})
	}
	var walk func(prefix string, fs []TagField)
	walk = func(prefix string, fs []TagField) {
		for _, f := range fs {
			if len(f.Fields) > 0 {
				walk(prefix+f.Name+".", f.Fields)
				continue
			}
			v := fmt.Sprint(f.Value)
			if f.Value == nil {
				v = hexDump(f.Data)
			}
			rows = append(rows, []string{r.Tag + "." + prefix + f.Name, "-", f.Type, v})
		}
	}
	walk("", r.Fields)
	if len(rows) == 0 {
		rows = append(rows, []string{r.Tag, "-", r.Type, hexDump(r.Data)})
	}
	return rows
}

// TagWriteResult is the result of `tag write`.
type TagWriteResult struct {
	Messaging
	Tag    string `json:"tag"`
	Type   string `json:"type"`
	Count  int    `json:"count"`
	Values []any  `json:"values"`
	// Bit is the bit written with Read-Modify-Write, for a name that ends in a bit number.
	Bit   *int    `json:"bit,omitempty"`
	RTTMs float64 `json:"rtt_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *TagWriteResult) MarshalTextOutput() (string, error) {
	parts := make([]string, len(r.Values))
	for i, v := range r.Values {
		parts[i] = valueText(v)
	}
	return fmt.Sprintf("Wrote %s = %s[%d] {%s} at %s in %.1f ms", r.Tag, r.Type, r.Count, strings.Join(parts, ", "), r.via(), r.RTTMs), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *TagWriteResult) TableHeaders() []string { return []string{"TAG", "TYPE", "COUNT", "VALUES"} }

// TableRows implements format.TableMarshaler.
func (r *TagWriteResult) TableRows() [][]string {
	parts := make([]string, len(r.Values))
	for i, v := range r.Values {
		parts[i] = valueText(v)
	}
	return [][]string{{r.Tag, r.Type, strconv.Itoa(r.Count), strings.Join(parts, ",")}}
}

// TagEntry is an entry of the tag list.
type TagEntry struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Dims     []uint32 `json:"dims"`
	Instance uint32   `json:"instance"`
	// Template is the template ID of a structure tag.
	Template *uint16 `json:"template,omitempty"`
	System   bool    `json:"system"`
}

func (t TagEntry) dimsText() string {
	if len(t.Dims) == 0 {
		return ""
	}
	parts := make([]string, len(t.Dims))
	for i, d := range t.Dims {
		parts[i] = strconv.FormatUint(uint64(d), 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// TagListResult is the result of `tag list`.
type TagListResult struct {
	Messaging
	// Program is the program whose tags were listed; "" for controller scope.
	Program string `json:"program"`
	// ProgramName is the controller's project name, where the controller reports it.
	ProgramName string     `json:"program_name,omitempty"`
	Count       int        `json:"count"`
	Tags        []TagEntry `json:"tags"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *TagListResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	scope := "controller scope"
	if r.Program != "" {
		scope = r.Program
	}
	_, _ = fmt.Fprintf(&b, "Tags of %s at %s: %d", scope, r.via(), r.Count)
	if r.ProgramName != "" {
		_, _ = fmt.Fprintf(&b, " (project %q)", r.ProgramName)
	}
	b.WriteString("\n")
	for _, t := range r.Tags {
		_, _ = fmt.Fprintf(&b, "  %-32s %-16s instance %d\n", t.Name+t.dimsText(), t.Type, t.Instance)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *TagListResult) TableHeaders() []string { return []string{"NAME", "TYPE", "DIMS", "INSTANCE"} }

// TableRows implements format.TableMarshaler.
func (r *TagListResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Tags))
	for _, t := range r.Tags {
		rows = append(rows, []string{t.Name, t.Type, orDash(t.dimsText()), strconv.FormatUint(uint64(t.Instance), 10)})
	}
	return rows
}

// TemplateMember is a member of a structure template.
type TemplateMember struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Offset uint32 `json:"offset"`
	// Info is the array length of an array member, or the bit number of a BOOL.
	Info   uint16 `json:"info"`
	Hidden bool   `json:"hidden"`
}

// TemplateResult is the result of `tag types`.
type TemplateResult struct {
	Messaging
	ID      uint16           `json:"id"`
	Name    string           `json:"name"`
	Handle  uint16           `json:"handle"`
	Size    uint32           `json:"size"`
	Members []TemplateMember `json:"members"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *TemplateResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Template 0x%04X %s at %s: handle 0x%04X, %d octets, %d members\n", r.ID, r.Name, r.via(), r.Handle, r.Size, len(r.Members))
	for _, m := range r.Members {
		hidden := ""
		if m.Hidden {
			hidden = " (hidden)"
		}
		_, _ = fmt.Fprintf(&b, "  %-24s %-16s offset %-4d info %d%s\n", m.Name, m.Type, m.Offset, m.Info, hidden)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *TemplateResult) TableHeaders() []string {
	return []string{"MEMBER", "TYPE", "OFFSET", "INFO", "HIDDEN"}
}

// TableRows implements format.TableMarshaler.
func (r *TemplateResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Members))
	for _, m := range r.Members {
		rows = append(rows, []string{m.Name, m.Type, strconv.FormatUint(uint64(m.Offset), 10), strconv.Itoa(int(m.Info)), strconv.FormatBool(m.Hidden)})
	}
	return rows
}

// AdapterEvent is one record of the `adapter` stream.
type AdapterEvent struct {
	// Timestamp is when the event happened (RFC 3339, UTC, milliseconds).
	Timestamp string `json:"timestamp"`
	// Event is "listening", "connected", "disconnected", "request",
	// "connection-opened", "connection-closed", "assembly-write", "tag-write", "reset" or
	// "stopping".
	Event    string      `json:"event"`
	Remote   string      `json:"remote,omitempty"`
	Address  string      `json:"address,omitempty"`
	Service  string      `json:"service,omitempty"`
	Status   string      `json:"status,omitempty"`
	Instance uint32      `json:"instance,omitempty"`
	Size     int         `json:"size,omitempty"`
	Data     string      `json:"data,omitempty"`
	Run      *bool       `json:"run,omitempty"`
	Tag      string      `json:"tag,omitempty"`
	Conn     *Connection `json:"connection,omitempty"`
	Detail   string      `json:"detail,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (e *AdapterEvent) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s  %-18s", e.Timestamp, e.Event)
	for _, p := range e.parts() {
		b.WriteString("  ")
		b.WriteString(p)
	}
	return strings.TrimRight(b.String(), " "), nil
}

func (e *AdapterEvent) parts() []string {
	var p []string
	add := func(k, v string) {
		if v != "" {
			p = append(p, k+"="+v)
		}
	}
	add("address", e.Address)
	add("remote", e.Remote)
	add("service", e.Service)
	add("status", e.Status)
	if e.Instance != 0 {
		add("instance", strconv.FormatUint(uint64(e.Instance), 10))
	}
	if e.Data != "" || e.Size != 0 {
		add("size", strconv.Itoa(e.Size))
		add("data", e.Data)
	}
	if e.Run != nil {
		add("run", strconv.FormatBool(*e.Run))
	}
	add("tag", e.Tag)
	if e.Conn != nil {
		add("connection", fmt.Sprintf("class%d ot=0x%08X to=0x%08X serial=%d path=%s", e.Conn.Class, e.Conn.OTConnID, e.Conn.TOConnID, e.Conn.ConnectionSerial, e.Conn.Path))
	}
	add("detail", e.Detail)
	return p
}

// TableHeaders implements format.TableMarshaler.
func (e *AdapterEvent) TableHeaders() []string { return []string{"TIMESTAMP", "EVENT", "DETAIL"} }

// TableRows implements format.TableMarshaler.
func (e *AdapterEvent) TableRows() [][]string {
	return [][]string{{e.Timestamp, e.Event, orDash(strings.Join(e.parts(), " "))}}
}
