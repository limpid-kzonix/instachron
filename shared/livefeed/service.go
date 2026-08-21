package livefeed

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	// maxTCPClients caps how many downstream proxies may stream at once.
	maxTCPClients = 64
	// tcpWriteTimeout is how long a downstream client may stall before it is
	// disconnected, so one slow reader cannot hold up the others.
	tcpWriteTimeout = 2 * time.Second
	// shutdownTimeout is how long in-flight HTTP requests get to finish after
	// the process is asked to stop.
	shutdownTimeout = 5 * time.Second
	// livenessInterval is how often cameras are checked for having gone silent.
	livenessInterval = time.Second
)

// ServiceConfig holds the per-service settings of a restream proxy. Everything
// else about the four proxies is identical, which is why Serve can own it.
type ServiceConfig struct {
	Name            string // used in the startup log line
	HTTPAddr        string
	UpstreamTCPAddr string
	TCPAddr         string
	TCPEnabled      bool
}

// Serve runs a complete restream proxy process and blocks until the HTTP server
// stops: it reads frames from the upstream TCP server, hands each one to
// transform, republishes the result to downstream TCP clients and to the HTTP
// camera API, and shuts everything down when ctx is cancelled.
//
// transform is called once per upstream frame and must call push exactly once,
// either synchronously or from another goroutine. push republishes the bytes it
// is given under the original frame's camera ID, timestamp and sequence number.
//
// Serve does not return until the upstream reader has stopped, which means no
// further call to transform can be in flight once it has returned. Callers rely
// on that: a caller whose transform hands frames to a worker pool typically
// writes "defer pool.Close()" around Serve, and closing that pool while the
// reader was still running would be a send on a closed channel.
func Serve(ctx context.Context, cfg ServiceConfig, transform func(streamproto.Frame, func([]byte)), logger *log.Logger) error {
	registry := NewRegistry()
	go registry.RunLiveness(ctx, livenessInterval)

	// broadcaster fans processed frames out to downstream TCP proxy clients
	broadcaster := NewBroadcaster()

	if cfg.TCPEnabled {
		tcpSrv := streamproto.NewTCPServer(streamproto.TCPServerConfig{
			ListenAddr:   cfg.TCPAddr,
			MaxClients:   maxTCPClients,
			WriteTimeout: tcpWriteTimeout,
		}, broadcaster, logger)
		go func() {
			if err := tcpSrv.Run(ctx); err != nil {
				logger.Printf("TCP server error: %v", err)
			}
		}()
	}

	upstream := streamproto.NewTCPUpstream(
		streamproto.TCPUpstreamConfig{Addr: cfg.UpstreamTCPAddr},
		func(f streamproto.Frame) {
			id := f.CameraID.String()
			transform(f, func(out []byte) {
				broadcaster.Publish(streamproto.Frame{
					CameraID:  f.CameraID,
					Timestamp: f.Timestamp,
					Sequence:  f.Sequence,
					Payload:   out,
				})
				registry.Push(id, out)
			})
		},
		registry.MarkAllOffline,
		logger,
	)
	// upstreamDone lets Serve wait for the reader below. Without this join the
	// reader could still be inside transform when Serve returns, and the caller
	// would tear down whatever transform writes to underneath it.
	var upstreamDone sync.WaitGroup
	upstreamDone.Add(1)
	go func() {
		defer upstreamDone.Done()
		upstream.Run(ctx)
	}()
	defer upstreamDone.Wait()

	httpSrv := &http.Server{
		Addr:        cfg.HTTPAddr,
		Handler:     NewCameraAPI(registry, logger, APIOptions{}),
		ReadTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logger.Printf("HTTP shutdown error: %v", err)
		}
	}()

	logger.Printf("%s listening on %s  upstream=%s  tcp=%s (enabled=%v)",
		cfg.Name, cfg.HTTPAddr, cfg.UpstreamTCPAddr, cfg.TCPAddr, cfg.TCPEnabled)
	// ErrServerClosed is the expected result of the shutdown above, not a failure.
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server failed: %w", err)
	}
	return nil
}
