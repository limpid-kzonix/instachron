package recorder

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
)

// These tests exercise the segment lifecycle — open, write, rotate, finish,
// discard — with no ffmpeg subprocess and no files on disk. That is possible
// because Camera takes an EncoderFactory rather than calling encoder.Start
// directly; before that seam existed, none of this could be tested at all.

// fakeEncoder records what the recording loop did to it.
type fakeEncoder struct {
	mu       sync.Mutex
	frames   [][]byte
	closed   bool
	killed   bool
	closeErr error
	writeErr error
}

func (f *fakeEncoder) WriteJPEG(jpeg []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return f.writeErr
	}
	f.frames = append(f.frames, append([]byte(nil), jpeg...))
	return nil
}

func (f *fakeEncoder) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.closeErr
}

func (f *fakeEncoder) Kill() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killed = true
}

func (f *fakeEncoder) frameCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.frames)
}

func (f *fakeEncoder) state() (closed, killed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed, f.killed
}

// fakeWriter is a storage.SegmentWriter that keeps the bytes in memory.
type fakeWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	closed  bool
	sizeOut int64
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *fakeWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *fakeWriter) BytesWritten() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sizeOut > 0 {
		return w.sizeOut
	}
	return int64(w.buf.Len())
}

// fakeStore is a storage.Store that keeps every segment in memory and records
// which of the two terminal calls each pending segment received.
type fakeStore struct {
	mu        sync.Mutex
	begun     int
	completed int
	discarded int
	pruned    int
	beginErr  error
	writers   []*fakeWriter
}

func (s *fakeStore) BeginSegment(_ context.Context, cameraID string, start time.Time, outputFPS, timelapseFactor int) (*storage.PendingSegment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	s.begun++
	w := &fakeWriter{}
	s.writers = append(s.writers, w)
	return &storage.PendingSegment{
		Info: storage.SegmentInfo{
			CameraID:  cameraID,
			FileName:  "segment.mp4",
			StartedAt: start,
			OutputFPS: outputFPS,
		},
		Writer: w,
	}, nil
}

func (s *fakeStore) CompleteSegment(_ context.Context, seg *storage.PendingSegment, end time.Time) (storage.SegmentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed++
	info := seg.Info
	info.EndedAt = end
	return info, nil
}

func (s *fakeStore) DiscardSegment(context.Context, *storage.PendingSegment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discarded++
	return nil
}

func (s *fakeStore) Prune(context.Context, string, int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruned++
	return nil
}

func (s *fakeStore) List(context.Context, storage.ListFilter) ([]storage.SegmentInfo, error) {
	return nil, nil
}

func (s *fakeStore) Open(context.Context, string, string) (storage.ReadSeekCloser, storage.SegmentInfo, error) {
	return nil, storage.SegmentInfo{}, errors.New("not implemented in the fake")
}

func (s *fakeStore) UsageBytes(context.Context) (int64, error) { return 0, nil }

func (s *fakeStore) Cameras(context.Context) ([]string, error) { return nil, nil }

func (s *fakeStore) counts() (begun, completed, discarded int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.begun, s.completed, s.discarded
}

// testCamera builds a Camera wired to fakes, with the segment lifecycle
// thresholds set wide enough that only the test triggers a rotation.
func testCamera(t *testing.T, store storage.Store, factory EncoderFactory) *Camera {
	t.Helper()
	cfg := Config{
		OutputFPS:             10,
		TimelapseFactor:       1,
		SegmentRawDuration:    time.Hour,
		MaxFileBytes:          1 << 40,
		KeepFilesPerCamera:    5,
		QueueSizePerCamera:    4,
		InactiveCloseDuration: time.Hour,
		FFmpeg:                encoder.Config{Path: "ffmpeg-does-not-run-in-this-test"},
	}
	return &Camera{
		id:         1,
		idText:     "1",
		cfg:        cfg,
		store:      store,
		metrics:    metrics.New(),
		logger:     log.New(io.Discard, "", 0),
		newEncoder: factory,
	}
}

func jpegFrame() []byte { return []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9} }

func TestOpenSegmentStartsAnEncoder(t *testing.T) {
	store := &fakeStore{}
	enc := &fakeEncoder{}
	c := testCamera(t, store, func(context.Context, encoder.Config, io.Writer) (Encoder, error) {
		return enc, nil
	})

	seg, err := c.openSegment(context.Background(), context.Background(), time.Now())
	if err != nil {
		t.Fatalf("openSegment() error = %v", err)
	}
	if seg.encoder != Encoder(enc) {
		t.Error("the segment is not using the encoder the factory returned")
	}
	if begun, _, _ := store.counts(); begun != 1 {
		t.Errorf("store opened %d segments, want 1", begun)
	}
}

// TestOpenSegmentDiscardsTheFileWhenTheEncoderFails is the leak regression: the
// store has already created a temporary file by the time the encoder is
// started, so a failure there has to clean that file up.
func TestOpenSegmentDiscardsTheFileWhenTheEncoderFails(t *testing.T) {
	store := &fakeStore{}
	c := testCamera(t, store, func(context.Context, encoder.Config, io.Writer) (Encoder, error) {
		return nil, errors.New("ffmpeg is not installed")
	})

	if _, err := c.openSegment(context.Background(), context.Background(), time.Now()); err == nil {
		t.Fatal("openSegment() succeeded, want an error")
	}
	begun, completed, discarded := store.counts()
	if begun != 1 || discarded != 1 || completed != 0 {
		t.Errorf("store saw begun=%d completed=%d discarded=%d; want 1/0/1 — the temporary file must be cleaned up",
			begun, completed, discarded)
	}
}

