package detect

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// TestLetterbox16by9IntoSquare checks the reshaping the model input needs: a
// 640x360 frame (16:9) has to become a 640x640 square without stretching, so it
// keeps its size and gets gray bars above and below.
func TestLetterbox16by9IntoSquare(t *testing.T) {
	const (
		srcW   = 640
		srcH   = 360
		target = 640
	)

	fill := color.NRGBA{R: 10, G: 200, B: 30, A: 255}
	src := image.NewNRGBA(image.Rect(0, 0, srcW, srcH))
	draw.Draw(src, src.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)

	lb := letterbox(src, target, target)

	if lb.scale != 1 {
		t.Errorf("scale = %v, want 1 (the width already fits, so nothing is resized)", lb.scale)
	}
	if lb.padLeft != 0 {
		t.Errorf("padLeft = %d, want 0", lb.padLeft)
	}
	// 640 - 360 = 280 pixels of leftover height, split evenly top and bottom.
	if lb.padTop != 140 {
		t.Errorf("padTop = %d, want 140", lb.padTop)
	}
	if got := lb.img.Bounds(); got != image.Rect(0, 0, target, target) {
		t.Fatalf("bounds = %v, want %v", got, image.Rect(0, 0, target, target))
	}

	// YOLO's convention is to pad with mid-gray rather than black, so the bars
	// do not look like a strong edge to the model.
	gray := color.NRGBA{R: 114, G: 114, B: 114, A: 255}
	padRows := []int{0, 139, lb.padTop + srcH, target - 1} // above and below the frame
	for _, y := range padRows {
		for _, x := range []int{0, target / 2, target - 1} {
			if got := lb.img.NRGBAAt(x, y); got != gray {
				t.Errorf("padding pixel (%d, %d) = %+v, want %+v", x, y, got, gray)
			}
		}
	}

	// The frame itself is copied in unchanged, starting at row padTop.
	for _, p := range []image.Point{{X: 0, Y: lb.padTop}, {X: target - 1, Y: lb.padTop + srcH - 1}} {
		if got := lb.img.NRGBAAt(p.X, p.Y); got != fill {
			t.Errorf("image pixel (%d, %d) = %+v, want %+v", p.X, p.Y, got, fill)
		}
	}
}

// TestLetterboxScalesDown covers the other branch: an image larger than the
// target is shrunk to fit, and the reported scale is what maps model
// coordinates back to source ones.
func TestLetterboxScalesDown(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1280, 640))

	lb := letterbox(src, 640, 640)

	if lb.scale != 0.5 {
		t.Errorf("scale = %v, want 0.5", lb.scale)
	}
	if lb.padLeft != 0 {
		t.Errorf("padLeft = %d, want 0", lb.padLeft)
	}
	// 1280x640 halved is 640x320, leaving 320 rows to split top and bottom.
	if lb.padTop != 160 {
		t.Errorf("padTop = %d, want 160", lb.padTop)
	}
}

// TestToTensor checks the layout toTensor produces. ONNX wants CHW: every red
// value for the whole image first, then every green value, then every blue one,
// each scaled from a 0–255 byte to a 0–1 float. Writing a channel into the
// wrong plane swaps colours for the model and quietly degrades detection.
func TestToTensor(t *testing.T) {
	const w, h = 2, 2
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 51, G: 102, B: 153, A: 255}) // 0.2, 0.4, 0.6

	buf := make([]float32, 3*w*h)
	toTensor(img, buf)

	want := []float32{
		1, 0, 0, 0.2, // red plane, pixels in row-major order
		0, 1, 0, 0.4, // green plane
		0, 0, 1, 0.6, // blue plane
	}
	for i := range want {
		if !approxEqual(buf[i], want[i], 1e-4) {
			t.Errorf("buf[%d] = %v, want %v (full tensor %v)", i, buf[i], want[i], buf)
		}
	}
}
