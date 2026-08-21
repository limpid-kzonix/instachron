package recorder

import (
	"context"
	"io"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
)

// Encoder is the part of a video encoder that the recording loop actually uses.
//
// It is declared here, in the package that consumes it, rather than in the
// encoder package that implements it. That is the Go convention, and it earns
// its keep here for a concrete reason: the real implementation launches an
// ffmpeg subprocess, so with a direct dependency the only way to exercise the
// segment lifecycle — open, write, rotate, finish, discard — is to have ffmpeg
// installed and to let it write real files. With this interface a test can
// supply a few lines of fake instead, and the lifecycle can be tested for its
// own logic rather than for whether ffmpeg is on the machine.
type Encoder interface {
	// WriteJPEG appends one frame to the video being encoded.
	WriteJPEG(jpeg []byte) error
	// Close finishes the video cleanly, flushing whatever is buffered. An error
	// means the output cannot be trusted and the segment must be thrown away.
	Close() error
	// Kill abandons the encoding without finishing it, for a segment that is
	// already known to be unusable.
	Kill()
}

// EncoderFactory starts a new encoder writing into out.
//
// ctx belongs to the encoder process and deliberately outlives a shutdown
// request, so that a segment in progress can still be finished rather than
// truncated. cfg carries the ffmpeg settings, including the output frame rate
// the caller has already resolved.
type EncoderFactory func(ctx context.Context, cfg encoder.Config, out io.Writer) (Encoder, error)

// startFFmpeg is the production EncoderFactory: it launches a real ffmpeg
// subprocess. It exists as a named function because encoder.Start returns a
// concrete *encoder.FFmpeg, and a function returning a concrete type cannot be
// assigned to one returning an interface — the conversion has to be written out.
func startFFmpeg(ctx context.Context, cfg encoder.Config, out io.Writer) (Encoder, error) {
	return encoder.Start(ctx, cfg, out)
}
