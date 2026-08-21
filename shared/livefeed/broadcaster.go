package livefeed

import (
	"sync"
	"sync/atomic"

	"github.com/w0rxbend/instachron/shared/dropchan"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// Broadcaster fans streamproto frames out to all subscribed channels.
// It stores the latest frame and delivers it immediately to new subscribers.
// Slow subscribers get their oldest buffered frame dropped rather than
// blocking the publish path.
type Broadcaster struct {
	mu     sync.Mutex
	subs   map[chan streamproto.Frame]struct{}
	latest atomic.Pointer[streamproto.Frame]
}

// NewBroadcaster returns an idle Broadcaster with no subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[chan streamproto.Frame]struct{})}
}

// Publish stores f as the latest frame and fans it out to all subscribers.
// Channels that are full have their oldest entry drained before the new frame
// is queued, so subscribers always hold the freshest data.
func (b *Broadcaster) Publish(f streamproto.Frame) {
	b.latest.Store(&f)

	b.mu.Lock()
	for ch := range b.subs {
		// A subscriber that has fallen behind loses its oldest queued frame
		// rather than holding up the fan-out to everyone else. Nothing is
		// counted here: a subscriber is a live viewer, and a viewer skipping a
		// frame is normal rather than an error worth recording.
		dropchan.Send(ch, f)
	}
	b.mu.Unlock()
}

// Subscribe registers a new subscriber. The latest known frame is sent
// immediately so the client renders without waiting for the next push.
// The returned unsubscribe function must be called exactly once when done.
func (b *Broadcaster) Subscribe() (<-chan streamproto.Frame, func()) {
	ch := make(chan streamproto.Frame, 2)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	if p := b.latest.Load(); p != nil {
		select {
		case ch <- *p:
		default:
		}
	}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}
}
