package app

import (
	"context"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/w0rxbend/instachron/services/camera-web-restream-upscaler-api/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-web-restream-upscaler-api/internal/pipeline"
	"github.com/w0rxbend/instachron/shared/envconf"
	"github.com/w0rxbend/instachron/shared/livefeed"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

type appConfig struct {
	httpAddr        string
	upstreamTCPAddr string
	tcpAddr         string
	tcpEnabled      bool
	numWorkers      int
	queueMultiplier int
	maxInputWidth   int
	maxInputHeight  int
	scale           int
	jpegQuality     int
	metricsInterval time.Duration
}

func loadConfig() *appConfig {
	numCPU := runtime.NumCPU()
	workers := envconf.Int("NUM_WORKERS", max(1, numCPU/2))

	return &appConfig{
		httpAddr:        envconf.String("HTTP_ADDR", ":8092"),
		upstreamTCPAddr: envconf.String("UPSTREAM_TCP_ADDR", "localhost:9001"),
		tcpAddr:         envconf.String("TCP_ADDR", ":9004"),
		tcpEnabled:      envconf.String("TCP_ENABLED", "true") != "false",
		numWorkers:      workers,
		queueMultiplier: envconf.Int("QUEUE_MULTIPLIER", 2),
		maxInputWidth:   envconf.Int("MAX_INPUT_WIDTH", 960),
		maxInputHeight:  envconf.Int("MAX_INPUT_HEIGHT", 540),
		scale:           envconf.Int("UPSCALE_FACTOR", 2),
		jpegQuality:     envconf.Int("JPEG_QUALITY", 85),
		metricsInterval: envconf.Seconds("METRICS_INTERVAL_SEC", 60*time.Second),
	}
}

// Run serves the upscaler proxy until ctx is cancelled. Frames are upscaled by
// a fixed worker pool, so the transform below hands each frame to the pool and
// the pool calls push once the enlarged JPEG is ready.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	cfg := loadConfig()

	logger.Printf("camera-web-restream-upscaler-api config: transform=lanczos-%dx workers=%d queue=%d max=%dx%d",
		cfg.scale, cfg.numWorkers,
		cfg.numWorkers*cfg.queueMultiplier,
		cfg.maxInputWidth, cfg.maxInputHeight)

	m := &metrics.Pipeline{}
	pool := pipeline.New(pipeline.Config{
		Workers:     cfg.numWorkers,
		QueueSize:   cfg.numWorkers * cfg.queueMultiplier,
		JPEGQuality: cfg.jpegQuality,
		MaxWidth:    cfg.maxInputWidth,
		MaxHeight:   cfg.maxInputHeight,
		Scale:       cfg.scale,
	}, m)
	defer pool.Close()

	go m.RunReporter(ctx, cfg.metricsInterval, logger)

	transform := func(f streamproto.Frame, push func([]byte)) {
		pool.Process(f.Payload, push)
	}

	return livefeed.Serve(ctx, livefeed.ServiceConfig{
		Name:            "camera-web-restream-upscaler-api",
		HTTPAddr:        cfg.httpAddr,
		UpstreamTCPAddr: cfg.upstreamTCPAddr,
		TCPAddr:         cfg.tcpAddr,
		TCPEnabled:      cfg.tcpEnabled,
	}, transform, logger)
}
