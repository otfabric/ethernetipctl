package eip

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/encap"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// stateNames are the meanings of the Identity object's state attribute (8).
var stateNames = map[uint8]string{
	0:    "nonexistent",
	1:    "self-testing",
	2:    "standby",
	3:    "operational",
	4:    "major-recoverable-fault",
	5:    "major-unrecoverable-fault",
	0xFF: "default",
}

// StateName returns the meaning of a state of the Identity object.
func StateName(s uint8) string {
	if n, ok := stateNames[s]; ok {
		return n
	}
	return "state-" + strconv.Itoa(int(s))
}

// IdentityOf converts an identity; withState includes attribute 8, which ListIdentity
// carries and Get_Attributes_All of attributes 1 to 7 does not.
func IdentityOf(id cip.Identity, withState bool) types.Identity {
	out := types.Identity{
		VendorID:             id.VendorID,
		VendorName:           cip.VendorName(id.VendorID),
		DeviceType:           uint16(id.DeviceType),
		DeviceTypeName:       id.DeviceType.String(),
		ProductCode:          id.ProductCode,
		Revision:             id.Revision.String(),
		Status:               uint16(id.Status),
		StatusFlags:          id.Status.String(),
		ExtendedDeviceStatus: id.Status.ExtendedDeviceStatus(),
		SerialNumber:         id.SerialNumber,
		ProductName:          id.ProductName,
	}
	if withState {
		st := id.State
		out.State = &st
		out.StateName = StateName(st)
	}
	return out
}

// listIdentityOf converts the identity item of a ListIdentity reply.
func listIdentityOf(it encap.IdentityItem) types.ListIdentity {
	return types.ListIdentity{ProtocolVersion: it.ProtocolVersion, Socket: it.Socket.AddrPort().String(), Identity: IdentityOf(it.Identity, true)}
}

// Identify runs `identify`: ListIdentity over the session, then Get_Attributes_All of the
// Identity object.
func (e Env) Identify(ctx context.Context, host string, m config.MessagingConfig) (*types.IdentifyResult, error) {
	start := time.Now()
	s, err := e.Open(ctx, host, m)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	li, err := s.Client.ListIdentity(ctx)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.IdentifyResult{Target: s.Target.String(), ListIdentity: listIdentityOf(li)}
	data, err := s.Messenger().GetAttributesAll(ctx, cip.ClassIdentity, 1)
	if err == nil {
		var id cip.Identity
		if id, err = cip.DecodeIdentity(data); err == nil {
			v := IdentityOf(id, false)
			r.Identity = &v
		}
	}
	r.IdentityError = ErrorInfo(err)
	r.DurationMs = ms(time.Since(start))
	return r, nil
}

// faultBits are the fault flags of the Identity status word.
var faultBits = []struct {
	bit  cip.IdentityStatus
	name string
}{
	{cip.StatusMinorRecoverableFault, "minor-recoverable-fault"},
	{cip.StatusMinorUnrecoverableFault, "minor-unrecoverable-fault"},
	{cip.StatusMajorRecoverableFault, "major-recoverable-fault"},
	{cip.StatusMajorUnrecoverableFault, "major-unrecoverable-fault"},
}

// Status runs `status`: one ListIdentity over the session, judged.
func (e Env) Status(ctx context.Context, host string) (*types.StatusResult, error) {
	s, err := e.Open(ctx, host, config.MessagingConfig{})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	start := time.Now()
	li, err := s.Client.ListIdentity(ctx)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.StatusResult{Target: s.Target.String(), Identity: IdentityOf(li.Identity, true), RTTMs: ms(time.Since(start)), Faults: []string{}}
	for _, f := range faultBits {
		if li.Status&f.bit != 0 {
			r.Faults = append(r.Faults, f.name)
		}
	}
	if li.State != 3 && li.State != 0xFF {
		r.Faults = append(r.Faults, "state "+StateName(li.State))
	}
	r.Healthy = len(r.Faults) == 0
	return r, nil
}

// Services runs `services`.
func (e Env) Services(ctx context.Context, host string) (*types.ServicesResult, error) {
	s, err := e.Open(ctx, host, config.MessagingConfig{})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	items, err := s.Client.ListServices(ctx)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.ServicesResult{Target: s.Target.String(), Services: []types.Service{}}
	for _, it := range items {
		r.Services = append(r.Services, types.Service{Name: it.Name, Version: it.Version, Capabilities: it.Capabilities,
			TCP: it.Capabilities&encap.CapabilityTCP != 0, UDP: it.Capabilities&encap.CapabilityUDP != 0})
	}
	r.Count = len(r.Services)
	return r, nil
}

