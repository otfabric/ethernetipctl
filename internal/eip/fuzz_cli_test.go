// SPDX-License-Identifier: MIT

package eip

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/eds"
	"github.com/otfabric/go-ethernetip/logix"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
)

// Fuzz targets of the CLI-side parsers of what a user types: values per CIP type, tag
// values, the EDS generate specs, and the write and I/O checks. None may panic; what they
// accept must hold in its type, and round-trip where it can.

// FuzzEncode: a value Encode accepts decodes to the same octets when encoded again from
// its decoded text, and never has more octets than its type gives.
func FuzzEncode(f *testing.F) {
	for i, name := range ValueTypes {
		for _, v := range []string{"0", "1", "-1", "127", "128", "255", "256", "65535", "65536", "2147483648", "4294967296",
			"9223372036854775808", "18446744073709551615", "1.5", "3.5e38", "NaN", "-Inf", "0x7f", "true", "on", "1,2,3", "1,,2", ""} {
			f.Add(uint8(i), v)
		}
		f.Add(uint8(i), strings.Repeat("x", 300))
		_ = name
	}
	f.Fuzz(func(t *testing.T, ti uint8, value string) {
		dt, err := ParseType(ValueTypes[int(ti)%len(ValueTypes)])
		if err != nil {
			t.Fatal(err)
		}
		b, err := Encode(dt, value)
		if err != nil {
			if e, ok := errs.As(err); !ok || e.Kind != errs.KindInvalidInput {
				t.Fatalf("%s %q: error %v is not invalid input", dt, value, err)
			}
			return
		}
		if dt.IsString() {
			v, err := Decode(dt, b)
			if err != nil {
				t.Fatalf("%s %q: encoded % x does not decode: %v", dt, value, b, err)
			}
			// STRING2 is UTF-16: text that is not UTF-8 is replaced, not kept.
			if v != value && (dt != cip.TypeSTRING2 || utf8.ValidString(value)) {
				t.Fatalf("%s %q: round trip gives %q", dt, value, v)
			}
			return
		}
		if size := dt.Size(); len(b) == 0 || len(b)%size != 0 || len(b)/size != strings.Count(value, ",")+1 {
			t.Fatalf("%s %q: %d octets", dt, value, len(b))
		}
		v, err := Decode(dt, b)
		if err != nil {
			t.Fatalf("%s %q: encoded % x does not decode: %v", dt, value, b, err)
		}
		vals, ok := v.([]any)
		if !ok {
			vals = []any{v}
		}
		texts := make([]string, len(vals))
		for i, x := range vals {
			switch y := x.(type) {
			case bool:
				texts[i] = strconv.FormatBool(y)
			case float64:
				if dt == cip.TypeREAL && math.Abs(y) > math.MaxFloat32 {
					t.Fatalf("REAL %q encoded as %v", value, y)
				}
				texts[i] = strconv.FormatFloat(y, 'g', -1, 64)
			case string: // NaN, +Inf, -Inf
				texts[i] = y
			default:
				texts[i] = ValueText(y)
			}
		}
		again, err := Encode(dt, strings.Join(texts, ","))
		if err != nil {
			t.Fatalf("%s %q: decoded text %v does not encode: %v", dt, value, texts, err)
		}
		if !bytes.Equal(again, b) && !hasNaN(texts) {
			t.Fatalf("%s %q: % x, then % x", dt, value, b, again)
		}
	})
}

func hasNaN(texts []string) bool {
	for _, s := range texts {
		if s == "NaN" {
			return true
		}
	}
	return false
}

// FuzzTagValue: tag write's values encode exactly Count elements of their type; an
// integer that does not fit its type is refused, never wrapped.
func FuzzTagValue(f *testing.F) {
	for i := range TagTypes {
		for _, v := range []string{"0", "1", "-1", "128", "-129", "65536", "2147483648", "9223372036854775807", "1.5", "true", "1,2", "x", ""} {
			f.Add(uint8(i), v)
		}
	}
	f.Add(uint8(len(TagTypes)-1), strings.Repeat("x", 83))
	f.Fuzz(func(t *testing.T, ti uint8, value string) {
		typ := TagTypes[int(ti)%len(TagTypes)]
		v, vals, err := tagValue(typ, value)
		if err != nil {
			if e, ok := errs.As(err); !ok || e.Kind != errs.KindInvalidInput {
				t.Fatalf("%s %q: error %v is not invalid input", typ, value, err)
			}
			return
		}
		if v.Count != len(vals) {
			t.Fatalf("%s %q: count %d, %d values", typ, value, v.Count, len(vals))
		}
		if typ == "STRING" {
			if len(v.Data) != logix.StringSize {
				t.Fatalf("STRING %q: %d octets", value, len(v.Data))
			}
			return
		}
		if size := v.Type.Size(); len(v.Data) != size*v.Count {
			t.Fatalf("%s %q: %d octets for %d elements", typ, value, len(v.Data), v.Count)
		}
		if v.Type.IsInteger() && v.Type != cip.TypeBOOL {
			for i, x := range vals {
				got, err := v.Int(i)
				if err != nil {
					// ULINT above MaxInt64 cannot be read back as an int64.
					continue
				}
				// Unsigned types carry uint64 values: compare the numbers.
				if want := fmt.Sprint(x); fmt.Sprint(got) != want {
					t.Fatalf("%s %q: element %d is %d, want %s (wrapped)", typ, value, i, got, want)
				}
			}
		}
	})
}

