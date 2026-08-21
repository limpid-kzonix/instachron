package recorder

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/shared/dropchan"

	"github.com/w0rxbend/instachron/shared/imageutil"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

// Camera records the frames of a single upstream camera. Frames arrive through
// Submit and are handled by one goroutine started in newCamera; Close stops that
// goroutine and waits for the segment in progress to be finished.
type Camera struct {
	id      streamproto.CameraID
	idText  string
	cfg     Config
	store   storage.Store
	metrics *metrics.Metrics
	logger  *log.Logger
	// newEncoder is how this camera starts an encoder for each new segment.
	// Production wiring passes startFFmpeg; tests pass a fake.
	newEncoder EncoderFactory

	cancel context.CancelFunc
	queue  chan streamproto.Frame
	wg     sync.WaitGroup
}

type activeSegment struct {
	pending      *storage.PendingSegment
	encoder      Encoder
	startedAt    time.Time
	lastRecorded time.Time
}

// newCamera starts a recorder for one camera. parent is the process lifetime
// context: cancelling it, or calling Close, stops the recording loop.
func newCamera(parent context.Context, id streamproto.CameraID, cfg Config, store storage.Store, m *metrics.Metrics, logger *log.Logger, newEncoder EncoderFactory) *Camera {
	if newEncoder == nil {
		newEncoder = startFFmpeg
	}
	ctx, cancel := context.WithCancel(parent)
	c := &Camera{
		id:         id,
		idText:     id.String(),
		cfg:        cfg,
		store:      store,
		metrics:    m,
		logger:     logger,
		newEncoder: newEncoder,
		cancel:     cancel,
		queue:      make(chan streamproto.Frame, cfg.QueueSizePerCamera),
	}
	c.wg.Add(1)
	go c.run(ctx)
	return c
}

func (c *Camera) Submit(f streamproto.Frame) {
	c.metrics.IncFramesReceived(c.idText)
	// Recording must never slow the camera feed down, so a full queue costs a
	// frame rather than a stall. Every lost frame is counted, because for a
	// recorder — unlike a live viewer — a gap in the queue becomes a gap in the
	// recorded file.
	if dropchan.Send(c.queue, f).Lost() {
		c.metrics.IncFramesDropped(c.idText)
	}
}

func (c *Camera) Close() {
	c.cancel()
	c.wg.Wait()
}

// state is everything the recording loop carries from one frame to the next.
type state struct {
	// seg is the segment currently being written, or nil when none is open.
	seg *activeSegment
	// nextKeepAt is the earliest timestamp at which the next frame may be kept;
	// frames arriving before it are dropped to produce the timelapse effect.
	nextKeepAt time.Time
	// lastFrameAt is the timestamp of the most recent frame of any kind, used to
	// notice that a camera has gone quiet.
	lastFrameAt time.Time
}

func (c *Camera) run(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Finishing a segment has to outlive cancellation: when ctx is cancelled the
	// loop still has to stop ffmpeg, rename the half-written temporary file to its
	// final name and write its metadata. A cancelled context would make every one
	// of those store calls fail immediately and leave the file stranded in .tmp,
	// so the close path deliberately uses a context that is never cancelled.
	finalizeCtx := context.Background()

	var st state

	for {
		select {
		case <-ctx.Done():
			c.finishSegment(finalizeCtx, st.seg)
			return
		case <-ticker.C:
			if st.seg != nil && !st.lastFrameAt.IsZero() && time.Since(st.lastFrameAt) > c.cfg.InactiveCloseDuration {
				c.logger.Printf("camera-recorder camera=%s closing inactive segment", c.idText)
				c.rotate(finalizeCtx, &st)
			}
		case f := <-c.queue:
			c.handleFrame(ctx, finalizeCtx, &st, f)
		}
	}
}

// handleFrame processes one frame from the queue: it drops the frame if it is
// not a usable JPEG or if the timelapse decimation says to skip it, opens a
// segment when none is open, writes the frame, and rotates the segment when it
// has reached its time or size limit.
//
// ctx governs opening a new segment and stops the moment the recorder is shut
// down; finalizeCtx governs closing one and is never cancelled, so a segment
// already on disk still gets finished.
func (c *Camera) handleFrame(ctx, finalizeCtx context.Context, st *state, f streamproto.Frame) {
	ts := f.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	st.lastFrameAt = ts
	if !imageutil.LooksLikeJPEG(f.Payload) {
		c.metrics.IncFramesDropped(c.idText)
		return
	}
	if !shouldKeep(ts, &st.nextKeepAt, c.keepEvery()) {
		c.metrics.IncFramesDropped(c.idText)
		return
	}
	if st.seg != nil && c.shouldRotate(st.seg, ts) {
		c.rotate(finalizeCtx, st)
	}
	if st.seg == nil {
		seg, err := c.openSegment(ctx, finalizeCtx, ts)
		if err != nil {
			c.metrics.IncEncoderError()
			c.logger.Printf("camera-recorder camera=%s open segment: %v", c.idText, err)
			return
		}
		st.seg = seg
	}
	if err := st.seg.encoder.WriteJPEG(f.Payload); err != nil {
		// A failing encoder cannot be recovered, so the partial segment is thrown
		// away and the next surviving frame starts a fresh one.
		c.metrics.IncEncoderError()
		c.logger.Printf("camera-recorder camera=%s write frame: %v", c.idText, err)
		c.discardSegment(finalizeCtx, st.seg)
		st.seg = nil
		return
	}
	st.seg.lastRecorded = ts
	c.metrics.IncFramesRecorded(c.idText)
	if c.shouldRotate(st.seg, ts) {
		c.rotate(finalizeCtx, st)
	}
}

