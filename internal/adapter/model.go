// Package adapter runs the simulated EtherNet/IP adapter of `ethernetipctl adapter`: a
// go-ethernetip server built from a model (YAML or JSON) or from flags.
package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/logix"
	"github.com/otfabric/go-ethernetip/server"
	"gopkg.in/yaml.v3"
)

// Model describes a simulated adapter.
type Model struct {
	Identity   Identity   `json:"identity" yaml:"identity"`
	Assemblies []Assembly `json:"assemblies" yaml:"assemblies"`
	// Echo copies the data written to an output assembly into an input assembly.
	Echo  []Echo `json:"echo,omitempty" yaml:"echo,omitempty"`
	Logix *Logix `json:"logix,omitempty" yaml:"logix,omitempty"`
}

// Identity is the identity the adapter reports.
type Identity struct {
	VendorID     uint16 `json:"vendor_id" yaml:"vendor_id"`
	DeviceType   uint16 `json:"device_type" yaml:"device_type"`
	ProductCode  uint16 `json:"product_code" yaml:"product_code"`
	Revision     string `json:"revision" yaml:"revision"`
	SerialNumber uint32 `json:"serial_number" yaml:"serial_number"`
	ProductName  string `json:"product_name" yaml:"product_name"`
}

// Assembly is an assembly instance.
type Assembly struct {
	Instance uint32 `json:"instance" yaml:"instance"`
	// Kind is input, output, config, input-only or listen-only.
	Kind string `json:"kind" yaml:"kind"`
	Size int    `json:"size" yaml:"size"`
	// Data is the initial data as hex; zeros when empty.
	Data string `json:"data,omitempty" yaml:"data,omitempty"`
}

// Echo copies output assembly Output into input assembly Input.
type Echo struct {
	Output uint32 `json:"output" yaml:"output"`
	Input  uint32 `json:"input" yaml:"input"`
}

// Logix is an emulated Logix tag database (Rockwell, vendor specific).
type Logix struct {
	ProgramName string     `json:"program_name,omitempty" yaml:"program_name,omitempty"`
	Templates   []Template `json:"templates,omitempty" yaml:"templates,omitempty"`
	Tags        []Tag      `json:"tags" yaml:"tags"`
}

// Template is a structure (UDT) definition.
type Template struct {
	ID      uint16   `json:"id" yaml:"id"`
	Name    string   `json:"name" yaml:"name"`
	Handle  uint16   `json:"handle" yaml:"handle"`
	Size    uint32   `json:"size" yaml:"size"`
	Members []Member `json:"members" yaml:"members"`
}

// Member is a member of a template: an atomic type, STRING, or the template ID of a
// structure in Template.
type Member struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Template uint16 `json:"template,omitempty" yaml:"template,omitempty"`
	// Array is the array length of an array member; Bit the bit of a BOOL member in the
	// octet at Offset.
	Array  uint16 `json:"array,omitempty" yaml:"array,omitempty"`
	Bit    uint16 `json:"bit,omitempty" yaml:"bit,omitempty"`
	Offset uint32 `json:"offset" yaml:"offset"`
}

// Tag is a tag of the database.
type Tag struct {
	Name string `json:"name" yaml:"name"`
	// Type is an atomic type (DINT, REAL, ...), STRING, or "struct" with Template.
	Type     string `json:"type" yaml:"type"`
	Template uint16 `json:"template,omitempty" yaml:"template,omitempty"`
	Dims     []int  `json:"dims,omitempty" yaml:"dims,omitempty"`
	// Value is the initial value: one value per element, comma-separated (one text for a
	// STRING).
	Value string `json:"value,omitempty" yaml:"value,omitempty"`
}

// DefaultIdentity is the identity of an adapter without one.
var DefaultIdentity = Identity{VendorID: 0xFFFF, DeviceType: uint16(cip.DeviceTypeCommunicationsAdapter), ProductCode: 1, Revision: "1.1", SerialNumber: 1, ProductName: "ethernetipctl adapter"}

// DefaultAssemblies are the assemblies of an adapter without any: input 100 and output 150
// of 32 octets, configuration 151 of 10, and the heartbeat points 152 (input-only) and 153
// (listen-only), after OpENer's sample application.
var DefaultAssemblies = []Assembly{
	{Instance: 100, Kind: "input", Size: 32},
	{Instance: 150, Kind: "output", Size: 32},
	{Instance: 151, Kind: "config", Size: 10},
	{Instance: 152, Kind: "input-only"},
	{Instance: 153, Kind: "listen-only"},
}

