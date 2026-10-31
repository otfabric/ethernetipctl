// SPDX-License-Identifier: MIT

package eip

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/client"
	"github.com/otfabric/go-ethernetip/logix"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// TagTypes are the types `tag write` encodes.
var TagTypes = []string{"BOOL", "SINT", "INT", "DINT", "LINT", "USINT", "UINT", "UDINT", "ULINT", "REAL", "LREAL", "BYTE", "WORD", "DWORD", "LWORD", "STRING"}

// openLogix opens a session and a Logix client over it.
func (e Env) openLogix(ctx context.Context, host string, m config.MessagingConfig) (*Session, *logix.Client, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return nil, nil, err
	}
	route, err := ParseRoute(m.Route)
	if err != nil {
		return nil, nil, err
	}
	co := client.ConnectionOptions{}
	var opts []logix.ClientOption
	if m.Connected {
		// Logix tools use a large Class 3 connection: 4000 octets.
		co = client.ConnectionOptions{Large: true, Size: 4000}
		opts = append(opts, logix.WithMaxData(3960))
	}
	s, err := e.openTarget(ctx, t, route, m.Connected, co)
	if err != nil {
		return nil, nil, err
	}
	return s, logix.NewClient(s.Doer(), opts...), nil
}

// checkTag validates a tag name.
func checkTag(tag string) (logix.TagPath, error) {
	tp, err := logix.ParseTag(strings.TrimSpace(tag))
	if err != nil {
		return logix.TagPath{}, errs.InvalidInput(errs.CodeInvalidPath, fmt.Sprintf("invalid tag name %q: %v", tag, trimPrefix(err.Error())), err)
	}
	return tp, nil
}

// typeName renders the type of a value.
func typeName(v logix.Value) (string, *uint16) {
	switch {
	case v.IsStruct() && v.Handle == logix.StringHandle:
		return "STRING", nil
	case v.IsStruct():
		h := v.Handle
		return "struct", &h
	}
	return v.Type.String(), nil
}

// valuesOf returns the elements of an atomic or STRING value.
func valuesOf(v logix.Value) []any {
	out := make([]any, 0, v.Count)
	for i := 0; i < v.Count; i++ {
		var x any
		var err error
		switch {
		case v.IsStruct() && v.Handle == logix.StringHandle:
			x, err = v.Text(i)
		case v.IsStruct():
			return nil
		case v.Type == cip.TypeBOOL:
			x, err = v.Bool(i)
		case v.Type.IsFloat():
			var f float64
			f, err = v.Float(i)
			x = floatValue(f, 8*v.Type.Size())
		case v.Type == cip.TypeULINT || v.Type == cip.TypeLWORD:
			x, err = v.Uint(i)
		default:
			x, err = v.Int(i)
		}
		if err != nil {
			return nil
		}
		out = append(out, x)
	}
	return out
}

// CheckTagRead validates `tag read`.
func CheckTagRead(tag string, cfg config.TagReadConfig) error {
	if _, err := checkTag(tag); err != nil {
		return err
	}
	if cfg.Count < 1 || cfg.Count > 65535 {
		return invalid(errs.CodeInvalidValue, "invalid --count %d: 1 to 65535", cfg.Count)
	}
	return nil
}

// TagRead runs `tag read` (and `tag decode`, with cfg.Decode).
func (e Env) TagRead(ctx context.Context, host, tag string, cfg config.TagReadConfig) (*types.TagReadResult, error) {
	if err := CheckTagRead(tag, cfg); err != nil {
		return nil, err
	}
	s, lc, err := e.openLogix(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	start := time.Now()
	v, err := lc.Read(ctx, strings.TrimSpace(tag), cfg.Count)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.TagReadResult{Messaging: messaging(s, cfg.MessagingConfig), Tag: strings.TrimSpace(tag), Count: v.Count, Data: Hex(v.Data), RTTMs: ms(time.Since(start))}
	r.Type, r.Handle = typeName(v)
	r.Values = valuesOf(v)
	if cfg.Decode && v.IsStruct() && v.Handle != logix.StringHandle {
		id, err := templateOfHandle(ctx, lc, tag)
		if err != nil {
			return nil, err
		}
		fs, err := lc.DecodeStruct(ctx, id, v)
		if err != nil {
			return nil, WrapError(err)
		}
		r.Fields = fieldsOf(fs)
		if tpl, err := lc.Template(ctx, id); err == nil {
			r.Struct = tpl.Name
		}
	}
	return r, nil
}

// templateOfHandle finds the template ID of a structure tag (or of an element of a
// structure array) from the tag list: the read reply carries only the structure handle.
func templateOfHandle(ctx context.Context, lc *logix.Client, tag string) (uint16, error) {
	name := strings.TrimSpace(tag)
	program, rest := "", name
	if strings.HasPrefix(strings.ToLower(name), "program:") {
		i := strings.Index(name, ".")
		if i < 0 {
			return 0, invalid(errs.CodeInvalidPath, "invalid tag name %q", name)
		}
		program, rest = name[:i], name[i+1:]
	}
	base := rest
	if i := strings.IndexAny(rest, ".["); i >= 0 {
		base = rest[:i]
		if strings.Contains(rest[i:], ".") {
			return 0, invalid(errs.CodeInvalidPath, "--decode decodes structure tags and their array elements, not members: %q", name)
		}
	}
	if program != "" {
		base = program + "." + base
	}
	tags, err := lc.ListTags(ctx, program)
	if err != nil {
		return 0, WrapError(err)
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, base) && t.Type.IsStruct() {
			return t.Type.Template(), nil
		}
	}
	return 0, invalid(errs.CodeInvalidPath, "--decode: %q is not a structure tag of the tag list", base)
}

