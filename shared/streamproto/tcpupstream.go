package streamproto

import (
	"context"
	"log"
	"net"
	"time"
)

// TCPUpstreamConfig controls connection and retry behaviour.
type TCPUpstreamConfig struct {
	Addr       string
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

// TCPUpstream connects to an upstream TCP frame server, reads frames in this
// package's wire format, and calls onFrame synchronously for each one.
// Reconnection uses exponential backoff. onOffline fires whenever the
// connection drops; it may be nil.
type TCPUpstream struct {
	cfg       TCPUpstreamConfig
	onFrame   func(Frame)
	onOffline func()
	logger    *log.Logger
}

// NewTCPUpstream creates a TCPUpstream.
// onFrame is called in the read loop and must not block for extended periods;
// use a goroutine internally if processing is heavy.
func NewTCPUpstream(
	cfg TCPUpstreamConfig,
	onFrame func(Frame),
	onOffline func(),
	logger *log.Logger,
) *TCPUpstream {
	if cfg.MinBackoff == 0 {
		cfg.MinBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	return &TCPUpstream{cfg: cfg, onFrame: onFrame, onOffline: onOffline, logger: logger}
}

// Run connects and reads frames until ctx is cancelled, reconnecting with
// exponential backoff on every disconnect.
func (u *TCPUpstream) Run(ctx context.Context) {
	backoff := u.cfg.MinBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		frames, err := u.connect(ctx)

		if ctx.Err() != nil {
			return
		}
		if u.onOffline != nil {
			u.onOffline()
		}

		if frames > 0 {
			backoff = u.cfg.MinBackoff
		} else {
			backoff = min(backoff*2, u.cfg.MaxBackoff)
		}
		if err != nil {
			u.logger.Printf("TCP upstream %s: %v (retry in %s)", u.cfg.Addr, err, backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (u *TCPUpstream) connect(ctx context.Context) (int, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", u.cfg.Addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	// The read loop below blocks in ReadFrame with no way to observe ctx, so a
	// watchdog goroutine closes the connection to unblock it. connCtx is
	// cancelled when connect returns, which stops the watchdog from outliving
	// the connection it belongs to — without it every reconnect would strand a
	// goroutine waiting on the process-lifetime context forever.
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-connCtx.Done()
		_ = conn.Close()
	}()

	u.logger.Printf("TCP upstream connected to %s", u.cfg.Addr)

	r := NewReader(conn)
	frames := 0
	for {
		f, err := r.ReadFrame()
		if err != nil {
			if ctx.Err() != nil {
				return frames, nil
			}
			return frames, err
		}
		if u.onFrame != nil {
			u.onFrame(f)
		}
		frames++
	}
}
