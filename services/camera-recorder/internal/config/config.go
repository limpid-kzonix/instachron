package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/w0rxbend/instachron/shared/envconf"
)

// DefaultPath is the config file the service reads when CONFIG_FILE is unset.
const DefaultPath = "config.json"

// Config is the whole camera-recorder configuration, as read from the JSON
// config file and then overridden by environment variables.
type Config struct {
	HTTPAddr        string          `json:"http_addr"`
	UpstreamTCPAddr string          `json:"upstream_tcp_addr"`
	Recording       RecordingConfig `json:"recording"`
	Storage         StorageConfig   `json:"storage"`
	FFmpeg          FFmpegConfig    `json:"ffmpeg"`

	// The two duration fields below are the parsed form of the matching string
	// fields in Recording. Load fills them in after Validate has confirmed the
	// strings parse, so nothing downstream has to handle a parse error again.
	segmentDuration time.Duration
	inactiveTimeout time.Duration
}

// RecordingConfig controls how incoming frames become segment files: how many
// of them are kept, how long each file covers, and how big it may grow.
type RecordingConfig struct {
	OutputFPS             int    `json:"output_fps"`
	TimelapseFactor       int    `json:"timelapse_factor"`
	SegmentRawDuration    string `json:"segment_raw_duration"`
	MaxFileBytes          int64  `json:"max_file_bytes"`
	KeepFilesPerCamera    int    `json:"keep_files_per_camera"`
	QueueSizePerCamera    int    `json:"queue_size_per_camera"`
	InactiveCloseDuration string `json:"inactive_close_duration"`
}

// StorageConfig says where finished recordings are written. Only the "local"
// type (a directory on disk) is supported today.
type StorageConfig struct {
	Type    string `json:"type"`
	RootDir string `json:"root_dir"`
}

// FFmpegConfig holds the settings passed to the ffmpeg process that encodes
// each segment.
type FFmpegConfig struct {
	Path   string `json:"path"`
	Preset string `json:"preset"`
	CRF    int    `json:"crf"`
}

// Defaults returns the configuration used when no config file is present. Every
// field is set, and the result passes Validate.
func Defaults() Config {
	return Config{
		HTTPAddr:        ":8094",
		UpstreamTCPAddr: "localhost:9001",
		Recording: RecordingConfig{
			OutputFPS:             10,
			TimelapseFactor:       10,
			SegmentRawDuration:    "10m",
			MaxFileBytes:          100 * 1024 * 1024,
			KeepFilesPerCamera:    144,
			QueueSizePerCamera:    128,
			InactiveCloseDuration: "30s",
		},
		Storage: StorageConfig{
			Type:    "local",
			RootDir: "./recordings",
		},
		FFmpeg: FFmpegConfig{
			Path:   "ffmpeg",
			Preset: "veryfast",
			CRF:    23,
		},
	}
}

// Load reads the config file at path (or starts from Defaults when path is
// empty), applies the environment-variable overrides, and validates the result.
// An os.ErrNotExist error means the file itself was missing.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	// Validate already rejected unparseable values, so these cannot fail here.
	cfg.segmentDuration, _ = time.ParseDuration(cfg.Recording.SegmentRawDuration)
	cfg.inactiveTimeout, _ = time.ParseDuration(cfg.Recording.InactiveCloseDuration)
	return cfg, nil
}

// SegmentDuration returns how much wall-clock time one recorded segment covers,
// parsed from Recording.SegmentRawDuration. It is only meaningful on a Config
// that came back from Load.
func (c Config) SegmentDuration() time.Duration {
	return c.segmentDuration
}

// InactiveTimeout returns how long a camera may go without delivering a frame
// before its open segment is closed, parsed from Recording.InactiveCloseDuration.
// It is only meaningful on a Config that came back from Load.
func (c Config) InactiveTimeout() time.Duration {
	return c.inactiveTimeout
}