// fieldsOf converts decoded structure members.
func fieldsOf(fs []logix.Field) []types.TagField {
	out := make([]types.TagField, 0, len(fs))
	for _, f := range fs {
		tf := types.TagField{Name: f.Name}
		tf.Type, _ = typeName(f.Value)
		if tf.Type == "STRING" {
			if vs := valuesOf(f.Value); len(vs) == 1 {
				tf.Value = vs[0]
			}
		} else if len(f.Struct) > 0 {
			tf.Fields = fieldsOf(f.Struct)
		} else if vs := valuesOf(f.Value); len(vs) == 1 {
			tf.Value = vs[0]
		} else if len(vs) > 1 {
			tf.Value = vs
		} else {
			tf.Data = Hex(f.Value.Data)
		}
		out = append(out, tf)
	}
	return out
}

// tagValue encodes the values of `tag write`.
func tagValue(typ, value string) (logix.Value, []any, error) {
	name := strings.ToUpper(strings.TrimSpace(typ))
	if name == "STRING" {
		v, err := logix.Strings(value)
		if err != nil {
			return logix.Value{}, nil, invalid(errs.CodeInvalidValue, "invalid STRING value: %v", trimPrefix(err.Error()))
		}
		return v, []any{value}, nil
	}
	t, err := ParseType(name)
	if err != nil || t.IsString() {
		return logix.Value{}, nil, invalid(errs.CodeInvalidValue, "unknown tag type %q (expected one of %s)", typ, strings.Join(TagTypes, ", "))
	}
	parts := strings.Split(value, ",")
	var vals []any
	switch {
	case t.IsFloat():
		fs := make([]float64, 0, len(parts))
		for _, p := range parts {
			f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil {
				return logix.Value{}, nil, invalid(errs.CodeInvalidValue, "invalid %s value %q", t, p)
			}
			fs = append(fs, f)
			vals = append(vals, f)
		}
		v, err := logix.Floats(t, fs...)
		return v, vals, wrapValueErr(err)
	default:
		is := make([]int64, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			var n int64
			if t == cip.TypeBOOL {
				b, err := parseBool(p)
				if err != nil {
					return logix.Value{}, nil, invalid(errs.CodeInvalidValue, "invalid BOOL value %q: %v", p, err)
				}
				if b {
					n = 1
				}
				vals = append(vals, b)
			} else {
				var err error
				if n, err = strconv.ParseInt(p, 0, 64); err != nil {
					return logix.Value{}, nil, invalid(errs.CodeInvalidValue, "invalid %s value %q", t, p)
				}
				vals = append(vals, n)
			}
			is = append(is, n)
		}
		v, err := logix.Ints(t, is...)
		return v, vals, wrapValueErr(err)
	}
}

func wrapValueErr(err error) error {
	if err != nil {
		return invalid(errs.CodeInvalidValue, "invalid value: %v", trimPrefix(err.Error()))
	}
	return nil
}

// CheckTagWrite validates `tag write` before anything is sent.
func CheckTagWrite(tag string, cfg config.TagWriteConfig) (logix.TagPath, logix.Value, []any, error) {
	tp, err := checkTag(tag)
	if err != nil {
		return tp, logix.Value{}, nil, err
	}
	if cfg.Type == "" || cfg.Value == "" && !strings.EqualFold(strings.TrimSpace(cfg.Type), "STRING") {
		return tp, logix.Value{}, nil, invalid(errs.CodeInvalidInput, "--type and --value are required")
	}
	v, vals, err := tagValue(cfg.Type, cfg.Value)
	if err != nil {
		return tp, logix.Value{}, nil, err
	}
	if tp.Bit >= 0 {
		if v.Type == 0 || !v.Type.IsInteger() || v.Type == cip.TypeBOOL || len(vals) != 1 {
			return tp, logix.Value{}, nil, invalid(errs.CodeInvalidFlagCombination, "%q names a bit: give the --type of the integer tag (SINT, INT, DINT, LINT) and one --value of 0 or 1", tag)
		}
		x, _ := v.Int(0)
		if x != 0 && x != 1 {
			return tp, logix.Value{}, nil, invalid(errs.CodeInvalidValue, "a bit is written with 0 or 1, not %d", x)
		}
	}
	return tp, v, vals, confirm(cfg.Yes, "tag write")
}

