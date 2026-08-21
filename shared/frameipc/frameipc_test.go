package frameipc_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/w0rxbend/instachron/shared/frameipc"
)

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  frameipc.Frame
	}{
		{
			name: "frame with payload",
			msg:  frameipc.Frame{Kind: frameipc.KindFrame, CameraID: 42, Payload: []byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}},
		},
		{
			name: "offline with no payload",
			msg:  frameipc.Frame{Kind: frameipc.KindOffline, CameraID: 7},
		},
		{
			name: "camera id zero",
			msg:  frameipc.Frame{Kind: frameipc.KindFrame, CameraID: 0, Payload: []byte{1, 2, 3}},
		},
		{
			name: "max camera id",
			msg:  frameipc.Frame{Kind: frameipc.KindFrame, CameraID: 0xFFFFFFFF, Payload: []byte("jpeg")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := frameipc.Write(&buf, tc.msg); err != nil {
				t.Fatalf("Write: %v", err)
			}

			got, err := frameipc.Read(&buf)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}

			if got.Kind != tc.msg.Kind {
				t.Errorf("Kind = %s, want %s", got.Kind, tc.msg.Kind)
			}
			if got.CameraID != tc.msg.CameraID {
				t.Errorf("CameraID = %d, want %d", got.CameraID, tc.msg.CameraID)
			}
			if !bytes.Equal(got.Payload, tc.msg.Payload) {
				t.Errorf("Payload = %v, want %v", got.Payload, tc.msg.Payload)
			}
		})
	}
}

func TestReadBadMagic(t *testing.T) {
	buf := bytes.Repeat([]byte{0x00}, frameipc.HeaderSize)
	_, err := frameipc.Read(bytes.NewReader(buf))
	if err == nil {
		t.Fatal("Read: expected error for bad magic, got nil")
	}
}

func TestReadShortHeader(t *testing.T) {
	buf := bytes.Repeat([]byte{0xAA}, 5) // too short
	_, err := frameipc.Read(bytes.NewReader(buf))
	if err == nil {
		t.Fatal("Read: expected error for short header, got nil")
	}
}

func TestReadTruncatedPayload(t *testing.T) {
	var buf bytes.Buffer
	msg := frameipc.Frame{Kind: frameipc.KindFrame, CameraID: 1, Payload: []byte{1, 2, 3, 4, 5}}
	if err := frameipc.Write(&buf, msg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Truncate the buffer so payload is incomplete.
	truncated := buf.Bytes()[:frameipc.HeaderSize+2]
	_, err := frameipc.Read(bytes.NewReader(truncated))
	if err == nil {
		t.Fatal("Read: expected error for truncated payload, got nil")
	}
}

func TestMultipleMessages(t *testing.T) {
	msgs := []frameipc.Frame{
		{Kind: frameipc.KindFrame, CameraID: 1, Payload: []byte("frame1")},
		{Kind: frameipc.KindOffline, CameraID: 2},
		{Kind: frameipc.KindFrame, CameraID: 3, Payload: []byte("frame3")},
	}

	var buf bytes.Buffer
	for _, m := range msgs {
		if err := frameipc.Write(&buf, m); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	for i, want := range msgs {
		got, err := frameipc.Read(&buf)
		if err != nil {
			t.Fatalf("msg %d Read: %v", i, err)
		}
		if got.Kind != want.Kind || got.CameraID != want.CameraID || !bytes.Equal(got.Payload, want.Payload) {
			t.Errorf("msg %d: got %+v, want %+v", i, got, want)
		}
	}

	// Verify stream is exhausted.
	if _, err := frameipc.Read(&buf); err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Errorf("expected EOF after all messages, got %v", err)
	}
}
