package cmd

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/server"
)

func TestIdentifyStatusServicesInterfaces(t *testing.T) {
	f := startFixture(t)
	f.run(t, "identify", "ADDR").want(t, 0).has(t, `product name:   "test adapter"`, "vendor:         1 (Rockwell Automation/Allen-Bradley)",
		"revision:       3.7", "serial number:  0x12345678", "state:          3 (operational)", "Identity object (Get_Attributes_All @1/1):")
	m := f.run(t, "identify", "ADDR", "--format", "json", "--route", "1/0").want(t, 0).json(t)
	if m["identity"].(map[string]any)["product_name"] != "test adapter" || m["list_identity"].(map[string]any)["socket"] == "" {
		t.Errorf("identify json: %v", m)
	}
	f.run(t, "identify", "ADDR", "--format", "table", "--connected").want(t, 0).has(t, "ATTRIBUTE", "identity.product_name", "list_identity.state")

	f.run(t, "status", "ADDR").want(t, 0).has(t, ": ok  state 3 operational")
	f.run(t, "status", "ADDR", "--format", "table").want(t, 0).has(t, "HEALTHY", "true", "operational")
	f.srv.SetIdentityStatus(cip.StatusOwned | cip.StatusMajorRecoverableFault | 0x0030)
	m = f.run(t, "status", "ADDR", "--format", "json").want(t, 0).json(t)
	if m["healthy"] != false || !strings.Contains(m["identity"].(map[string]any)["status_flags"].(string), "OWNED") {
		t.Errorf("status json: %v", m)
	}
	f.run(t, "status", "ADDR").want(t, 0).has(t, "FAULT: major-recoverable-fault", "extended 3")

	f.run(t, "services", "ADDR").want(t, 0).has(t, "Communications", "tcp=true udp=true")
	f.run(t, "services", "ADDR", "--format", "table").want(t, 0).has(t, "CAPABILITIES", "0x0120")
	f.run(t, "interfaces", "ADDR").want(t, 0).has(t, "0 interface(s)")
	m = f.run(t, "interfaces", "ADDR", "--format", "json").want(t, 0).json(t)
	if m["count"] != 0.0 || m["interfaces"] == nil {
		t.Errorf("interfaces json: %v", m)
	}
	f.run(t, "interfaces", "ADDR", "--format", "table").want(t, 0).has(t, "TYPE", "NAME")

	// An Identity object that does not answer Get_Attributes_All: identify still reports
	// ListIdentity, with the error.
	f.srv.RegisterObject(server.NewObject(cip.ClassIdentity, 1))
	f.run(t, "identify", "ADDR").want(t, 0).has(t, "error: the target refused the request")
	f.run(t, "identify", "ADDR", "--format", "table").want(t, 0).has(t, "identity.error")
	m = f.run(t, "identify", "ADDR", "--format", "json").want(t, 0).json(t)
	if m["identity_error"].(map[string]any)["code"] != "path_destination_unknown" {
		t.Errorf("identity error: %v", m["identity_error"])
	}
}

