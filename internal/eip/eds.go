package eip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/eds"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// ReadEDS reads and parses an EDS file.
func ReadEDS(path string) (*eds.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("cannot open %s: %v", path, err), err)
	}
	defer func() { _ = f.Close() }()
	file, err := eds.Parse(f)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidEDS, fmt.Sprintf("%s: %v", path, err), err)
	}
	return file, nil
}

// issuesOf converts validation issues and counts them.
func issuesOf(is []eds.Issue) ([]types.EDSIssue, int, int) {
	out := make([]types.EDSIssue, 0, len(is))
	var nerr, nwarn int
	for _, i := range is {
		if i.Severity == eds.Error {
			nerr++
		} else {
			nwarn++
		}
		out = append(out, types.EDSIssue{Severity: i.Severity.String(), Line: i.Line, Section: i.Section, Message: i.Message})
	}
	return out, nerr, nwarn
}

// EDSValidate runs `eds validate`. It returns the exit code: 0 for a file without
// errors, 3 (invalid input) for a file with errors; the report is written either way.
func EDSValidate(path string) (*types.EDSValidateResult, int, error) {
	f, err := ReadEDS(path)
	if err != nil {
		return nil, 0, err
	}
	issues, nerr, nwarn := issuesOf(eds.Validate(f))
	r := &types.EDSValidateResult{File: path, Valid: nerr == 0, Errors: nerr, Warnings: nwarn, Issues: issues}
	if nerr > 0 {
		return r, errs.ExitInvalidInput, nil
	}
	return r, errs.ExitOK, nil
}

// EDSInspect runs `eds inspect`.
func EDSInspect(path string) (*types.EDSInspectResult, error) {
	f, err := ReadEDS(path)
	if err != nil {
		return nil, err
	}
	_, nerr, nwarn := issuesOf(eds.Validate(f))
	r := &types.EDSInspectResult{File: path, Errors: nerr, Warnings: nwarn, Sections: []string{},
		Params: []types.EDSParam{}, Assemblies: []types.EDSAssembly{}, Connections: []types.EDSConnection{}}
	for _, s := range f.Sections {
		r.Sections = append(r.Sections, s.Name)
	}
	d, err := f.Device()
	if err != nil {
		return nil, edsErr(path, err)
	}
	r.Device = types.EDSDevice{VendorID: d.VendorID, VendorName: d.VendorName, DeviceType: uint16(d.DeviceType), TypeName: d.TypeName,
		ProductCode: d.ProductCode, Revision: d.Revision.String(), ProductName: d.ProductName, Catalog: d.Catalog}
	params, err := f.Params()
	if err != nil {
		return nil, edsErr(path, err)
	}
	for _, p := range params {
		ep := types.EDSParam{Number: p.Number, Name: p.Name, Type: p.Type.String(), Size: p.Size, Units: p.Units, Min: p.Min, Max: p.Max, Default: p.Default, Enum: len(p.Enum)}
		if len(p.Path) > 0 {
			ep.Path = p.Path.String()
		}
		r.Params = append(r.Params, ep)
	}
	asms, err := f.Assemblies()
	if err != nil {
		return nil, edsErr(path, err)
	}
	for _, a := range asms {
		in, _ := a.Instance()
		r.Assemblies = append(r.Assemblies, types.EDSAssembly{Number: a.Number, Name: a.Name, Instance: in, Size: a.Size, Members: len(a.Members)})
	}
	conns, err := f.Connections()
	if err != nil {
		return nil, edsErr(path, err)
	}
	for _, c := range conns {
		p := c.PathText
		if len(c.Path) > 0 {
			p = c.Path.String()
		}
		r.Connections = append(r.Connections, types.EDSConnection{Number: c.Number, Name: c.Name, Class: c.Class(),
			OTRPI: c.OTRPI, TORPI: c.TORPI, OTSize: c.OTSize, TOSize: c.TOSize, Path: p})
	}
	return r, nil
}

func edsErr(path string, err error) error {
	return errs.InvalidInput(errs.CodeInvalidEDS, fmt.Sprintf("%s: %v (run eds validate)", path, err), err)
}

// EDSCodegen runs `eds codegen`: Go source for the device an EDS describes.
func EDSCodegen(path string, cfg config.EDSCodegenConfig) ([]byte, error) {
	if cfg.Package == "" || !validIdent(cfg.Package) {
		return nil, invalid(errs.CodeInvalidValue, "invalid --package %q: a Go package name", cfg.Package)
	}
	f, err := ReadEDS(path)
	if err != nil {
		return nil, err
	}
	src, err := eds.GenerateGo(f, eds.GenerateOptions{Package: cfg.Package, Source: filepath.Base(path)})
	if err != nil {
		return nil, edsErr(path, err)
	}
	return src, nil
}

func validIdent(s string) bool {
	for i, r := range s {
		letter := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// parseRevision parses major.minor.
func parseRevision(s string) (cip.Revision, error) {
	maj, min, ok := strings.Cut(strings.TrimSpace(s), ".")
	a, err1 := strconv.ParseUint(maj, 10, 7)
	b, err2 := strconv.ParseUint(min, 10, 8)
	if !ok || err1 != nil || err2 != nil || a == 0 {
		return cip.Revision{}, invalid(errs.CodeInvalidValue, "invalid revision %q: major.minor, e.g. 1.1", s)
	}
	return cip.Revision{Major: uint8(a), Minor: uint8(b)}, nil
}

// parseSpecAssembly parses instance:size[:name].
func parseSpecAssembly(s string) (eds.SpecAssembly, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return eds.SpecAssembly{}, invalid(errs.CodeInvalidValue, "invalid --assembly %q: instance:size[:name]", s)
	}
	in, err1 := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 32)
	size, err2 := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 16)
	if err1 != nil || err2 != nil || in == 0 {
		return eds.SpecAssembly{}, invalid(errs.CodeInvalidValue, "invalid --assembly %q: instance:size[:name]", s)
	}
	a := eds.SpecAssembly{Instance: uint32(in), Size: int(size), Name: fmt.Sprintf("Assembly %d", in)}
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		a.Name = strings.TrimSpace(parts[2])
	}
	return a, nil
}

