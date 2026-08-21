// Package rotation provides JPEG rotation support driven by a JSON config file.
package rotation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"
)

// Angle is a clockwise rotation in degrees, restricted to the four values a
// JPEG can be rotated by without resampling it.
//
// It is a named type rather than a plain int so that the four legal values are
// stated once, here, instead of being an unwritten rule that every function
// taking an "int degrees" has to re-check. A value of this type can only be
// produced by Normalize or by the constants below, so code downstream can rely
// on it being one of the four without validating it again.
type Angle int

// The four rotations. Any other value is not a valid Angle.
const (
	Angle0   Angle = 0
	Angle90  Angle = 90
	Angle180 Angle = 180
	Angle270 Angle = 270
)

// Degrees returns the angle as a plain integer number of degrees, for the
// boundaries where a number is what is wanted — the JSON camera list, for one.
func (a Angle) Degrees() int { return int(a) }

// IsZero reports whether the angle is a no-op, which is worth asking before
// paying to decode and re-encode a frame that does not need rotating.
func (a Angle) IsZero() bool { return a == Angle0 }

// String returns the angle in the form "90°", for logs.
func (a Angle) String() string { return fmt.Sprintf("%d°", int(a)) }

// Normalize converts any integer angle to the nearest of the four legal
// rotations. Negative angles and angles beyond a full turn are wrapped first,
// so -90 becomes 270 and 450 becomes 90, and anything that is not close to a
// multiple of 90 is rounded to the nearest one.
func Normalize(deg int) Angle {
	n := ((deg % 360) + 360) % 360
	switch {
	case n < 45 || n >= 315:
		return Angle0
	case n < 135:
		return Angle90
	case n < 225:
		return Angle180
	default:
		return Angle270
	}
}

// Config maps camera ID strings to clockwise rotation angles.
// The file format is a JSON object: {"0": 90, "1": -90, "2": 180}.
// Any integer is accepted; values are normalised to one of the four Angles.
type Config struct {
	mu     sync.RWMutex
	angles map[string]Angle
	path   string
}

// Load reads a JSON file at path and returns the parsed Config, along with the
// number of camera entries it contains so the caller can report what it loaded.
// A missing file is not an error — it returns an empty (no-op) config.
func Load(path string) (*Config, int, error) {
	rc := &Config{angles: make(map[string]Angle), path: path}
	n, err := rc.Reload()
	if err != nil {
		return nil, 0, err
	}
	return rc, n, nil
}

// Reload re-reads the config file and returns the number of entries now in
// effect. Safe to call concurrently.
func (rc *Config) Reload() (int, error) {
	data, err := os.ReadFile(rc.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read %s: %w", rc.path, err)
	}

	var raw map[string]int
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, fmt.Errorf("parse %s: %w", rc.path, err)
	}

	angles := make(map[string]Angle, len(raw))
	for id, deg := range raw {
		angles[id] = Normalize(deg)
	}

	rc.mu.Lock()
	rc.angles = angles
	rc.mu.Unlock()

	return len(angles), nil
}

// Get returns the configured clockwise rotation for a camera ID.
// A camera with no entry in the config is not rotated, because the zero value
// of Angle is Angle0.
func (rc *Config) Get(cameraID string) Angle {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.angles[cameraID]
}

// Degrees returns the configured rotation for a camera ID as a plain integer.
// It exists for the JSON camera list, which publishes the angle as a number.
func (rc *Config) Degrees(cameraID string) int { return rc.Get(cameraID).Degrees() }

// Apply decodes the JPEG, rotates it clockwise by angle, and re-encodes.
//
// The original bytes are returned unchanged when there is nothing to do or when
// anything goes wrong — an undecodable frame, a failed re-encode. For a live
// camera feed a frame at the wrong orientation is better than no frame, and the
// alternative would be dropping frames on the floor for a cosmetic reason.
func Apply(data []byte, angle Angle) []byte {
	if angle.IsZero() {
		return data
	}

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	var rotated image.Image
	switch angle {
	case Angle90:
		rotated = rotate90(img)
	case Angle180:
		rotated = rotate180(img)
	case Angle270:
		rotated = rotate270(img)
	default:
		// Unreachable for an Angle produced by Normalize or the constants, but
		// a caller can still write rotation.Angle(45) and the compiler will
		// allow it. Returning the frame untouched matches every other failure
		// path here.
		return data
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rotated, &jpeg.Options{Quality: 90}); err != nil {
		return data
	}
	return buf.Bytes()
}

func rotate90(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate270(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
