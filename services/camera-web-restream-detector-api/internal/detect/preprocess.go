package detect

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/disintegration/imaging"
)

// letterboxResult carries the resize/pad parameters needed to map detections
// back to the original image coordinate space.
//
// "Letterboxing" here means the same thing it means on a television: the image
// is scaled to fit the model's fixed square input without distorting it, and
// the leftover strips are filled with flat gray. The model therefore reports
// boxes in the coordinates of that padded square, and every one of them has to
// be translated back to where it belongs in the original frame. toOriginal does
// that translation, and it lives here because scale, padLeft and padTop are the
// only things it needs — keeping it next to them means no caller has to take
// those three values apart and pass them around separately.
type letterboxResult struct {
	img     *image.NRGBA
	scale   float32
	padLeft int
	padTop  int
}

// toOriginal maps one point from letterboxed coordinates back to coordinates in
// the original image, clamped to the bounds origW×origH.
//
// The two steps undo letterbox in reverse order: subtract the padding that was
// added around the scaled image, then divide by the scale that was applied.
// Clamping matters because a model often predicts a box that runs slightly off
// the edge of the object, and without it a detection could carry a negative
// coordinate or one past the end of the frame — which would later be drawn
// outside the image.
func (lb letterboxResult) toOriginal(x, y float32, origW, origH int) (float32, float32) {
	ox := clamp((x-float32(lb.padLeft))/lb.scale, 0, float32(origW))
	oy := clamp((y-float32(lb.padTop))/lb.scale, 0, float32(origH))
	return ox, oy
}

// letterbox resizes img to fit within targetW×targetH while preserving aspect
// ratio, padding the remainder with gray (114, 114, 114).
func letterbox(img image.Image, targetW, targetH int) letterboxResult {
	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	scale := float32(targetW) / float32(origW)
	if hs := float32(targetH) / float32(origH); hs < scale {
		scale = hs
	}

	newW := int(float32(origW)*scale + 0.5)
	newH := int(float32(origH)*scale + 0.5)
	padLeft := (targetW - newW) / 2
	padTop := (targetH - newH) / 2

	resized := imaging.Resize(img, newW, newH, imaging.Linear)

	out := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	gray := image.NewUniform(color.NRGBA{R: 114, G: 114, B: 114, A: 255})
	draw.Draw(out, out.Bounds(), gray, image.Point{}, draw.Src)
	draw.Draw(out, image.Rect(padLeft, padTop, padLeft+newW, padTop+newH), resized, image.Point{}, draw.Over)

	return letterboxResult{img: out, scale: scale, padLeft: padLeft, padTop: padTop}
}

// toTensor fills buf with the CHW float32 representation of img normalized to [0,1].
// buf must have length 3 * img.Bounds().Dx() * img.Bounds().Dy().
func toTensor(img *image.NRGBA, buf []float32) {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	planeSize := w * h
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := img.NRGBAAt(x, y)
			i := y*w + x
			buf[i] = float32(px.R) / 255.0             // R plane
			buf[planeSize+i] = float32(px.G) / 255.0   // G plane
			buf[2*planeSize+i] = float32(px.B) / 255.0 // B plane
		}
	}
}
