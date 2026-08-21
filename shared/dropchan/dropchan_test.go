package dropchan

import (
	"sync"
	"testing"
)

func TestSendIntoRoomySpaceDelivers(t *testing.T) {
	ch := make(chan int, 2)
	if got := Send(ch, 1); got != Delivered {
		t.Errorf("Send() = %v, want %v", got, Delivered)
	}
	if got := <-ch; got != 1 {
		t.Errorf("received %d, want 1", got)
	}
}

func TestSendOnFullChannelDropsTheOldest(t *testing.T) {
	ch := make(chan int, 2)
	Send(ch, 1)
	Send(ch, 2)

	if got := Send(ch, 3); got != ReplacedOldest {
		t.Errorf("Send() on a full channel = %v, want %v", got, ReplacedOldest)
	}

	// The oldest value, 1, is the one that should be gone.
	var got []int
	for len(ch) > 0 {
		got = append(got, <-ch)
	}
	want := []int{2, 3}
	if len(got) != len(want) {
		t.Fatalf("channel holds %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("channel holds %v, want %v", got, want)
		}
	}
}

func TestSendNeverBlocks(t *testing.T) {
	// An unbuffered channel with no reader is the hardest case: there is no
	// room and nothing to displace, so Send must still return.
	ch := make(chan int)
	done := make(chan Outcome, 1)
	go func() { done <- Send(ch, 1) }()

	got := <-done
	if got != Dropped {
		t.Errorf("Send() on an unbuffered channel with no reader = %v, want %v", got, Dropped)
	}
}

func TestSendKeepsTheNewestUnderSustainedOverload(t *testing.T) {
	const capacity = 4
	ch := make(chan int, capacity)
	for i := range 100 {
		Send(ch, i)
	}

	if len(ch) != capacity {
		t.Fatalf("channel holds %d values, want %d", len(ch), capacity)
	}
	// After 100 sends into a queue of 4, the survivors must be the last 4.
	for want := 96; want < 100; want++ {
		if got := <-ch; got != want {
			t.Errorf("received %d, want %d — the newest values should be the ones kept", got, want)
		}
	}
}

func TestOutcomeLost(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    bool
	}{
		{outcome: Delivered, want: false},
		{outcome: ReplacedOldest, want: true},
		{outcome: Dropped, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.outcome.String(), func(t *testing.T) {
			if got := tt.outcome.Lost(); got != tt.want {
				t.Errorf("%v.Lost() = %v, want %v", tt.outcome, got, tt.want)
			}
		})
	}
}

// TestSendIsSafeForConcurrentProducers is here to be run under -race: it makes
// no claim about which values survive, only that nothing panics or corrupts.
func TestSendIsSafeForConcurrentProducers(t *testing.T) {
	ch := make(chan int, 8)
	var wg sync.WaitGroup

	// One consumer, deliberately slower than the producers.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-ch:
			}
		}
	}()

	for p := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				Send(ch, p*1000+i)
			}
		}()
	}
	wg.Wait()
	close(stop)
}
