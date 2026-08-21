package detect

import (
	"fmt"
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const boxThickness = 2

// Annotate draws bounding boxes and class labels onto a copy of img.
func Annotate(img *image.NRGBA, dets []Detection) *image.NRGBA {
	out := image.NewNRGBA(img.Bounds())
	copy(out.Pix, img.Pix)

	for _, d := range dets {
		col := classColor(d.ClassID)
		x1, y1 := int(d.X1+0.5), int(d.Y1+0.5)
		x2, y2 := int(d.X2+0.5), int(d.Y2+0.5)

		drawRect(out, x1, y1, x2, y2, col)
		label := fmt.Sprintf("%s %.0f%%", d.ClassName, d.Confidence*100)
		drawLabel(out, label, x1, y1, col)
	}

	return out
}

func drawRect(img *image.NRGBA, x1, y1, x2, y2 int, col color.NRGBA) {
	b := img.Bounds()
	for t := 0; t < boxThickness; t++ {
		for x := clampInt(x1, b.Min.X, b.Max.X-1); x <= clampInt(x2, b.Min.X, b.Max.X-1); x++ {
			if y := y1 - t; y >= b.Min.Y && y < b.Max.Y {
				img.SetNRGBA(x, y, col)
			}
			if y := y2 + t; y >= b.Min.Y && y < b.Max.Y {
				img.SetNRGBA(x, y, col)
			}
		}
		for y := clampInt(y1, b.Min.Y, b.Max.Y-1); y <= clampInt(y2, b.Min.Y, b.Max.Y-1); y++ {
			if x := x1 - t; x >= b.Min.X && x < b.Max.X {
				img.SetNRGBA(x, y, col)
			}
			if x := x2 + t; x >= b.Min.X && x < b.Max.X {
				img.SetNRGBA(x, y, col)
			}
		}
	}
}

func drawLabel(img *image.NRGBA, text string, x, y int, col color.NRGBA) {
	const (
		charW = 7
		charH = 13
		padX  = 2
		padY  = 2
	)
	tw := len(text)*charW + 2*padX
	th := charH + 2*padY

	// shift label up so it sits above the box
	labelY := y - th
	if labelY < 0 {
		labelY = y
	}

	// filled background (semi-opaque variant of the box color)
	bg := color.NRGBA{R: col.R / 2, G: col.G / 2, B: col.B / 2, A: 200}
	b := img.Bounds()
	for dy := 0; dy < th; dy++ {
		for dx := 0; dx < tw; dx++ {
			px, py := x+dx, labelY+dy
			if px >= b.Min.X && px < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
				img.SetNRGBA(px, py, bg)
			}
		}
	}

	// text in white for readability
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.NRGBA{R: 255, G: 255, B: 255, A: 255}),
		Face: basicfont.Face7x13,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6((x + padX) << 6),
			Y: fixed.Int26_6((labelY + padY + charH - 2) << 6),
		},
	}
	d.DrawString(text)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
