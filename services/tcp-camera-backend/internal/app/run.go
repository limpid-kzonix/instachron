package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/config"
	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/publisher"
	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/server"
)

// Run starts the TCP frame server and the IPC publisher, and returns once ctx is
// cancelled or the server stops with an error. Deciding when to cancel ctx (a
// signal from the operating system, a test finishing) is left to the caller.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	cfg := config.LoadFromEnv()

	if err := os.MkdirAll(filepath.Dir(cfg.IPCSocketPath), 0o755); err != nil {
		return fmt.Errorf("create IPC socket directory: %w", err)
	}

	pub := publisher.New(cfg.IPCSocketPath, logger)
	srv := server.New(server.Config{
		Addr:          cfg.TCPAddr,
		MaxFrameBytes: cfg.MaxFrameBytes,
		ReadTimeout:   cfg.ReadTimeout,
		Publisher:     pub,
		Logger:        logger,
	})

	go func() {
		if err := pub.Listen(ctx); err != nil {
			logger.Printf("IPC publisher error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(ctx); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
