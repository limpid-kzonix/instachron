package usage

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sync"
	"testing"
	"time"
)

// fakeMeasurer stands in for a storage backend. Extracting the loop into this
// package is what makes it substitutable: the loop used to sit in run.go welded
// to a real directory on disk.
type fakeMeasurer struct {
	mu     sync.Mutex
	values []int64
	errs   []error
	calls  int
}

func (f *fakeMeasurer) UsageBytes(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return 0, f.errs[i]
	}
	if i < len(f.values) {
		return f.values[i], nil
	}
	return 0, nil
}

func (f *fakeMeasurer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeReporter struct {
	mu   sync.Mutex
	seen []int64
}

func (r *fakeReporter) SetStorageBytes(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, n)
}

func (r *fakeReporter) values() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.seen...)
}

func TestRunMeasuresBeforeWaiting(t *testing.T) {
	m := &fakeMeasurer{values: []int64{4096}}
	r := &fakeReporter{}
	logger := log.New(bytes.NewBuffer(nil), "", 0)

	// A long interval guarantees the ticker never fires, so anything reported
	// must have come from the measurement taken before the first wait.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, m, r, time.Hour, logger)
		close(done)
	}()

	waitFor(t, func() bool { return len(r.values()) == 1 })
	cancel()
	<-done

	got := r.values()
	if len(got) != 1 || got[0] != 4096 {
		t.Errorf("reported %v, want [4096] from the startup measurement", got)
	}
}

func TestRunKeepsGoingAfterAFailedMeasurement(t *testing.T) {
	m := &fakeMeasurer{
		errs:   []error{errors.New("disk went away"), nil},
		values: []int64{0, 512},
	}
	r := &fakeReporter{}
	var logged bytes.Buffer
	logger := log.New(&logged, "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, m, r, time.Millisecond, logger)
		close(done)
	}()

	waitFor(t, func() bool { return len(r.values()) >= 1 })
	cancel()
	<-done

	// The first measurement failed, so nothing was published for it; the gauge
	// keeps its old value rather than being reset to zero.
	got := r.values()
	if len(got) == 0 || got[0] != 512 {
		t.Errorf("reported %v, want the first successful value 512 first", got)
	}
	if !bytes.Contains(logged.Bytes(), []byte("disk went away")) {
		t.Errorf("the failure was not logged; log was %q", logged.String())
	}
}

func TestRunDoesNotLogFailuresDuringShutdown(t *testing.T) {
	m := &fakeMeasurer{errs: []error{errors.New("interrupted by shutdown")}}
	r := &fakeReporter{}
	var logged bytes.Buffer
	logger := log.New(&logged, "", 0)

	// A context that is already cancelled is what the loop sees when the
	// process is stopping mid-measurement.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Run(ctx, m, r, time.Millisecond, logger)

	if logged.Len() != 0 {
		t.Errorf("a failure during shutdown was logged: %q", logged.String())
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	m := &fakeMeasurer{}
	r := &fakeReporter{}
	logger := log.New(bytes.NewBuffer(nil), "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, m, r, time.Millisecond, logger)
		close(done)
	}()

	waitFor(t, func() bool { return m.callCount() > 0 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

func TestRunRejectsANonPositiveInterval(t *testing.T) {
	// A zero interval would make time.NewTicker panic, so Run substitutes the
	// default rather than crashing the service on a misconfigured value.
	m := &fakeMeasurer{}
	r := &fakeReporter{}
	logger := log.New(bytes.NewBuffer(nil), "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Run(ctx, m, r, 0, logger) // must return rather than panic
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for the expected condition")
}
