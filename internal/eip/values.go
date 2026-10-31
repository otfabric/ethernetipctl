// SPDX-License-Identifier: MIT

package eip

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/otfabric/go-ethernetip/cip"

	"github.com/otfabric/ethernetipctl/internal/errs"
)

// ValueTypes are the CIP data types read and write decode and encode, in the order help
// and completion list them.
var ValueTypes = []string{
	"BOOL", "SINT", "INT", "DINT", "LINT", "USINT", "UINT", "UDINT", "ULINT",
	"REAL", "LREAL", "BYTE", "WORD", "DWORD", "LWORD", "SHORT_STRING", "STRING", "STRING2",
}

// ParseType returns the CIP data type named name, case-insensitively, when ethernetipctl
// can decode and encode it.
func ParseType(name string) (cip.DataType, error) {
	n := strings.ToUpper(strings.TrimSpace(name))
	for _, v := range ValueTypes {
		if v == n {
			t, ok := cip.ParseDataType(n)
			if ok {
				return t, nil
			}
		}
	}
	return 0, invalid(errs.CodeInvalidValue, "unknown data type %q (expected one of %s)", name, strings.Join(ValueTypes, ", "))
}

// ParseHex parses hex octets: "0a00", "0a 00", "0x0a00", "0a:00".
func ParseHex(s string) ([]byte, error) {
	clean := strings.NewReplacer(" ", "", ":", "", "-", "", "\t", "").Replace(strings.TrimSpace(s))
	clean = strings.TrimPrefix(strings.TrimPrefix(clean, "0x"), "0X")
	b, err := hex.DecodeString(clean)
	if err != nil {
		return nil, invalid(errs.CodeInvalidValue, "invalid hex data %q: %v", s, err)
	}
	return b, nil
}

// Hex renders data as lower-case hex without separators.
func Hex(b []byte) string { return hex.EncodeToString(b) }

// Decode decodes data as values of type t: one value when data holds exactly one, an array
// when it holds several of a fixed-size type. A string type decodes one string.
func Decode(t cip.DataType, data []byte) (any, error) {
	r := cip.NewReader(data)
	if t.IsString() {
		var s string
		switch t {
		case cip.TypeShortString:
			s = r.ShortString()
		case cip.TypeSTRING2:
			s = r.String2()
		default:
			s = r.String()
		}
		if err := r.Err(); err != nil {
			return nil, invalid(errs.CodeInvalidValue, "the data is not a %s: %v", t, err)
		}
		return s, nil
	}
	size := t.Size()
	if size == 0 {
		return nil, invalid(errs.CodeInvalidValue, "%s has no fixed size: cannot decode it", t)
	}
	if len(data) == 0 || len(data)%size != 0 {
		return nil, invalid(errs.CodeInvalidValue, "%d octets of data are not a whole number of %s values (%d octets each)", len(data), t, size)
	}
	vals := make([]any, 0, len(data)/size)
	for r.Len() > 0 {
		vals = append(vals, decodeOne(t, r))
	}
	if len(vals) == 1 {
		return vals[0], nil
	}
	return vals, nil
}

// decodeOne reads one value of the fixed-size type t.
func decodeOne(t cip.DataType, r *cip.Reader) any {
	switch t {
	case cip.TypeBOOL:
		return r.Uint8() != 0
	case cip.TypeSINT:
		return int64(r.Int8())
	case cip.TypeINT:
		return int64(r.Int16())
	case cip.TypeDINT:
		return int64(r.Int32())
	case cip.TypeLINT:
		return r.Int64()
	case cip.TypeUSINT, cip.TypeBYTE:
		return uint64(r.Uint8())
	case cip.TypeUINT, cip.TypeWORD:
		return uint64(r.Uint16())
	case cip.TypeUDINT, cip.TypeDWORD:
		return uint64(r.Uint32())
	case cip.TypeREAL:
		return floatValue(float64(r.Float32()), 32)
	case cip.TypeLREAL:
		return floatValue(r.Float64(), 64)
	default: // ULINT, LWORD
		return r.Uint64()
	}
}

