package app

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/w0rxbend/instachron/services/camera-web-restream-enhancer-api/internal/config"
	"github.com/w0rxbend/instachron/services/camera-web-restream-enhancer-api/internal/enhance"
	"github.com/w0rxbend/instachron/shared/envconf"
	"github.com/w0rxbend/instachron/shared/livefeed"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	defaultAddr         = ":8091"
	defaultUpstreamAddr = "localhost:9001"
	defaultTCPAddr      = ":9003"
	// defaultStatsInterval is how often the pass-through counters are logged.
	defaultStatsInterval = 60 * time.Second
)

// Run serves the enhancer proxy until ctx is cancelled. Every upstream frame is
// passed through the per-camera enhancement pipeline before being republished.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	cfg := livefeed.ServiceConfig{
		Name:            "camera-web-restream-enhancer-api",
		HTTPAddr:        envconf.String("HTTP_ADDR", defaultAddr),
		UpstreamTCPAddr: envconf.String("UPSTREAM_TCP_ADDR", defaultUpstreamAddr),
		TCPAddr:         envconf.String("TCP_ADDR", defaultTCPAddr),
		TCPEnabled:      envconf.String("TCP_ENABLED", "true") != "false",
	}

	configPath := envconf.String("CONFIG_FILE", config.DefaultPath)
	cameraCfgs, err := config.Load(configPath)
	if err != nil {
		logger.Printf("%v — using built-in defaults", err)
	} else {
		logger.Printf("loaded enhancer config from %s (%d camera overrides)", configPath, len(cameraCfgs.Cameras))
	}

	enhancer := enhance.New(cameraCfgs)
	statsInterval := envconf.Seconds("STATS_INTERVAL_SEC", defaultStatsInterval)
	go reportStats(ctx, enhancer, statsInterval, logger)

	transform := func(f streamproto.Frame, push func([]byte)) {
		enhancer.ProcessCamera(f.CameraID.String(), f.Payload, push)
	}

	return livefeed.Serve(ctx, cfg, transform, logger)
}

// reportStats logs the enhancer's counters every interval until ctx is
// cancelled. Only the change since the previous line is reported, so a steady
// stream of "0 passed through" lines means the pipeline is healthy right now
// rather than that it was healthy at some point since startup. Nothing is
// logged while the service is idle, to keep a quiet night quiet.
func reportStats(ctx context.Context, e *enhance.Enhancer, interval time.Duration, logger *log.Logger) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prev enhance.Stats
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cur := e.Stats()
			frames := cur.Processed - prev.Processed
			decodeFailed := cur.DecodeFailed - prev.DecodeFailed
			encodeFailed := cur.EncodeFailed - prev.EncodeFailed
			prev = cur
			if frames == 0 {
				continue
			}
			if decodeFailed == 0 && encodeFailed == 0 {
				logger.Printf("enhancer: %d frames enhanced", frames)
				continue
			}
			logger.Printf("enhancer: %d frames, %d passed through undecodable, %d passed through unencodable",
				frames, decodeFailed, encodeFailed)
		}
	}
}
