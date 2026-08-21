package streamproto_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

func TestRoundTrip(t *testing.T) {
	payload := []byte{0xFF, 0xD8, 0x00, 0x01, 0xFF, 0xD9}
	ts := time.Unix(1000, 500)
	f := streamproto.Frame{
		Timestamp: ts,
		Sequence:  42,
		CameraID:  7,
		Payload:   payload,
	}

	var buf bytes.Buffer
	w := streamproto.NewWriter(&buf)
	if err := w.WriteFrame(f); err != nil {
		t.Fatal(err)
	}

	r := streamproto.NewReader(&buf)
	got, err := r.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}

	if got.Sequence != f.Sequence {
		t.Errorf("Sequence: got %d want %d", got.Sequence, f.Sequence)
	}
	if got.CameraID != f.CameraID {
		t.Errorf("CameraID: got %d want %d", got.CameraID, f.CameraID)
	}
	if !got.Timestamp.Equal(f.Timestamp) {
		t.Errorf("Timestamp: got %v want %v", got.Timestamp, f.Timestamp)
	}
	if !bytes.Equal(got.Payload, f.Payload) {
		t.Errorf("Payload mismatch: got %v want %v", got.Payload, f.Payload)
	}
}

func TestMultipleFramesRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := streamproto.NewWriter(&buf)
	r := streamproto.NewReader(&buf)

	for i := uint64(0); i < 5; i++ {
		f := streamproto.Frame{
			Timestamp: time.Now(),
			Sequence:  i,
			CameraID:  streamproto.CameraID(i % 3),
			Payload:   []byte{0xFF, 0xD8, byte(i), 0xFF, 0xD9},
		}
		if err := w.WriteFrame(f); err != nil {
			t.Fatalf("write frame %d: %v", i, err)
		}
		got, err := r.ReadFrame()
		if err != nil {
			t.Fatalf("read frame %d: %v", i, err)
		}
		if got.Sequence != f.Sequence || got.CameraID != f.CameraID {
			t.Errorf("frame %d: got seq=%d cam=%d want seq=%d cam=%d",
				i, got.Sequence, got.CameraID, f.Sequence, f.CameraID)
		}
	}
}

func TestInvalidMagic(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(make([]byte, 40)) // all zeros — bad magic
	r := streamproto.NewReader(&buf)
	_, err := r.ReadFrame()
	if err != streamproto.ErrInvalidMagic {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestEmptyPayloadWrite(t *testing.T) {
	var buf bytes.Buffer
	w := streamproto.NewWriter(&buf)
	err := w.WriteFrame(streamproto.Frame{Timestamp: time.Now()})
	if err != streamproto.ErrEmptyPayload {
		t.Fatalf("expected ErrEmptyPayload, got %v", err)
	}
}

func TestFrameTooLargeWrite(t *testing.T) {
	var buf bytes.Buffer
	w := streamproto.NewWriter(&buf)
	f := streamproto.Frame{
		Timestamp: time.Now(),
		Payload:   make([]byte, streamproto.DefaultMaxFrameSize+1),
	}
	if err := w.WriteFrame(f); err != streamproto.ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestTruncatedHeader(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{'M', 'J', 'P', 'G'}) // only 4 bytes, header needs 32
	r := streamproto.NewReader(&buf)
	_, err := r.ReadFrame()
	if err == nil {
		t.Fatal("expected error on truncated header, got nil")
	}
}
