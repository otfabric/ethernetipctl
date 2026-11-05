// SPDX-License-Identifier: MIT

package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoadModel feeds model files, YAML and JSON: Load, Check and the tag database of a
// model never panic, and a model Check accepts has assemblies the server can hold.
func FuzzLoadModel(f *testing.F) {
	id := "identity: {vendor_id: 1, product_code: 1, revision: \"1.1\", serial_number: 1, product_name: x}\n"
	for _, s := range []string{
		id,
		id + "assemblies: [{instance: 100, kind: input, size: 4, data: \"01020304\"}, {instance: 150, kind: output, size: 4}]\necho: [{output: 150, input: 100}]\n",
		id + "assemblies: [{instance: 1, kind: input, size: 65000}]\n",
		id + "assemblies: [{instance: 1, kind: input, size: -1}]\n",
		id + "logix: {tags: [{name: X, type: DINT, dims: [10], value: \"1,2\"}, {name: S, type: STRING, value: \"a|b\"}]}\n",
		id + "logix: {templates: [{id: 1, name: T, handle: 1, size: 4, members: [{name: A, type: DINT, offset: 0}]}], tags: [{name: X, type: struct, template: 1}]}\n",
		id + "logix: {tags: [{name: X, type: SINT, value: \"128\"}, {name: Y, type: REAL, value: \"1e39\"}]}\n",
		`{"identity": {"vendor_id": 1, "revision": "1.1", "product_name": "x"}, "assemblies": [{"instance": 1, "kind": "config", "size": 2}]}`,
		"a: &a [1]\nb: [*a, *a]\n",
		"",
	} {
		f.Add([]byte(s), false)
		f.Add([]byte(s), true)
	}
	f.Fuzz(func(t *testing.T, data []byte, asJSON bool) {
		name := "model.yaml"
		if asJSON {
			name = "model.json"
		}
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		m, err := Load(p)
		if err != nil {
			return
		}
		if m.Check() != nil {
			return
		}
		for _, a := range m.Assemblies {
			if a.Instance == 0 || a.Size < 0 || a.Size > 65000 {
				t.Fatalf("accepted assembly %+v", a)
			}
		}
		if m.Logix != nil {
			for _, tag := range m.Logix.Tags {
				n := 1
				for _, d := range tag.Dims {
					if d <= 0 || d > 1<<16 {
						return // the tag server decides; a huge array would only test memory
					}
					n *= d
				}
				if n > 1<<16 {
					return
				}
			}
			_, _ = m.Logix.buildTagServer()
		}
	})
}
