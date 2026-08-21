package storage

import (
	"context"
	"io"
	"time"
)

// SegmentInfo is the metadata describing one finished recording file. It is
// stored next to the video as JSON and served to API clients.
type SegmentInfo struct {
	CameraID        string    `json:"camera_id"`
	FileName        string    `json:"file_name"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	RawDurationSec  float64   `json:"raw_duration_sec"`
	TimelapseFactor int       `json:"timelapse_factor"`
	OutputFPS       int       `json:"output_fps"`
	SizeBytes       int64     `json:"size_bytes"`
	RelativePath    string    `json:"relative_path"`
	DownloadURL     string    `json:"download_url,omitempty"`
}

// SegmentWriter is the sink an encoder writes a segment's video bytes into. It
// also reports the running byte count, which drives the max-file-size rotation.
type SegmentWriter interface {
	io.WriteCloser
	BytesWritten() int64
}

// PendingSegment is a recording that has been opened but not yet finished: its
// metadata so far, plus the writer the encoder feeds.
type PendingSegment struct {
	Info   SegmentInfo
	Writer SegmentWriter
}

// ListFilter narrows a Store.List query. The zero value matches every segment
// of every camera.
type ListFilter struct {
	CameraID string
	From     time.Time
	To       time.Time
	Limit    int
}

// ReadSeekCloser is a readable stream that supports seeking, which HTTP range
// requests need in order to serve part of a video file.
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// Store is where recorded segments live. Local is the only implementation.
//
// The three segment methods form a protocol: BeginSegment opens a segment and
// returns a *PendingSegment, which must then be passed to exactly one of
// CompleteSegment or DiscardSegment. Passing it to neither leaves a temporary
// file behind on disk; passing it to both, or to either one twice, operates on
// a segment that no longer exists.
type Store interface {
	BeginSegment(ctx context.Context, cameraID string, start time.Time, outputFPS, timelapseFactor int) (*PendingSegment, error)
	CompleteSegment(ctx context.Context, segment *PendingSegment, end time.Time) (SegmentInfo, error)
	DiscardSegment(ctx context.Context, segment *PendingSegment) error
	Prune(ctx context.Context, cameraID string, keep int) error
	List(ctx context.Context, filter ListFilter) ([]SegmentInfo, error)
	Open(ctx context.Context, cameraID, fileName string) (ReadSeekCloser, SegmentInfo, error)
	UsageBytes(ctx context.Context) (int64, error)
	Cameras(ctx context.Context) ([]string, error)
}
