package server

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"

	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/protocol"
)

const testMaxFrameBytes = 1 << 20

// recordingPublisher stands in for the real IPC publisher and remembers what it
// was asked to send. handleConnection calls it from its own goroutine, so the
// mutex guards against a data race even though the test only reads the records
// after that goroutine has finished.
type recordingPublisher struct {
	mu       sync.Mutex
	frames   []publishedFrame
	offlines []streamproto.CameraID
}

type publishedFrame struct {
	cameraID streamproto.CameraID
	jpeg     string
}

func (p *recordingPublisher) Publish(cameraID streamproto.CameraID, jpeg []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frames = append(p.frames, publishedFrame{cameraID: cameraID, jpeg: string(jpeg)})
}

func (p *recordingPublisher) PublishOffline(cameraID streamproto.CameraID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offlines = append(p.offlines, cameraID)
}

// frameHeader builds the 16 fixed bytes every frame starts with.
func frameHeader(magic, sequence, payloadSize uint32) []byte {
	header := make([]byte, protocol.HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], magic)
	binary.BigEndian.PutUint32(header[4:8], sequence)
	binary.BigEndian.PutUint32(header[8:12], payloadSize)
	binary.BigEndian.PutUint32(header[12:16], 0)
	return header
}

// legacyFrame builds a complete JPGS frame, which carries no camera id.
func legacyFrame(sequence uint32, payload []byte) []byte {
	return append(frameHeader(protocol.MagicLegacy, sequence, uint32(len(payload))), payload...)
}

// deviceFrame builds a complete JPGD frame: the fixed header, four camera-id
// bytes, then the payload.
func deviceFrame(cameraID streamproto.CameraID, sequence uint32, payload []byte) []byte {
	frame := frameHeader(protocol.MagicWithDevice, sequence, uint32(len(payload)))
	cameraIDBytes := make([]byte, protocol.CameraIDSize)
	binary.BigEndian.PutUint32(cameraIDBytes, uint32(cameraID))
	frame = append(frame, cameraIDBytes...)
	return append(frame, payload...)
}

// jpeg returns bytes that protocol.LooksLikeJPEG accepts: the start-of-image
// marker 0xFFD8 at the front and the end-of-image marker 0xFFD9 at the back.
func jpeg(body ...byte) []byte {
	frame := []byte{0xFF, 0xD8}
	frame = append(frame, body...)
	return append(frame, 0xFF, 0xD9)
}

func TestHandleConnection(t *testing.T) {
	firstJPEG := jpeg(0x01, 0x02)
	secondJPEG := jpeg(0x03, 0x04)

	tests := []struct {
		name string
		// input is the byte stream the camera writes before hanging up.
		input []byte
		// wantFrames is what the publisher must have been handed, in order.
		wantFrames []publishedFrame
		// wantOfflines is the set of camera ids reported offline on disconnect,
		// compared after sorting because it comes from a map.
		wantOfflines []streamproto.CameraID
	}{
		{
			name:         "legacy frame is published as the default camera",
			input:        legacyFrame(1, firstJPEG),
			wantFrames:   []publishedFrame{{cameraID: protocol.DefaultCameraID, jpeg: string(firstJPEG)}},
			wantOfflines: []streamproto.CameraID{protocol.DefaultCameraID},
		},
		{
			name:         "device frame is published with its decoded camera id",
			input:        deviceFrame(17, 1, firstJPEG),
			wantFrames:   []publishedFrame{{cameraID: 17, jpeg: string(firstJPEG)}},
			wantOfflines: []streamproto.CameraID{17},
		},
		{
			name:  "unknown magic ends the connection",
			input: frameHeader(0xDEADBEEF, 1, uint32(len(firstJPEG))),
		},
		{
			name:  "empty payload ends the connection",
			input: frameHeader(protocol.MagicLegacy, 1, 0),
		},
		{
			name:  "oversized payload ends the connection",
			input: frameHeader(protocol.MagicLegacy, 1, testMaxFrameBytes+1),
		},
		{
			name: "non-JPEG payload is dropped without disconnecting",
			// The first payload has no JPEG markers, so it is discarded; the
			// second one still arrives, which proves the stream stayed open.
			input:        append(legacyFrame(1, []byte{0x00, 0x01, 0x02, 0x03}), legacyFrame(2, secondJPEG)...),
			wantFrames:   []publishedFrame{{cameraID: protocol.DefaultCameraID, jpeg: string(secondJPEG)}},
			wantOfflines: []streamproto.CameraID{protocol.DefaultCameraID},
		},
		{
			name: "every camera seen on the connection is reported offline once",
			input: func() []byte {
				var stream []byte
				for _, cameraID := range []streamproto.CameraID{1, 2, 3} {
					// Two frames per camera, to show the offline report is per
					// camera rather than per frame.
					stream = append(stream, deviceFrame(cameraID, 1, firstJPEG)...)
					stream = append(stream, deviceFrame(cameraID, 2, firstJPEG)...)
				}
				return stream
			}(),
			wantFrames: []publishedFrame{
				{cameraID: 1, jpeg: string(firstJPEG)},
				{cameraID: 1, jpeg: string(firstJPEG)},
				{cameraID: 2, jpeg: string(firstJPEG)},
				{cameraID: 2, jpeg: string(firstJPEG)},
				{cameraID: 3, jpeg: string(firstJPEG)},
				{cameraID: 3, jpeg: string(firstJPEG)},
			},
			wantOfflines: []streamproto.CameraID{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &recordingPublisher{}
			s := New(Config{
				MaxFrameBytes: testMaxFrameBytes,
				Publisher:     pub,
				Logger:        log.New(io.Discard, "", 0),
			})

			// net.Pipe gives two connected in-memory endpoints, so the whole
			// exchange happens without a real network listener.
			serverConn, cameraConn := net.Pipe()

			done := make(chan struct{})
			go func() {
				defer close(done)
				s.handleConnection(serverConn)
			}()

			go func() {
				// A pipe write blocks until the far side reads it, and it fails
				// once handleConnection closes its end after rejecting a frame.
				// Either way the write goroutine finishes and the close below
				// gives handleConnection the end of file it waits for.
				defer func() { _ = cameraConn.Close() }()
				_, _ = cameraConn.Write(tt.input)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("handleConnection did not return")
			}

			pub.mu.Lock()
			frames := pub.frames
			offlines := append([]streamproto.CameraID(nil), pub.offlines...)
			pub.mu.Unlock()

			sort.Slice(offlines, func(i, j int) bool { return offlines[i] < offlines[j] })

			if len(frames) != len(tt.wantFrames) {
				t.Fatalf("published %d frames (%+v), want %d (%+v)", len(frames), frames, len(tt.wantFrames), tt.wantFrames)
			}
			for i, want := range tt.wantFrames {
				if frames[i] != want {
					t.Fatalf("frame %d = %+v, want %+v", i, frames[i], want)
				}
			}

			if len(offlines) != len(tt.wantOfflines) {
				t.Fatalf("offline reports = %v, want %v", offlines, tt.wantOfflines)
			}
			for i, want := range tt.wantOfflines {
				if offlines[i] != want {
					t.Fatalf("offline report %d = %d, want %d", i, offlines[i], want)
				}
			}
		})
	}
}
