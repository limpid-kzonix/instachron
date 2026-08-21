package imageutil

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestLooksLikeJPEG(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    bool
	}{
		{name: "nil", payload: nil, want: false},
		{name: "empty", payload: []byte{}, want: false},
		{name: "too short to hold both markers", payload: []byte{0xFF, 0xD8, 0xFF}, want: false},
		{name: "shortest accepted payload", payload: []byte{0xFF, 0xD8, 0xFF, 0xD9}, want: true},
		{name: "markers with body between", payload: []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}, want: true},
		{name: "wrong start marker", payload: []byte{0x00, 0xD8, 0xAA, 0xFF, 0xD9}, want: false},
		{name: "wrong end marker", payload: []byte{0xFF, 0xD8, 0xAA, 0xFF, 0x00}, want: false},
		{name: "truncated frame keeping its tail still passes", payload: []byte{0xFF, 0xD8, 0xFF, 0xD9}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeJPEG(tt.payload); got != tt.want {
				t.Errorf("LooksLikeJPEG(%#v) = %v, want %v", tt.payload, got, tt.want)
			}
		})
	}
}

func TestEncodeJPEGProducesAJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	out, err := EncodeJPEG(img, 80, &buf)
	if err != nil {
		t.Fatalf("EncodeJPEG() error = %v", err)
	}
	if !LooksLikeJPEG(out) {
		t.Error("EncodeJPEG() produced bytes that do not carry JPEG markers")
	}
}

// TestEncodeJPEGResultSurvivesBufferReuse is the regression test for the reason
// EncodeJPEG copies: a caller holding an earlier result must not see it change
// when the pooled buffer is reused for the next frame.
func TestEncodeJPEGResultSurvivesBufferReuse(t *testing.T) {
	var buf bytes.Buffer

	first := image.NewRGBA(image.Rect(0, 0, 8, 8))
	firstOut, err := EncodeJPEG(first, 80, &buf)
	if err != nil {
		t.Fatalf("EncodeJPEG() error = %v", err)
	}
	kept := append([]byte(nil), firstOut...)

	second := image.NewRGBA(image.Rect(0, 0, 64, 64))
	second.Set(3, 3, color.RGBA{B: 255, A: 255})
	if _, err := EncodeJPEG(second, 20, &buf); err != nil {
		t.Fatalf("EncodeJPEG() error = %v", err)
	}

	if !bytes.Equal(firstOut, kept) {
		t.Error("the first result changed when the buffer was reused; EncodeJPEG must return a copy")
	}
}

func TestEncodeJPEGRejectsNothingButReportsQuality(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for x := range 32 {
		for y := range 32 {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), A: 255})
		}
	}
	var buf bytes.Buffer
	low, err := EncodeJPEG(img, 10, &buf)
	if err != nil {
		t.Fatalf("EncodeJPEG() error = %v", err)
	}
	high, err := EncodeJPEG(img, 95, &buf)
	if err != nil {
		t.Fatalf("EncodeJPEG() error = %v", err)
	}
	// Higher quality keeps more detail, so it cannot produce fewer bytes.
	if len(high) <= len(low) {
		t.Errorf("quality 95 produced %d bytes, quality 10 produced %d; expected the higher quality to be larger", len(high), len(low))
	}
}

func TestCapResolution(t *testing.T) {
	tests := []struct {
		name       string
		w, h       int
		maxW, maxH int
		wantW      int
		wantH      int
	}{
		{name: "already within bounds is untouched", w: 100, h: 50, maxW: 200, maxH: 200, wantW: 100, wantH: 50},
		{name: "exactly at the limit is untouched", w: 200, h: 200, maxW: 200, maxH: 200, wantW: 200, wantH: 200},
		{name: "width binds", w: 400, h: 100, maxW: 200, maxH: 200, wantW: 200, wantH: 50},
		{name: "height binds", w: 100, h: 400, maxW: 200, maxH: 200, wantW: 50, wantH: 200},
		{name: "both exceed, the tighter limit wins", w: 800, h: 600, maxW: 400, maxH: 200, wantW: 266, wantH: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := image.NewRGBA(image.Rect(0, 0, tt.w, tt.h))
			got := CapResolution(src, tt.maxW, tt.maxH).Bounds()
			if got.Dx() != tt.wantW || got.Dy() != tt.wantH {
				t.Errorf("CapResolution() = %dx%d, want %dx%d", got.Dx(), got.Dy(), tt.wantW, tt.wantH)
			}
		})
	}
}