// Interfaces runs `interfaces`.
func (e Env) Interfaces(ctx context.Context, host string) (*types.InterfacesResult, error) {
	s, err := e.Open(ctx, host, config.MessagingConfig{})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	items, err := s.Client.ListInterfaces(ctx)
	if err != nil {
		return nil, WrapError(err)
	}
	r := &types.InterfacesResult{Target: s.Target.String(), Interfaces: []types.Interface{}}
	for _, it := range items {
		r.Interfaces = append(r.Interfaces, types.Interface{Type: uint16(it.Type), TypeName: it.Type.String(), Data: Hex(it.Data)})
	}
	r.Count = len(r.Interfaces)
	return r, nil
}

// AttributePath is a parsed @class/instance[/attribute] argument.
type AttributePath struct {
	Class     cip.Class
	Instance  uint32
	Attribute uint16
	// HasAttribute is false for @class/instance.
	HasAttribute bool
}

// String renders the path as @class/instance[/attribute].
func (p AttributePath) String() string {
	s := fmt.Sprintf("@%d/%d", p.Class, p.Instance)
	if p.HasAttribute {
		s += fmt.Sprintf("/%d", p.Attribute)
	}
	return s
}

// ParseAttributePath parses @class/instance[/attribute] (numbers decimal or 0x hex), or
// the hex octets of a padded EPATH of logical segments.
func ParseAttributePath(s string) (AttributePath, error) {
	p, err := cip.ParsePath(strings.TrimSpace(s))
	if err != nil {
		return AttributePath{}, invalid(errs.CodeInvalidPath, "invalid path %q: %v (use @class/instance/attribute, e.g. @1/1/7)", s, err)
	}
	c, okc := p.Class()
	in, oki := p.Instance()
	if !okc || !oki || p.Symbolic() {
		return AttributePath{}, invalid(errs.CodeInvalidPath, "invalid path %q: it must name a class and an instance (@class/instance[/attribute])", s)
	}
	ap := AttributePath{Class: c, Instance: in}
	ap.Attribute, ap.HasAttribute = p.Attribute()
	return ap, nil
}

