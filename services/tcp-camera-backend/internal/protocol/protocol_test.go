package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestParseFrameHeader(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], MagicLegacy)
	binary.BigEndian.PutUint32(header[4:8], 42)
	binary.BigEndian.PutUint32(header[8:12], 123456)
	binary.BigEndian.PutUint32(header[12:16], 987654321)

	got, err := ParseLegacyHeader(header)
	if err != nil {
		t.Fatalf("ParseLegacyHeader returned error: %v", err)
	}

	if got.Sequence != 42 {
		t.Fatalf("Sequence = %d, want 42", got.Sequence)
	}
	if got.CameraID != DefaultCameraID {
		t.Fatalf("CameraID = %d, want %d", got.CameraID, DefaultCameraID)
	}
	if got.PayloadSize != 123456 {
		t.Fatalf("PayloadSize = %d, want 123456", got.PayloadSize)
	}
	if got.TimestampMs != 987654321 {
		t.Fatalf("TimestampMs = %d, want 987654321", got.TimestampMs)
	}
}

func TestParseFrameHeaderWithCameraID(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], MagicWithDevice)
	binary.BigEndian.PutUint32(header[4:8], 42)
	binary.BigEndian.PutUint32(header[8:12], 123456)
	binary.BigEndian.PutUint32(header[12:16], 987654321)
	cameraID := make([]byte, CameraIDSize)
	binary.BigEndian.PutUint32(cameraID, 17)

	got, err := ParseDeviceHeader(header, cameraID)
	if err != nil {
		t.Fatalf("ParseDeviceHeader returned error: %v", err)
	}

	if got.CameraID != 17 {
		t.Fatalf("CameraID = %d, want 17", got.CameraID)
	}
	if got.Sequence != 42 {
		t.Fatalf("Sequence = %d, want 42", got.Sequence)
	}
	if got.PayloadSize != 123456 {
		t.Fatalf("PayloadSize = %d, want 123456", got.PayloadSize)
	}
	if got.TimestampMs != 987654321 {
		t.Fatalf("TimestampMs = %d, want 987654321", got.TimestampMs)
	}
}

func TestParseFrameHeaderRejectsBadMagic(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], 0xDEADBEEF)

	if _, err := ParseLegacyHeader(header); err == nil {
		t.Fatal("ParseLegacyHeader returned nil error for bad magic")
	}
}

func TestLooksLikeJPEG(t *testing.T) {
	validJPEG := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}
	if !LooksLikeJPEG(validJPEG) {
		t.Fatal("LooksLikeJPEG returned false for valid JPEG markers")
	}

	invalidJPEG := []byte{0x00, 0xD8, 0xAA, 0xBB, 0xFF, 0x00}
	if LooksLikeJPEG(invalidJPEG) {
		t.Fatal("LooksLikeJPEG returned true for invalid JPEG markers")
	}
}

func TestReadHeader(t *testing.T) {
	// fixedBytes builds the 16 bytes that both frame variants start with.
	fixedBytes := func(magic, sequence, payloadSize, timestampMs uint32) []byte {
		header := make([]byte, HeaderSize)
		binary.BigEndian.PutUint32(header[0:4], magic)
		binary.BigEndian.PutUint32(header[4:8], sequence)
		binary.BigEndian.PutUint32(header[8:12], payloadSize)
		binary.BigEndian.PutUint32(header[12:16], timestampMs)
		return header
	}
	cameraIDBytes := func(cameraID uint32) []byte {
		raw := make([]byte, CameraIDSize)
		binary.BigEndian.PutUint32(raw, cameraID)
		return raw
	}

	tests := []struct {
		name string
		// input is the byte stream a camera would send.
		input []byte
		// want is the header expected when wantErr is nil.
		want Header
		// wantErr, when set, is the error ReadHeader must report through errors.Is.
		wantErr error
		// wantAnyErr marks a case whose failure has no sentinel to compare
		// against, so the test only requires that some error came back.
		wantAnyErr bool
		// wantRemaining is how many bytes ReadHeader must leave unread.
		wantRemaining int
	}{
		{
			name:          "legacy header",
			input:         append(fixedBytes(MagicLegacy, 42, 123456, 987654321), 0xAA),
			want:          Header{CameraID: DefaultCameraID, Sequence: 42, PayloadSize: 123456, TimestampMs: 987654321},
			wantRemaining: 1,
		},
		{
			name:          "device header consumes the camera id bytes",
			input:         append(append(fixedBytes(MagicWithDevice, 7, 64, 99), cameraIDBytes(17)...), 0xAA),
			want:          Header{CameraID: 17, Sequence: 7, PayloadSize: 64, TimestampMs: 99},
			wantRemaining: 1,
		},
		{
			name:       "unknown magic",
			input:      fixedBytes(0xDEADBEEF, 1, 2, 3),
			wantAnyErr: true,
		},
		{
			name:    "empty stream reports a clean end of file",
			input:   nil,
			wantErr: io.EOF,
		},
		{
			name:    "truncated fixed bytes",
			input:   fixedBytes(MagicLegacy, 1, 2, 3)[:8],
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "truncated camera id",
			input:   append(fixedBytes(MagicWithDevice, 1, 2, 3), 0x00, 0x00),
			wantErr: io.ErrUnexpectedEOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.input)
			got, err := ReadHeader(reader)

			switch {
			case tt.wantAnyErr:
				if err == nil {
					t.Fatalf("ReadHeader returned nil error, want an error")
				}
				return
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReadHeader error = %v, want %v", err, tt.wantErr)
				}
				return
			case err != nil:
				t.Fatalf("ReadHeader returned error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("ReadHeader = %+v, want %+v", got, tt.want)
			}
			if reader.Len() != tt.wantRemaining {
				t.Fatalf("unread bytes = %d, want %d", reader.Len(), tt.wantRemaining)
			}
		})
	}
}

func TestParseFrameHeaderRejectsJPGD(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], MagicWithDevice)

	if _, err := ParseLegacyHeader(header); err == nil {
		t.Fatal("ParseLegacyHeader should reject JPGD magic")
	}
}
