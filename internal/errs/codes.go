// SPDX-License-Identifier: MIT

package errs

// Stable machine-oriented codes for JSON error.code and scripts.
const (
	CodeInvalidFormat          = "invalid_format"
	CodeInvalidInput           = "invalid_input"
	CodeInvalidAddress         = "invalid_address"
	CodeInvalidPath            = "invalid_path"
	CodeInvalidValue           = "invalid_value"
	CodeInvalidFlagCombination = "invalid_flag_combination"
	CodeInvalidConfig          = "invalid_config"
	// A command that changes the target was given without --yes: nothing was sent.
	CodeConfirmationRequired = "confirmation_required"
	// An EDS file has errors (eds validate) or cannot be parsed.
	CodeInvalidEDS = "invalid_eds"

	CodeTransportConnectFailed = "transport_connect_failed"
	CodeConnectionLost         = "connection_lost"
	CodeListenFailed           = "listen_failed"

	// The target did not answer a request, RegisterSession or I/O data in time.
	CodeRequestTimeout = "request_timeout"

	CodeProtocolViolation = "protocol_violation"
	CodeRequestBusy       = "request_busy"
	// The target refused the encapsulation command or the session (encapsulation status).
	CodeSessionRefused = "session_refused"

	// The target answered with a CIP general status other than success.
	CodeServiceNotSupported   = "service_not_supported"
	CodeAttributeNotSupported = "attribute_not_supported"
	CodePathUnknown           = "path_destination_unknown"
	CodePathSegmentError      = "path_segment_error"
	CodeAttributeNotSettable  = "attribute_not_settable"
	CodeInvalidAttributeValue = "invalid_attribute_value"
	CodeNotEnoughData         = "not_enough_data"
	CodeTooMuchData           = "too_much_data"
	CodeConnectionFailure     = "connection_failure"
	CodeResourceUnavailable   = "resource_unavailable"
	CodePrivilegeViolation    = "privilege_violation"
	CodeStateConflict         = "state_conflict"
	CodeCIPError              = "cip_error"
	// The target does not support an encapsulation command or the operation.
	CodeUnsupported = "unsupported"

	// Context was canceled (not invalid user input).
	CodeContextCanceled = "context_canceled"

	CodeJSONEncodeFailed       = "json_encode_failed"
	CodeTableRenderFailed      = "table_render_failed"
	CodeOutputWriteFailed      = "output_write_failed"
	CodeOutputFileWriteFailed  = "output_file_write_failed"
	CodeOutputFileCreateFailed = "output_file_create_failed"
	CodeInputFileOpenFailed    = "input_file_open_failed"

	CodeInternalUnexpected = "internal_unexpected"
)
