package livefeed

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Registry owns all per-camera Hubs. Cameras are added lazily and never
// removed, so offline cameras remain discoverable via the API.
type Registry struct {
	mu      sync.RWMutex
	hubs    map[string]*Hub
	ordered []string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{hubs: make(map[string]*Hub)}
}

// Hub returns the hub for id, creating it if this is the first time the camera
// has been seen. Stream handlers use this so a client can attach to a camera
// that has not delivered its first frame yet.
func (r *Registry) Hub(id string) *Hub {
	return r.getOrCreate(id)
}

// Lookup returns the Hub for id, or nil if it has never been seen.
func (r *Registry) Lookup(id string) *Hub {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hubs[id]
}

// MarkOffline marks a single camera as offline. Unknown ids are ignored.
func (r *Registry) MarkOffline(id string) {
	if h := r.Lookup(id); h != nil {
		h.MarkOffline()
	}
}

// MarkAllOffline marks every known camera as offline.
func (r *Registry) MarkAllOffline() {
	for _, h := range r.snapshotHubs() {
		h.MarkOffline()
	}
}

// CheckLiveness marks cameras offline when they exceed the silence threshold.
// Designed to be called from a periodic ticker goroutine.
func (r *Registry) CheckLiveness() {
	for _, h := range r.snapshotHubs() {
		if h.IsStale() {
			h.MarkOffline()
		}
	}
}

// RunLiveness calls CheckLiveness every interval until ctx is cancelled.
// Run it in its own goroutine: it blocks for the lifetime of the process.
func (r *Registry) RunLiveness(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.CheckLiveness()
		}
	}
}

// Push delivers a JPEG frame to the Hub for id, creating it lazily on first call.
func (r *Registry) Push(id string, jpeg []byte) {
	r.getOrCreate(id).Push(jpeg)
}

// KnownCameras returns camera info for every camera seen since startup, sorted
// by ID so the order is stable across calls. Index is the entry's position in
// that sorted list. rotation supplies the configured rotation angle for a camera
// ID; pass nil when the service has no rotation configuration, which reports
// every camera as rotation 0.
func (r *Registry) KnownCameras(rotation func(id string) int) []Info {
	// One read lock covers the whole snapshot so the ids and the hub pointers
	// they refer to come from the same moment in time.
	r.mu.RLock()
	entries := make([]cameraEntry, 0, len(r.ordered))
	for _, id := range r.ordered {
		entries = append(entries, cameraEntry{id: id, hub: r.hubs[id]})
	}
	r.mu.RUnlock()

	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })

	infos := make([]Info, 0, len(entries))
	for i, e := range entries {
		rot := 0
		if rotation != nil {
			rot = rotation(e.id)
		}
		infos = append(infos, e.hub.info(i, rot))
	}
	return infos
}

// cameraEntry pairs a camera ID with its hub so both can be snapshotted from
// the map under a single lock and then sorted together.
type cameraEntry struct {
	id  string
	hub *Hub
}

// getOrCreate returns the hub for id, creating and registering it when absent.
func (r *Registry) getOrCreate(id string) *Hub {
	r.mu.Lock()
	defer r.mu.Unlock()

	if h, ok := r.hubs[id]; ok {
		return h
	}
	h := newHub(id)
	r.hubs[id] = h
	r.ordered = append(r.ordered, id)
	return h
}

// snapshotHubs copies the current set of hubs so callers can act on each one
// without holding the registry lock while they do it.
func (r *Registry) snapshotHubs() []*Hub {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hubs := make([]*Hub, 0, len(r.hubs))
	for _, h := range r.hubs {
		hubs = append(hubs, h)
	}
	return hubs
}
