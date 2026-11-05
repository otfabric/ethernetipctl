// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Hostile files: EDS and adapter model files that are huge, malformed, binary, symlinks,
// directories, unreadable; and output files, which must be private (0600), must not
// replace what exists without --force, and must not truncate their own input.

// fileCodes are the codes a file that cannot be used may give: exit 3.
var fileCodes = map[string]bool{"input_file_open_failed": true, "invalid_eds": true, "invalid_config": true, "invalid_input": true}

// wantFileRefused runs args and wants a fatal invalid input of a file, in time.
func wantFileRefused(t *testing.T, args ...string) result {
	t.Helper()
	start := time.Now()
	r := run(t, append(args, "--format", "json")...).want(t, 3)
	if kind, code := r.errorCode(t); kind != "invalid_input" || !fileCodes[code] {
		t.Errorf("%s %s", kind, code)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("took %v", d)
	}
	return r
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	must(t, os.WriteFile(p, data, 0o600))
	return p
}

// junk returns n seeded random octets.
func junk(seed uint64, n int) []byte {
	r := rand.New(rand.NewPCG(seed, seed+1))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.UintN(256))
	}
	return b
}

func TestHostileEDSFiles(t *testing.T) {
	dir := t.TempDir()
	valid, err := os.ReadFile(adapterEDS)
	must(t, err)
	huge := bytes.Repeat([]byte("$ a comment line that only makes the file larger\n"), (17<<20)/49)
	longLine := append([]byte("[File]\nDescText = \""), bytes.Repeat([]byte("x"), 8<<20)...)
	manyFields := append([]byte("[Params]\nParam1 = "), bytes.Repeat([]byte("0,"), 10000)...)
	manyEntries := append([]byte("[Params]\n"), bytes.Repeat([]byte("X = 1;\n"), 300000)...)
	for name, data := range map[string][]byte{
		"huge.eds":          huge,
		"long-line.eds":     longLine,
		"many-fields.eds":   append(manyFields, ";\n"...),
		"many-entries.eds":  manyEntries,
		"binary.eds":        junk(7, 64<<10),
		"nul.eds":           make([]byte, 4096),
		"unterminated.eds":  []byte("[File]\nDescText = \"never closed\n"),
		"bad-section.eds":   []byte("[File\nDescText = \"x\";\n"),
		"truncated.eds":     valid[:len(valid)/2],
		"utf16.eds":         append([]byte{0xFF, 0xFE}, bytes.ReplaceAll(valid, []byte{}, []byte{0})...),
		"nested-braces.eds": append([]byte("[Params]\nParam1 = "), append(bytes.Repeat([]byte("{"), 100000), ";\n"...)...),
	} {
		t.Run(name, func(t *testing.T) {
			p := writeFile(t, dir, name, data)
			for _, cmd := range []string{"validate", "inspect", "codegen"} {
				r := run(t, "eds", cmd, p, "--format", "json")
				switch r.code {
				case 3:
					if !jsonObject(r.stdout) {
						t.Errorf("eds %s: not one JSON object:\n%.300s", cmd, r.stdout)
					}
				case 0:
					if cmd == "validate" || cmd == "inspect" {
						_ = r.json(t)
					}
				default:
					t.Errorf("eds %s: exit %d\n%.300s", cmd, r.code, r.stderr)
				}
			}
		})
	}
	// Files that are not regular files, or cannot be read.
	wantFileRefused(t, "eds", "validate", dir)
	wantFileRefused(t, "eds", "validate", filepath.Join(dir, "missing.eds"))
	if runtime.GOOS != "windows" {
		must(t, os.Symlink(filepath.Join(dir, "nowhere.eds"), filepath.Join(dir, "dangling.eds")))
		wantFileRefused(t, "eds", "inspect", filepath.Join(dir, "dangling.eds"))
		loop := filepath.Join(dir, "loop.eds")
		must(t, os.Symlink(loop, loop))
		wantFileRefused(t, "eds", "inspect", loop)
		// A symlink to a good file is that file.
		abs, err := filepath.Abs(adapterEDS)
		must(t, err)
		must(t, os.Symlink(abs, filepath.Join(dir, "link.eds")))
		run(t, "eds", "validate", filepath.Join(dir, "link.eds")).want(t, 0)
		if os.Getuid() != 0 {
			locked := writeFile(t, dir, "locked.eds", valid)
			must(t, os.Chmod(locked, 0))
			r := wantFileRefused(t, "eds", "validate", locked)
			if _, code := r.errorCode(t); code != "input_file_open_failed" {
				t.Errorf("unreadable: %s", code)
			}
		}
	}
	if _, err := os.Stat(os.DevNull); err == nil {
		r := run(t, "eds", "validate", os.DevNull, "--format", "json")
		if r.code != 3 || !jsonObject(r.stdout) {
			t.Errorf("%s: exit %d\n%s", os.DevNull, r.code, r.stdout)
		}
	}
}

// jsonObject reports whether s is exactly one JSON object.
func jsonObject(s string) bool {
	var m map[string]any
	return json.Unmarshal([]byte(s), &m) == nil
}

