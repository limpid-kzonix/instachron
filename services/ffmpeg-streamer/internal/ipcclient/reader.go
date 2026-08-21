// Package ipcclient connects to the tcp-camera-backend IPC Unix socket and
// maintains the latest JPEG frame for each active camera. Used by ffmpeg-streamer
// to pull the current frame for each camera at a configured frame rate.
//
// The connection lifecycle — dialling, reading, reconnecting — lives in
// shared/frameipc. What is local to this service is the cache: camera-web-api
// pushes each frame onward as it arrives, whereas ffmpeg-streamer is polled and
// so has to hold on to the most recent frame per camera.
package ipcclient

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	"github.com/w0rxbend/instachron/shared/streamproto"

	"github.com/w0rxbend/instachron/shared/frameipc"
)

// Reader connects to the IPC socket and caches the latest JPEG per camera.
// It reconnects automatically and clears all frames when the connection is lost.
type Reader struct {
	client *frameipc.Client

	mu      sync.RWMutex
	frames  map[streamproto.CameraID][]byte
	version atomic.Uint64
}

// New returns a Reader for the given socket path.
func New(socketPath string, logger *log.Logger) *Reader {
	r := &Reader{frames: make(map[streamproto.CameraID][]byte)}
	r.client = frameipc.NewClient(socketPath, frameipc.Handler{
		OnFrame:      r.storeFrame,
		OnOffline:    r.dropFrame,
		OnDisconnect: r.dropAllFrames,
	}, logger)
	return r
}

// Run is the reconnect loop. It blocks until ctx is cancelled.
func (r *Reader) Run(ctx context.Context) {
	r.client.Run(ctx)
}

func (r *Reader) storeFrame(cameraID streamproto.CameraID, jpeg []byte) {
	r.mu.Lock()
	r.frames[cameraID] = jpeg
	r.mu.Unlock()
	r.version.Add(1)
}

func (r *Reader) dropFrame(cameraID streamproto.CameraID) {
	r.mu.Lock()
	delete(r.frames, cameraID)
	r.mu.Unlock()
	r.version.Add(1)
}

func (r *Reader) dropAllFrames() {
	// Lost the connection — wipe frames so ffmpeg doesn't loop stale data.
	r.mu.Lock()
	r.frames = make(map[streamproto.CameraID][]byte)
	r.mu.Unlock()
	r.version.Add(1)
}

// Latest returns the most recent JPEG for a single camera, or nil if none.
func (r *Reader) Latest(cameraID streamproto.CameraID) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frames[cameraID]
}

// AllLatest returns a snapshot of the latest JPEG for every active camera.
func (r *Reader) AllLatest() map[streamproto.CameraID][]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make(map[streamproto.CameraID][]byte, len(r.frames))
	for id, f := range r.frames {
		cp[id] = f
	}
	return cp
}

// CurrentVersion returns a monotonically increasing counter that increments
// whenever any frame changes. Use it to avoid redundant canvas recompositions.
func (r *Reader) CurrentVersion() uint64 {
	return r.version.Load()
}
