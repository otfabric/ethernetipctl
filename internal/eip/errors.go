// Package eip runs the operations of ethernetipctl with go-ethernetip and turns their
// outcome into the result types of package types, and their failures into typed errors.
package eip

import (
	"context"
	"errors"
	"fmt"
	"net"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/logix"

	"github.com/otfabric/ethernetipctl/internal/errs"
	"github.com/otfabric/ethernetipctl/internal/types"
)

// statusCodes are the error codes of the CIP general statuses that have their own.
var statusCodes = map[cip.GeneralStatus]string{
	cip.StatusConnectionFailure:      errs.CodeConnectionFailure,
	cip.StatusResourceUnavailable:    errs.CodeResourceUnavailable,
	cip.StatusPathSegmentError:       errs.CodePathSegmentError,
	cip.StatusPathDestinationUnknown: errs.CodePathUnknown,
	cip.StatusServiceNotSupported:    errs.CodeServiceNotSupported,
	cip.StatusInvalidAttributeValue:  errs.CodeInvalidAttributeValue,
	cip.StatusAttributeNotSettable:   errs.CodeAttributeNotSettable,
	cip.StatusPrivilegeViolation:     errs.CodePrivilegeViolation,
	cip.StatusObjectStateConflict:    errs.CodeStateConflict,
	cip.StatusDeviceStateConflict:    errs.CodeStateConflict,
	cip.StatusNotEnoughData:          errs.CodeNotEnoughData,
	cip.StatusAttributeNotSupported:  errs.CodeAttributeNotSupported,
	cip.StatusTooMuchData:            errs.CodeTooMuchData,
}

// WrapError classifies an error of go-ethernetip as a typed CLI error with a stable kind
// and code. A typed error is returned as it is.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	var (
		cipErr   *ethernetip.CIPError
		encapErr *ethernetip.EncapError
	)
	switch {
	case errors.As(err, &cipErr):
		code, ok := statusCodes[cipErr.Status]
		if !ok {
			code = errs.CodeCIPError
		}
		e := errs.New(errs.KindRefused, code, fmt.Sprintf("the target refused the request: %s", trimPrefix(cipErr.Error())), err)
		e.CIP = &errs.CIPStatus{General: uint8(cipErr.Status), Extended: cipErr.Extended}
		return e
	case errors.As(err, &encapErr):
		if errors.Is(err, ethernetip.ErrUnsupported) {
			return errs.New(errs.KindRefused, errs.CodeUnsupported, fmt.Sprintf("the target does not support the command: %s", trimPrefix(encapErr.Error())), err)
		}
		return errs.New(errs.KindProtocol, errs.CodeSessionRefused, fmt.Sprintf("the target refused the session: %s", trimPrefix(encapErr.Error())), err)
	case errors.Is(err, context.Canceled):
		return errs.New(errs.KindInternal, errs.CodeContextCanceled, "interrupted", err)
	case errors.Is(err, ethernetip.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return errs.New(errs.KindTimeout, errs.CodeRequestTimeout, fmt.Sprintf("timeout: no answer from the target: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrConnectFailed):
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, fmt.Sprintf("connection error: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrConnectionLost), errors.Is(err, ethernetip.ErrNotConnected), errors.Is(err, ethernetip.ErrClosed):
		return errs.New(errs.KindTransport, errs.CodeConnectionLost, fmt.Sprintf("connection error: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrSession):
		return errs.New(errs.KindProtocol, errs.CodeSessionRefused, fmt.Sprintf("session error: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrProtocol), errors.Is(err, cip.ErrMalformed), errors.Is(err, cip.ErrTruncated):
		return errs.New(errs.KindProtocol, errs.CodeProtocolViolation, fmt.Sprintf("protocol error: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrBusy):
		return errs.New(errs.KindProtocol, errs.CodeRequestBusy, fmt.Sprintf("protocol error: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrUnsupported):
		return errs.New(errs.KindRefused, errs.CodeUnsupported, fmt.Sprintf("unsupported: %s", trimPrefix(err.Error())), err)
	case errors.Is(err, ethernetip.ErrInvalidOption), errors.Is(err, logix.ErrTagName), errors.Is(err, logix.ErrType), errors.Is(err, cip.ErrTooLarge):
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("invalid input: %s", trimPrefix(err.Error())), err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return errs.New(errs.KindTransport, errs.CodeConnectionLost, fmt.Sprintf("connection error: %v", err), err)
	}
	return errs.Internal(err)
}

// trimPrefix drops the "ethernetip: " prefix the library's errors start with.
func trimPrefix(s string) string {
	const p = "ethernetip: "
	if len(s) > len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return s
}

// ErrorInfo renders err as the error object of a row or check in a result.
func ErrorInfo(err error) *types.ErrorInfo {
	if err == nil {
		return nil
	}
	e, _ := errs.As(WrapError(err))
	info := &types.ErrorInfo{Kind: string(e.Kind), Code: e.Code, Message: e.Error()}
	if e.CIP != nil {
		st := e.CIP.General
		info.CIPStatus = &st
		info.ExtendedStatus = e.CIP.Extended
	}
	return info
}

// invalid returns an invalid input error with the code given.
func invalid(code, format string, args ...any) error {
	return errs.InvalidInput(code, fmt.Sprintf(format, args...), nil)
}