// Read runs `read`: Get_Attribute_Single, or Get_Attributes_All with --all.
func (e Env) Read(ctx context.Context, host, path string, cfg config.ReadConfig) (*types.AttributeResult, error) {
	ap, err := ParseAttributePath(path)
	if err != nil {
		return nil, err
	}
	if cfg.All == ap.HasAttribute {
		if cfg.All {
			return nil, invalid(errs.CodeInvalidFlagCombination, "--all reads a whole instance: give the path as @class/instance, without an attribute")
		}
		return nil, invalid(errs.CodeInvalidPath, "path %q names no attribute: give @class/instance/attribute, or --all to read every attribute of the instance", path)
	}
	var t cip.DataType
	if cfg.Type != "" {
		if t, err = ParseType(cfg.Type); err != nil {
			return nil, err
		}
	}
	s, err := e.Open(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	start := time.Now()
	var data []byte
	service := cip.ServiceGetAttributeSingle
	if cfg.All {
		service = cip.ServiceGetAttributesAll
		data, err = s.Messenger().GetAttributesAll(ctx, ap.Class, ap.Instance)
	} else {
		data, err = s.Messenger().GetAttributeSingle(ctx, ap.Class, ap.Instance, ap.Attribute)
	}
	if err != nil {
		return nil, WrapError(err)
	}
	r := attributeResult(s, cfg.MessagingConfig, service, ap, data, time.Since(start))
	if cfg.Type != "" {
		if err := r.decode(t, data); err != nil {
			return nil, err
		}
	}
	return &r.AttributeResult, nil
}

// attrResult is an AttributeResult under construction.
type attrResult struct{ types.AttributeResult }

func (r *attrResult) decode(t cip.DataType, data []byte) error {
	v, err := Decode(t, data)
	if err != nil {
		return err
	}
	r.Type, r.Value, r.ValueText = t.String(), v, ValueText(v)
	return nil
}

func attributeResult(s *Session, m config.MessagingConfig, service cip.Service, ap AttributePath, data []byte, rtt time.Duration) *attrResult {
	r := &attrResult{types.AttributeResult{
		Messaging: types.Messaging{Target: s.Target.String(), Route: strings.TrimSpace(m.Route), Connected: m.Connected},
		Service:   service.String(), Path: ap.String(), Class: uint16(ap.Class), ClassName: ap.Class.String(),
		Instance: ap.Instance, Length: len(data), Data: Hex(data), RTTMs: ms(rtt),
	}}
	if ap.HasAttribute {
		a := ap.Attribute
		r.Attribute = &a
	}
	return r
}

// confirm refuses a command that changes the target without --yes.
func confirm(yes bool, what string) error {
	if !yes {
		return errs.New(errs.KindUsage, errs.CodeConfirmationRequired, what+" changes the target: add --yes to confirm (nothing was sent)", nil)
	}
	return nil
}

// WriteData returns the octets of `write`: --data, or --value encoded as --type.
func WriteData(data, typ, value string) ([]byte, cip.DataType, error) {
	switch {
	case data != "" && (typ != "" || value != ""):
		return nil, 0, invalid(errs.CodeInvalidFlagCombination, "--data and --type/--value are mutually exclusive")
	case data != "":
		b, err := ParseHex(data)
		return b, 0, err
	case typ == "" || value == "" && !strings.Contains(strings.ToUpper(typ), "STRING"):
		return nil, 0, invalid(errs.CodeInvalidInput, "nothing to write: give --data HEX, or --type and --value")
	}
	t, err := ParseType(typ)
	if err != nil {
		return nil, 0, err
	}
	b, err := Encode(t, value)
	return b, t, err
}

// CheckWrite validates `write` before anything is sent.
func CheckWrite(path string, cfg config.WriteConfig) (AttributePath, []byte, cip.DataType, error) {
	ap, err := ParseAttributePath(path)
	if err != nil {
		return AttributePath{}, nil, 0, err
	}
	if !ap.HasAttribute {
		return AttributePath{}, nil, 0, invalid(errs.CodeInvalidPath, "path %q names no attribute: give @class/instance/attribute", path)
	}
	data, t, err := WriteData(cfg.Data, cfg.Type, cfg.Value)
	if err != nil {
		return AttributePath{}, nil, 0, err
	}
	return ap, data, t, confirm(cfg.Yes, "write")
}

// Write runs `write`: Set_Attribute_Single.
func (e Env) Write(ctx context.Context, host, path string, cfg config.WriteConfig) (*types.AttributeResult, error) {
	ap, data, t, err := CheckWrite(path, cfg)
	if err != nil {
		return nil, err
	}
	s, err := e.Open(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	start := time.Now()
	if err := s.Messenger().SetAttributeSingle(ctx, ap.Class, ap.Instance, ap.Attribute, data); err != nil {
		return nil, WrapError(err)
	}
	r := attributeResult(s, cfg.MessagingConfig, cip.ServiceSetAttributeSingle, ap, data, time.Since(start))
	if t != 0 {
		_ = r.decode(t, data)
	}
	return &r.AttributeResult, nil
}

// Objects runs `objects`: the class list of the Message Router (attribute 1 of @2/1).
func (e Env) Objects(ctx context.Context, host string, cfg config.ObjectConfig) (*types.ObjectsResult, error) {
	s, err := e.Open(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	data, err := s.Messenger().GetAttributeSingle(ctx, cip.ClassMessageRouter, 1, 1)
	if err != nil {
		return nil, WrapError(err)
	}
	// Attribute 1 is a UINT count followed by that many UINT class codes.
	r := cip.NewReader(data)
	n := int(r.Uint16())
	res := &types.ObjectsResult{Messaging: messaging(s, cfg.MessagingConfig), Objects: []types.ClassEntry{}}
	for i := 0; i < n && r.Err() == nil; i++ {
		c := cip.Class(r.Uint16())
		if r.Err() != nil {
			break
		}
		res.Objects = append(res.Objects, types.ClassEntry{Class: uint16(c), Hex: fmt.Sprintf("0x%02X", uint16(c)), Name: c.String(), VendorSpecific: c.VendorSpecific()})
	}
	if err := r.Err(); err != nil {
		return nil, WrapError(fmt.Errorf("%w: object list of %d octets: %v", cip.ErrMalformed, len(data), err))
	}
	res.Count = len(res.Objects)
	return res, nil
}

func messaging(s *Session, m config.MessagingConfig) types.Messaging {
	return types.Messaging{Target: s.Target.String(), Route: strings.TrimSpace(m.Route), Connected: m.Connected}
}

// ParseClass parses a class argument: a number (decimal or 0x hex) or the name of a class
// of the catalog, case and punctuation ignored ("identity", "tcpip-interface").
func ParseClass(s string) (cip.Class, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseUint(s, 0, 16); err == nil {
		return cip.Class(n), nil
	}
	want := normalizeName(s)
	if want != "" {
		for c := 1; c <= 0x3FF; c++ {
			name := cip.Class(c).String()
			if strings.HasPrefix(name, "class ") {
				continue
			}
			if n := normalizeName(name); n == want || strings.TrimSuffix(n, "rockwell") == want {
				return cip.Class(c), nil
			}
		}
	}
	return 0, invalid(errs.CodeInvalidPath, "unknown class %q: give a class number (1, 0xF5) or a class name (identity)", s)
}

func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// classAttributes are the common class attributes `object` reads.
var classAttributes = []struct {
	id   uint16
	name string
}{
	{1, "revision"},
	{2, "max_instance"},
	{3, "number_of_instances"},
	{6, "max_class_attribute"},
	{7, "max_instance_attribute"},
}

// Object runs `object`: the class attributes and Get_Attributes_All of an instance.
func (e Env) Object(ctx context.Context, host, class string, instance string, cfg config.ObjectConfig) (*types.ObjectResult, error) {
	c, err := ParseClass(class)
	if err != nil {
		return nil, err
	}
	in := uint64(1)
	if instance != "" {
		if in, err = strconv.ParseUint(strings.TrimSpace(instance), 0, 32); err != nil {
			return nil, invalid(errs.CodeInvalidPath, "invalid instance %q: a number of 0 to 4294967295", instance)
		}
	}
	s, err := e.Open(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	r := &types.ObjectResult{Messaging: messaging(s, cfg.MessagingConfig), Class: uint16(c), ClassName: c.String(), Instance: uint32(in), ClassAttributes: []types.AttributeValue{}}
	for _, a := range classAttributes {
		data, err := s.Messenger().GetAttributeSingle(ctx, c, 0, a.id)
		if fatal(err) {
			return nil, WrapError(err)
		}
		r.ClassAttributes = append(r.ClassAttributes, attributeValue(a.id, a.name, data, err))
	}
	data, err := s.Messenger().GetAttributesAll(ctx, c, uint32(in))
	if fatal(err) {
		return nil, WrapError(err)
	}
	r.InstanceData, r.InstanceError = Hex(data), ErrorInfo(err)
	return r, nil
}

// fatal reports whether err ends the session, rather than being the target's answer to
// one request.
func fatal(err error) bool {
	if err == nil {
		return false
	}
	e, _ := errs.As(WrapError(err))
	return e.Kind != errs.KindRefused
}

// attributeValue renders an attribute: a UINT or UDINT is decoded as a number.
func attributeValue(id uint16, name string, data []byte, err error) types.AttributeValue {
	v := types.AttributeValue{Attribute: id, Name: name, Data: Hex(data), Error: ErrorInfo(err)}
	if err == nil {
		v.Value = uintValue(data)
	}
	return v
}

// uintValue decodes 1, 2 or 4 octets as an unsigned number; nil for other sizes.
func uintValue(data []byte) any {
	switch len(data) {
	case 1:
		return uint64(data[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(data))
	case 4:
		return uint64(binary.LittleEndian.Uint32(data))
	}
	return nil
}

// CheckReset validates `reset` before anything is sent.
func CheckReset(cfg config.ResetConfig) error {
	if cfg.Type != 0 && cfg.Type != 1 {
		return invalid(errs.CodeInvalidValue, "invalid --type %d: 0 (power cycle) or 1 (factory defaults)", cfg.Type)
	}
	return confirm(cfg.Yes, "reset")
}

// Reset runs `reset`: the Reset service of the Identity object.
func (e Env) Reset(ctx context.Context, host string, cfg config.ResetConfig) (*types.ResetResult, error) {
	if err := CheckReset(cfg); err != nil {
		return nil, err
	}
	s, err := e.Open(ctx, host, config.MessagingConfig{RouteConfig: cfg.RouteConfig})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	start := time.Now()
	if err := s.Client.Reset(ctx, cip.ClassIdentity, 1, cfg.Type); err != nil {
		return nil, WrapError(err)
	}
	return &types.ResetResult{Target: s.Target.String(), Route: strings.TrimSpace(cfg.Route), Type: cfg.Type, RTTMs: ms(time.Since(start))}, nil
}

// parseInstance parses an assembly instance argument.
func parseInstance(s string) (uint32, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 0, 32)
	if err != nil || n == 0 {
		return 0, invalid(errs.CodeInvalidPath, "invalid instance %q: a number of 1 to 4294967295", s)
	}
	return uint32(n), nil
}

// AssemblyRead runs `assembly read`: attribute 3 (data) of an Assembly instance.
func (e Env) AssemblyRead(ctx context.Context, host, instance string, cfg config.AssemblyConfig) (*types.AttributeResult, error) {
	in, err := parseInstance(instance)
	if err != nil {
		return nil, err
	}
	return e.Read(ctx, host, fmt.Sprintf("@4/%d/3", in), config.ReadConfig{MessagingConfig: cfg.MessagingConfig})
}

// CheckAssemblyWrite validates `assembly write` before anything is sent.
func CheckAssemblyWrite(instance string, cfg config.AssemblyConfig) (uint32, error) {
	in, err := parseInstance(instance)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(cfg.Data) == "" {
		return 0, invalid(errs.CodeInvalidInput, "nothing to write: give --data HEX")
	}
	if _, err := ParseHex(cfg.Data); err != nil {
		return 0, err
	}
	return in, confirm(cfg.Yes, "assembly write")
}

// AssemblyWrite runs `assembly write`.
func (e Env) AssemblyWrite(ctx context.Context, host, instance string, cfg config.AssemblyConfig) (*types.AttributeResult, error) {
	in, err := CheckAssemblyWrite(instance, cfg)
	if err != nil {
		return nil, err
	}
	return e.Write(ctx, host, fmt.Sprintf("@4/%d/3", in), config.WriteConfig{MessagingConfig: cfg.MessagingConfig, Data: cfg.Data, Yes: cfg.Yes})
}

// cmCounters are the Connection Manager's counters, attributes 1 to 8.
var cmCounters = []string{
	"open_requests", "open_format_rejects", "open_resource_rejects", "open_other_rejects",
	"close_requests", "close_format_rejects", "close_other_rejects", "connection_timeouts",
}

// Connections runs `connections`: the Message Router's connection attributes and the
// Connection Manager's counters.
func (e Env) Connections(ctx context.Context, host string, cfg config.ObjectConfig) (*types.ConnectionsResult, error) {
	s, err := e.Open(ctx, host, cfg.MessagingConfig)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	r := &types.ConnectionsResult{Target: s.Target.String(), Counters: []types.Counter{}}
	for _, a := range []struct {
		id  uint16
		dst **uint64
	}{{2, &r.MaxConnections}, {3, &r.ActiveConnections}} {
		data, err := s.Messenger().GetAttributeSingle(ctx, cip.ClassMessageRouter, 1, a.id)
		if fatal(err) {
			return nil, WrapError(err)
		}
		if v, ok := uintValue(data).(uint64); ok && err == nil {
			*a.dst = &v
		}
	}
	counters, err := e.counters(ctx, s.Messenger())
	if err != nil {
		return nil, err
	}
	r.Counters = counters
	return r, nil
}

// counters reads the Connection Manager's counters.
func (e Env) counters(ctx context.Context, m Messenger) ([]types.Counter, error) {
	out := make([]types.Counter, 0, len(cmCounters))
	for i, name := range cmCounters {
		data, err := m.GetAttributeSingle(ctx, cip.ClassConnectionManager, 1, uint16(i+1))
		if fatal(err) {
			return nil, WrapError(err)
		}
		c := types.Counter{Attribute: uint16(i + 1), Name: name, Error: ErrorInfo(err)}
		if v, ok := uintValue(data).(uint64); ok && err == nil {
			c.Value = &v
		} else if err == nil {
			c.Error = ErrorInfo(fmt.Errorf("%w: %d octets", cip.ErrMalformed, len(data)))
		}
		out = append(out, c)
	}
	return out, nil
}
