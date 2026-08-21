package frameipc

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// reconnDelay is how long the client waits after losing (or failing to make)
// a connection before dialling the socket again.
const reconnDelay = time.Second

// Handler holds the callbacks a Client invokes as messages arrive. Every field
// is optional: a nil callback means the corresponding event is ignored.
//
// All callbacks run on the Client's own goroutine, one at a time, so a slow
// callback stalls reading from the socket. Handlers that share state with other
// goroutines are responsible for their own locking.
type Handler struct {
	// OnFrame is called for each received JPEG frame. The jpeg slice is owned
	// by the callback once it is called; the Client does not reuse it.
	OnFrame func(cameraID streamproto.CameraID, jpeg []byte)
	// OnOffline is called when the backend signals a camera went offline.
	OnOffline func(cameraID streamproto.CameraID)
	// OnDisconnect is called whenever a connection attempt ends — both when an
	// established connection drops and when the dial itself fails — before the
	// reconnect delay. Use it to mark all cameras offline or drop cached frames.
	OnDisconnect func()
}

// Client connects to the tcp-camera-backend IPC Unix socket, decodes the
// messages arriving on it and dispatches them to a Handler. It reconnects
// automatically with a fixed delay after every disconnect.
type Client struct {
	socketPath string
	handler    Handler
	logger     *log.Logger

	// dial opens a new connection to the backend. It is a field rather than a
	// direct net.Dial call so tests can substitute an in-memory pipe instead of
	// creating a real socket on disk.
	dial func(ctx context.Context) (net.Conn, error)
}

// NewClient returns a Client that dials socketPath and dispatches to h.
// Nothing happens until Run is called.
func NewClient(socketPath string, h Handler, logger *log.Logger) *Client {
	return &Client{
		socketPath: socketPath,
		handler:    h,
		logger:     logger,
		dial: func(ctx context.Context) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
}

// Run is the reconnect loop: connect, read messages until the connection
// breaks, wait, repeat. It blocks until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := c.connect(ctx); err != nil && ctx.Err() == nil {
			c.logger.Printf("IPC disconnected (%v), retrying in %s", err, reconnDelay)
		}

		if c.handler.OnDisconnect != nil {
			c.handler.OnDisconnect()
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnDelay):
		}
	}
}

// connect makes one connection attempt and reads from it until the socket
// fails or ctx is cancelled. A nil return means the context was cancelled
// rather than the connection breaking on its own.
func (c *Client) connect(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.socketPath, err)
	}
	defer func() { _ = conn.Close() }()

	// connCtx is cancelled when this function returns, which stops the watchdog
	// goroutine below. Without the per-connection context the watchdog would
	// wait on the process-wide ctx and so outlive its connection, leaving one
	// parked goroutine behind for every reconnect attempt.
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.logger.Printf("IPC connected to %s", c.socketPath)

	// Read below blocks in a syscall and does not watch the context itself, so
	// closing the connection is what unblocks it on shutdown.
	go func() {
		<-connCtx.Done()
		_ = conn.Close()
	}()

	for {
		msg, err := Read(conn)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		switch msg.Kind {
		case KindFrame:
			if c.handler.OnFrame != nil {
				c.handler.OnFrame(msg.CameraID, msg.Payload)
			}
		case KindOffline:
			if c.handler.OnOffline != nil {
				c.handler.OnOffline(msg.CameraID)
			}
		default:
			c.logger.Printf("IPC: unknown message kind %s for camera=%d, skipping", msg.Kind, msg.CameraID)
		}
	}
}