// DemoLogix is the tag database of --logix.
func DemoLogix() *Logix {
	return &Logix{
		ProgramName: "OTFabric",
		Templates: []Template{{
			ID: 0x123, Name: "Motor", Handle: 0xBEEF, Size: 112,
			Members: []Member{
				{Name: "Speed", Type: "REAL", Offset: 0},
				{Name: "Count", Type: "DINT", Array: 4, Offset: 4},
				{Name: "ZZZZZZZZZZMotor0", Type: "SINT", Offset: 20},
				{Name: "Running", Type: "BOOL", Bit: 0, Offset: 20},
				{Name: "Name", Type: "STRING", Offset: 24},
			},
		}},
		Tags: []Tag{
			{Name: "Count", Type: "DINT", Value: "42"},
			{Name: "Speed", Type: "REAL", Value: "1.5"},
			{Name: "Flags", Type: "DINT"},
			{Name: "Values", Type: "INT", Dims: []int{10}, Value: "0,1,2,3,4,5,6,7,8,9"},
			{Name: "Name", Type: "STRING", Value: "line 1"},
			{Name: "Motors", Type: "struct", Template: 0x123, Dims: []int{2}},
			{Name: "Program:Main.Local", Type: "SINT", Value: "7"},
		},
	}
}

// Load reads a model file: YAML, or JSON for a .json file.
func Load(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &Model{}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		err = dec.Decode(m)
	} else {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		err = dec.Decode(m)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// assemblyKinds maps the kinds of a model.
var assemblyKinds = map[string]server.AssemblyKind{
	"input":       server.AssemblyInput,
	"output":      server.AssemblyOutput,
	"config":      server.AssemblyConfig,
	"input-only":  server.AssemblyInputOnly,
	"listen-only": server.AssemblyListenOnly,
}

// ParseAssembly parses instance:kind[:size] of --assembly.
func ParseAssembly(s string) (Assembly, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return Assembly{}, fmt.Errorf("invalid --assembly %q: instance:kind[:size]", s)
	}
	in, err := strconv.ParseUint(parts[0], 0, 32)
	if err != nil {
		return Assembly{}, fmt.Errorf("invalid --assembly %q: instance %q", s, parts[0])
	}
	a := Assembly{Instance: uint32(in), Kind: strings.ToLower(parts[1])}
	if len(parts) == 3 {
		n, err := strconv.ParseUint(parts[2], 0, 16)
		if err != nil {
			return Assembly{}, fmt.Errorf("invalid --assembly %q: size %q", s, parts[2])
		}
		a.Size = int(n)
	}
	return a, nil
}

// identity converts the identity of the model.
func (id Identity) identity() (cip.Identity, error) {
	out := cip.Identity{VendorID: id.VendorID, DeviceType: cip.DeviceType(id.DeviceType), ProductCode: id.ProductCode,
		SerialNumber: id.SerialNumber, ProductName: id.ProductName, State: 3}
	maj, min, ok := strings.Cut(id.Revision, ".")
	a, err1 := strconv.ParseUint(maj, 10, 7)
	b, err2 := strconv.ParseUint(min, 10, 8)
	if !ok || err1 != nil || err2 != nil || a == 0 {
		return cip.Identity{}, fmt.Errorf("identity: invalid revision %q: major.minor", id.Revision)
	}
	out.Revision = cip.Revision{Major: uint8(a), Minor: uint8(b)}
	if len(id.ProductName) == 0 || len(id.ProductName) > 32 {
		return cip.Identity{}, fmt.Errorf("identity: the product name must have 1 to 32 characters")
	}
	return out, nil
}

// Check validates the model without building a server.
func (m *Model) Check() error {
	if _, err := m.Identity.identity(); err != nil {
		return err
	}
	seen := map[uint32]Assembly{}
	for _, a := range m.Assemblies {
		k, ok := assemblyKinds[a.Kind]
		switch {
		case a.Instance == 0:
			return fmt.Errorf("assembly: instance 0")
		case !ok:
			return fmt.Errorf("assembly %d: unknown kind %q (input, output, config, input-only or listen-only)", a.Instance, a.Kind)
		case seen[a.Instance].Instance != 0:
			return fmt.Errorf("assembly %d: defined twice", a.Instance)
		case (k == server.AssemblyInputOnly || k == server.AssemblyListenOnly) && a.Size != 0:
			return fmt.Errorf("assembly %d: a %s point has no data (size 0)", a.Instance, a.Kind)
		case a.Size < 0 || a.Size > 65000:
			return fmt.Errorf("assembly %d: size %d", a.Instance, a.Size)
		}
		if a.Data != "" {
			d, err := parseHex(a.Data)
			if err != nil || len(d) != a.Size {
				return fmt.Errorf("assembly %d: data must be %d octets of hex", a.Instance, a.Size)
			}
		}
		seen[a.Instance] = a
	}
	for _, e := range m.Echo {
		o, i := seen[e.Output], seen[e.Input]
		if o.Kind != "output" || i.Kind != "input" || o.Size != i.Size {
			return fmt.Errorf("echo %d -> %d: needs an output and an input assembly of the same size", e.Output, e.Input)
		}
	}
	return nil
}