// TagWrite runs `tag write`.
func (e Env) TagWrite(ctx context.Context, host, tag string, cfg config.TagWriteConfig) (*types.TagWriteResult, error) {
	tp, v, vals, err := CheckTagWrite(tag, cfg)
	if err != nil {
		return nil, err
	}
	s, lc, err := e.openLogix(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	name := strings.TrimSpace(tag)
	start := time.Now()
	r := &types.TagWriteResult{Messaging: messaging(s, cfg.MessagingConfig), Tag: name, Values: vals, Count: len(vals)}
	r.Type, _ = typeName(v)
	if tp.Bit >= 0 {
		x, _ := v.Int(0)
		bit := tp.Bit
		r.Bit = &bit
		err = lc.WriteBit(ctx, name, v.Type.Size(), x == 1)
	} else {
		err = lc.Write(ctx, name, v)
	}
	if err != nil {
		return nil, WrapError(err)
	}
	r.RTTMs = ms(time.Since(start))
	return r, nil
}

// symbolTypeName renders the type of a tag of the tag list.
func symbolTypeName(t logix.SymbolType) string {
	switch {
	case t.IsStruct() && t.Template() == logix.StringTemplate.ID:
		return "STRING"
	case t.IsStruct():
		return fmt.Sprintf("struct 0x%04X", t.Template())
	case t.Dims() > 0 && t.Atomic() == cip.TypeDWORD:
		// Logix lists a BOOL array as a DWORD array (its dimension in
		// bits); a user cannot declare DWORD tags.
		return "BOOL"
	}
	return t.Atomic().String()
}

// TagList runs `tag list`.
func (e Env) TagList(ctx context.Context, host string, cfg config.TagListConfig) (*types.TagListResult, error) {
	program := strings.TrimSpace(cfg.Program)
	if program != "" && !strings.HasPrefix(strings.ToLower(program), "program:") {
		program = "Program:" + program
	}
	s, lc, err := e.openLogix(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	tags, err := lc.ListTags(ctx, program)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.TagListResult{Messaging: messaging(s, cfg.MessagingConfig), Program: program, Tags: []types.TagEntry{}}
	if name, err := lc.ProgramName(ctx); err == nil {
		r.ProgramName = name
	} else if fatal(err) {
		return nil, WrapError(err)
	}
	// Structure types are named by their template, read once per template;
	// a template that cannot be read keeps its number.
	names := map[uint16]string{}
	for _, t := range tags {
		if t.Type.IsSystem() && !cfg.System {
			continue
		}
		te := types.TagEntry{Name: t.Name, Type: symbolTypeName(t.Type), Instance: t.Instance, System: t.Type.IsSystem(), Dims: []uint32{}}
		if id := t.Type.Template(); t.Type.IsStruct() && id != logix.StringTemplate.ID {
			name, ok := names[id]
			if !ok {
				if tpl, err := lc.Template(ctx, id); err == nil {
					name = tpl.Name
				} else if fatal(err) {
					return nil, WrapError(err)
				}
				names[id] = name
			}
			if name != "" {
				te.Type = name
			}
		}
		for i := 0; i < t.Type.Dims() && i < len(t.Dims); i++ {
			te.Dims = append(te.Dims, t.Dims[i])
		}
		if t.Type.IsStruct() {
			id := t.Type.Template()
			te.Template = &id
		}
		r.Tags = append(r.Tags, te)
	}
	r.Count = len(r.Tags)
	return r, nil
}

// memberTypeName renders the type of a template member.
func memberTypeName(m logix.Member) string {
	s := symbolTypeName(m.Type)
	if m.Type.Dims() > 0 {
		s += fmt.Sprintf("[%d]", m.Info)
	}
	return s
}

// TagTemplate runs `tag types`: a structure template and its members.
func (e Env) TagTemplate(ctx context.Context, host, id string, cfg config.MessagingConfig) (*types.TemplateResult, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(id), 0, 16)
	if err != nil || n == 0 || n > 0x0FFF {
		return nil, invalid(errs.CodeInvalidValue, "invalid template ID %q: 1 to 0x0FFF (the template of a structure tag in tag list)", id)
	}
	s, lc, err := e.openLogix(ctx, host, cfg)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	tpl, err := lc.Template(ctx, uint16(n))
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.TemplateResult{Messaging: messaging(s, cfg), ID: tpl.ID, Name: tpl.Name, Handle: tpl.Handle, Size: tpl.Size, Members: []types.TemplateMember{}}
	for _, m := range tpl.Members {
		r.Members = append(r.Members, types.TemplateMember{Name: m.Name, Type: memberTypeName(m), Offset: m.Offset, Info: m.Info, Hidden: m.Hidden()})
	}
	return r, nil
}
