// Package ipcclient connects to the tcp-camera-backend IPC Unix socket and
// dispatches incoming messages to caller-provided handlers.
//
// The connection lifecycle — dialling, reading, reconnecting — lives in
// shared/frameipc so that this service and ffmpeg-streamer cannot drift apart
// on reconnect policy. What is left here is the name this service uses for it.
package ipcclient

import (
	"context"
	"log"

	"github.com/w0rxbend/instachron/shared/frameipc"
)

// Handler receives decoded IPC messages. See frameipc.Handler for the
// individual callbacks.
type Handler = frameipc.Handler

// Reader connects to the IPC socket and dispatches messages until ctx is done.
// It reconnects automatically with a fixed delay on every disconnect.
type Reader struct {
	client *frameipc.Client
}

// New returns a Reader that dispatches to handler.
func New(socketPath string, handler Handler, logger *log.Logger) *Reader {
	return &Reader{client: frameipc.NewClient(socketPath, handler, logger)}
}

// Run is the reconnect loop. It blocks until ctx is cancelled.
func (r *Reader) Run(ctx context.Context) {
	r.client.Run(ctx)
}
