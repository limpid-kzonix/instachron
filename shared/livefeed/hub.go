package livefeed

import (
	"sync"
	"sync/atomic"
	"time"
)

// offlineThreshold is how long a camera may stay silent after its last frame
// before it is reported as offline.
const offlineThreshold = 5 * time.Second

// Hub fans received frames out to all active HTTP stream subscribers.
// The latest frame and liveness state are stored via atomics for lock-free reads
// on the hot path; subscriber management uses a mutex.
type Hub struct {
	id string

	latest   atomic.Pointer[[]byte]
	lastSeen atomic.Int64
	online   atomic.Bool

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newHub(id string) *Hub {
	return &Hub{
		id:   id,
		subs: make(map[chan []byte]struct{}),
	}
}

// subBufSize is the per-subscriber channel buffer. A larger buffer absorbs the
// transient delay between subscription and the handler goroutine starting its
// read loop, preventing frames from being dropped before the stream is visible.
const subBufSize = 8

// Subscribe registers a new subscriber channel. The latest known frame is sent
// immediately so the client renders without waiting for the next push.
// Caller must call Unsubscribe when done.
func (h *Hub) Subscribe() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan []byte, subBufSize)
	h.subs[ch] = struct{}{}

	if p := h.latest.Load(); p != nil {
		select {
		case ch <- *p:
		default:
		}
	}
	return ch
}

// Unsubscribe removes the subscriber and closes its channel.
func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

// Push stores jpeg as the latest frame and fans it out to all subscribers.
// Slow subscribers get the frame dropped rather than blocking the push path.
func (h *Hub) Push(jpeg []byte) {
	h.latest.Store(&jpeg)
	h.lastSeen.Store(time.Now().UnixNano())
	h.online.Store(true)

	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- jpeg:
		default:
		}
	}
	h.mu.Unlock()
}

// MarkOffline records that the camera is no longer sending frames.
func (h *Hub) MarkOffline() {
	h.online.Store(false)
}

// Online reports whether the camera is currently considered to be sending frames.
func (h *Hub) Online() bool {
	return h.online.Load()
}

// IsStale returns true if the camera was online but has been silent beyond offlineThreshold.
func (h *Hub) IsStale() bool {
	if !h.online.Load() {
		return false
	}
	ls := h.lastSeen.Load()
	return ls != 0 && time.Since(time.Unix(0, ls)) > offlineThreshold
}

// info builds the API state for this camera. Index and rotation are supplied by
// the caller because the hub itself has no way to know either: the index is the
// camera's position in the sorted list being built, and the rotation angle comes
// from whatever configuration the surrounding service holds.
func (h *Hub) info(index, rotation int) Info {
	return Info{ID: h.id, Index: index, Online: h.online.Load(), Rotation: rotation}
}

// LatestFrame returns the most recently received JPEG, or nil if none yet.
func (h *Hub) LatestFrame() []byte {
	if p := h.latest.Load(); p != nil {
		return *p
	}
	return nil
}
