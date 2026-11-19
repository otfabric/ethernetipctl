// SPDX-License-Identifier: MIT

package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ethernetip "github.com/otfabric/go-ethernetip"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

const modelIdentity = "identity: {vendor_id: 1, product_code: 1, revision: \"1.1\", serial_number: 1, product_name: x}\n"

// resolveModel writes a model file and resolves it as `adapter --config` does.
func resolveModel(t *testing.T, model string) (*Model, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "model.yaml")
	if err := os.WriteFile(p, []byte(model), 0o600); err != nil {
		t.Fatal(err)
	}
	return Resolve(&config.AdapterConfig{Config: p})
}

// wantInvalidConfig fails unless err is the refusal of a model: invalid_config, exit 3.
func wantInvalidConfig(t *testing.T, name string, err error) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Kind != errs.KindInvalidInput || e.Code != errs.CodeInvalidConfig || errs.ExitCodeForKind(e.Kind) != 3 {
		t.Errorf("%s: %v, want invalid_config (exit 3)", name, err)
	}
}

// repeat returns n YAML list entries of format, with the index as its argument.
func repeat(n int, format string) string {
	var b strings.Builder
	for i := range n {
		_, _ = fmt.Fprintf(&b, format, i+1)
	}
	return b.String()
}

// The bit of a BOOL member is the bit in the octet at its offset: 0 to 7.
func TestModelBoolMemberBit(t *testing.T) {
	for _, c := range []struct {
		bit int
		ok  bool
	}{{0, true}, {7, true}, {8, false}, {9, false}, {65535, false}} {
		model := modelIdentity + fmt.Sprintf(`logix:
  templates: [{id: 1, name: T, handle: 1, size: 4, members: [{name: B, type: BOOL, bit: %d, offset: 0}]}]
  tags: [{name: X, type: struct, template: 1}]
`, c.bit)
		_, err := resolveModel(t, model)
		if c.ok {
			if err != nil {
				t.Errorf("bit %d: %v", c.bit, err)
			}
			continue
		}
		wantInvalidConfig(t, fmt.Sprintf("bit %d", c.bit), err)
		if err != nil && !strings.Contains(err.Error(), "0 to 7") {
			t.Errorf("bit %d: %v", c.bit, err)
		}
	}
}

// A model is bounded as a whole: at most 4096 assemblies, tags and templates, and 64 MiB of
// assembly and tag data together. One above a bound is refused as invalid_config before
// the server is built, and a model above the data bound costs what its file costs, not
// the data it declares; one at the bound is accepted.
func TestModelBounds(t *testing.T) {
	const (
		asm   = "assemblies:\n"
		input = "  - {instance: %d, kind: input, size: 65000}\n"
		point = "  - {instance: %d, kind: input-only}\n"
		lint  = "    - {name: L%d, type: LINT, dims: [1048576, 2]}\n" // 16 MiB each
		tags  = "logix:\n  tags:\n"
	)
	bigTemplate := "logix:\n  templates: [{id: 1, name: T, handle: 1, size: 1048576, members: [{name: A, type: SINT, offset: 0}]}]\n  tags:\n"
	for _, c := range []struct {
		name  string
		model string
		ok    bool
		data  bool // above the data bound
	}{
		{"1032 assemblies of 65000", asm + repeat(1032, input), true, false},
		{"1033 assemblies of 65000", asm + repeat(1033, input), false, true},
		{"4096 assemblies", asm + repeat(4096, point), true, false},
		{"4097 assemblies", asm + repeat(4097, point), false, false},
		{"4096 tags", tags + repeat(4096, "    - {name: T%d, type: DINT}\n"), true, false},
		{"4097 tags", tags + repeat(4097, "    - {name: T%d, type: DINT}\n"), false, false},
		{"4097 templates", "logix:\n  templates:\n" + repeat(4097, "    - {id: %[1]d, name: T, handle: %[1]d, size: 4, members: [{name: A, type: DINT, offset: 0}]}\n") + "  tags: []\n", false, false},
		{"4 LINT tags of 16 MiB", tags + repeat(4, lint), true, false},
		{"5 LINT tags of 16 MiB", tags + repeat(5, lint), false, true},
		{"5 BOOL arrays of 16 MiB", tags + repeat(5, "    - {name: B%d, type: BOOL, dims: [1048576, 128]}\n"), false, true},
		{"5 STRING arrays of 16 MiB", tags + repeat(5, "    - {name: S%d, type: STRING, dims: [190000]}\n"), false, true},
		{"5 struct arrays of 16 MiB", bigTemplate + repeat(5, "    - {name: U%d, type: struct, template: 1, dims: [16]}\n"), false, true},
		{"assemblies and tags", asm + repeat(600, input) + tags + repeat(2, lint), false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			m, err := resolveModel(t, modelIdentity+c.model)
			if err == nil && !c.ok {
				// What the old bounds let through is built, as the adapter would.
				e := newEmitter(func(types.AdapterEvent) {})
				srv, berr := build(m, &config.AdapterConfig{IOAddress: "127.0.0.1:0"}, ethernetip.NopLogger(), e)
				if berr == nil {
					_ = srv.Close()
				}
				e.close(0)
			}
			runtime.ReadMemStats(&after)
			if c.ok {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			wantInvalidConfig(t, c.name, err)
			if n := after.TotalAlloc - before.TotalAlloc; c.data && n > 16<<20 {
				t.Errorf("%d MiB allocated for a refused model", n>>20)
			}
		})
	}
}

// TestModelUnsignedValues: unsigned tags take values up to their type's
// maximum, ULINT and LWORD above the int64 range included; a value above
// the type or a negative one is refused.
func TestModelUnsignedValues(t *testing.T) {
	for _, c := range []struct {
		typ, value string
		want       uint64
		ok         bool
	}{
		{"ULINT", "18446744073709551615", 1<<64 - 1, true},
		{"LWORD", "0x8000000000000000", 1 << 63, true},
		{"UDINT", "4294967295", 1<<32 - 1, true},
		{"USINT", "255", 255, true},
		{"BOOL", "1", 0xFF, true}, // Logix stores true as 0xFF
		{"USINT", "256", 0, false},
		{"UINT", "-1", 0, false},
		{"ULINT", "18446744073709551616", 0, false},
	} {
		m, err := resolveModel(t, modelIdentity+fmt.Sprintf("logix:\n  tags: [{name: X, type: %s, value: %q}]\n", c.typ, c.value))
		if err != nil {
			t.Errorf("%s %s: %v", c.typ, c.value, err)
			continue
		}
		// Values are checked when the tag database is built.
		ts, err := m.Logix.buildTagServer()
		if !c.ok {
			if err == nil {
				t.Errorf("%s %s: accepted", c.typ, c.value)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s %s: %v", c.typ, c.value, err)
			continue
		}
		v, err := ts.Get("X")
		if err != nil {
			t.Fatal(err)
		}
		var got uint64
		for i := len(v.Data) - 1; i >= 0; i-- {
			got = got<<8 | uint64(v.Data[i])
		}
		if got != c.want {
			t.Errorf("%s %s: %x, want %x", c.typ, c.value, v.Data, c.want)
		}
	}
}
