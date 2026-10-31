// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"strconv"
	"strings"
)

// EDSDevice is the Device section of an EDS.
type EDSDevice struct {
	VendorID    uint16 `json:"vendor_id"`
	VendorName  string `json:"vendor_name"`
	DeviceType  uint16 `json:"device_type"`
	TypeName    string `json:"type_name"`
	ProductCode uint16 `json:"product_code"`
	Revision    string `json:"revision"`
	ProductName string `json:"product_name"`
	Catalog     string `json:"catalog"`
}

// EDSParam is a ParamN entry.
type EDSParam struct {
	Number  int    `json:"number"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Size    int    `json:"size"`
	Units   string `json:"units"`
	Min     string `json:"min"`
	Max     string `json:"max"`
	Default string `json:"default"`
	Path    string `json:"path,omitempty"`
	Enum    int    `json:"enum_values"`
}

// EDSAssembly is an AssemN entry.
type EDSAssembly struct {
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Instance uint32 `json:"instance"`
	Size     int    `json:"size"`
	Members  int    `json:"members"`
}

// EDSConnection is a ConnectionN entry.
type EDSConnection struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Class  int    `json:"class"`
	OTRPI  string `json:"ot_rpi"`
	TORPI  string `json:"to_rpi"`
	OTSize string `json:"ot_size"`
	TOSize string `json:"to_size"`
	Path   string `json:"path"`
}

// EDSInspectResult is the result of `eds inspect`.
type EDSInspectResult struct {
	File        string          `json:"file"`
	Device      EDSDevice       `json:"device"`
	Sections    []string        `json:"sections"`
	Params      []EDSParam      `json:"params"`
	Assemblies  []EDSAssembly   `json:"assemblies"`
	Connections []EDSConnection `json:"connections"`
	Errors      int             `json:"errors"`
	Warnings    int             `json:"warnings"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *EDSInspectResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	d := r.Device
	_, _ = fmt.Fprintf(&b, "%s: %q rev %s, vendor %d (%s), %s (type %d), product code %d\n", r.File, d.ProductName, d.Revision, d.VendorID, orDash(d.VendorName), orDash(d.TypeName), d.DeviceType, d.ProductCode)
	if d.Catalog != "" {
		kv(&b, "  ", "catalog", d.Catalog)
	}
	kv(&b, "  ", "sections", strings.Join(r.Sections, ", "))
	kv(&b, "  ", "validation", fmt.Sprintf("%d error(s), %d warning(s)", r.Errors, r.Warnings))
	_, _ = fmt.Fprintf(&b, "Parameters (%d):\n", len(r.Params))
	for _, p := range r.Params {
		_, _ = fmt.Fprintf(&b, "  Param%-3d %-24s %-6s %s..%s default %s %s\n", p.Number, strconv.Quote(p.Name), p.Type, orDash(p.Min), orDash(p.Max), orDash(p.Default), p.Units)
	}
	_, _ = fmt.Fprintf(&b, "Assemblies (%d):\n", len(r.Assemblies))
	for _, a := range r.Assemblies {
		_, _ = fmt.Fprintf(&b, "  Assem%-3d %-24s instance %-4d %d octets, %d members\n", a.Number, strconv.Quote(a.Name), a.Instance, a.Size, a.Members)
	}
	_, _ = fmt.Fprintf(&b, "Connections (%d):\n", len(r.Connections))
	for _, c := range r.Connections {
		_, _ = fmt.Fprintf(&b, "  Connection%-2d %-24s class %d  O->T %s octets, T->O %s octets  RPI %s  path %s\n", c.Number, strconv.Quote(c.Name), c.Class, orDash(c.OTSize), orDash(c.TOSize), orDash(c.OTRPI), c.Path)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *EDSInspectResult) TableHeaders() []string {
	return []string{"KIND", "NUMBER", "NAME", "DETAIL"}
}

// TableRows implements format.TableMarshaler.
func (r *EDSInspectResult) TableRows() [][]string {
	d := r.Device
	rows := [][]string{{"device", "-", d.ProductName, fmt.Sprintf("vendor %d product %d rev %s", d.VendorID, d.ProductCode, d.Revision)}}
	for _, p := range r.Params {
		rows = append(rows, []string{"param", strconv.Itoa(p.Number), p.Name, fmt.Sprintf("%s %s..%s", p.Type, p.Min, p.Max)})
	}
	for _, a := range r.Assemblies {
		rows = append(rows, []string{"assembly", strconv.Itoa(a.Number), a.Name, fmt.Sprintf("instance %d size %d", a.Instance, a.Size)})
	}
	for _, c := range r.Connections {
		rows = append(rows, []string{"connection", strconv.Itoa(c.Number), c.Name, fmt.Sprintf("class %d path %s", c.Class, c.Path)})
	}
	return rows
}

// EDSIssue is a finding of `eds validate`.
type EDSIssue struct {
	// Severity is "error" or "warning".
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Section  string `json:"section"`
	Message  string `json:"message"`
}

// EDSValidateResult is the result of `eds validate`.
type EDSValidateResult struct {
	File string `json:"file"`
	// Valid is true when the file has no error (warnings allowed).
	Valid    bool       `json:"valid"`
	Errors   int        `json:"errors"`
	Warnings int        `json:"warnings"`
	Issues   []EDSIssue `json:"issues"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *EDSValidateResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	verdict := "valid"
	if !r.Valid {
		verdict = "INVALID"
	}
	_, _ = fmt.Fprintf(&b, "%s: %s, %d error(s), %d warning(s)\n", r.File, verdict, r.Errors, r.Warnings)
	for _, i := range r.Issues {
		_, _ = fmt.Fprintf(&b, "  %-7s line %-4d [%s] %s\n", i.Severity, i.Line, i.Section, i.Message)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *EDSValidateResult) TableHeaders() []string {
	return []string{"SEVERITY", "LINE", "SECTION", "MESSAGE"}
}

// TableRows implements format.TableMarshaler.
func (r *EDSValidateResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Issues))
	for _, i := range r.Issues {
		rows = append(rows, []string{i.Severity, strconv.Itoa(i.Line), i.Section, i.Message})
	}
	return rows
}

// FileResult is the result of a command that wrote a file (`eds generate`, `eds codegen`
// with --output).
type FileResult struct {
	// Kind is "eds" or "go".
	Kind   string `json:"kind"`
	Output string `json:"output"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *FileResult) MarshalTextOutput() (string, error) {
	return fmt.Sprintf("wrote %s (%s, %d octets, sha256 %s)", r.Output, r.Kind, r.Bytes, r.SHA256), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *FileResult) TableHeaders() []string { return []string{"OUTPUT", "KIND", "BYTES", "SHA256"} }

// TableRows implements format.TableMarshaler.
func (r *FileResult) TableRows() [][]string {
	return [][]string{{r.Output, r.Kind, strconv.Itoa(r.Bytes), r.SHA256}}
}
