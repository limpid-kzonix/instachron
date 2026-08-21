package protocol

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/w0rxbend/instachron/shared/streamproto"

	"github.com/w0rxbend/instachron/shared/imageutil"
)

const (
	MagicLegacy     uint32 = 0x4A504753 // JPGS
	MagicWithDevice uint32 = 0x4A504744 // JPGD

	HeaderSize      = 16
	CameraIDSize    = 4
	DefaultCameraID = 0
)

type Header struct {
	CameraID    streamproto.CameraID
	Sequence    uint32
	PayloadSize uint32
	TimestampMs uint32
}

// ParseLegacyHeader parses a legacy JPGS frame header. It rejects JPGD frames
// so callers do not accidentally leave camera-id bytes unread on the stream.
func ParseLegacyHeader(header []byte) (Header, error) {
	if len(header) != HeaderSize {
		return Header{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), HeaderSize)
	}
	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != MagicLegacy {
		return Header{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
	return Header{
		CameraID:    DefaultCameraID,
		Sequence:    binary.BigEndian.Uint32(header[4:8]),
		PayloadSize: binary.BigEndian.Uint32(header[8:12]),
		TimestampMs: binary.BigEndian.Uint32(header[12:16]),
	}, nil
}

// ParseDeviceHeader parses a JPGD frame header plus its camera-id bytes.
func ParseDeviceHeader(header []byte, cameraIDBytes []byte) (Header, error) {
	if len(header) != HeaderSize {
		return Header{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), HeaderSize)
	}
	if len(cameraIDBytes) != CameraIDSize {
		return Header{}, fmt.Errorf("invalid camera id size: got %d, want %d", len(cameraIDBytes), CameraIDSize)
	}
	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != MagicWithDevice {
		return Header{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
	return Header{
		CameraID:    streamproto.CameraID(binary.BigEndian.Uint32(cameraIDBytes)),
		Sequence:    binary.BigEndian.Uint32(header[4:8]),
		PayloadSize: binary.BigEndian.Uint32(header[8:12]),
		TimestampMs: binary.BigEndian.Uint32(header[12:16]),
	}, nil
}

// ReadHeader reads one complete frame header from r.
//
// The wire format has two variants that differ in length. Both start with the
// same 16 fixed bytes, but a device frame (magic "JPGD") follows them with four
// more bytes holding the camera id, while a legacy frame (magic "JPGS") does
// not and is reported as coming from DefaultCameraID. Reading the magic and the
// optional camera-id bytes in one place is what keeps a caller from stopping
// after the 16 fixed bytes and leaving the camera id sitting unread on the
// stream, where it would be mistaken for the start of the next frame.
//
// Failures from r are wrapped, so a caller can still recognise a client that
// disconnected with errors.Is(err, io.EOF) or errors.Is(err, io.ErrUnexpectedEOF).
func ReadHeader(r io.Reader) (Header, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return Header{}, fmt.Errorf("read header: %w", err)
	}

	magic := binary.BigEndian.Uint32(header[0:4])
	switch magic {
	case MagicLegacy:
		return ParseLegacyHeader(header)
	case MagicWithDevice:
		cameraIDBytes := make([]byte, CameraIDSize)
		if _, err := io.ReadFull(r, cameraIDBytes); err != nil {
			return Header{}, fmt.Errorf("read camera id: %w", err)
		}
		return ParseDeviceHeader(header, cameraIDBytes)
	default:
		return Header{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
}

// LooksLikeJPEG reports whether payload is plausibly a JPEG frame. It is kept
// here, as a thin call through to the shared helper, because it is part of what
// this package promises its callers about the frames it parses.
func LooksLikeJPEG(payload []byte) bool {
	return imageutil.LooksLikeJPEG(payload)
}
