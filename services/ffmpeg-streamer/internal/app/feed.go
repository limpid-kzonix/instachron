package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/w0rxbend/instachron/services/ffmpeg-streamer/internal/compose"
	"github.com/w0rxbend/instachron/services/ffmpeg-streamer/internal/ipcclient"
)

// feedFrames streams one camera's latest JPEG into ffmpeg's stdin.
func feedFrames(ctx context.Context, cfg config, ipc *ipcclient.Reader, writer io.Writer) error {
	return pump(ctx, cfg.frameRate, writer, func() []byte {
		return ipc.Latest(cfg.cameraID)
	}, "frame")
}

// feedMergedFrames streams a grid of every camera's latest JPEG into ffmpeg's stdin.
func feedMergedFrames(ctx context.Context, cfg config, ipc *ipcclient.Reader, writer io.Writer, logger *log.Logger) error {
	return pump(ctx, cfg.frameRate, writer, mergedCanvasSource(cfg, ipc, logger), "merged canvas")
}

// pump writes whatever next returns into w on a fixed clock, one write per
// 1/frameRate of a second, until ctx is cancelled. ffmpeg is told to read its
// input at that same rate, so the pacing here is what keeps the outgoing stream
// running at real-time speed. A tick where next returns no bytes is skipped
// rather than written, because ffmpeg would reject a zero-length frame. The
// what argument names the payload in the error message ("frame", "merged
// canvas") so a failed write says which feed broke.
func pump(ctx context.Context, frameRate int, w io.Writer, next func() []byte, what string) error {
	frameInterval := time.Second / time.Duration(frameRate)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			payload := next()
			if len(payload) == 0 {
				continue
			}
			if _, err := w.Write(payload); err != nil {
				return fmt.Errorf("write %s to ffmpeg: %w", what, err)
			}
		}
	}
}

// mergedCanvasSource returns a function that hands back the current merged
// canvas as JPEG bytes. Composing the grid means decoding and re-encoding every
// camera's frame, which is far too expensive to redo on every tick, so the
// returned function re-composes only when the IPC reader's version counter has
// moved — that counter changes whenever any camera delivers a new frame or goes
// offline. Between changes it returns the previously composed bytes, and ffmpeg
// receives the same picture again, which is what holds the stream's frame rate
// steady while the cameras are idle.
func mergedCanvasSource(cfg config, ipc *ipcclient.Reader, logger *log.Logger) func() []byte {
	var current []byte
	var lastVersion uint64

	return func() []byte {
		if v := ipc.CurrentVersion(); v != lastVersion {
			if encoded := composeCanvas(cfg, ipc, logger); encoded != nil {
				current = encoded
				lastVersion = v
			}
		}
		return current
	}
}

// composeCanvas builds one merged JPEG from the cameras' latest frames. It
// returns nil when there is nothing to show or the compose failed, in which case
// the caller keeps serving the canvas it already had.
func composeCanvas(cfg config, ipc *ipcclient.Reader, logger *log.Logger) []byte {
	frames := ipc.AllLatest()

	encoded, err := compose.Canvas(frames, cfg.cellWidth, cfg.cellHeight)
	if err != nil {
		logger.Printf("compose failed: %v", err)
		return nil
	}
	if encoded == nil {
		return nil
	}

	logger.Printf("merged canvas: cameras=%d grid bytes=%d", len(frames), len(encoded))
	return encoded
}
