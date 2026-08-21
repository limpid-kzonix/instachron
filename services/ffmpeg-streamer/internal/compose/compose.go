// Package compose builds a grid canvas from multiple camera frames for the
// merged-canvas streaming mode of ffmpeg-streamer.
package compose

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"sort"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// Canvas builds a single JPEG from the frames map keyed by camera ID.
// Returns nil if there are no frames to compose or decoding all frames fails.
func Canvas(frames map[streamproto.CameraID][]byte, cellW, cellH int) ([]byte, error) {
	if len(frames) == 0 {
		return nil, nil
	}

	ids := make([]streamproto.CameraID, 0, len(frames))
	for id := range frames {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	type cell struct {
		id  streamproto.CameraID
		img image.Image
	}
	cells := make([]cell, 0, len(ids))
	for _, id := range ids {
		img, err := decodeJPEG(frames[id])
		if err != nil {
			continue
		}
		cells = append(cells, cell{id: id, img: img})
	}
	if len(cells) == 0 {
		return nil, nil
	}

	cols, rows := gridLayout(len(cells))
	canvas := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))

	for i, c := range cells {
		scaled := fitImage(c.img, cellW, cellH)
		sb := scaled.Bounds()
		col := i % cols
		row := i / cols
		offsetX := col*cellW + (cellW-sb.Dx())/2
		offsetY := row*cellH + (cellH-sb.Dy())/2
		dstRect := image.Rect(offsetX, offsetY, offsetX+sb.Dx(), offsetY+sb.Dy())
		draw.Draw(canvas, dstRect, scaled, sb.Min, draw.Src)
	}

	encoded, err := encodeJPEG(canvas)
	if err != nil {
		return nil, fmt.Errorf("encode canvas: %w", err)
	}
	return encoded, nil
}

// gridLayout returns the number of columns and rows for a grid of n items.
func gridLayout(n int) (cols, rows int) {
	if n == 0 {
		return 0, 0
	}
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	rows = (n + cols - 1) / cols
	return cols, rows
}

// fitImage scales src to fit within maxW×maxH, preserving aspect ratio.
func fitImage(src image.Image, maxW, maxH int) image.Image {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW == 0 || srcH == 0 {
		return image.NewRGBA(image.Rect(0, 0, maxW, maxH))
	}

	scaleX := float64(maxW) / float64(srcW)
	scaleY := float64(maxH) / float64(srcH)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW == srcW && dstH == srcH {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			sx := sb.Min.X + x*srcW/dstW
			sy := sb.Min.Y + y*srcH/dstH
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func decodeJPEG(data []byte) (image.Image, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	return img, nil
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
