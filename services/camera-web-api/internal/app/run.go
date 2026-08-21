package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/w0rxbend/instachron/services/camera-web-api/internal/ipcclient"
	"github.com/w0rxbend/instachron/services/camera-web-api/internal/rotation"
	"github.com/w0rxbend/instachron/shared/envconf"
	"github.com/w0rxbend/instachron/shared/livefeed"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	defaultAddr         = ":8080"
	defaultSocketPath   = "/tmp/instachron/frames.sock"
	defaultCameraConfig = "./cameras.json"
	defaultTCPAddr      = ":9001"
	defaultMaxClients   = 64
)

// Run serves the camera web API until ctx is cancelled. Frames arrive over a
// local IPC socket from the capture process, are rotated if the camera is
// configured that way, and are then published to both HTTP clients and
// downstream TCP proxies.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envconf.String("HTTP_ADDR", defaultAddr)
	socketPath := envconf.String("IPC_SOCKET_PATH", defaultSocketPath)
	cameraConfigPath := envconf.String("CAMERA_CONFIG", defaultCameraConfig)
	tcpAddr := envconf.String("TCP_ADDR", defaultTCPAddr)
	tcpEnabled := envconf.String("TCP_ENABLED", "true") != "false"

	rotCfg, rotEntries, err := rotation.Load(cameraConfigPath)
	if err != nil {
		return fmt.Errorf("load rotation config: %w", err)
	}
	logger.Printf("rotation config loaded from %s: %d entries", cameraConfigPath, rotEntries)

	registry := livefeed.NewRegistry()
	go registry.RunLiveness(ctx, time.Second)

	// broadcaster fans frames out to TCP downstream clients (proxy-to-proxy transport)
	broadcaster := livefeed.NewBroadcaster()

	// per-camera sequence counters; only written by the single IPC reader goroutine
	seqs := make(map[streamproto.CameraID]uint64)

	reader := ipcclient.New(socketPath, ipcclient.Handler{
		OnFrame: func(cameraID streamproto.CameraID, jpeg []byte) {
			id := cameraID.String()
			if angle := rotCfg.Get(id); !angle.IsZero() {
				jpeg = rotation.Apply(jpeg, angle)
			}
			registry.Push(id, jpeg)

			seqs[cameraID]++
			broadcaster.Publish(streamproto.Frame{
				Timestamp: time.Now(),
				Sequence:  seqs[cameraID],
				CameraID:  cameraID,
				Payload:   jpeg,
			})
		},
		OnOffline: func(cameraID streamproto.CameraID) {
			registry.MarkOffline(cameraID.String())
		},
		OnDisconnect: registry.MarkAllOffline,
	}, logger)
	go reader.Run(ctx)

	if tcpEnabled {
		tcpSrv := streamproto.NewTCPServer(streamproto.TCPServerConfig{
			ListenAddr:   tcpAddr,
			MaxClients:   defaultMaxClients,
			WriteTimeout: 2 * time.Second,
		}, broadcaster, logger)
		go func() {
			if err := tcpSrv.Run(ctx); err != nil {
				logger.Printf("TCP server error: %v", err)
			}
		}()
	}

	api := livefeed.NewCameraAPI(registry, logger, livefeed.APIOptions{
		Rotation: rotCfg.Degrees,
		// This service talks to the cameras directly, so a viewer may well open
		// a stream before a camera has connected. Attaching to a camera that
		// has sent nothing yet lets that viewer wait rather than get a 404.
		CreateOnSubscribe: true,
		// Humans point their browsers at this service, so knowing who attached
		// and when is worth a log line.
		LogSubscribers: true,
	})
	httpSrv := &http.Server{
		Addr:        addr,
		Handler:     api,
		ReadTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logger.Printf("HTTP shutdown error: %v", err)
		}
	}()

	logger.Printf("camera-web-api listening on %s  ipc=%s  tcp=%s (enabled=%v)",
		addr, socketPath, tcpAddr, tcpEnabled)
	// ErrServerClosed is the expected result of the shutdown above, not a failure.
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server failed: %w", err)
	}
	return nil
}