func TestHostileAdapterModelFiles(t *testing.T) {
	dir := t.TempDir()
	id := "identity: {vendor_id: 1, product_code: 1, revision: \"1.1\", serial_number: 1, product_name: x}\n"
	laughs := "a: &a [\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\",\"lol\"]\n"
	prev := "a"
	for _, n := range []string{"b", "c", "d", "e", "f", "g", "h", "i"} {
		laughs += n + ": &" + n + " [*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + "]\n"
		prev = n
	}
	var manyAssemblies strings.Builder
	manyAssemblies.WriteString(id + "assemblies:\n")
	for i := 1; i <= 20000; i++ {
		manyAssemblies.WriteString("  - {instance: " + strconv.Itoa(i) + ", kind: input, size: 65000}\n")
	}
	for name, data := range map[string][]byte{
		"binary.yaml":       junk(11, 64<<10),
		"nul.yaml":          make([]byte, 4096),
		"empty.yaml":        nil,
		"laughs.yaml":       []byte(laughs),
		"deep.yaml":         []byte(strings.Repeat("[", 100000)),
		"deep.json":         []byte(strings.Repeat("[", 100000)),
		"huge-name.yaml":    []byte("identity: {revision: \"1.1\", product_name: \"" + strings.Repeat("x", 1<<20) + "\"}\n"),
		"tabs.yaml":         []byte("identity:\n\t vendor_id: 1\n"),
		"two-docs.yaml":     []byte(id + "---\n" + id),
		"number.json":       []byte(`{"identity": {"vendor_id": 65536, "revision": "1.1", "product_name": "x"}}`),
		"negative.json":     []byte(`{"identity": {"vendor_id": -1, "revision": "1.1", "product_name": "x"}}`),
		"serial.yaml":       []byte("identity: {serial_number: 4294967296, revision: \"1.1\", product_name: x}\n"),
		"size-neg.yaml":     []byte(id + "assemblies: [{instance: 1, kind: input, size: -1}]\n"),
		"size-huge.yaml":    []byte(id + "assemblies: [{instance: 1, kind: input, size: 9223372036854775807}]\n"),
		"instance-big.yaml": []byte(id + "assemblies: [{instance: 4294967296, kind: input, size: 1}]\n"),
		"dims-huge.yaml":    []byte(id + "logix: {tags: [{name: X, type: DINT, dims: [2147483647]}]}\n"),
		"dims-neg.yaml":     []byte(id + "logix: {tags: [{name: X, type: DINT, dims: [-1]}]}\n"),
		"dims-many.yaml":    []byte(id + "logix: {tags: [{name: X, type: DINT, dims: [65535, 65535, 65535]}]}\n"),
		"value-wrap.yaml":   []byte(id + "logix: {tags: [{name: X, type: SINT, value: \"128\"}]}\n"),
		"many-asm.yaml":     []byte(manyAssemblies.String()),
	} {
		t.Run(name, func(t *testing.T) {
			p := writeFile(t, dir, name, data)
			start := time.Now()
			r := run(t, "adapter", "--config", p, "--listen", "127.0.0.1:0", "--io-address", "127.0.0.1:0", "--duration", "1ms", "--format", "json")
			if d := time.Since(start); d > 10*time.Second {
				t.Errorf("took %v", d)
			}
			switch r.code {
			case 3:
				if kind, code := r.errorCode(t); kind != "invalid_input" || code != "invalid_config" {
					t.Errorf("%s %s", kind, code)
				}
			case 0:
				// A model the adapter accepts: only those whose values hold.
				if strings.HasPrefix(name, "value-wrap") || strings.HasPrefix(name, "dims-") || strings.HasPrefix(name, "size-") {
					t.Errorf("accepted a model with a value that does not hold:\n%.300s", r.stdout)
				}
				jsonLines(t, r.stdout)
			default:
				t.Errorf("exit %d\n%.300s\n%.300s", r.code, r.stdout, r.stderr)
			}
		})
	}
	wantFileRefused(t, "adapter", "--config", dir, "--listen", "127.0.0.1:0", "--duration", "1ms")
	if runtime.GOOS != "windows" {
		must(t, os.Symlink(filepath.Join(dir, "nowhere.yaml"), filepath.Join(dir, "dangling.yaml")))
		wantFileRefused(t, "adapter", "--config", filepath.Join(dir, "dangling.yaml"), "--listen", "127.0.0.1:0", "--duration", "1ms")
		if os.Getuid() != 0 {
			locked := writeFile(t, dir, "locked.yaml", []byte(id))
			must(t, os.Chmod(locked, 0))
			wantFileRefused(t, "adapter", "--config", locked, "--listen", "127.0.0.1:0", "--duration", "1ms")
		}
	}
}

// genFlags are the flags of a small eds generate.
var genFlags = []string{"eds", "generate", "--vendor-id", "1", "--product-name", "x", "--revision", "1.1", "--assembly", "100:4", "--date", "2026-01-02"}

