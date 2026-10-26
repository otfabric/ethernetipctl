package types

// ErrorInfo is the structured error object for JSON output and embedded per-row failures.
type ErrorInfo struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// CIPStatus and ExtendedStatus are the CIP general status and additional status words
	// the target answered with, for a refused request (kind "refused"); omitted otherwise.
	CIPStatus      *uint8   `json:"cip_status,omitempty"`
	ExtendedStatus []uint16 `json:"extended_status,omitempty"`
}

// ErrorEnvelope is the top-level JSON shape for fatal errors (--format json).
type ErrorEnvelope struct {
	Error ErrorInfo `json:"error"`
}

// ErrorMessage returns the display/API message for an embedded error pointer.
func ErrorMessage(e *ErrorInfo) string {
	if e == nil {
		return ""
	}
	return e.Message
}
