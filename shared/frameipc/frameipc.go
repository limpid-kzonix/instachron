// Package frameipc owns the binary wire format shared between tcp-camera-backend
// (writer) and every IPC consumer (camera-web-api, ffmpeg-streamer).
//
// Wire format: magic(2) | type(1) | cameraID(4 BE) | payloadSize(4 BE) | payload
package frameipc

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	Magic1 byte = 0xAA
	Magic2 byte = 0xBB

	HeaderSize = 11 // 2 magic + 1 kind + 4 cameraID + 4 payloadSize
)

// Kind says what a message means. It is a named type rather than a bare byte so
// that the two legal values are stated here, and so that a reader of a function
// signature can tell a message kind from the many other bytes in this package —
// the magic bytes, the payload — instead of every one of them being "byte".
//
// The underlying values are part of the wire format and must not change: a
// writer and a reader from different builds have to agree on them.
type Kind byte

const (
	// KindFrame carries one JPEG image in Payload.
	KindFrame Kind = 0x01
	// KindOffline announces that a camera has stopped sending. It carries no
	// payload.
	KindOffline Kind = 0x02
)

// String returns a readable name for the kind, for logs and test failures.
func (k Kind) String() string {
	switch k {
	case KindFrame:
		return "frame"
	case KindOffline:
		return "offline"
	default:
		return fmt.Sprintf("unknown(0x%02x)", byte(k))
	}
}

// Frame is a single IPC message: one JPEG image, or one notice that a camera
// went away.
type Frame struct {
	Kind     Kind
	CameraID streamproto.CameraID
	Payload  []byte // nil for KindOffline
}

// Write serialises m to w. It performs two Write calls: one for the fixed
// header and one for the payload (skipped when empty).
func Write(w io.Writer, m Frame) error {
	var hdr [HeaderSize]byte
	hdr[0] = Magic1
	hdr[1] = Magic2
	hdr[2] = byte(m.Kind)
	binary.BigEndian.PutUint32(hdr[3:7], uint32(m.CameraID))
	binary.BigEndian.PutUint32(hdr[7:11], uint32(len(m.Payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(m.Payload) > 0 {
		if _, err := w.Write(m.Payload); err != nil {
			return err
		}
	}
	return nil
}

// Read reads exactly one message from r. It blocks until the full message
// (header + payload) is available or an error occurs.
func Read(r io.Reader) (Frame, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	if hdr[0] != Magic1 || hdr[1] != Magic2 {
		return Frame{}, fmt.Errorf("frameipc: bad magic 0x%02x%02x", hdr[0], hdr[1])
	}

	kind := Kind(hdr[2])
	cameraID := streamproto.CameraID(binary.BigEndian.Uint32(hdr[3:7]))
	payloadSize := binary.BigEndian.Uint32(hdr[7:11])

	var payload []byte
	if payloadSize > 0 {
		payload = make([]byte, payloadSize)
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, fmt.Errorf("frameipc: read payload camera=%d: %w", cameraID, err)
		}
	}

	return Frame{Kind: kind, CameraID: cameraID, Payload: payload}, nil
}
