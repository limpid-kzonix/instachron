package config

import (
	"time"

	"github.com/w0rxbend/instachron/shared/envconf"
)

const (
	defaultAddr          = "0.0.0.0:5000"
	defaultSocketPath    = "/tmp/instachron/frames.sock"
	defaultMaxFrameBytes = 5 * 1024 * 1024
	defaultReadTimeout   = 30 * time.Second
)

type Config struct {
	TCPAddr       string
	IPCSocketPath string
	MaxFrameBytes uint32
	ReadTimeout   time.Duration
}

func LoadFromEnv() Config {
	return Config{
		TCPAddr:       envconf.String("TCP_ADDR", defaultAddr),
		IPCSocketPath: envconf.String("IPC_SOCKET_PATH", defaultSocketPath),
		MaxFrameBytes: envconf.Uint32("MAX_FRAME_BYTES", defaultMaxFrameBytes),
		ReadTimeout:   envconf.Duration("READ_TIMEOUT", defaultReadTimeout),
	}
}
