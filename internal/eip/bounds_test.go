package eip

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
)

func codeOf(err error) string {
	var e *errs.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// The I/O sizes are bounded so that a connection size never wraps in
// Forward_Open's 16 bits.
func TestCheckIOSizes(t *testing.T) {
	base := config.IOConnConfig{ConnectionPathConfig: config.ConnectionPathConfig{Output: 100, Input: 101}, RPI: 10 * time.Millisecond}
	for _, tc := range []struct {
		name   string
		mod    func(*config.IOConnConfig)
		wantOK bool
	}{
		{"largest output", func(c *config.IOConnConfig) { c.OutputSize = uint16(maxOutputSize) }, true},
		{"output one more", func(c *config.IOConnConfig) { c.OutputSize = uint16(maxOutputSize + 1) }, false},
		{"output 65535", func(c *config.IOConnConfig) { c.OutputSize = math.MaxUint16 }, false},
		{"largest input", func(c *config.IOConnConfig) { c.InputSize = uint16(maxInputSize) }, true},
		{"input one more", func(c *config.IOConnConfig) { c.InputSize = uint16(maxInputSize + 1) }, false},
	} {
		c := base
		tc.mod(&c)
		_, err := checkIO(c)
		if tc.wantOK != (err == nil) {
			t.Errorf("%s: %v", tc.name, err)
		}
		if err != nil && codeOf(err) != errs.CodeInvalidValue {
			t.Errorf("%s: code %q", tc.name, codeOf(err))
		}
	}
	if maxOutputSize+6 != 0xFFFF || maxInputSize+2 != 0xFFFF {
		t.Errorf("max sizes %d %d", maxOutputSize, maxInputSize)
	}
	for n, want := range map[int]uint16{-1: 0, 0: 0, 0xFFFF: 0xFFFF, 0x10000: 0xFFFF} {
		if got := size16(n); got != want {
			t.Errorf("size16(%d) = %d", n, got)
		}
	}
}

func TestTwos(t *testing.T) {
	for v, want := range map[int64]uint64{0: 0, 1: 1, -1: math.MaxUint64, math.MinInt64: 1 << 63, math.MaxInt64: math.MaxInt64} {
		if got := twos(v); got != want {
			t.Errorf("twos(%d) = %#x, want %#x", v, got, want)
		}
	}
}

// The sizes of --connection are UINTs; instances UDINTs.
func TestSpecConnectionBounds(t *testing.T) {
	if c, err := parseSpecConnection("4294967295:1:2:65535:65535"); err != nil || c.Config != math.MaxUint32 || c.OutputSize != 65535 {
		t.Fatalf("largest: %+v %v", c, err)
	}
	for _, s := range []string{"0:1:2:65536:1", "0:1:2:1:65536", "4294967296:1:2:1:1", "0:0:2:1:1", "0:1:2:-1:1"} {
		if _, err := parseSpecConnection(s); codeOf(err) != errs.CodeInvalidValue {
			t.Errorf("%q: %v", s, err)
		}
	}
}

// Output files are created for the user only.
func TestWriteOutputMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.eds")
	if _, err := WriteOutput(path, "EDS", []byte("x"), false); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
}

// Names of assemblies and connections are EDS strings: one line.
func TestEDSGenerateControlCharacters(t *testing.T) {
	base := config.EDSGenerateConfig{VendorID: 1, ProductName: "p", Revision: "1.1"}
	for name, mod := range map[string]func(*config.EDSGenerateConfig){
		"assembly name":   func(c *config.EDSGenerateConfig) { c.Assemblies = []string{"100:4:two\tparts"} },
		"connection name": func(c *config.EDSGenerateConfig) { c.Connections = []string{"0:150:100:4:4:10ms:a\x00b"} },
		"vendor name":     func(c *config.EDSGenerateConfig) { c.VendorName = "a\rb" },
		"description":     func(c *config.EDSGenerateConfig) { c.Description = "a\nb" },
	} {
		c := base
		mod(&c)
		if _, err := CheckEDSGenerate(c); codeOf(err) != errs.CodeInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := CheckEDSGenerate(base); err != nil {
		t.Fatalf("plain text: %v", err)
	}
}

// A failed write is an output error, not a written file (/dev/full takes
// the open and refuses every write).
func TestWriteOutputWriteFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/dev/full is Linux")
	}
	_, err := WriteOutput("/dev/full", "EDS", []byte("x"), true)
	if codeOf(err) != errs.CodeOutputFileWriteFailed {
		t.Fatalf("write to /dev/full: %v", err)
	}
}

// A bit is refused before connecting when the type has no such bit.
func TestTagWriteBitWidth(t *testing.T) {
	for tag, want := range map[string]string{"Count.31": "", "Count.32": errs.CodeInvalidValue} {
		_, _, _, err := CheckTagWrite(tag, config.TagWriteConfig{Type: "DINT", Value: "1", Yes: true})
		if codeOf(err) != want {
			t.Errorf("%s: %v", tag, err)
		}
	}
}

// A program-scoped name needs the tag after the program.
func TestTemplateOfHandleProgramWithoutTag(t *testing.T) {
	if _, err := templateOfHandle(context.Background(), nil, "Program:Main"); codeOf(err) != errs.CodeInvalidPath {
		t.Fatalf("Program:Main: %v", err)
	}
}

// Tag values go through the library constructors: each type's whole range
// is accepted, and a value outside it is refused with the value named.
func TestTagValueRanges(t *testing.T) {
	for _, tc := range []struct {
		typ, value string
		want       any
	}{
		{"ULINT", "18446744073709551615", uint64(math.MaxUint64)},
		{"LWORD", "0x8000000000000000", uint64(1 << 63)},
		{"LINT", "-9223372036854775808", int64(math.MinInt64)},
		{"SINT", "-128", int64(-128)},
		{"USINT", "255", uint64(255)},
		{"REAL", "3.4e38", 3.4e38},
		{"REAL", "-inf", "-Inf"},
		{"LREAL", "1e300", 1e300},
	} {
		v, vals, err := tagValue(tc.typ, tc.value)
		if err != nil || len(vals) != 1 || vals[0] != tc.want || v.Count != 1 {
			t.Errorf("%s %s: %v %v", tc.typ, tc.value, vals, err)
		}
	}
	for _, tc := range []struct{ typ, value, msg string }{
		{"REAL", "3.5e38", "invalid value: 3.5e+38 does not fit REAL"},
		{"SINT", "300", "invalid value: 300 does not fit SINT"},
		{"USINT", "256", "invalid value: 256 does not fit USINT"},
		{"UDINT", "-1", `invalid UDINT value "-1"`},
		{"LINT", "x", `invalid LINT value "x"`},
		{"LREAL", "1e999", `invalid LREAL value "1e999"`},
		{"STRING", string(make([]byte, 90)), "invalid STRING value: STRING of 90 characters"},
	} {
		_, _, err := tagValue(tc.typ, tc.value)
		if codeOf(err) != errs.CodeInvalidValue || !strings.HasPrefix(err.Error(), tc.msg) {
			t.Errorf("%s %q: %v", tc.typ, tc.value, err)
		}
	}
}
