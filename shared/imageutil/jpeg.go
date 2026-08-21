// Package imageutil holds the JPEG helpers shared by the services that move
// camera frames around: the marker check that decides whether a payload is
// plausibly a JPEG at all, and the re-encode step used by every service that
// modifies a frame before republishing it.
package imageutil

import (
	"bytes"
	"image"

	"github.com/disintegration/imaging"
)

// JPEG files begin with the Start Of Image marker and end with the End Of Image
// marker. These two byte pairs are what LooksLikeJPEG checks for.
const (
	markerSOI0, markerSOI1 = 0xFF, 0xD8
	markerEOI0, markerEOI1 = 0xFF, 0xD9
)

// LooksLikeJPEG reports whether payload starts with the JPEG Start Of Image
// marker and ends with the End Of Image marker.
//
// This is a cheap sanity check, not a validation: it confirms the first and
// last two bytes look right and says nothing about the millions of bytes in
// between, so a truncated frame with its tail intact would still pass. It is
// meant to catch the common failure of a frame that was cut short in transit or
// never was an image at all, without paying to decode every frame at the point
// where it arrives.
func LooksLikeJPEG(payload []byte) bool {
	if len(payload) < 4 {
		return false
	}
	return payload[0] == markerSOI0 &&
		payload[1] == markerSOI1 &&
		payload[len(payload)-2] == markerEOI0 &&
		payload[len(payload)-1] == markerEOI1
}

// EncodeJPEG encodes img as a JPEG at the given quality (1-100), using buf as
// scratch space, and returns the encoded bytes.
//
// buf is expected to come from a sync.Pool so that a busy service does not
// allocate a fresh buffer for every frame. That is also why the returned slice
// is a copy rather than buf.Bytes(): once buf goes back into the pool another
// goroutine may reset and overwrite it, which would corrupt a frame that is
// still on its way to a client. The copy is what makes the result safe to hold.
func EncodeJPEG(img image.Image, quality int, buf *bytes.Buffer) ([]byte, error) {
	buf.Reset()
	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, err
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

// CapResolution downsizes img so that neither dimension exceeds maxW or maxH,
// preserving the aspect ratio by scaling both dimensions by whichever of the
// two limits binds first. An image already within both limits is returned
// unchanged, with no copy and no resampling.
func CapResolution(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return img
	}
	scale := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	return imaging.Resize(img, int(float64(w)*scale), int(float64(h)*scale), imaging.Lanczos)
}