// floatValue returns f as the shortest decimal its type holds (a REAL 1.1 is 1.1, not
// 1.100000023841858), and NaN and the infinities, which JSON cannot carry, as text.
func floatValue(f float64, bits int) any {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	v, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'g', -1, bits), 64)
	return v
}

// Encode encodes the comma-separated values of value as type t. A string type encodes the
// whole text as one string.
func Encode(t cip.DataType, value string) ([]byte, error) {
	switch t {
	case cip.TypeShortString:
		return wrapEncode(cip.AppendShortString(nil, value))
	case cip.TypeSTRING:
		return wrapEncode(cip.AppendString(nil, value))
	case cip.TypeSTRING2:
		return wrapEncode(cip.AppendString2(nil, value))
	default:
	}
	parts := strings.Split(value, ",")
	var b []byte
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, invalid(errs.CodeInvalidValue, "empty value in %q", value)
		}
		var err error
		if b, err = encodeOne(b, t, p); err != nil {
			return nil, err
		}
	}
	return b, nil
}

func wrapEncode(b []byte, err error) ([]byte, error) {
	if err != nil {
		return nil, invalid(errs.CodeInvalidValue, "invalid value: %v", err)
	}
	return b, nil
}

// signedBits and unsignedBits are the widths of the integer types.
var (
	signedBits   = map[cip.DataType]int{cip.TypeSINT: 8, cip.TypeINT: 16, cip.TypeDINT: 32, cip.TypeLINT: 64}
	unsignedBits = map[cip.DataType]int{
		cip.TypeUSINT: 8, cip.TypeBYTE: 8, cip.TypeUINT: 16, cip.TypeWORD: 16,
		cip.TypeUDINT: 32, cip.TypeDWORD: 32, cip.TypeULINT: 64, cip.TypeLWORD: 64,
	}
)

// encodeOne appends one value of the fixed-size type t.
func encodeOne(b []byte, t cip.DataType, s string) ([]byte, error) {
	bad := func(err error) error {
		return invalid(errs.CodeInvalidValue, "invalid %s value %q: %v", t, s, err)
	}
	switch {
	case t == cip.TypeBOOL:
		v, err := parseBool(s)
		if err != nil {
			return nil, bad(err)
		}
		if v {
			return append(b, 1), nil
		}
		return append(b, 0), nil
	case t == cip.TypeREAL:
		f, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return nil, bad(err)
		}
		return cip.AppendFloat32(b, float32(f)), nil
	case t == cip.TypeLREAL:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, bad(err)
		}
		return cip.AppendFloat64(b, f), nil
	case signedBits[t] > 0:
		v, err := strconv.ParseInt(s, 0, signedBits[t])
		if err != nil {
			return nil, bad(err)
		}
		return appendLE(b, uint64(v), signedBits[t]), nil
	case unsignedBits[t] > 0:
		v, err := strconv.ParseUint(s, 0, unsignedBits[t])
		if err != nil {
			return nil, bad(err)
		}
		return appendLE(b, v, unsignedBits[t]), nil
	}
	return nil, invalid(errs.CodeInvalidValue, "cannot encode type %s", t)
}

// appendLE appends the low bits of v, little-endian.
func appendLE(b []byte, v uint64, bits int) []byte {
	for i := 0; i < bits/8; i++ {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

// parseBool accepts true/false, 1/0, on/off.
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "1", "true", "on":
		return true, nil
	case "0", "false", "off":
		return false, nil
	}
	return false, fmt.Errorf("expected true, false, 1 or 0")
}

// ValueText renders a decoded value for text and table output.
func ValueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = ValueText(e)
		}
		return "[" + strings.Join(parts, " ") + "]"
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return strconv.Quote(x)
	default:
		return fmt.Sprint(x)
	}
}
