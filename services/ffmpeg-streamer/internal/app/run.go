// Package app wires up ffmpeg-streamer: it reads configuration, connects to the
// frame IPC socket published by tcp-camera-backend, and pushes JPEG frames into
// an ffmpeg subprocess that encodes them to an RTMP live stream.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/w0rxbend/instachron/services/ffmpeg-streamer/internal/ipcclient"
)

// Run starts the streamer and blocks until ctx is cancelled or ffmpeg can no
// longer be kept alive. Cancelling ctx — which is how the caller asks for a
// shutdown — is a normal ending, so Run reports it as success rather than as an
// error.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config failed: %w", err)
	}

	ipc := ipcclient.New(cfg.socketPath, logger)
	go ipc.Run(ctx)

	if err := run(ctx, cfg, ipc, logger); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("streamer failed: %w", err)
	}
	return nil
}
