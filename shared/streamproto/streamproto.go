// Package streamproto implements the proxy-to-proxy TCP frame transport.
//
// Wire format — 32-byte fixed header followed by a raw JPEG payload:
//
//	[0:4]   "MJPG" magic
//	[4]     version (1)
//	[5]     flags (0, reserved)
//	[6:8]   header length uint16 BE (32)
//	[8:16]  timestamp nanoseconds uint64 BE
//	[16:20] camera ID uint32 BE
//	[20:28] sequence number uint64 BE
//	[28:32] payload length uint32 BE
//	[32:]   raw JPEG payload
package streamproto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

const (
	protoVersion        = uint8(1)
	headerSize          = 32
	DefaultMaxFrameSize = 10 * 1024 * 1024 // 10 MiB
)

var magicBytes = [4]byte{'M', 'J', 'P', 'G'}

var (
	ErrInvalidMagic   = errors.New("streamproto: invalid magic")
	ErrInvalidVersion = errors.New("streamproto: unsupported version")
	ErrFrameTooLarge  = errors.New("streamproto: payload exceeds max frame size")
	ErrEmptyPayload   = errors.New("streamproto: empty payload")
)

// Frame is a single transport unit carrying one JPEG image.
type Frame struct {
	Timestamp time.Time
	Sequence  uint64
	CameraID  CameraID
	Payload   []byte
}

// Writer serialises Frames to an underlying io.Writer.
type Writer struct {
	w io.Writer
}

// NewWriter returns a Writer that serialises to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteFrame encodes f and writes it to the underlying writer.
// Caller is responsible for setting a write deadline on the underlying conn.
func (w *Writer) WriteFrame(f Frame) error {
	if len(f.Payload) == 0 {
		return ErrEmptyPayload
	}
	if len(f.Payload) > DefaultMaxFrameSize {
		return ErrFrameTooLarge
	}

	var hdr [headerSize]byte
	copy(hdr[0:4], magicBytes[:])
	hdr[4] = protoVersion
	// hdr[5] = flags = 0
	binary.BigEndian.PutUint16(hdr[6:8], uint16(headerSize))
	binary.BigEndian.PutUint64(hdr[8:16], uint64(f.Timestamp.UnixNano()))
	binary.BigEndian.PutUint32(hdr[16:20], uint32(f.CameraID))
	binary.BigEndian.PutUint64(hdr[20:28], f.Sequence)
	binary.BigEndian.PutUint32(hdr[28:32], uint32(len(f.Payload)))

	if _, err := w.w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.w.Write(f.Payload)
	return err
}

// Reader deserialises Frames from an underlying io.Reader.
type Reader struct {
	r io.Reader
}

// NewReader returns a Reader that deserialises from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

// ReadFrame reads exactly one Frame from the underlying reader.
// Uses io.ReadFull to guarantee complete reads.
func (r *Reader) ReadFrame() (Frame, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r.r, hdr[:]); err != nil {
		return Frame{}, err
	}

	if hdr[0] != magicBytes[0] || hdr[1] != magicBytes[1] ||
		hdr[2] != magicBytes[2] || hdr[3] != magicBytes[3] {
		return Frame{}, ErrInvalidMagic
	}
	if hdr[4] != protoVersion {
		return Frame{}, ErrInvalidVersion
	}
	// hdr[5] flags — ignored
	// hdr[6:8] headerLen — currently always 32; future versions may extend

	tsNs := int64(binary.BigEndian.Uint64(hdr[8:16]))
	cameraID := CameraID(binary.BigEndian.Uint32(hdr[16:20]))
	seq := binary.BigEndian.Uint64(hdr[20:28])
	payloadLen := binary.BigEndian.Uint32(hdr[28:32])

	if payloadLen == 0 {
		return Frame{}, ErrEmptyPayload
	}
	if int(payloadLen) > DefaultMaxFrameSize {
		return Frame{}, ErrFrameTooLarge
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r.r, payload); err != nil {
		return Frame{}, err
	}

	return Frame{
		Timestamp: time.Unix(0, tsNs),
		Sequence:  seq,
		CameraID:  cameraID,
		Payload:   payload,
	}, nil
}

// CameraID identifies one camera across every service and every wire format in
// this system.
//
// It is a named type over uint32 rather than a bare uint32 because a camera ID
// travels through a lot of code that also handles other uint32s — payload
// sizes, sequence numbers, byte counts — and the compiler cannot tell them
// apart when they are all spelled the same way. With a distinct type, passing a
// payload size where a camera ID belongs stops compiling instead of producing a
// frame filed under camera 43514.
//
// The underlying width is part of every wire format here and must not change:
// the TCP device protocol, the IPC protocol and the restream protocol all
// encode it as four big-endian bytes.
type CameraID uint32

// String returns the camera ID in the form used everywhere a camera is named as
// text: the HTTP camera list, log lines, and the keys of the camera registry.
// Having it here means the several services that used to write
// fmt.Sprintf("%d", id) by hand all produce the same spelling by construction.
func (id CameraID) String() string { return strconv.FormatUint(uint64(id), 10) }

// ParseCameraID converts the text form produced by String back into a
// CameraID. It is the inverse of String, for the HTTP handlers that receive a
// camera ID as part of a URL path.
func ParseCameraID(s string) (CameraID, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid camera id %q: %w", s, err)
	}
	return CameraID(n), nil
}