func TestRead(t *testing.T) {
	f := startFixture(t)
	f.run(t, "read", "ADDR", "@1/1/7", "--type", "short_string").want(t, 0).has(t, "Read @1/1/7 (Identity) Get_Attribute_Single", `SHORT_STRING "test adapter"`)
	m := f.run(t, "read", "ADDR", "@1/1/1", "--type", "UINT", "--format", "json").want(t, 0).json(t)
	if m["value"] != 1.0 || m["data"] != "0100" || m["attribute"] != 1.0 || m["class_name"] != "Identity" {
		t.Errorf("read json: %v", m)
	}
	f.run(t, "read", "ADDR", "@1/1", "--all").want(t, 0).has(t, "Get_Attributes_All", "01 00 0c 00 2a 00 03 07")
	f.run(t, "get", "ADDR", "20 01 24 01 30 06", "--type", "UDINT", "--connected").want(t, 0).has(t, "(connected)", "UDINT 305419896")
	f.run(t, "attribute", "ADDR", "@0x04/100/3", "--route", "1/0", "--format", "table").want(t, 0).has(t, "PATH", "@4/100/3", "Get_Attribute_Single", "32")
	f.run(t, "read", "ADDR", "@4/100/3", "--type", "DINT").want(t, 0).has(t, "DINT [0 0 0 0 0 0 0 0]")
	f.run(t, "read", "ADDR", "@0xF5/1/8", "--type", "BOOL").want(t, 0).has(t, "BOOL true")

	// Invalid input: nothing is sent.
	before := f.metrics.connects.Load()
	for _, args := range [][]string{
		{"read", "ADDR", "@1"},
		{"read", "ADDR", "symbol"},
		{"read", "ADDR", "@1/1"},
		{"read", "ADDR", "@1/1/7", "--all"},
		{"read", "ADDR", "@1/1/7", "--type", "FLOAT"},
		{"read", "ADDR", "@1/1/7", "--route", "x/y"},
		{"read", "10.0.0.1:0", "@1/1/7"},
		{"read", "ADDR", "@1/1/7", "--format", "xml"},
	} {
		f.run(t, args...).want(t, 3)
	}
	run(t, "read", f.addr, "@1/1/7", "--timeout", "-1s").want(t, 3).errHas(t, "invalid --timeout")
	if n := f.metrics.connects.Load(); n != before {
		t.Errorf("invalid invocations opened %d sessions", n-before)
	}
	// Data that is not the type asked for is invalid input.
	f.run(t, "read", "ADDR", "@1/1/7", "--type", "UDINT").want(t, 3).errHas(t, "not a whole number of UDINT values")

	r := f.run(t, "read", "ADDR", "@1/1/99", "--format", "json").want(t, 6)
	if kind, code := r.errorCode(t); kind != "refused" || code != "attribute_not_supported" {
		t.Errorf("refused read: %s %s", kind, code)
	}
	f.run(t, "read", "ADDR", "@0x77/1/1").want(t, 6).errHas(t, "path destination unknown")
}

func TestWrite(t *testing.T) {
	f := startFixture(t)
	// Without --yes nothing is sent: not even a session is opened.
	r := f.run(t, "write", "ADDR", "@0xF5/1/8", "--type", "USINT", "--value", "5", "--format", "json").want(t, 3)
	if kind, code := r.errorCode(t); kind != "invalid_input" || code != "confirmation_required" {
		t.Errorf("write without --yes: %s %s", kind, code)
	}
	f.run(t, "assembly", "write", "ADDR", "150", "--data", "00").want(t, 3).errHas(t, "--yes")
	f.run(t, "reset", "ADDR").want(t, 3)
	f.run(t, "tag", "write", "ADDR", "Count", "--type", "DINT", "--value", "1").want(t, 3)
	if n, s := f.metrics.connects.Load(), f.metrics.sets.Load(); n != 0 || s != 0 {
		t.Fatalf("writes without --yes reached the adapter: %d sessions, %d sets", n, s)
	}
	if f.resets.Load() != 0 {
		t.Fatal("a reset without --yes reached the adapter")
	}

	f.run(t, "write", "ADDR", "@0xF5/1/8", "--type", "USINT", "--value", "5", "--yes").want(t, 0).has(t, "Wrote @245/1/8", "USINT 5")
	f.run(t, "read", "ADDR", "@0xF5/1/8", "--type", "USINT").want(t, 0).has(t, "USINT 5")
	f.run(t, "write", "ADDR", "@0xF5/1/13", "--data", "3c 00", "--yes", "--connected", "--format", "table").want(t, 0).has(t, "Set_Attribute_Single", "3c00")
	m := f.run(t, "write", "ADDR", "@0xF5/1/13", "--type", "UINT", "--value", "0x40", "--yes", "--format", "json").want(t, 0).json(t)
	if m["value"] != 64.0 || m["data"] != "4000" {
		t.Errorf("write json: %v", m)
	}
	if f.metrics.sets.Load() != 3 {
		t.Errorf("%d sets", f.metrics.sets.Load())
	}
	for _, args := range [][]string{
		{"write", "ADDR", "@0xF5/1/8", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--data", "05", "--type", "USINT", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--data", "0g", "--yes"},
		{"write", "ADDR", "@0xF5/1", "--data", "05", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "USINT", "--value", "300", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "BOOL", "--value", "maybe", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "REAL", "--value", "x", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "LREAL", "--value", "x", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "INT", "--value", "1,,2", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "SINT", "--value", "x", "--yes"},
		{"write", "ADDR", "@0xF5/1/8", "--type", "SHORT_STRING", "--value", strings.Repeat("x", 256), "--yes"},
	} {
		f.run(t, args...).want(t, 3)
	}
	f.run(t, "write", "ADDR", "@1/1/1", "--data", "0100", "--yes").want(t, 6).errHas(t, "attribute not settable")
	f.run(t, "write", "ADDR", "@0xF5/1/8", "--data", "0000", "--yes").want(t, 6)
}

