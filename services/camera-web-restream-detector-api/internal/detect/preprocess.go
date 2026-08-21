package detect

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/disintegration/imaging"
)

// letterboxResult carries the resize/pad parameters needed to map detections
// back to the original image coordinate space.
type letterboxResult struct {
	img     *image.NRGBA
	scale   float32
	padLeft int
	padTop  int
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
