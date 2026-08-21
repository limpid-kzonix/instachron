package frameipc

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// newTestClient builds a Client whose dial returns one end of an in-memory
// net.Pipe instead of opening a real Unix socket, and returns the other end for
// the test to write messages into. Only the first dial succeeds; every later
// attempt reports a connection failure, so Run makes exactly one pass over the
// pipe and then spins harmlessly on the reconnect delay until the test is done.
//
// net.Pipe is synchronous and unbuffered: a Write on the server end blocks
// until the Client reads it, which is what lets the tests below assert on
// callback effects without polling or sleeping.
func newTestClient(t *testing.T, h Handler) (*Client, net.Conn) {
	t.Helper()

	server, client := net.Pipe()
	c := NewClient("/test/not-a-real-socket", h, log.New(io.Discard, "", 0))

	var once sync.Once
	c.dial = func(context.Context) (net.Conn, error) {
		var conn net.Conn
		once.Do(func() { conn = client })
		if conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	}
	return c, server
}

// runClient starts c.Run in the background and returns a function that stops it
// and waits for the goroutine to exit, so a leftover Run cannot outlive the test.
func runClient(t *testing.T, c *Client) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()

	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run did not return after context cancellation")
		}
	}
}

func TestClientDispatch(t *testing.T) {
	type call struct {
		typ      Kind
		cameraID streamproto.CameraID
		payload  []byte
	}

	tests := []struct {
		name string
		msg  Frame
		want call
	}{
		{
			name: "frame is dispatched to OnFrame",
			msg:  Frame{Kind: KindFrame, CameraID: 42, Payload: []byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}},
			want: call{typ: KindFrame, cameraID: 42, payload: []byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}},
		},
		{
			name: "offline is dispatched to OnOffline",
			msg:  Frame{Kind: KindOffline, CameraID: 7},
			want: call{typ: KindOffline, cameraID: 7},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Buffered by one so the callback never blocks on an unread channel.
			got := make(chan call, 1)
			h := Handler{
				OnFrame: func(cameraID streamproto.CameraID, jpeg []byte) {
					got <- call{typ: KindFrame, cameraID: cameraID, payload: jpeg}
				},
				OnOffline: func(cameraID streamproto.CameraID) {
					got <- call{typ: KindOffline, cameraID: cameraID}
				},
			}

			c, server := newTestClient(t, h)
			stop := runClient(t, c)
			defer stop()

			if err := Write(server, tc.msg); err != nil {
				t.Fatalf("Write: %v", err)
			}

			select {
			case c := <-got:
				if c.typ != tc.want.typ {
					t.Errorf("callback type = 0x%02x, want 0x%02x", c.typ, tc.want.typ)
				}
				if c.cameraID != tc.want.cameraID {
					t.Errorf("cameraID = %d, want %d", c.cameraID, tc.want.cameraID)
				}
				if !bytes.Equal(c.payload, tc.want.payload) {
					t.Errorf("payload = %v, want %v", c.payload, tc.want.payload)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for callback")
			}
		})
	}
}

func TestClientDispatchesMessagesInOrder(t *testing.T) {
	got := make(chan streamproto.CameraID, 3)
	h := Handler{
		OnFrame:   func(cameraID streamproto.CameraID, _ []byte) { got <- cameraID },
		OnOffline: func(cameraID streamproto.CameraID) { got <- cameraID },
	}

	c, server := newTestClient(t, h)
	stop := runClient(t, c)
	defer stop()

	msgs := []Frame{
		{Kind: KindFrame, CameraID: 1, Payload: []byte("frame1")},
		{Kind: KindOffline, CameraID: 2},
		{Kind: KindFrame, CameraID: 3, Payload: []byte("frame3")},
	}
	for i, m := range msgs {
		if err := Write(server, m); err != nil {
			t.Fatalf("Write msg %d: %v", i, err)
		}
	}

	for i, want := range msgs {
		select {
		case id := <-got:
			if id != want.CameraID {
				t.Errorf("callback %d: cameraID = %d, want %d", i, id, want.CameraID)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for callback %d", i)
		}
	}
}

func TestClientOnDisconnectFiresOnceWhenConnectionCloses(t *testing.T) {
	frames := make(chan struct{}, 1)
	disconnects := make(chan struct{}, 8)
	h := Handler{
		OnFrame:      func(streamproto.CameraID, []byte) { frames <- struct{}{} },
		OnDisconnect: func() { disconnects <- struct{}{} },
	}

	c, server := newTestClient(t, h)
	stop := runClient(t, c)
	defer stop()

	// Send one frame first so the test knows the client is inside the read loop
	// and not still waiting to be dialled.
	if err := Write(server, Frame{Kind: KindFrame, CameraID: 1, Payload: []byte("x")}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case <-frames:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first frame")
	}

	_ = server.Close()

	select {
	case <-disconnects:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnDisconnect")
	}

	// A second OnDisconnect within the reconnect delay would mean the closed
	// connection was reported more than once. Later reconnect attempts fail and
	// legitimately fire OnDisconnect again, so the window checked here stays
	// shorter than reconnDelay.
	select {
	case <-disconnects:
		t.Fatal("OnDisconnect fired more than once for a single closed connection")
	case <-time.After(reconnDelay / 2):
	}
}

func TestClientUnknownMessageTypeIsSkipped(t *testing.T) {
	got := make(chan streamproto.CameraID, 1)
	h := Handler{
		OnFrame: func(cameraID streamproto.CameraID, _ []byte) { got <- cameraID },
		OnOffline: func(cameraID streamproto.CameraID) {
			t.Errorf("OnOffline called for camera %d, want no callback", cameraID)
		},
	}

	c, server := newTestClient(t, h)
	stop := runClient(t, c)
	defer stop()

	// An unrecognised type must be logged and stepped over, leaving the stream
	// aligned so the following frame still decodes.
	if err := Write(server, Frame{Kind: 0x7F, CameraID: 99, Payload: []byte("mystery")}); err != nil {
		t.Fatalf("Write unknown: %v", err)
	}
	if err := Write(server, Frame{Kind: KindFrame, CameraID: 5, Payload: []byte("frame")}); err != nil {
		t.Fatalf("Write frame: %v", err)
	}

	select {
	case id := <-got:
		if id != 5 {
			t.Errorf("cameraID = %d, want 5", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the frame after an unknown message type")
	}
}