func TestFinishSegmentClosesTheEncoderAndPublishesTheFile(t *testing.T) {
	store := &fakeStore{}
	enc := &fakeEncoder{}
	c := testCamera(t, store, func(context.Context, encoder.Config, io.Writer) (Encoder, error) {
		return enc, nil
	})

	seg, err := c.openSegment(context.Background(), context.Background(), time.Now())
	if err != nil {
		t.Fatalf("openSegment() error = %v", err)
	}
	c.finishSegment(context.Background(), seg)

	closed, killed := enc.state()
	if !closed {
		t.Error("the encoder was not closed, so the video would be truncated")
	}
	if killed {
		t.Error("the encoder was killed during a clean finish")
	}
	if _, completed, discarded := store.counts(); completed != 1 || discarded != 0 {
		t.Errorf("store saw completed=%d discarded=%d, want 1/0", completed, discarded)
	}
}

// TestFinishSegmentThrowsTheFileAwayWhenTheEncoderFails pins the rule stated in
// finishSegment's doc comment: a segment that cannot be finalised must not be
// published as if it were a good recording.
func TestFinishSegmentThrowsTheFileAwayWhenTheEncoderFails(t *testing.T) {
	store := &fakeStore{}
	enc := &fakeEncoder{closeErr: errors.New("ffmpeg exited badly")}
	c := testCamera(t, store, func(context.Context, encoder.Config, io.Writer) (Encoder, error) {
		return enc, nil
	})

	seg, err := c.openSegment(context.Background(), context.Background(), time.Now())
	if err != nil {
		t.Fatalf("openSegment() error = %v", err)
	}
	c.finishSegment(context.Background(), seg)

	if _, completed, discarded := store.counts(); completed != 0 || discarded != 1 {
		t.Errorf("store saw completed=%d discarded=%d, want 0/1 — a broken segment must not be published",
			completed, discarded)
	}
}

func TestDiscardSegmentKillsRatherThanCloses(t *testing.T) {
	store := &fakeStore{}
	enc := &fakeEncoder{}
	c := testCamera(t, store, func(context.Context, encoder.Config, io.Writer) (Encoder, error) {
		return enc, nil
	})

	seg, err := c.openSegment(context.Background(), context.Background(), time.Now())
	if err != nil {
		t.Fatalf("openSegment() error = %v", err)
	}
	c.discardSegment(context.Background(), seg)

	closed, killed := enc.state()
	if !killed {
		t.Error("discardSegment did not kill the encoder; waiting for a flush gains nothing for a broken segment")
	}
	if closed {
		t.Error("discardSegment closed the encoder cleanly instead of killing it")
	}
	if _, completed, discarded := store.counts(); completed != 0 || discarded != 1 {
		t.Errorf("store saw completed=%d discarded=%d, want 0/1", completed, discarded)
	}
}

// TestFinishSegmentOnNilIsSafe matters because the recording loop calls
// finishSegment unconditionally on shutdown, including when no segment is open.
func TestFinishSegmentOnNilIsSafe(t *testing.T) {
	store := &fakeStore{}
	c := testCamera(t, store, func(context.Context, encoder.Config, io.Writer) (Encoder, error) {
		return &fakeEncoder{}, nil
	})
	c.finishSegment(context.Background(), nil)
	c.discardSegment(context.Background(), nil)
	if begun, completed, discarded := store.counts(); begun+completed+discarded != 0 {
		t.Error("a nil segment touched the store")
	}
}

func TestShouldRotate(t *testing.T) {
	store := &fakeStore{}
	c := testCamera(t, store, nil)
	start := time.Now()

	tests := []struct {
		name         string
		rawDuration  time.Duration
		maxFileBytes int64
		bytesWritten int64
		elapsed      time.Duration
		want         bool
	}{
		{name: "fresh and small: keep going", rawDuration: time.Hour, maxFileBytes: 1 << 30, elapsed: time.Minute, want: false},
		{name: "old enough: rotate on time", rawDuration: time.Minute, maxFileBytes: 1 << 30, elapsed: 2 * time.Minute, want: true},
		{name: "exactly at the duration limit rotates", rawDuration: time.Minute, maxFileBytes: 1 << 30, elapsed: time.Minute, want: true},
		{name: "big enough: rotate on size", rawDuration: time.Hour, maxFileBytes: 1000, bytesWritten: 1000, elapsed: time.Minute, want: true},
		{name: "just under the size limit keeps going", rawDuration: time.Hour, maxFileBytes: 1000, bytesWritten: 999, elapsed: time.Minute, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.cfg.SegmentRawDuration = tt.rawDuration
			c.cfg.MaxFileBytes = tt.maxFileBytes
			w := &fakeWriter{sizeOut: tt.bytesWritten}
			seg := &activeSegment{
				pending:   &storage.PendingSegment{Writer: w},
				startedAt: start,
			}
			if got := c.shouldRotate(seg, start.Add(tt.elapsed)); got != tt.want {
				t.Errorf("shouldRotate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldRotateOnNilSegment(t *testing.T) {
	c := testCamera(t, &fakeStore{}, nil)
	if c.shouldRotate(nil, time.Now()) {
		t.Error("shouldRotate(nil) = true, want false — there is nothing to rotate")
	}
}
