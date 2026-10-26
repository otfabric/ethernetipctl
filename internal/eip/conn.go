package eip

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	ethernetip "github.com/otfabric/go-ethernetip"
	"github.com/otfabric/go-ethernetip/cip"
	"github.com/otfabric/go-ethernetip/client"

	"github.com/otfabric/ethernetipctl/internal/config"
	"github.com/otfabric/ethernetipctl/internal/errs"
)

// Env is what every operation needs besides its own flags: the global flags and where the
// debug trace goes.
type Env struct {
	Global *config.Global
	// Stderr receives the --debug trace and progress messages.
	Stderr io.Writer
}

// Timeout returns the timeout of connecting and of each request.
func (e Env) Timeout() time.Duration {
	if e.Global == nil || e.Global.Timeout <= 0 {
		return config.DefaultTimeout
	}
	return e.Global.Timeout
}

// Port returns the default port.
func (e Env) Port() uint16 {
	if e.Global == nil || e.Global.Port == 0 {
		return config.DefaultPort
	}
	return e.Global.Port
}

// Logger returns the logger of go-ethernetip: a debug trace on stderr with --debug,
// silence otherwise.
func (e Env) Logger() ethernetip.Logger {
	if e.Global == nil || !e.Global.Debug || e.Stderr == nil {
		return ethernetip.NopLogger()
	}
	return ethernetip.NewSlogLogger(slog.NewTextHandler(e.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// ParseTarget parses the host argument with the default port.
func (e Env) ParseTarget(host string) (config.Target, error) {
	t, err := config.ParseHost(host, e.Port())
	if err != nil {
		return config.Target{}, errs.InvalidInput(errs.CodeInvalidAddress, err.Error(), err)
	}
	return t, nil
}

// ParseRoute parses --route; an empty route is none.
func ParseRoute(s string) (cip.Path, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	p, err := cip.ParseRoute(s)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidPath, "invalid --route "+strings.TrimSpace(s)+": "+err.Error(), err)
	}
	return p, nil
}

// dial connects a client to t: dial, RegisterSession.
func (e Env) dial(ctx context.Context, t config.Target, route cip.Path, extra ...client.Option) (*client.Client, error) {
	opts := []client.Option{client.WithTimeout(e.Timeout()), client.WithLogger(e.Logger())}
	if len(route) > 0 {
		opts = append(opts, client.WithRoute(route))
	}
	opts = append(opts, extra...)
	c, err := client.New(t.Addr(), opts...)
	if err != nil {
		return nil, WrapError(err)
	}
	if err := c.Connect(ctx); err != nil {
		return nil, WrapError(err)
	}
	return c, nil
}

// Messenger is what read, write and the object commands send requests through: a client,
// or a Class 3 connection of it.
type Messenger interface {
	GetAttributeSingle(ctx context.Context, class cip.Class, instance uint32, attribute uint16) ([]byte, error)
	GetAttributesAll(ctx context.Context, class cip.Class, instance uint32) ([]byte, error)
	SetAttributeSingle(ctx context.Context, class cip.Class, instance uint32, attribute uint16, value []byte) error
	Do(ctx context.Context, req cip.Request) (cip.Response, error)
}

// Session is a connected client and, with --connected, its Class 3 connection.
type Session struct {
	Target config.Target
	Client *client.Client
	Conn   *client.Conn
	Route  cip.Path
	env    Env
}

// Messenger returns the connection when there is one, the client otherwise.
func (s *Session) Messenger() Messenger {
	if s.Conn != nil {
		return s.Conn
	}
	return s.Client
}

// Doer returns the connection when there is one, the client otherwise.
func (s *Session) Doer() client.Doer { return s.Messenger() }

// Close closes the connection and the session.
func (s *Session) Close() {
	if s.Conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.env.Timeout())
		_ = s.Conn.Close(ctx)
		cancel()
	}
	_ = s.Client.Close()
}

// Open parses host and route and opens a session, with a Class 3 connection when connected.
func (e Env) Open(ctx context.Context, host string, m config.MessagingConfig) (*Session, error) {
	t, err := e.ParseTarget(host)
	if err != nil {
		return nil, err
	}
	route, err := ParseRoute(m.Route)
	if err != nil {
		return nil, err
	}
	return e.openTarget(ctx, t, route, m.Connected, client.ConnectionOptions{})
}

// openTarget opens a session with t.
func (e Env) openTarget(ctx context.Context, t config.Target, route cip.Path, connected bool, co client.ConnectionOptions) (*Session, error) {
	c, err := e.dial(ctx, t, route)
	if err != nil {
		return nil, err
	}
	s := &Session{Target: t, Client: c, Route: route, env: e}
	if connected {
		cn, err := c.OpenConnection(ctx, co)
		if err != nil {
			_ = c.Close()
			return nil, WrapError(err)
		}
		s.Conn = cn
	}
	return s, nil
}

// ms converts a duration to milliseconds with fractions.
func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// InvalidTimeout is the error of a negative --timeout.
func InvalidTimeout(d time.Duration) error {
	return invalid(errs.CodeInvalidValue, "invalid --timeout %v: it must be positive (0 = 5s)", d)
}