func TestValueTypesRoundTrip(t *testing.T) {
	f := startFixture(t)
	// The configuration assembly (10 octets) takes any encoding of 10 octets.
	for _, c := range []struct{ typ, value, text string }{
		{"SINT", "-1,2,-3,4,-5,6,-7,8,-9,10", "SINT [-1 2 -3 4 -5 6 -7 8 -9 10]"},
		{"BYTE", "1,2,3,4,5,6,7,8,9,255", "BYTE [1 2 3 4 5 6 7 8 9 255]"},
		{"INT", "-1,2,-3,4,-5", "INT [-1 2 -3 4 -5]"},
		{"WORD", "1,2,3,4,65535", "WORD [1 2 3 4 65535]"},
		{"STRING", "abcdefgh", `STRING "abcdefgh"`},
		{"STRING2", "abcd", `STRING2 "abcd"`},
		{"SHORT_STRING", "abcdefghi", `SHORT_STRING "abcdefghi"`},
		{"BOOL", "1,0,true,false,on,off,1,0,1,0", "BOOL [true false true false true false true false true false]"},
	} {
		f.run(t, "write", "ADDR", "@4/151/3", "--type", c.typ, "--value", c.value, "--yes").want(t, 0).has(t, c.text)
		f.run(t, "read", "ADDR", "@4/151/3", "--type", c.typ).want(t, 0).has(t, c.text)
	}
	// 8-octet types: 32 octets of the output assembly.
	for _, c := range []struct{ typ, value, text string }{
		{"LINT", "-1,2,-3,4", "LINT [-1 2 -3 4]"},
		{"ULINT", "1,2,3,18446744073709551615", "ULINT [1 2 3 18446744073709551615]"},
		{"LWORD", "1,2,3,4", "LWORD [1 2 3 4]"},
		{"LREAL", "1.5,-2.25,3,4e100", "LREAL [1.5 -2.25 3 4e+100]"},
		{"REAL", "1.1,2,3,4,5,6,7,8", "REAL [1.1 2 3 4 5 6 7 8]"},
		{"DINT", "-1,2,-3,4,-5,6,-7,8", "DINT [-1 2 -3 4 -5 6 -7 8]"},
		{"UDINT", "1,2,3,4,5,6,7,4294967295", "UDINT [1 2 3 4 5 6 7 4294967295]"},
		{"DWORD", "1,2,3,4,5,6,7,8", "DWORD [1 2 3 4 5 6 7 8]"},
		{"UINT", "1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16", "UINT [1 2 3"},
		{"USINT", strings.TrimSuffix(strings.Repeat("7,", 32), ","), "USINT [7 7"},
	} {
		f.run(t, "assembly", "write", "ADDR", "150", "--data", hexOf(t, c.typ, c.value), "--yes").want(t, 0)
		f.run(t, "read", "ADDR", "@4/100/3", "--type", c.typ).want(t, 0).has(t, c.text)
	}
	// A REAL that is not a number is written as text, which JSON can carry.
	f.run(t, "assembly", "write", "ADDR", "150", "--data", strings.Repeat("0000c07f", 8), "--yes").want(t, 0)
	m := f.run(t, "read", "ADDR", "@4/100/3", "--type", "REAL", "--format", "json").want(t, 0).json(t)
	if m["value"].([]any)[0] != "NaN" {
		t.Errorf("NaN: %v", m["value"])
	}
	// A string that does not decode is invalid input.
	f.run(t, "read", "ADDR", "@1/1/1", "--type", "STRING").want(t, 3).errHas(t, "not a STRING")
}