// EchoPairs returns, for each output assembly, the input assembly of the same size with
// the lowest instance: the echo of --echo.
func EchoPairs(as []Assembly) []Echo {
	sorted := append([]Assembly(nil), as...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Instance < sorted[j].Instance })
	var out []Echo
	for _, o := range sorted {
		if o.Kind != "output" {
			continue
		}
		for _, i := range sorted {
			if i.Kind == "input" && i.Size == o.Size {
				out = append(out, Echo{Output: o.Instance, Input: i.Instance})
				break
			}
		}
	}
	return out
}

// buildTagServer builds the tag database of the model.
func (l *Logix) buildTagServer() (*logix.TagServer, error) {
	name := l.ProgramName
	if name == "" {
		name = "OTFabric"
	}
	ts := logix.NewTagServer(logix.WithProgramName(name))
	for _, t := range l.Templates {
		tpl := logix.Template{ID: t.ID, Name: t.Name, Handle: t.Handle, Size: t.Size}
		for _, m := range t.Members {
			st, err := symbolType(m.Type, m.Template, m.Array > 0, m.Bit)
			if err != nil {
				return nil, fmt.Errorf("template %s member %s: %w", t.Name, m.Name, err)
			}
			info := m.Array
			if strings.EqualFold(m.Type, "BOOL") {
				info = m.Bit
			}
			tpl.Members = append(tpl.Members, logix.Member{Name: m.Name, Type: st, Info: info, Offset: m.Offset})
		}
		if err := ts.AddTemplate(tpl); err != nil {
			return nil, fmt.Errorf("template %s: %w", t.Name, err)
		}
	}
	for _, t := range l.Tags {
		if err := addTag(ts, t); err != nil {
			return nil, fmt.Errorf("tag %s: %w", t.Name, err)
		}
	}
	return ts, nil
}

// symbolType returns the type of a template member.
func symbolType(typ string, template uint16, array bool, bit uint16) (logix.SymbolType, error) {
	dims := 0
	if array {
		dims = 1
	}
	switch strings.ToUpper(typ) {
	case "STRING":
		return logix.NewStructType(logix.StringTemplate.ID, dims), nil
	case "STRUCT":
		return logix.NewStructType(template, dims), nil
	}
	t, ok := cip.ParseDataType(strings.ToUpper(typ))
	if !ok || t.Size() == 0 {
		return 0, fmt.Errorf("unknown type %q", typ)
	}
	return logix.NewAtomicType(t, dims, int(bit)), nil
}

// addTag adds a tag and sets its initial value.
func addTag(ts *logix.TagServer, t Tag) error {
	upper := strings.ToUpper(t.Type)
	var err error
	switch upper {
	case "STRING":
		err = ts.AddStructTag(t.Name, logix.StringTemplate.ID, t.Dims...)
	case "STRUCT":
		err = ts.AddStructTag(t.Name, t.Template, t.Dims...)
	default:
		dt, ok := cip.ParseDataType(upper)
		if !ok {
			return fmt.Errorf("unknown type %q", t.Type)
		}
		err = ts.AddTag(t.Name, dt, t.Dims...)
	}
	if err != nil || t.Value == "" {
		return err
	}
	var v logix.Value
	switch upper {
	case "STRING":
		v, err = logix.Strings(strings.Split(t.Value, "|")...)
	case "STRUCT":
		return fmt.Errorf("a structure tag has no initial value in the model")
	case "REAL", "LREAL":
		var fs []float64
		for _, p := range strings.Split(t.Value, ",") {
			f, perr := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if perr != nil {
				return fmt.Errorf("value %q", p)
			}
			fs = append(fs, f)
		}
		dt, _ := cip.ParseDataType(upper)
		v, err = logix.Floats(dt, fs...)
	default:
		var is []int64
		for _, p := range strings.Split(t.Value, ",") {
			n, perr := strconv.ParseInt(strings.TrimSpace(p), 0, 64)
			if perr != nil {
				return fmt.Errorf("value %q", p)
			}
			is = append(is, n)
		}
		dt, _ := cip.ParseDataType(upper)
		v, err = logix.Ints(dt, is...)
	}
	if err != nil {
		return err
	}
	return ts.Set(t.Name, v)
}

func parseHex(s string) ([]byte, error) {
	clean := strings.NewReplacer(" ", "", ":", "").Replace(s)
	out := make([]byte, 0, len(clean)/2)
	if len(clean)%2 != 0 {
		return nil, fmt.Errorf("odd hex")
	}
	for i := 0; i < len(clean); i += 2 {
		n, err := strconv.ParseUint(clean[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out = append(out, byte(n))
	}
	return out, nil
}
