package eip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/encap"
	"github.com/otfabric/go-ethernetip/logix"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestWrapError(t *testing.T) {
	if WrapError(nil) != nil || ErrorInfo(nil) != nil {
		t.Fatal("nil is not an error")
	}
	typed := errs.InvalidInput(errs.CodeInvalidInput, "x", nil)
	if WrapError(typed) != typed {
		t.Error("a typed error is not passed through")
	}
	for _, c := range []struct {
		err        error
		kind, code string
	}{
		{&ethernetip.CIPError{Service: cip.ServiceGetAttributeSingle, Status: cip.StatusAttributeNotSupported}, "refused", errs.CodeAttributeNotSupported},
		{&ethernetip.CIPError{Service: cip.ServiceGetAttributeSingle, Status: cip.StatusEmbeddedServiceError}, "refused", errs.CodeCIPError},
		{fmt.Errorf("x: %w", &ethernetip.EncapError{Command: encap.CommandSendRRData, Status: encap.StatusInvalidSessionHandle}), "protocol", errs.CodeSessionRefused},
		{&ethernetip.EncapError{Command: encap.CommandListInterfaces, Status: encap.StatusInvalidCommand}, "unsupported", errs.CodeUnsupported},
		{context.Canceled, "interrupted", errs.CodeContextCanceled},
		{context.DeadlineExceeded, "timeout", errs.CodeRequestTimeout},
		{ethernetip.ErrTimeout, "timeout", errs.CodeRequestTimeout},
		{ethernetip.ErrConnectFailed, "transport", errs.CodeTransportConnectFailed},
		{ethernetip.ErrConnectionLost, "transport", errs.CodeConnectionLost},
		{ethernetip.ErrNotConnected, "transport", errs.CodeConnectionLost},
		{ethernetip.ErrClosed, "transport", errs.CodeConnectionLost},
		{ethernetip.ErrSession, "protocol", errs.CodeSessionRefused},
		{ethernetip.ErrProtocol, "protocol", errs.CodeProtocolViolation},
		{cip.ErrTruncated, "protocol", errs.CodeProtocolViolation},
		{ethernetip.ErrBusy, "protocol", errs.CodeRequestBusy},
		{ethernetip.ErrUnsupported, "unsupported", errs.CodeUnsupported},
		{ethernetip.ErrInvalidOption, "invalid_input", errs.CodeInvalidInput},
		{logix.ErrType, "invalid_input", errs.CodeInvalidInput},
		{&net.OpError{Op: "read", Err: timeoutErr{}}, "transport", errs.CodeConnectionLost},
		{errors.New("strange"), "internal", errs.CodeInternalUnexpected},
	} {
		e, ok := errs.As(WrapError(c.err))
		if !ok || string(e.Kind) != c.kind || e.Code != c.code {
			t.Errorf("WrapError(%v) = %+v, want %s %s", c.err, e, c.kind, c.code)
		}
	}
	info := ErrorInfo(&ethernetip.CIPError{Status: cip.StatusConnectionFailure, Extended: []uint16{0x0107}})
	if info.CIPStatus == nil || *info.CIPStatus != 1 || info.ExtendedStatus[0] != 0x0107 {
		t.Errorf("ErrorInfo: %+v", info)
	}
	if trimPrefix("x") != "x" {
		t.Error("trimPrefix")
	}
}

func TestValues(t *testing.T) {
	if _, err := ParseType("float"); err == nil {
		t.Error("FLOAT is not a CIP type")
	}
	if _, err := Decode(cip.TypeDINT, nil); err == nil {
		t.Error("no data decoded")
	}
	if b, err := ParseHex("0x0A:0b-0c 0d"); err != nil || Hex(b) != "0a0b0c0d" {
		t.Errorf("ParseHex: %x %v", b, err)
	}
	if v, _ := Decode(cip.TypeREAL, cip.AppendFloat32(nil, float32(1.1))); v != 1.1 {
		t.Errorf("REAL 1.1 = %v", v)
	}
	for v, want := range map[any]string{nil: "", 1.5: "1.5", "a": `"a"`, int64(-1): "-1", true: "true"} {
		if got := ValueText(v); got != want {
			t.Errorf("ValueText(%v) = %q", v, got)
		}
	}
	if ValueText([]any{int64(1), "b"}) != `[1 "b"]` {
		t.Error("ValueText of an array")
	}
	if _, err := encodeOne(nil, cip.TypeSTRING, "x"); err == nil {
		t.Error("encodeOne of a string")
	}
	if _, err := Encode(cip.TypeSTRING2, "\U0001F600"); err == nil {
		t.Error("STRING2 outside the BMP")
	}
	if StateName(9) != "state-9" || StateName(0xFF) != "default" {
		t.Error("StateName")
	}
	for in, want := range map[string]cip.Class{"identity": 1, "0xF5": 0xF5, "TCP/IP Interface": 0xF5, "ethernet-link": 0xF6, "symbol": 0x6B, "245": 0xF5} {
		if c, err := ParseClass(in); err != nil || c != want {
			t.Errorf("ParseClass(%q) = %v, %v", in, c, err)
		}
	}
	if _, err := ParseClass("!!"); err == nil {
		t.Error("ParseClass of punctuation")
	}
	if ipText(0xC0A8010A) != "192.168.1.10" {
		t.Error("ipText")
	}
	if uintValue([]byte{1, 2, 3}) != nil {
		t.Error("uintValue of 3 octets")
	}
}

func TestEnv(t *testing.T) {
	var e Env
	if e.Timeout() != config.DefaultTimeout || e.Port() != config.DefaultPort {
		t.Error("defaults without global flags")
	}
	g := config.NewGlobal()
	g.Debug = true
	e = Env{Global: g, Stderr: io.Discard}
	if _, ok := e.Logger().(ethernetip.FieldLogger); !ok {
		t.Error("--debug does not log")
	}
	if InvalidTimeout(-time.Second) == nil {
		t.Error("InvalidTimeout")
	}
	if !strings.Contains(InvalidTimeout(-time.Second).Error(), "-1s") {
		t.Error("InvalidTimeout message")
	}
	if fatal(nil) || !fatal(ethernetip.ErrConnectionLost) || fatal(&ethernetip.CIPError{Status: cip.StatusPathDestinationUnknown}) {
		t.Error("fatal")
	}
}
