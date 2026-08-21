package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"

	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/protocol"
)

// Accept can fail for two very different reasons, and the retry policy below
// tells them apart. A per-connection problem (the peer vanished between the
// handshake and the accept, for instance) affects only that one connection: the
// next Accept succeeds, so retrying immediately is right. A process-wide
// problem such as running out of file descriptors makes every Accept fail
// instantly, and retrying immediately turns the loop into a hot spin that burns
// a whole CPU core and floods the log with the same line thousands of times a
// second. Backing off gives the condition a chance to clear, and giving up
// after a while turns a silent spin into a process exit an operator can see.
const (
	// minAcceptBackoff is the pause after the first failure in a run.
	minAcceptBackoff = 5 * time.Millisecond
	// maxAcceptBackoff caps the pause, so recovery stays prompt once the
	// underlying condition clears.
	maxAcceptBackoff = time.Second
	// maxConsecutiveAcceptFailures is how many failures in a row are tolerated
	// before ListenAndServe reports the last error instead of retrying. With
	// the backoff above this is a little over a minute of continuous failure,
	// which no transient per-connection fault survives.
	maxConsecutiveAcceptFailures = 64
)

type Publisher interface {
	Publish(cameraID streamproto.CameraID, jpeg []byte)
	PublishOffline(cameraID streamproto.CameraID)
}

type Config struct {
	Addr          string
	MaxFrameBytes uint32
	ReadTimeout   time.Duration
	Publisher     Publisher
	Logger        *log.Logger
}

type Server struct {
	addr          string
	maxFrameBytes uint32
	readTimeout   time.Duration
	publisher     Publisher
	logger        *log.Logger
	mu            sync.Mutex
	conns         map[net.Conn]struct{}
}

func New(cfg Config) *Server {
	return &Server{
		addr:          cfg.Addr,
		maxFrameBytes: cfg.MaxFrameBytes,
		readTimeout:   cfg.ReadTimeout,
		publisher:     cfg.Publisher,
		logger:        cfg.Logger,
		conns:         make(map[net.Conn]struct{}),
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	defer func() { _ = listener.Close() }()

	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		s.mu.Lock()
		for c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
	}()

	s.logger.Printf("TCP frame server listening on %s", listener.Addr())

	backoff := minAcceptBackoff
	failures := 0

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Accept fails once the goroutine above closes the listener, which
			// happens only when ctx is done. A shutdown asked for by the caller
			// is a success, so nothing is reported back.
			if ctx.Err() != nil {
				wg.Wait()
				return nil
			}

			failures++
			if failures >= maxConsecutiveAcceptFailures {
				wg.Wait()
				return fmt.Errorf("accept failed %d times in a row, giving up: %w", failures, err)
			}
			s.logger.Printf("accept failed (%d in a row, retrying in %s): %v", failures, backoff, err)

			// Wait out the backoff, but abandon it immediately if the caller
			// asks to shut down in the meantime.
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				wg.Wait()
				return nil
			case <-timer.C:
			}
			backoff = min(backoff*2, maxAcceptBackoff)
			continue
		}

		// A successful accept means the failure run, if any, is over.
		failures = 0
		backoff = minAcceptBackoff

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		wg.Add(1)
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
				wg.Done()
			}()
			s.handleConnection(conn)
		}()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	remoteAddr := conn.RemoteAddr().String()
	s.logger.Printf("client connected: %s", remoteAddr)

	seenCameraIDs := make(map[streamproto.CameraID]struct{})
	defer func() {
		for id := range seenCameraIDs {
			s.logger.Printf("camera offline: camera=%d addr=%s", id, remoteAddr)
			if s.publisher != nil {
				s.publisher.PublishOffline(id)
			}
		}
		s.logger.Printf("client disconnected: %s", remoteAddr)
	}()

	stats := newFrameStats(s.logger, remoteAddr, frameStatsInterval)
	defer stats.Stop()
	go stats.Run()

	for {
		header, payload, err := s.readFrame(conn)
		if err != nil {
			// A client that goes away shows up as io.EOF between frames, or as
			// io.ErrUnexpectedEOF when it vanished part way through one. Both
			// are ordinary disconnects rather than faults, so they stay quiet.
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				s.logger.Printf("read frame from %s failed: %v", remoteAddr, err)
			}
			return
		}

		if !protocol.LooksLikeJPEG(payload) {
			s.logger.Printf("dropping non-JPEG payload from %s: camera=%d seq=%d size=%d",
				remoteAddr, header.CameraID, header.Sequence, header.PayloadSize)
			continue
		}

		seenCameraIDs[header.CameraID] = struct{}{}

		if s.publisher != nil {
			s.publisher.Publish(header.CameraID, payload)
		}

		stats.Record(header.CameraID)
	}
}

// readFrame reads the next whole frame from conn: its header first, then the
// payload the header announced. Every failure here ends the connection, because
// a stream whose framing has gone wrong cannot be resynchronised.
func (s *Server) readFrame(conn net.Conn) (protocol.Header, []byte, error) {
	if s.readTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	}

	// protocol.ReadHeader already describes which part of the header failed, so
	// its error travels up unchanged.
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		return protocol.Header{}, nil, err
	}

	// An empty frame carries nothing, and an outsized one would let a client
	// dictate an arbitrarily large allocation, so both are refused.
	if header.PayloadSize == 0 || header.PayloadSize > s.maxFrameBytes {
		return header, nil, fmt.Errorf("invalid payload size: camera=%d seq=%d size=%d max=%d",
			header.CameraID, header.Sequence, header.PayloadSize, s.maxFrameBytes)
	}

	payload := make([]byte, int(header.PayloadSize))
	if _, err := io.ReadFull(conn, payload); err != nil {
		return header, nil, fmt.Errorf("read payload: camera=%d seq=%d size=%d: %w",
			header.CameraID, header.Sequence, header.PayloadSize, err)
	}

	return header, payload, nil
}
