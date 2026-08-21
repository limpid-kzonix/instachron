package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/config"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/recorder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/usage"
	"github.com/w0rxbend/instachron/shared/envconf"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

// Run starts the recorder and blocks until ctx is cancelled or the HTTP server
// fails. Returning the error instead of exiting matters here: the deferred
// rec.Close below is what stops the running ffmpeg encoders and finishes their
// segments, and a call to os.Exit would skip it.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	configPath := envconf.String("CONFIG_FILE", config.DefaultPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Printf("config file %q not found, using defaults and env overrides", configPath)
			cfg, err = config.Load("")
		}
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	}

	store := storage.NewLocal(cfg.Storage.RootDir)
	m := metrics.New()
	rec := recorder.NewSessions(recorder.Config{
		OutputFPS:             cfg.Recording.OutputFPS,
		TimelapseFactor:       cfg.Recording.TimelapseFactor,
		SegmentRawDuration:    cfg.SegmentDuration(),
		MaxFileBytes:          cfg.Recording.MaxFileBytes,
		KeepFilesPerCamera:    cfg.Recording.KeepFilesPerCamera,
		QueueSizePerCamera:    cfg.Recording.QueueSizePerCamera,
		InactiveCloseDuration: cfg.InactiveTimeout(),
		FFmpeg: encoder.Config{
			Path:   cfg.FFmpeg.Path,
			Preset: cfg.FFmpeg.Preset,
			CRF:    cfg.FFmpeg.CRF,
		},
	}, store, m, logger)
	defer rec.Close()

	go usage.Run(ctx, store, m, usage.DefaultInterval, logger)

	upstream := streamproto.NewTCPUpstream(
		streamproto.TCPUpstreamConfig{Addr: cfg.UpstreamTCPAddr},
		func(f streamproto.Frame) {
			rec.Submit(ctx, f)
		},
		nil,
		logger,
	)
	go upstream.Run(ctx)

	api := newAPI(store, rec, m, logger)
	httpSrv := &http.Server{
		Addr:        cfg.HTTPAddr,
		Handler:     api.routes(),
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

	logger.Printf("camera-recorder listening on %s upstream=%s storage=%s timelapse=%dx output_fps=%d",
		cfg.HTTPAddr, cfg.UpstreamTCPAddr, cfg.Storage.RootDir, cfg.Recording.TimelapseFactor, cfg.Recording.OutputFPS)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server on %s: %w", cfg.HTTPAddr, err)
	}
	return nil
}