// parseSpecConnection parses config:output:input:output-size:input-size[:rpi[:name]].
func parseSpecConnection(s string) (eds.SpecConnection, error) {
	bad := invalid(errs.CodeInvalidValue, "invalid --connection %q: config:output:input:output-size:input-size[:rpi[:name]]", s)
	parts := strings.SplitN(s, ":", 7)
	if len(parts) < 5 {
		return eds.SpecConnection{}, bad
	}
	var n [5]uint64
	for i := 0; i < 5; i++ {
		v, err := strconv.ParseUint(strings.TrimSpace(parts[i]), 0, 32)
		if err != nil {
			return eds.SpecConnection{}, bad
		}
		n[i] = v
	}
	if n[1] == 0 || n[2] == 0 || n[3] > 65535 || n[4] > 65535 {
		return eds.SpecConnection{}, bad
	}
	c := eds.SpecConnection{Config: uint32(n[0]), Output: uint32(n[1]), Input: uint32(n[2]), OutputSize: int(n[3]), InputSize: int(n[4]), Name: "Exclusive Owner"}
	if len(parts) > 5 && strings.TrimSpace(parts[5]) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(parts[5]))
		if err != nil || d <= 0 {
			return eds.SpecConnection{}, bad
		}
		c.RPI = d
	}
	if len(parts) > 6 && strings.TrimSpace(parts[6]) != "" {
		c.Name = strings.TrimSpace(parts[6])
	}
	return c, nil
}

// parseDate parses --date.
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, nil
		}
	}
	return time.Time{}, invalid(errs.CodeInvalidValue, "invalid --date %q: YYYY-MM-DD or RFC 3339", s)
}

// CheckEDSGenerate validates `eds generate` and returns the spec without the identity of
// --from-device.
func CheckEDSGenerate(cfg config.EDSGenerateConfig) (eds.Spec, error) {
	var spec eds.Spec
	var err error
	if spec.Date, err = parseDate(cfg.Date); err != nil {
		return spec, err
	}
	if cfg.FromDevice == "" {
		if cfg.ProductName == "" || cfg.VendorID == 0 {
			return spec, invalid(errs.CodeInvalidInput, "give --from-device, or the identity: --vendor-id and --product-name at least")
		}
		rev, err := parseRevision(cfg.Revision)
		if err != nil {
			return spec, err
		}
		spec.Identity = cip.Identity{VendorID: cfg.VendorID, DeviceType: cip.DeviceType(cfg.DeviceType), ProductCode: cfg.ProductCode, Revision: rev, ProductName: cfg.ProductName}
	}
	for _, a := range cfg.Assemblies {
		sa, err := parseSpecAssembly(a)
		if err != nil {
			return spec, err
		}
		spec.Assemblies = append(spec.Assemblies, sa)
	}
	for _, c := range cfg.Connections {
		sc, err := parseSpecConnection(c)
		if err != nil {
			return spec, err
		}
		spec.Connections = append(spec.Connections, sc)
	}
	spec.VendorName, spec.Description = cfg.VendorName, cfg.Description
	return spec, nil
}

// EDSGenerate runs `eds generate`: an EDS for a device, from its identity (read from the
// device or given by flags) and the assemblies and connections of the flags.
func (e Env) EDSGenerate(ctx context.Context, cfg config.EDSGenerateConfig) ([]byte, error) {
	spec, err := CheckEDSGenerate(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.FromDevice != "" {
		s, err := e.Open(ctx, cfg.FromDevice, config.MessagingConfig{})
		if err != nil {
			return nil, err
		}
		id, err := s.Client.Identity(ctx)
		s.Close()
		if err != nil {
			return nil, WrapError(err)
		}
		spec.Identity = id
	}
	if spec.VendorName == "" {
		spec.VendorName = cip.VendorName(spec.Identity.VendorID)
	}
	f, err := eds.Build(spec)
	if err != nil {
		return nil, invalid(errs.CodeInvalidInput, "cannot build the EDS: %v", err)
	}
	return []byte(f.Format()), nil
}

// WriteOutput writes data to path ("" or "-" is stdout, which the caller handles) and
// returns the result; an existing file is not replaced without force.
func WriteOutput(path, kind string, data []byte, force bool) (*types.FileResult, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, errs.InvalidInput(errs.CodeOutputFileCreateFailed, fmt.Sprintf("%s exists: pass --force to overwrite it", path), err)
		}
		return nil, errs.Output(errs.CodeOutputFileCreateFailed, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, errs.Output(errs.CodeOutputFileWriteFailed, err)
	}
	if err := f.Close(); err != nil {
		return nil, errs.Output(errs.CodeOutputFileWriteFailed, err)
	}
	sum := sha256.Sum256(data)
	return &types.FileResult{Kind: kind, Output: path, Bytes: len(data), SHA256: hex.EncodeToString(sum[:])}, nil
}
