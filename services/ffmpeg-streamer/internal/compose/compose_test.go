package compose

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

func TestGridLayout(t *testing.T) {
	cases := []struct {
		n, cols, rows int
	}{
		{0, 0, 0},
		{1, 1, 1},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 2},
		{5, 3, 2},
		{6, 3, 2},
		{7, 3, 3},
		{9, 3, 3},
	}
	for _, c := range cases {
		cols, rows := gridLayout(c.n)
		if cols != c.cols || rows != c.rows {
			t.Errorf("gridLayout(%d) = %dx%d, want %dx%d", c.n, cols, rows, c.cols, c.rows)
		}
	}
}

func TestFitImagePreservesAspect(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 480))
	out := fitImage(src, 320, 240)
	b := out.Bounds()
	if b.Dx() != 320 || b.Dy() != 240 {
		t.Errorf("fitImage(640x480→320x240) = %dx%d, want 320x240", b.Dx(), b.Dy())
	}
}

func TestFitImageLetterboxes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 360))
	out := fitImage(src, 320, 240)
	b := out.Bounds()
	if b.Dx() != 320 || b.Dy() != 180 {
		t.Errorf("fitImage(640x360→320x240) = %dx%d, want 320x180", b.Dx(), b.Dy())
	}
}

func TestFitImageNoScaleNeeded(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 240))
	out := fitImage(src, 320, 240)
	if out != image.Image(src) {
		t.Error("fitImage returned a copy when source already fits")
	}
}

func TestDecodeJPEG(t *testing.T) {
	src := solidColorImage(color.RGBA{200, 100, 50, 255}, 32, 32)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}

	out, err := decodeJPEG(buf.Bytes())
	if err != nil {
		t.Fatalf("decodeJPEG returned error: %v", err)
	}
	b := out.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("decoded image = %dx%d, want 32x32", b.Dx(), b.Dy())
	}
}

// TestCanvasPlacesCamerasSideBySide feeds Canvas two 320x240 JPEGs — one solid
// red, one solid blue — and checks the result. Two cameras lay out as two
// columns by one row, so the canvas should be 640x240 wide with camera 0 (red)
// on the left and camera 1 (blue) on the right. Cameras are ordered by their
// numeric ID, which is why the lower ID lands in the left cell.
func TestCanvasPlacesCamerasSideBySide(t *testing.T) {
	frames := map[streamproto.CameraID][]byte{
		0: encodeSolidJPEG(t, color.RGBA{255, 0, 0, 255}, 320, 240),
		1: encodeSolidJPEG(t, color.RGBA{0, 0, 255, 255}, 320, 240),
	}

	encoded, err := Canvas(frames, 320, 240)
	if err != nil {
		t.Fatalf("Canvas returned error: %v", err)
	}
	if encoded == nil {
		t.Fatal("Canvas returned no bytes")
	}

	got, err := decodeJPEG(encoded)
	if err != nil {
		t.Fatalf("decoding the canvas failed: %v", err)
	}
	b := got.Bounds()
	if b.Dx() != 640 || b.Dy() != 240 {
		t.Fatalf("canvas = %dx%d, want 640x240", b.Dx(), b.Dy())
	}

	assertColorNear(t, got, 160, 120, color.RGBA{255, 0, 0, 255}, "left half")
	assertColorNear(t, got, 480, 120, color.RGBA{0, 0, 255, 255}, "right half")
}

// assertColorNear checks one pixel against an expected colour with a tolerance,
// because JPEG is a lossy format: a pixel encoded as pure red decodes back as
// something very close to, but rarely exactly, pure red.
func assertColorNear(t *testing.T, img image.Image, x, y int, want color.RGBA, where string) {
	t.Helper()

	const tolerance = 16

	r, g, b, _ := img.At(x, y).RGBA()
	// RGBA() returns 16-bit channels; shifting by 8 brings them back to 0-255.
	got := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}

	near := func(a, b uint8) bool {
		diff := int(a) - int(b)
		if diff < 0 {
			diff = -diff
		}
		return diff <= tolerance
	}
	if !near(got.R, want.R) || !near(got.G, want.G) || !near(got.B, want.B) {
		t.Errorf("%s pixel at (%d,%d) = %v, want about %v", where, x, y, got, want)
	}
}

func encodeSolidJPEG(t *testing.T, c color.RGBA, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solidColorImage(c, w, h), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encoding a %dx%d test frame failed: %v", w, h, err)
	}
	return buf.Bytes()
}

func solidColorImage(c color.RGBA, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