// Validate reports the first configuration value that would stop the service
// from running correctly, or nil when every value is usable.
func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("http_addr is required")
	}
	if c.UpstreamTCPAddr == "" {
		return fmt.Errorf("upstream_tcp_addr is required")
	}
	if c.Recording.OutputFPS <= 0 {
		return fmt.Errorf("recording.output_fps must be greater than 0")
	}
	if c.Recording.TimelapseFactor <= 0 {
		return fmt.Errorf("recording.timelapse_factor must be greater than 0")
	}
	if _, err := time.ParseDuration(c.Recording.SegmentRawDuration); err != nil {
		return fmt.Errorf("recording.segment_raw_duration: %w", err)
	}
	if c.Recording.MaxFileBytes <= 0 {
		return fmt.Errorf("recording.max_file_bytes must be greater than 0")
	}
	if c.Recording.KeepFilesPerCamera <= 0 {
		return fmt.Errorf("recording.keep_files_per_camera must be greater than 0")
	}
	if c.Recording.QueueSizePerCamera <= 0 {
		return fmt.Errorf("recording.queue_size_per_camera must be greater than 0")
	}
	if _, err := time.ParseDuration(c.Recording.InactiveCloseDuration); err != nil {
		return fmt.Errorf("recording.inactive_close_duration: %w", err)
	}
	if c.Storage.Type != "local" {
		return fmt.Errorf("storage.type %q is unsupported", c.Storage.Type)
	}
	if c.Storage.RootDir == "" {
		return fmt.Errorf("storage.root_dir is required")
	}
	if c.FFmpeg.Path == "" {
		return fmt.Errorf("ffmpeg.path is required")
	}
	if c.FFmpeg.Preset == "" {
		return fmt.Errorf("ffmpeg.preset is required")
	}
	if c.FFmpeg.CRF < 0 || c.FFmpeg.CRF > 51 {
		return fmt.Errorf("ffmpeg.crf must be between 0 and 51")
	}
	return nil
}

// applyEnv overlays environment variables on top of a config that has already
// been read from the config file. Every setting keeps whatever the file gave it
// unless the matching variable is set, which is what the fallback argument to
// each envconf reader expresses: "leave this alone".
func applyEnv(c *Config) {
	c.HTTPAddr = envconf.String("HTTP_ADDR", c.HTTPAddr)
	c.UpstreamTCPAddr = envconf.String("UPSTREAM_TCP_ADDR", c.UpstreamTCPAddr)
	c.Storage.RootDir = envconf.String("STORAGE_ROOT_DIR", c.Storage.RootDir)
	c.FFmpeg.Path = envconf.String("FFMPEG_PATH", c.FFmpeg.Path)
	c.FFmpeg.Preset = envconf.String("FFMPEG_PRESET", c.FFmpeg.Preset)
	c.FFmpeg.CRF = envconf.Int("FFMPEG_CRF", c.FFmpeg.CRF)

	c.Recording.OutputFPS = envconf.Int("OUTPUT_FPS", c.Recording.OutputFPS)
	c.Recording.TimelapseFactor = envconf.Int("TIMELAPSE_FACTOR", c.Recording.TimelapseFactor)
	c.Recording.MaxFileBytes = envconf.Int64("MAX_FILE_BYTES", c.Recording.MaxFileBytes)
	c.Recording.KeepFilesPerCamera = envconf.Int("KEEP_FILES_PER_CAMERA", c.Recording.KeepFilesPerCamera)
	c.Recording.QueueSizePerCamera = envconf.Int("QUEUE_SIZE_PER_CAMERA", c.Recording.QueueSizePerCamera)
	c.Recording.SegmentRawDuration = envconf.String("SEGMENT_RAW_DURATION", c.Recording.SegmentRawDuration)
	c.Recording.InactiveCloseDuration = envconf.String("INACTIVE_CLOSE_DURATION", c.Recording.InactiveCloseDuration)
}