// hexOf encodes values with the write command's encoder through a dry run: write without
// --yes validates and sends nothing, so the hex comes from the encoder directly.
func hexOf(t *testing.T, typ, value string) string {
	t.Helper()
	b, err := encodeForTest(typ, value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestObjects(t *testing.T) {
	f := startFixture(t)
	f.run(t, "objects", "ADDR").want(t, 0).has(t, "0x01       1  Identity", "0x6B     107  Symbol (Rockwell)", "0xF6     246  Ethernet Link")
	m := f.run(t, "objects", "ADDR", "--format", "json", "--connected").want(t, 0).json(t)
	if m["count"].(float64) < 7 {
		t.Errorf("objects: %v", m)
	}
	f.run(t, "objects", "ADDR", "--format", "table").want(t, 0).has(t, "VENDOR_SPECIFIC", "true")

	f.run(t, "object", "ADDR", "identity").want(t, 0).has(t, "Class 0x01 Identity", "max_instance_attribute", "Instance 1 (Get_Attributes_All)")
	f.run(t, "object", "ADDR", "TCP/IP Interface", "1", "--format", "table").want(t, 0).has(t, "INSTANCE", "Get_Attributes_All")
	f.run(t, "object", "ADDR", "symbol", "--format", "table").want(t, 0)
	m = f.run(t, "object", "ADDR", "0xF6", "2", "--format", "json").want(t, 0).json(t)
	if m["instance_error"] == nil || m["class_name"] != "Ethernet Link" {
		t.Errorf("object json: %v", m)
	}
	f.run(t, "object", "ADDR", "0x02", "2").want(t, 0).has(t, "error: the target refused")
	f.run(t, "object", "ADDR", "frobnicator").want(t, 3).errHas(t, "unknown class")
	f.run(t, "object", "ADDR", "1", "x").want(t, 3)
	f.run(t, "object", "ADDR").want(t, 2)
}

func TestResetAndConnections(t *testing.T) {
	f := startFixture(t)
	f.run(t, "reset", "ADDR", "--type", "2", "--yes").want(t, 3)
	f.run(t, "reset", "ADDR", "--yes").want(t, 0).has(t, "Reset type 0 (power cycle)")
	f.run(t, "reset", "ADDR", "--yes", "--type", "1", "--format", "table").want(t, 0).has(t, "TYPE")
	m := f.run(t, "reset", "ADDR", "--yes", "--type", "1", "--format", "json").want(t, 0).json(t)
	if m["type"] != 1.0 || f.resets.Load() != 3 {
		t.Errorf("reset: %v, %d resets", m, f.resets.Load())
	}
	f.run(t, "reset", "ADDR", "--yes", "--route", "x").want(t, 3)

	f.run(t, "connections", "ADDR").want(t, 0).has(t, "active:         0", "maximum:        128", "open_requests            0")
	f.run(t, "connections", "ADDR", "--format", "table").want(t, 0).has(t, "connection_timeouts")
	// A Connection Manager without counters, and a Message Router without the connection
	// attributes, report the error of each.
	f.srv.RegisterObject(server.NewObject(cip.ClassConnectionManager, 1))
	mr := server.NewObject(cip.ClassMessageRouter, 1)
	mr.AddInstance(1)
	f.srv.RegisterObject(mr)
	m = f.run(t, "connections", "ADDR", "--format", "json").want(t, 0).json(t)
	if m["active_connections"] != nil || m["counters"].([]any)[0].(map[string]any)["error"] == nil {
		t.Errorf("connections without counters: %v", m)
	}
	f.run(t, "connections", "ADDR").want(t, 0).has(t, "active:         -", "error:")
}

func TestSessionAndForwardOpen(t *testing.T) {
	f := startFixture(t)
	f.run(t, "session", "ADDR").want(t, 0).has(t, "Session 0x", "with "+f.addr)
	f.run(t, "connect", "ADDR", "--connected", "--hold", "10ms", "--route", "1/0").want(t, 0).has(t, "Class 3 connection:", "path:           @2/1", "held for")
	m := f.run(t, "session", "ADDR", "--connected", "--large", "--size", "1000", "--format", "json").want(t, 0).json(t)
	if m["connection"].(map[string]any)["class"] != 3.0 {
		t.Errorf("session json: %v", m)
	}
	f.run(t, "session", "ADDR", "--connected", "--format", "table").want(t, 0).has(t, "connection.ot_connection_id")
	f.run(t, "session", "ADDR", "--hold", "-1s").want(t, 3)
	f.run(t, "session", "ADDR", "--route", "x").want(t, 3)
	f.run(t, "session", "ADDR", "--connected", "--size", "65535").want(t, 3)

	io := []string{"--config", "151", "--output", "150", "--input", "100", "--output-size", "32", "--input-size", "32", "--rpi", "10ms"}
	f.run(t, append([]string{"forward-open", "ADDR"}, io...)...).want(t, 0).has(t, "Forward_Open to", "O->T 38 octets, T->O 34 octets", "closed again with Forward_Close")
	f.run(t, append([]string{"forward-open", "ADDR", "--large", "--format", "table", "--config-data", "01020304050607080910"}, io...)...).want(t, 0).has(t, "Large_Forward_Open")
	if n := len(f.srv.Connections()); n != 0 {
		t.Fatalf("%d connections left open", n)
	}

	keep := []string{"forward-open", "ADDR", "--config", "151", "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "1s",
		"--timeout-multiplier", "7", "--keep", "--connection-serial", "77", "--originator-serial", "1234"}
	f.run(t, "forward-open", "ADDR", "--config", "151", "--output", "152", "--input", "100", "--input-size", "32", "--rpi", "100ms", "--multicast").
		want(t, 0).has(t, "closed again")
	m = f.run(t, append(keep, "--format", "json")...).want(t, 0).json(t)
	conn := m["connection"].(map[string]any)
	if m["closed"] != false || conn["connection_serial"] != 77.0 || conn["originator_serial"] != 1234.0 || m["ot_size"] != 2.0 {
		t.Errorf("forward-open --keep: %v", m)
	}
	f.run(t, keep...).want(t, 6).errHas(t, "connection failure") // the same triad again
	f.run(t, "connections", "ADDR").want(t, 0).has(t, "active:         1")
	if n := len(f.srv.Connections()); n != 1 {
		t.Fatalf("%d connections open", n)
	}
	closeArgs := []string{"forward-close", "ADDR", "--connection-serial", "77", "--originator-serial", "1234", "--config", "151", "--output", "152", "--input", "100"}
	f.run(t, closeArgs...).want(t, 0).has(t, "Forward_Close of connection serial 77 vendor 65535 originator 1234")
	r := f.run(t, append(closeArgs, "--format", "json")...).want(t, 6)
	if kind, code := r.errorCode(t); kind != "refused" || code != "connection_failure" || !strings.Contains(r.stdout, `"extended_status": [`) {
		t.Errorf("forward-close of a closed connection: %s", r.stdout)
	}
	f.run(t, append(closeArgs, "--format", "table", "--route", "1/0")...).want(t, 6)

	for _, args := range [][]string{
		{"forward-open", "ADDR", "--output", "150", "--input", "100"},
		{"forward-open", "ADDR", "--output", "150", "--rpi", "10ms"},
		{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--timeout-multiplier", "8"},
		{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ns"},
		{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--config-data", "010"},
		{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--config-data", "010203"},
		{"forward-open", "ADDR", "--output", "150", "--input", "100", "--rpi", "10ms", "--route", "x"},
		{"forward-open", "-", "--output", "150", "--input", "100", "--rpi", "10ms"},
		{"forward-close", "ADDR", "--output", "150", "--input", "100"},
		{"forward-close", "ADDR", "--connection-serial", "1", "--originator-serial", "1"},
		{"forward-close", "ADDR", "--connection-serial", "1", "--originator-serial", "1", "--output", "1", "--input", "1", "--route", "x"},
		{"forward-close", "-", "--connection-serial", "1", "--originator-serial", "1", "--output", "1", "--input", "1"},
	} {
		f.run(t, args...).want(t, 3)
	}
	// A wrong size is refused by the Connection Manager with its extended status.
	f.run(t, "forward-open", "ADDR", "--config", "151", "--output", "150", "--input", "100", "--output-size", "31", "--input-size", "32", "--rpi", "10ms").
		want(t, 6).errHas(t, "connection failure")
	f.run(t, "forward-open", refusedAddr(t), "--output", "150", "--input", "100", "--rpi", "10ms").want(t, 4)
	f.run(t, "forward-close", refusedAddr(t), "--connection-serial", "1", "--originator-serial", "1", "--output", "1", "--input", "1").want(t, 4)
}

func TestAssembly(t *testing.T) {
	f := startFixture(t)
	f.run(t, "assembly", "write", "ADDR", "150", "--data", strings.Repeat("ab", 32), "--yes").want(t, 0).has(t, "Wrote @4/150/3")
	f.run(t, "assembly", "read", "ADDR", "100").want(t, 0).has(t, "Read @4/100/3", "ab ab ab")
	f.run(t, "assembly", "read", "ADDR", "100", "--connected", "--format", "json").want(t, 0).has(t, `"data": "abab`)
	f.run(t, "assembly", "read", "ADDR", "0").want(t, 3)
	f.run(t, "assembly", "write", "ADDR", "x", "--data", "00", "--yes").want(t, 3)
	f.run(t, "assembly", "write", "ADDR", "150", "--yes").want(t, 3)
	f.run(t, "assembly", "write", "ADDR", "150", "--data", "zz", "--yes").want(t, 3)
	f.run(t, "assembly", "write", "ADDR", "150", "--data", "00", "--yes").want(t, 6).errHas(t, "not enough data")
	f.run(t, "assembly", "read", "ADDR", "999").want(t, 6)
}

func TestDiagnose(t *testing.T) {
	f := startFixture(t, server.WithInterface(server.Interface{HostName: "plc-1", MAC: [6]byte{0, 1, 2, 3, 4, 5}, SpeedMbps: 1000}))
	f.run(t, "diagnose", "ADDR").want(t, 0).has(t, "8 passed, 0 failed, 0 skipped", "host_name=plc-1", "mac=00:01:02:03:04:05", "speed_mbps=1000")
	f.run(t, "diagnose", "ADDR", "--format", "table").want(t, 0).has(t, "CHECK", "connection-manager")

	// A silent listener: TCP works, RegisterSession times out, the rest is skipped.
	r := run(t, "diagnose", silentListener(t), "--timeout", "200ms", "--format", "json").want(t, 7)
	m := r.json(t)
	checks := m["checks"].([]any)
	if m["ok"] != false || checks[0].(map[string]any)["status"] != "ok" || checks[1].(map[string]any)["status"] != "error" ||
		checks[1].(map[string]any)["error"].(map[string]any)["kind"] != "timeout" || checks[2].(map[string]any)["status"] != "skipped" {
		t.Errorf("diagnose of a silent listener: %s", r.stdout)
	}
	run(t, "diagnose", refusedAddr(t), "--timeout", "1s").want(t, 7).has(t, "ERROR   tcp-connect", "SKIPPED register-session")
	run(t, "diagnose", "300.0.0.1").want(t, 3)

	// A device without the TCP/IP Interface and Ethernet Link objects fails those checks.
	f.srv.RegisterObject(server.NewObject(cip.ClassTCPIPInterface, 1))
	f.srv.RegisterObject(server.NewObject(cip.ClassEthernetLink, 1))
	f.srv.RegisterObject(server.NewObject(cip.ClassConnectionManager, 1))
	f.run(t, "diagnose", "ADDR").want(t, 7).has(t, "5 passed, 3 failed", "ERROR   tcpip-interface", "ERROR   ethernet-link", "ERROR   connection-manager")
}

func TestDiscover(t *testing.T) {
	f := startFixture(t)
	if f.srv.UDPAddr() == nil {
		t.Skip("the UDP port of the fixture is taken")
	}
	r := run(t, "discover", "--address", f.addr, "--wait", "300ms", "--local", "127.0.0.1", "--max-delay", "10ms").want(t, 0)
	r.has(t, "1 device(s)", `"test adapter" (vendor 1 Rockwell Automation/Allen-Bradley, Communications Adapter, product 42, rev 3.7, serial 0x12345678)`)
	m := run(t, "discover", "--address", f.addr, "--wait", "300ms", "--max-devices", "1", "--format", "json").want(t, 0).json(t)
	if m["found"] != 1.0 || m["devices"].([]any)[0].(map[string]any)["address"] != f.addr {
		t.Errorf("discover json: %v", m)
	}
	run(t, "discover", "--address", f.addr, "--wait", "300ms", "--format", "table").want(t, 0).has(t, "PRODUCT", "test adapter", "operational")
	for _, args := range [][]string{
		{"discover"},
		{"discover", "--address", "192.168.1.0/24"},
		{"discover", "--address", "192.168.1.1-254"},
		{"discover", "--address", "10.0.0.1", "--local", "x"},
		{"discover", "--address", "10.0.0.1", "--wait", "-1s"},
	} {
		run(t, args...).want(t, 3)
	}
	run(t, "discover", "--address", "127.0.0.1:"+strconv.Itoa(udpFreePort(t)), "--wait", "100ms").want(t, 0).has(t, "0 device(s)")
}

func TestErrorKinds(t *testing.T) {
	// Connection refused: transport, 4.
	r := run(t, "identify", refusedAddr(t), "--format", "json").want(t, 4)
	if kind, code := r.errorCode(t); kind != "transport" || code != "transport_connect_failed" {
		t.Errorf("refused: %s %s", kind, code)
	}
	// A silent listener: RegisterSession times out, 5.
	r = run(t, "read", silentListener(t), "@1/1/7", "--timeout", "200ms", "--format", "json").want(t, 5)
	if kind, code := r.errorCode(t); kind != "timeout" || code != "request_timeout" {
		t.Errorf("silent: %s %s", kind, code)
	}
	run(t, "status", silentListener(t), "--timeout", "200ms", "--debug").want(t, 5).errHas(t, "error: timeout", "cause: ")
	// --debug traces go-ethernetip's log to stderr.
	f := startFixture(t)
	f.run(t, "identify", "ADDR", "--debug").want(t, 0).errHas(t, "level=")
	// An interrupted command is an internal error (10).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runCtx(t, ctx, "identify", silentListener(t)).want(t, 10)
	// A result that cannot be written is an output error (10).
	var stderr syncBuffer
	if code := Run(context.Background(), []string{"identify", f.addr}, failingWriter{}, &stderr); code != 10 {
		t.Errorf("unwritable stdout: exit %d", code)
	}
	if code := Run(context.Background(), []string{"identify", f.addr, "--format", "table"}, failingWriter{}, &stderr); code != 10 {
		t.Errorf("unwritable stdout (table): exit %d", code)
	}
	// The environment gives the format of every command.
	t.Setenv("ETHERNETIPCTL_OUTPUT_FORMAT", "json")
	f.run(t, "status", "ADDR").want(t, 0).json(t)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }
