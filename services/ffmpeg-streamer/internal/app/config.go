package app

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"

	"github.com/w0rxbend/instachron/shared/envconf"
)

const (
	defaultSocketPath   = "/tmp/instachron/frames.sock"
	defaultFFmpegPath   = "ffmpeg"
	defaultFrameRate    = 10
	defaultRestartDelay = 5 * time.Second
	defaultCameraID     = 0
	defaultCellWidth    = 320
	defaultCellHeight   = 240
)

type config struct {
	socketPath   string
	cameraID     streamproto.CameraID
	ffmpegPath   string
	streamURL    string
	frameRate    int
	restartDelay time.Duration
	mergeAll     bool
	cellWidth    int
	cellHeight   int
}

func loadConfig(args []string) (config, error) {
	streamURL, err := streamURLFromEnv()
	if err != nil {
		return config{}, err
	}

	cfg := config{
		socketPath:   envconf.String("IPC_SOCKET_PATH", defaultSocketPath),
		cameraID:     streamproto.CameraID(envconf.Uint32("CAMERA_ID", defaultCameraID)),
		ffmpegPath:   envconf.String("FFMPEG_PATH", defaultFFmpegPath),
		streamURL:    streamURL,
		frameRate:    envconf.Int("STREAM_FRAME_RATE", defaultFrameRate),
		restartDelay: envconf.Duration("FFMPEG_RESTART_DELAY", defaultRestartDelay),
		mergeAll:     envconf.Bool("MERGE_ALL", false),
		cellWidth:    envconf.Int("CELL_WIDTH", defaultCellWidth),
		cellHeight:   envconf.Int("CELL_HEIGHT", defaultCellHeight),
	}

	flags := flag.NewFlagSet("ffmpeg-streamer", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Func("camera-id", "camera id to stream", func(value string) error {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid camera id %q: %w", value, err)
		}
		cfg.cameraID = streamproto.CameraID(parsed)
		return nil
	})
	flags.BoolVar(&cfg.mergeAll, "merge", cfg.mergeAll, "merge all cameras into a single canvas")
	flags.IntVar(&cfg.cellWidth, "cell-width", cfg.cellWidth, "cell width per camera in merged canvas")
	flags.IntVar(&cfg.cellHeight, "cell-height", cfg.cellHeight, "cell height per camera in merged canvas")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}

	if cfg.frameRate <= 0 {
		return config{}, fmt.Errorf("STREAM_FRAME_RATE must be greater than 0")
	}
	if cfg.restartDelay <= 0 {
		return config{}, fmt.Errorf("FFMPEG_RESTART_DELAY must be greater than 0")
	}
	if cfg.cellWidth <= 0 {
		return config{}, fmt.Errorf("CELL_WIDTH must be greater than 0")
	}
	if cfg.cellHeight <= 0 {
		return config{}, fmt.Errorf("CELL_HEIGHT must be greater than 0")
	}
	if cfg.cellWidth%2 != 0 {
		cfg.cellWidth++
	}
	if cfg.cellHeight%2 != 0 {
		cfg.cellHeight++
	}

	return cfg, nil
}

func streamURLFromEnv() (string, error) {
	if direct := envconf.String("STREAM_URL", ""); direct != "" {
		return direct, nil
	}
	if direct := envconf.String("RTMP_URL", ""); direct != "" {
		return direct, nil
	}

	twitchKey := envconf.String("TWITCH_STREAM_KEY", "")
	youtubeKey := envconf.String("YOUTUBE_STREAM_KEY", "")

	switch {
	case twitchKey != "" && youtubeKey != "":
		return "", fmt.Errorf("set only one of TWITCH_STREAM_KEY or YOUTUBE_STREAM_KEY")
	case twitchKey != "":
		return "rtmp://live.twitch.tv/app/" + twitchKey, nil
	case youtubeKey != "":
		return "rtmp://a.rtmp.youtube.com/live2/" + youtubeKey, nil
	default:
		return "", fmt.Errorf("set STREAM_URL, RTMP_URL, TWITCH_STREAM_KEY, or YOUTUBE_STREAM_KEY")
	}
}