// Output files hold device descriptions and generated code: they are created private,
// 0600, whatever the umask lets through.
func TestOutputFilesAreCreated0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes")
	}
	dir := t.TempDir()
	eds := filepath.Join(dir, "device.eds")
	run(t, append(genFlags, "--output", eds)...).want(t, 0)
	gosrc := filepath.Join(dir, "device.go")
	run(t, "eds", "codegen", adapterEDS, "--output", gosrc).want(t, 0)
	for _, p := range []string{eds, gosrc} {
		st, err := os.Stat(p)
		must(t, err)
		if m := st.Mode().Perm(); m != 0o600 {
			t.Errorf("%s: mode %04o, want 0600", filepath.Base(p), m)
		}
	}
	// --force over an existing file of a wider mode leaves no wider file behind.
	wide := writeFile(t, dir, "wide.eds", []byte("old"))
	must(t, os.Chmod(wide, 0o644))
	run(t, append(genFlags, "--output", wide, "--force")...).want(t, 0)
	if st, err := os.Stat(wide); err == nil && st.Mode().Perm()&0o077 != 0 {
		t.Errorf("--force over a 0644 file: mode %04o, want 0600", st.Mode().Perm())
	}
}

// An output path that exists, as a file, a directory or a symlink (dangling or not), is
// not replaced without --force: exit 3 output_file_create_failed, nothing written.
func TestOutputDoesNotReplaceWithoutForce(t *testing.T) {
	dir := t.TempDir()
	existing := writeFile(t, dir, "existing.eds", []byte("keep me"))
	paths := []string{existing, dir}
	var target string
	if runtime.GOOS != "windows" {
		target = filepath.Join(dir, "target.eds")
		dangling := filepath.Join(dir, "dangling.eds")
		must(t, os.Symlink(target, dangling))
		live := filepath.Join(dir, "live.eds")
		must(t, os.Symlink(existing, live))
		paths = append(paths, dangling, live)
	}
	for _, p := range paths {
		r := run(t, append(genFlags, "--output", p, "--format", "json")...).want(t, 3)
		if kind, code := r.errorCode(t); kind != "invalid_input" || code != "output_file_create_failed" {
			t.Errorf("%s: %s %s", filepath.Base(p), kind, code)
		}
	}
	if b, _ := os.ReadFile(existing); string(b) != "keep me" {
		t.Errorf("the existing file was changed: %q", b)
	}
	if target != "" {
		if _, err := os.Lstat(target); err == nil {
			t.Error("the dangling symlink was followed and its target created")
		}
	}
	// --force into a directory cannot work: an output error (10).
	r := run(t, append(genFlags, "--output", dir, "--force", "--format", "json")...)
	if r.code != 10 && r.code != 3 {
		t.Errorf("--force into a directory: exit %d", r.code)
	}
	r.errorCode(t)
}

// codegen with --output naming its own input, with --force: the EDS is read in full
// before the output is opened, so the file holds the generated code, not nothing.
func TestCodegenOverItsOwnInput(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(adapterEDS)
	must(t, err)
	p := writeFile(t, dir, "self.eds", src)
	run(t, "eds", "codegen", p, "--output", p).want(t, 3) // exists: no --force
	if b, _ := os.ReadFile(p); !bytes.Equal(b, src) {
		t.Fatal("codegen without --force changed its input")
	}
	run(t, "eds", "codegen", p, "--output", p, "--force").want(t, 0)
	b, err := os.ReadFile(p)
	must(t, err)
	if len(b) == 0 || !bytes.Contains(b, []byte("package ")) {
		t.Errorf("the file is %d octets after codegen over itself:\n%.200s", len(b), b)
	}
}

// The text flags of eds generate end up in quoted EDS strings: a newline in one (or any
// control character) gives a file that does not parse, written with exit 0. It must be
// refused as invalid input, or the text escaped so that the file validates.
func TestEDSGenerateTextWithNewlineGivesAValidFile(t *testing.T) {
	dir := t.TempDir()
	base := []string{"eds", "generate", "--vendor-id", "1", "--revision", "1.1", "--assembly", "100:4", "--assembly", "150:4", "--date", "2026-01-02"}
	for name, extra := range map[string][]string{
		"product-name": {"--product-name", "x\nVendCode = 9;"},
		"description":  {"--product-name", "x", "--description", "line 1\nline 2"},
		"vendor-name":  {"--product-name", "x", "--vendor-name", "OT\rFabric"},
		"assembly":     {"--product-name", "x", "--assembly", "151:2:Config\n[Device]"},
		"connection":   {"--product-name", "x", "--connection", "0:150:100:4:4:10ms:Owner\n$"},
	} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(dir, name+".eds")
			r := run(t, append(append(append([]string(nil), base...), extra...), "--output", out)...)
			if r.code == 3 {
				return // refused: fine
			}
			if r.code != 0 {
				t.Fatalf("exit %d\n%s", r.code, r.stderr)
			}
			if v := run(t, "eds", "validate", out); v.code != 0 {
				t.Errorf("eds generate wrote an EDS that does not validate (exit %d):\n%s%s", v.code, v.stdout, v.stderr)
			}
		})
	}
}
