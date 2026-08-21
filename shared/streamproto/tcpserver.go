package streamproto

import (
	"context"
	"log"
	"net"
	"sync/atomic"
	"time"
)

// TCPServerConfig holds parameters for a TCP frame server.
type TCPServerConfig struct {
	ListenAddr   string
	MaxClients   int
	WriteTimeout time.Duration
}

// FrameSource is where a TCPServer gets the frames it streams to its clients.
//
// Subscribe returns a channel of frames and a function that must be called
// exactly once to unsubscribe. It is an interface rather than the concrete
// fan-out type so that this package, which owns the wire format, does not have
// to depend on the package that owns the camera model — that dependency only
// makes sense in the other direction.
type FrameSource interface {
	Subscribe() (<-chan Frame, func())
}

// TCPServer accepts downstream proxy connections and streams frames to them in
// this package's wire format. Frames come from a FrameSource; a slow client
// that cannot accept a frame within WriteTimeout is disconnected, so that one
// stalled reader cannot hold up the others.
type TCPServer struct {
	cfg     TCPServerConfig
	source  FrameSource
	logger  *log.Logger
	clients atomic.Int64
}

// NewTCPServer creates a TCPServer that streams the frames published by source.
func NewTCPServer(cfg TCPServerConfig, source FrameSource, logger *log.Logger) *TCPServer {
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 2 * time.Second
	}
	return &TCPServer{cfg: cfg, source: source, logger: logger}
}

// Run binds the listener and accepts clients until ctx is cancelled.
func (s *TCPServer) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	s.logger.Printf("TCP stream server listening on %s", s.cfg.ListenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if s.cfg.MaxClients > 0 && int(s.clients.Load()) >= s.cfg.MaxClients {
			s.logger.Printf("TCP: max clients (%d) reached, rejecting %s",
				s.cfg.MaxClients, conn.RemoteAddr())
			_ = conn.Close()
			continue
		}

		s.clients.Add(1)
		go func() {
			defer func() {
				_ = conn.Close()
				s.clients.Add(-1)
			}()
			s.serve(ctx, conn)
		}()
	}
}

func (s *TCPServer) serve(ctx context.Context, conn net.Conn) {
	frames, unsub := s.source.Subscribe()
	defer unsub()

	w := NewWriter(conn)
	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			// The write deadline is what disconnects a client that stopped
			// reading. If it cannot be set, the write below could block
			// forever, so give up on this connection instead: returning runs
			// the deferred close and client-count decrement set up in Run.
			if err := conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout)); err != nil {
				return
			}
			if err := w.WriteFrame(f); err != nil {
				return
			}
		}
	}
}

// ClientCount returns the current number of connected clients.
func (s *TCPServer) ClientCount() int64 { return s.clients.Load() }