// FuzzEDSSpecs: the specs of eds generate. An accepted assembly or connection has the
// sizes and instances the EDS can carry, and a positive RPI.
func FuzzEDSSpecs(f *testing.F) {
	for _, s := range []string{
		"100:32:Input", "150:32", "0:1", "1:65535", "1:65536", "0x10:0x20:name:with:colons", "4294967295:1", "4294967296:1",
		"151:150:100:32:32:10ms:Exclusive Owner", "0:152:100:0:32", "1:2:3:65535:65535:1ns", "1:2:3:4:5:-1ms", "1:2:3:4:5:0s",
		"1:2:3:4:5:2562047h", "::::", "1.1", "127.255", "0.1", "128.0", "2026-01-02", "2026-01-02T03:04:05Z", "",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if a, err := parseSpecAssembly(s); err == nil {
			if a.Instance == 0 || a.Size < 0 || a.Size > 0xFFFF || a.Name == "" {
				t.Fatalf("assembly %q: %+v", s, a)
			}
		}
		if c, err := parseSpecConnection(s); err == nil {
			if c.Output == 0 || c.Input == 0 || c.OutputSize < 0 || c.OutputSize > 0xFFFF || c.InputSize < 0 || c.InputSize > 0xFFFF || c.RPI < 0 || c.Name == "" {
				t.Fatalf("connection %q: %+v", s, c)
			}
		}
		if r, err := parseRevision(s); err == nil {
			if r.Major == 0 || r.Major > 127 {
				t.Fatalf("revision %q: %v", s, r)
			}
			if again, err := parseRevision(r.String()); err != nil || again != r {
				t.Fatalf("revision %q: %v, then %v %v", s, r, again, err)
			}
		}
		_, _ = parseDate(s)
		// The whole spec builds an EDS, or is refused: never a panic.
		cfg := config.EDSGenerateConfig{VendorID: 1, ProductName: "x", Revision: "1.1", Assemblies: []string{s}, Connections: []string{s}, Date: "2026-01-02"}
		if spec, err := CheckEDSGenerate(cfg); err == nil {
			if file, err := eds.Build(spec); err == nil {
				if _, err := eds.ParseString(file.Format()); err != nil {
					t.Fatalf("%q: the generated EDS does not parse: %v", s, err)
				}
			}
		}
	})
}

// FuzzChecks: the validation of write, assembly write, forward-open and io before
// anything is sent. Whatever passes holds in the types of the wire.
func FuzzChecks(f *testing.F) {
	f.Add("@1/1/7", "0a00", "USINT", "1", uint16(32), uint16(32), int64(10*time.Millisecond), uint8(0), "0102")
	f.Add("@0xF5/1/8", "", "SINT", "128", uint16(65535), uint16(65535), int64(-1), uint8(8), "010")
	f.Add("20 04 24 64", "zz", "STRING", "x", uint16(0), uint16(0), int64(math.MaxInt64), uint8(255), strings.Repeat("00", 600))
	f.Fuzz(func(t *testing.T, path, data, typ, value string, outSize, inSize uint16, rpi int64, mult uint8, configData string) {
		if _, b, _, err := CheckWrite(path, config.WriteConfig{Data: data, Type: typ, Value: value, Yes: true}); err == nil && len(b) > 0xFFFF {
			t.Fatalf("write of %d octets accepted", len(b))
		}
		_, _ = CheckAssemblyWrite(path, config.AssemblyConfig{Data: data, Yes: true})
		io := config.IOConnConfig{
			ConnectionPathConfig: config.ConnectionPathConfig{Config: 151, Output: 150, Input: 100},
			OutputSize:           outSize, InputSize: inSize, RPI: time.Duration(rpi), TimeoutMultiplier: mult, ConfigData: configData,
		}
		if err := CheckForwardOpen(config.ForwardOpenConfig{IOConnConfig: io}); err == nil {
			if io.RPI < time.Microsecond || io.RPI/time.Microsecond > math.MaxUint32 || mult > 7 {
				t.Fatalf("forward-open accepted RPI %v, multiplier %d", io.RPI, mult)
			}
		}
		_, _ = CheckIO(config.IOConfig{IOConnConfig: io, Yes: true})
	})
}
