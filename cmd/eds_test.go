package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/go-ethernetip/eds"
)

const adapterEDS = "../testdata/otfabric_adapter.eds"

func TestEDSInspectAndValidate(t *testing.T) {
	run(t, "eds", "inspect", adapterEDS).want(t, 0).has(t, `"e2e adapter" rev 3.7, vendor 65535 (OT Fabric)`, "catalog:        OTF-EIP-1",
		`Param2   "Speed setpoint"`, `Assem151 "Configuration"          instance 151  10 octets, 5 members`, `Connection1  "Exclusive owner"`)
	m := run(t, "eds", "inspect", adapterEDS, "--format", "json").want(t, 0).json(t)
	if len(m["params"].([]any)) != 5 || len(m["assemblies"].([]any)) != 5 || len(m["connections"].([]any)) != 3 {
		t.Errorf("inspect json: %v", m)
	}
	run(t, "eds", "inspect", adapterEDS, "--format", "table").want(t, 0).has(t, "KIND", "connection", "Exclusive owner")

	run(t, "eds", "validate", adapterEDS).want(t, 0).has(t, "valid, 0 error(s), 0 warning(s)")
	run(t, "eds", "validate", adapterEDS, "--format", "table").want(t, 0).has(t, "SEVERITY")
	// A file with errors: the report is written, and the exit code is 3.
	r := run(t, "eds", "validate", "../testdata/invalid.eds").want(t, 3).has(t, "INVALID, 4 error(s), 1 warning(s)", "ProdCode")
	if r.stderr != "" {
		t.Errorf("stderr: %s", r.stderr)
	}
	m = run(t, "eds", "validate", "../testdata/invalid.eds", "--format", "json").want(t, 3).json(t)
	if m["valid"] != false || m["errors"] != 4.0 {
		t.Errorf("validate json: %v", m)
	}
	run(t, "eds", "inspect", "../testdata/invalid.eds").want(t, 3).errHas(t, "run eds validate")
	r = run(t, "eds", "validate", "../testdata/garbage.eds", "--format", "json").want(t, 3)
	if kind, code := r.errorCode(t); kind != "invalid_input" || code != "invalid_eds" {
		t.Errorf("garbage: %s %s", kind, code)
	}
	r = run(t, "eds", "inspect", "no/such/file.eds", "--format", "json").want(t, 3)
	if _, code := r.errorCode(t); code != "input_file_open_failed" {
		t.Errorf("missing file: %s", code)
	}
	run(t, "eds", "inspect").want(t, 2)
}

func TestEDSCodegen(t *testing.T) {
	r := run(t, "eds", "codegen", adapterEDS, "--package", "adapter").want(t, 0).has(t, "package adapter", "DO NOT EDIT")
	dir := t.TempDir()
	out := filepath.Join(dir, "device.go")
	m := run(t, "eds", "codegen", adapterEDS, "--package", "adapter", "--output", out, "--format", "json").want(t, 0).json(t)
	if m["kind"] != "go" || m["output"] != out || int(m["bytes"].(float64)) != len(r.stdout) {
		t.Errorf("codegen file: %v", m)
	}
	data, err := os.ReadFile(out)
	if err != nil || string(data) != r.stdout {
		t.Errorf("the file is not the source: %v", err)
	}
	run(t, "eds", "codegen", adapterEDS, "--output", out).want(t, 3).errHas(t, "pass --force")
	run(t, "eds", "codegen", adapterEDS, "--output", out, "--force").want(t, 0).has(t, "wrote "+out, "sha256")
	run(t, "eds", "codegen", adapterEDS, "--output", out, "--force", "--format", "table").want(t, 0).has(t, "OUTPUT")
	run(t, "eds", "codegen", adapterEDS, "--output", filepath.Join(dir, "no", "dir.go")).want(t, 10)
	run(t, "eds", "codegen", adapterEDS, "--package", "9bad").want(t, 3)
	run(t, "eds", "codegen", adapterEDS, "--package", "").want(t, 3)
	run(t, "eds", "codegen", "../testdata/invalid.eds").want(t, 3)
	run(t, "eds", "codegen", "../testdata/garbage.eds").want(t, 3)
	run(t, "eds", "codegen", adapterEDS, "--format", "xml").want(t, 3)
	if code := Run(t.Context(), []string{"eds", "codegen", adapterEDS}, failingWriter{}, &syncBuffer{}); code != 10 {
		t.Errorf("unwritable stdout: %d", code)
	}
}

