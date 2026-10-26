package errs

// Kind is a coarse failure category for exit-code mapping and JSON error.kind.
type Kind string

const (
	KindUsage        Kind = "usage"
	KindInvalidInput Kind = "invalid_input"
	KindTransport    Kind = "transport"
	KindTimeout      Kind = "timeout"
	KindProtocol     Kind = "protocol"
	// KindRefused is a request the target answered and refused: a CIP general status
	// other than success, a refused Forward_Open, an unsupported command.
	KindRefused  Kind = "refused"
	KindOutput   Kind = "output"
	KindInternal Kind = "internal"
)
