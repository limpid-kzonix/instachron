// Package dropchan implements the one queueing policy every live-video path in
// this repository needs: never block the producer, and when the queue is full
// throw away the oldest item rather than the newest.
//
// The reasoning is specific to live video and does not generalise to queues in
// general. A camera produces frames whether or not anything is keeping up. If a
// consumer falls behind, blocking the producer stalls every other consumer too,
// and dropping the *newest* frame means the viewer keeps being shown stale
// pictures while fresh ones are discarded. Dropping the oldest keeps the queue
// full of the most recent frames, so a consumer that recovers resumes at
// something close to live instead of replaying a backlog.
package dropchan

// Outcome describes what happened to a value passed to Send.
type Outcome int

const (
	// Delivered means the value went into the channel with nothing displaced:
	// the consumer is keeping up.
	Delivered Outcome = iota
	// ReplacedOldest means the channel was full, the oldest queued value was
	// discarded, and the new value took its place. One value was lost.
	ReplacedOldest
	// Dropped means the channel was full and the new value could not be queued
	// even after making room, so the new value itself was lost. This happens
	// when another producer refills the channel in the instant between the two
	// steps, and is expected to be rare.
	Dropped
)

// Send delivers v to ch without ever blocking, discarding the oldest queued
// value if that is what it takes, and reports which of the three things
// happened so the caller can count losses.
//
// ch is a bidirectional channel rather than a send-only one because the
// drop-oldest step requires receiving: making room means taking the oldest
// value out, and a send-only channel cannot be received from.
//
// Send is safe to call from several goroutines at once, but note that with
// concurrent producers the steps below are not atomic with respect to one
// another — another producer may take or add a value in between. That costs an
// occasional extra drop under contention and never corrupts anything, which is
// the trade this package accepts in exchange for not holding a lock across the
// send.
//
// Sending on a closed channel panics, exactly as a plain channel send does, so
// callers must still make sure every producer has stopped before closing ch.
func Send[T any](ch chan T, v T) Outcome {
	// The ordinary case: the consumer is keeping up and nothing is displaced.
	select {
	case ch <- v:
		return Delivered
	default:
	}

	// Full, so discard the oldest queued value to make room. This receive is
	// itself non-blocking, because the consumer may have drained the channel in
	// the meantime — in which case there is nothing to discard and the send
	// below simply succeeds.
	displaced := false
	select {
	case <-ch:
		displaced = true
	default:
	}

	select {
	case ch <- v:
		if displaced {
			return ReplacedOldest
		}
		return Delivered
	default:
		// Another producer refilled the channel in between, so the new value is
		// lost too — along with the one displaced above, if there was one.
		return Dropped
	}
}

// Lost reports whether the outcome means a value was thrown away, which is the
// question every caller that keeps a dropped-frame counter actually asks.
func (o Outcome) Lost() bool { return o != Delivered }

// String returns a short name for the outcome, for logs and test failures.
func (o Outcome) String() string {
	switch o {
	case Delivered:
		return "delivered"
	case ReplacedOldest:
		return "replaced-oldest"
	case Dropped:
		return "dropped"
	default:
		return "unknown"
	}
}