func TestEDSGenerate(t *testing.T) {
	flags := []string{"eds", "generate", "--vendor-id", "65535", "--vendor-name", "OT Fabric", "--product-name", "My adapter", "--product-code", "7",
		"--revision", "2.3", "--assembly", "100:32:Input", "--assembly", "150:32", "--assembly", "151:10:Config", "--assembly", "152:0:Heartbeat",
		"--connection", "151:150:100:32:32:20ms:Exclusive Owner", "--connection", "0:152:100:0:32", "--date", "2026-01-02", "--description", "test"}
	r := run(t, flags...).want(t, 0).has(t, "[Device]", `ProdName = "My adapter";`, "MajRev = 2;", "CreateDate = 01-02-2026;")
	f, err := eds.ParseString(r.stdout)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range eds.Validate(f) {
		if i.Severity == eds.Error {
			t.Errorf("the generated EDS has an error: %v", i)
		}
	}
	// The same flags give the same file.
	if again := run(t, flags...).want(t, 0); again.stdout != r.stdout {
		t.Error("eds generate is not deterministic with --date")
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "device.eds")
	run(t, append(flags, "--output", out)...).want(t, 0).has(t, "wrote "+out+" (eds,")
	run(t, "eds", "validate", out).want(t, 0)
	run(t, append(flags, "--output", out)...).want(t, 3)

	// From a device: the identity of the fixture.
	fx := startFixture(t)
	r = run(t, "eds", "generate", "--from-device", fx.addr, "--assembly", "100:32", "--date", "2026-01-02T03:04:05Z").want(t, 0)
	r.has(t, `ProdName = "test adapter";`, "ProdCode = 42;", "MajRev = 3;", "MinRev = 7;", `VendName = "Rockwell Automation/Allen-Bradley";`)
	run(t, "eds", "generate", "--from-device", refusedAddr(t)).want(t, 4)
	run(t, "eds", "generate", "--from-device", "300.1.1.1").want(t, 3)

	for _, args := range [][]string{
		{},
		{"--vendor-id", "1"},
		{"--vendor-id", "1", "--product-name", "x", "--revision", "1"},
		{"--vendor-id", "1", "--product-name", "x", "--revision", "0.1"},
		{"--vendor-id", "1", "--product-name", "x", "--assembly", "100"},
		{"--vendor-id", "1", "--product-name", "x", "--assembly", "x:1"},
		{"--vendor-id", "1", "--product-name", "x", "--connection", "1:2:3"},
		{"--vendor-id", "1", "--product-name", "x", "--connection", "1:2:3:x:5"},
		{"--vendor-id", "1", "--product-name", "x", "--connection", "1:0:3:4:5"},
		{"--vendor-id", "1", "--product-name", "x", "--connection", "1:2:3:4:5:soon"},
		{"--vendor-id", "1", "--product-name", "x", "--date", "yesterday"},
		{"--vendor-id", "1", "--product-name", "x", "--timeout", "-1s"},
		{"--vendor-id", "1", "--product-name", "x", "--format", "xml"},
		{"--vendor-id", "1", "--product-name", strings.Repeat("x", 40), "--assembly", "100:32", "--connection", "0:150:100:32:32"},
	} {
		run(t, append([]string{"eds", "generate"}, args...)...).want(t, 3)
	}
}