// rotate finishes the open segment and clears it, so the next frame starts a new
// one. Closing and clearing belong together: writing to a finished segment would
// write to a closed encoder.
func (c *Camera) rotate(ctx context.Context, st *state) {
	c.finishSegment(ctx, st.seg)
	st.seg = nil
}

// keepEvery is how much incoming footage each recorded frame stands for. A
// timelapse factor of 10 with an output rate of 10 frames per second keeps one
// frame per second of real time, because a second of playback (10 frames) then
// covers 10 seconds of real time.
func (c *Camera) keepEvery() time.Duration {
	return time.Second * time.Duration(c.cfg.TimelapseFactor) / time.Duration(c.cfg.OutputFPS)
}

func shouldKeep(ts time.Time, next *time.Time, interval time.Duration) bool {
	if next.IsZero() {
		*next = ts.Add(interval)
		return true
	}
	if ts.Before(*next) {
		return false
	}
	for !next.After(ts) {
		*next = next.Add(interval)
	}
	return true
}

func (c *Camera) shouldRotate(seg *activeSegment, now time.Time) bool {
	if seg == nil {
		return false
	}
	if now.Sub(seg.startedAt) >= c.cfg.SegmentRawDuration {
		return true
	}
	return seg.pending.Writer.BytesWritten() >= c.cfg.MaxFileBytes
}

// openSegment starts a new recording. ctx cancels the store call that creates the
// file; finalizeCtx belongs to the encoder and to the cleanup that runs when the
// encoder fails to start, neither of which should be cut short by shutdown.
func (c *Camera) openSegment(ctx, finalizeCtx context.Context, start time.Time) (*activeSegment, error) {
	pending, err := c.store.BeginSegment(ctx, c.idText, start, c.cfg.OutputFPS, c.cfg.TimelapseFactor)
	if err != nil {
		return nil, err
	}
	encCfg := c.cfg.FFmpeg
	encCfg.OutputFPS = c.cfg.OutputFPS
	enc, err := c.newEncoder(finalizeCtx, encCfg, pending.Writer)
	if err != nil {
		_ = c.store.DiscardSegment(finalizeCtx, pending)
		return nil, err
	}
	c.metrics.IncActiveEncoders()
	c.logger.Printf("camera-recorder camera=%s started segment %s", c.idText, pending.Info.FileName)
	return &activeSegment{pending: pending, encoder: enc, startedAt: start, lastRecorded: start}, nil
}

// finishSegment shuts the encoder down cleanly and hands the finished file to
// the store, which renames it into place and writes its metadata. If any step
// fails the half-written file is thrown away instead of being published.
func (c *Camera) finishSegment(ctx context.Context, seg *activeSegment) {
	if seg == nil {
		return
	}
	c.metrics.DecActiveEncoders()
	if err := seg.encoder.Close(); err != nil {
		c.abortSegment(ctx, seg, "finalize encoder", err)
		return
	}
	if err := seg.pending.Writer.Close(); err != nil {
		c.abortSegment(ctx, seg, "close segment writer", err)
		return
	}
	info, err := c.store.CompleteSegment(ctx, seg.pending, seg.lastRecorded)
	if err != nil {
		c.abortSegment(ctx, seg, "complete segment", err)
		return
	}
	c.metrics.IncSegmentCompleted(c.idText)
	if err := c.store.Prune(ctx, c.idText, c.cfg.KeepFilesPerCamera); err != nil {
		c.logger.Printf("camera-recorder camera=%s prune: %v", c.idText, err)
	}
	c.logger.Printf("camera-recorder camera=%s completed %s size=%d", c.idText, info.FileName, info.SizeBytes)
}

// discardSegment throws a segment away: the encoder is killed rather than asked
// to finish, and the temporary file is deleted. Used when the recording is known
// to be broken, so waiting for ffmpeg to flush would gain nothing.
func (c *Camera) discardSegment(ctx context.Context, seg *activeSegment) {
	if seg == nil {
		return
	}
	c.metrics.DecActiveEncoders()
	seg.encoder.Kill()
	_ = c.store.DiscardSegment(ctx, seg.pending)
}

// abortSegment records that finishing a segment failed at the named stage and
// deletes the temporary file. stage appears verbatim in the log line, so it
// should read as a short description of what was being attempted.
func (c *Camera) abortSegment(ctx context.Context, seg *activeSegment, stage string, err error) {
	c.metrics.IncEncoderError()
	c.logger.Printf("camera-recorder camera=%s %s: %v", c.idText, stage, err)
	_ = c.store.DiscardSegment(ctx, seg.pending)
}
