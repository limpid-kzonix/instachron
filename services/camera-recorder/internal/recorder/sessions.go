package recorder

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

type Config struct {
	OutputFPS             int
	TimelapseFactor       int
	SegmentRawDuration    time.Duration
	MaxFileBytes          int64
	KeepFilesPerCamera    int
	QueueSizePerCamera    int
	InactiveCloseDuration time.Duration
	FFmpeg                encoder.Config
}

type Sessions struct {
	cfg     Config
	store   storage.Store
	metrics *metrics.Metrics
	logger  *log.Logger
	// newEncoder is handed to every Camera this Sessions creates. It is nil in
	// production, which newCamera reads as "use real ffmpeg"; tests set it with
	// WithEncoderFactory.
	newEncoder EncoderFactory

	mu      sync.Mutex
	cameras map[streamproto.CameraID]*Camera
}

// WithEncoderFactory replaces the encoder used for every segment recorded from
// now on. It exists so tests can record segments without ffmpeg being present;
// production code leaves the default alone.
//
// Call it before any frame is submitted. Changing the factory once recording is
// under way affects only segments opened afterwards.
func (s *Sessions) WithEncoderFactory(f EncoderFactory) *Sessions {
	s.newEncoder = f
	return s
}

func NewSessions(cfg Config, store storage.Store, m *metrics.Metrics, logger *log.Logger) *Sessions {
	return &Sessions{
		cfg:     cfg,
		store:   store,
		metrics: m,
		logger:  logger,
		cameras: make(map[streamproto.CameraID]*Camera),
	}
}

func (m *Sessions) Submit(ctx context.Context, f streamproto.Frame) {
	camera := m.camera(ctx, f.CameraID)
	camera.Submit(f)
}

func (m *Sessions) Close() {
	m.mu.Lock()
	cameras := make([]*Camera, 0, len(m.cameras))
	for _, c := range m.cameras {
		cameras = append(cameras, c)
	}
	m.mu.Unlock()
	for _, c := range cameras {
		c.Close()
	}
}

func (m *Sessions) ActiveCameraIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.cameras))
	for id := range m.cameras {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	return ids
}

// camera returns the recorder for id, starting one on first use. ctx is the
// process lifetime context that arrives with each frame; it is only used the
// first time a camera is seen, as the lifetime of that camera's goroutine.
func (m *Sessions) camera(ctx context.Context, id streamproto.CameraID) *Camera {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.cameras[id]; c != nil {
		return c
	}
	c := newCamera(ctx, id, m.cfg, m.store, m.metrics, m.logger, m.newEncoder)
	m.cameras[id] = c
	return c
}
