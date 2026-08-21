package app

import (
	"context"
	"log"
	"os"

	"github.com/w0rxbend/instachron/shared/envconf"
	"github.com/w0rxbend/instachron/shared/livefeed"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	defaultAddr         = ":8090"
	defaultUpstreamAddr = "localhost:9001"
	defaultTCPAddr      = ":9002"
)

// Run serves the plain restreamer until ctx is cancelled. It forwards every
// upstream frame unchanged, which is what makes it the reference proxy: the
// other three do the same job with a transform in the middle.
func Run(ctx context.Context) error {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	cfg := livefeed.ServiceConfig{
		Name:            "camera-web-restreamer-api",
		HTTPAddr:        envconf.String("HTTP_ADDR", defaultAddr),
		UpstreamTCPAddr: envconf.String("UPSTREAM_TCP_ADDR", defaultUpstreamAddr),
		TCPAddr:         envconf.String("TCP_ADDR", defaultTCPAddr),
		TCPEnabled:      envconf.String("TCP_ENABLED", "true") != "false",
	}

	passthrough := func(f streamproto.Frame, push func([]byte)) { push(f.Payload) }

	return livefeed.Serve(ctx, cfg, passthrough, logger)
}
