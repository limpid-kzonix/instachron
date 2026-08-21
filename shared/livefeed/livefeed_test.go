package livefeed_test

import (
	"testing"
	"time"

	"github.com/w0rxbend/instachron/shared/livefeed"
)

// --- Hub tests ---

func TestHubSubscribeReceivesLatestFrame(t *testing.T) {
	h := livefeed.NewRegistry()
	hub := h.Hub("1")

	jpeg := []byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}
	hub.Push(jpeg)

	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	select {
	case got := <-ch:
		if string(got) != string(jpeg) {
			t.Fatalf("got %v, want %v", got, jpeg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subscribe: timeout waiting for initial frame")
	}
}

func TestHubPushFansOut(t *testing.T) {
	mgr := livefeed.NewRegistry()
	hub := mgr.Hub("cam1")

	ch1 := hub.Subscribe()
	ch2 := hub.Subscribe()
	defer hub.Unsubscribe(ch1)
	defer hub.Unsubscribe(ch2)

	// Drain the initial frame sent on subscribe (hub has no prior frame here).
	jpeg := []byte("newframe")
	hub.Push(jpeg)

	for _, ch := range []chan []byte{ch1, ch2} {
		select {
		case got := <-ch:
			if string(got) != string(jpeg) {
				t.Errorf("got %q, want %q", got, jpeg)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("fan-out: timeout waiting for frame")
		}
	}
}

func TestHubUnsubscribeClosesChannel(t *testing.T) {
	mgr := livefeed.NewRegistry()
	hub := mgr.Hub("cam2")

	ch := hub.Subscribe()
	hub.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after Unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed after Unsubscribe")
	}
}

func TestHubMarkOffline(t *testing.T) {
	mgr := livefeed.NewRegistry()
	hub := mgr.Hub("cam3")

	hub.Push([]byte("frame"))
	if !hub.Online() {
		t.Fatal("hub should be online after push")
	}

	hub.MarkOffline()
	if hub.Online() {
		t.Fatal("hub should be offline after MarkOffline")
	}
}

func TestHubIsStale(t *testing.T) {
	mgr := livefeed.NewRegistry()
	hub := mgr.Hub("cam4")

	// Hub that has never received a frame is not stale.
	if hub.IsStale() {
		t.Fatal("never-pushed hub should not be stale")
	}

	hub.Push([]byte("frame"))
	// Just pushed — not stale yet.
	if hub.IsStale() {
		t.Fatal("hub should not be stale immediately after push")
	}
}

func TestHubLatestFrame(t *testing.T) {
	mgr := livefeed.NewRegistry()
	hub := mgr.Hub("cam5")

	if hub.LatestFrame() != nil {
		t.Fatal("LatestFrame should be nil before any push")
	}

	jpeg := []byte("jpeg_data")
	hub.Push(jpeg)
	if string(hub.LatestFrame()) != string(jpeg) {
		t.Fatalf("LatestFrame = %q, want %q", hub.LatestFrame(), jpeg)
	}
}

// --- Registry tests ---

func TestManagerLookup(t *testing.T) {
	mgr := livefeed.NewRegistry()
	if mgr.Lookup("missing") != nil {
		t.Fatal("Lookup for unknown id should return nil")
	}
	mgr.Hub("known")
	if mgr.Lookup("known") == nil {
		t.Fatal("Lookup for known id should return non-nil")
	}
}

func TestManagerMarkAllOffline(t *testing.T) {
	mgr := livefeed.NewRegistry()
	h1 := mgr.Hub("a")
	h2 := mgr.Hub("b")
	h1.Push([]byte("f"))
	h2.Push([]byte("f"))

	mgr.MarkAllOffline()

	if h1.Online() || h2.Online() {
		t.Fatal("all cameras should be offline after MarkAllOffline")
	}
}

func TestManagerMarkOffline(t *testing.T) {
	mgr := livefeed.NewRegistry()
	hub := mgr.Hub("a")
	hub.Push([]byte("f"))

	mgr.MarkOffline("a")
	if hub.Online() {
		t.Fatal("camera should be offline after MarkOffline")
	}

	// An id that was never seen must not panic or create a hub.
	mgr.MarkOffline("never-seen")
	if mgr.Lookup("never-seen") != nil {
		t.Fatal("MarkOffline should not create a hub for an unknown id")
	}
}

func TestManagerKnownCameras(t *testing.T) {
	// Frames arrive for camera "2" before camera "1" in every case, so the
	// returned order proves the result is sorted by ID rather than by arrival.
	tests := []struct {
		name     string
		rotation func(id string) int
		want     []want
	}{
		{
			name:     "nil rotation function reports every camera as 0 degrees",
			rotation: nil,
			want: []want{
				{id: "1", index: 0, rotation: 0},
				{id: "2", index: 1, rotation: 0},
			},
		},
		{
			name:     "rotation function is consulted per camera ID",
			rotation: func(id string) int { return map[string]int{"1": 90, "2": 180}[id] },
			want: []want{
				{id: "1", index: 0, rotation: 90},
				{id: "2", index: 1, rotation: 180},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := livefeed.NewRegistry()
			mgr.Push("2", []byte("frame"))
			mgr.Push("1", []byte("frame"))

			cams := mgr.KnownCameras(tt.rotation)
			if len(cams) != len(tt.want) {
				t.Fatalf("KnownCameras len = %d, want %d", len(cams), len(tt.want))
			}
			for i, w := range tt.want {
				got := cams[i]
				if got.ID != w.id || got.Index != w.index || got.Rotation != w.rotation {
					t.Errorf("cams[%d] = {ID:%s Index:%d Rotation:%d}, want {ID:%s Index:%d Rotation:%d}",
						i, got.ID, got.Index, got.Rotation, w.id, w.index, w.rotation)
				}
				if !got.Online {
					t.Errorf("cams[%d] (%s) should be online after a push", i, got.ID)
				}
			}
		})
	}
}

// want is the expected shape of one entry returned by KnownCameras.
type want struct {
	id       string
	index    int
	rotation int
}
