package app

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/w0rxbend/instachron/services/camera-web-restream-detector-api/internal/config"
	"github.com/w0rxbend/instachron/services/camera-web-restream-detector-api/internal/detect"
	"github.com/w0rxbend/instachron/shared/envconf"
	"github.com/w0rxbend/instachron/shared/livefeed"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	defaultAddr         = ":8093"
	defaultUpstreamAddr = "localhost:9001"
	defaultTCPAddr      = ":9005"
)

// frameProcessor turns one JPEG frame into the frame that gets published.
type frameProcessor interface {
	Process(jpeg []byte, push func([]byte))
}

// passthrough publishes frames unchanged; used when the model is unavailable.
type passthrough struct{}

func (passthrough) Process(jpeg []byte, push func([]byte)) { push(jpeg) }

// Run serves the detector proxy until ctx is cancelled. Frames are annotated
// with YOLOv8 bounding boxes when the model loads, and forwarded untouched when
// it does not, so a missing model degrades the service rather than stopping it.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	serviceCfg := livefeed.ServiceConfig{
		Name:            "camera-web-restream-detector-api",
		HTTPAddr:        normalizeListenAddr(envconf.String("HTTP_ADDR", defaultAddr)),
		UpstreamTCPAddr: envconf.String("UPSTREAM_TCP_ADDR", defaultUpstreamAddr),
		TCPAddr:         normalizeListenAddr(envconf.String("TCP_ADDR", defaultTCPAddr)),
		TCPEnabled:      envconf.String("TCP_ENABLED", "true") != "false",
	}

	configPath := envconf.String("CONFIG_FILE", config.DefaultPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Printf("%v — using built-in defaults", err)
	} else {
		logger.Printf("loaded detector config from %s (model=%s conf=%.2f nms=%.2f)",
			configPath, cfg.ModelPath, cfg.ConfThreshold, cfg.NMSThreshold)
	}

	logger.Printf("ORT library path: %q (override with ORT_LIB_PATH env var)", cfg.OrtLibPath)

	// load the detector; fall back to passthrough if the model isn't available yet
	var processor frameProcessor
	det, err := detect.New(cfg, logger)
	if err != nil {
		logger.Printf("detector unavailable (%v) — frames will pass through unchanged", err)
		logger.Printf("hint: download ONNX Runtime from https://github.com/microsoft/onnxruntime/releases and set ORT_LIB_PATH=/path/to/libonnxruntime.so.x.y.z")
		processor = passthrough{}
	} else {
		logger.Printf("YOLOv8 detector ready (input %dx%d, %d classes, output %s)",
			cfg.InputWidth, cfg.InputHeight, cfg.NumClasses, det.Layout())
		processor = det
	}

	if det != nil {
		go func() {
			<-ctx.Done()
			det.Destroy()
		}()
	}

	transform := func(f streamproto.Frame, push func([]byte)) {
		processor.Process(f.Payload, push)
	}

	return livefeed.Serve(ctx, serviceCfg, transform, logger)
}

// normalizeListenAddr makes a listen address acceptable to net.Listen: a bare
// port number such as "9005" has no colon, so ":" is prepended to turn it into
// "all interfaces, port 9005". Anything already containing a colon is returned
// unchanged.
func normalizeListenAddr(v string) string {
	if strings.Contains(v, ":") {
		return v
	}
	return ":" + v
}
