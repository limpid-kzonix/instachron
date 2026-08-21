// Package enhance applies image processing filters to JPEG frames.
package enhance

import (
	"bytes"
	"image"
	"image/color"
	"sync"
	"sync/atomic"

	"github.com/disintegration/gift"
	"github.com/disintegration/imaging"
)

// Config holds the processing parameters for one camera.
// Filters is an ordered list of registry keys to apply after the adaptive
// dark correction pass. Unknown keys are silently skipped.
type Config struct {
	DarkThreshold float64  `json:"dark_threshold"`
	BrightnessMax float64  `json:"brightness_max"`
	ContrastMax   float64  `json:"contrast_max"`
	JPEGQuality   int      `json:"jpeg_quality"`
	Filters       []string `json:"filters"`
}

// CameraConfigs holds a default pipeline config and optional per-camera overrides.
type CameraConfigs struct {
	Default Config            `json:"default"`
	Cameras map[string]Config `json:"cameras"`
}

// DefaultCameraConfigs returns a baseline CameraConfigs with no per-camera overrides.
func DefaultCameraConfigs() CameraConfigs {
	return CameraConfigs{
		Default: Config{
			DarkThreshold: 0.35,
			BrightnessMax: 35.0,
			ContrastMax:   35.0,
			JPEGQuality:   90,
			Filters:       []string{"gaussian_blur", "unsharp_mask"},
		},
		Cameras: make(map[string]Config),
	}
}

// filterRegistry is the built-in set of named gift filters available to configs.
// All filter instances are stateless and safe for concurrent use.
var filterRegistry = map[string]gift.Filter{
	"resize":              gift.Resize(100, 0, gift.LanczosResampling),
	"crop_to_size":        gift.CropToSize(100, 100, gift.LeftAnchor),
	"rotate_180":          gift.Rotate180(),
	"rotate_30":           gift.Rotate(30, color.Transparent, gift.CubicInterpolation),
	"brightness_increase": gift.Brightness(30),
	"brightness_decrease": gift.Brightness(-30),
	"contrast_increase":   gift.Contrast(30),
	"contrast_decrease":   gift.Contrast(-30),
	"saturation_increase": gift.Saturation(50),
	"saturation_decrease": gift.Saturation(-50),
	"gamma_1.5":           gift.Gamma(1.5),
	"gamma_0.5":           gift.Gamma(0.5),
	"gaussian_blur":       gift.GaussianBlur(1),
	"unsharp_mask":        gift.UnsharpMask(1, 1, 0),
	"sigmoid":             gift.Sigmoid(0.5, 7),
	"pixelate":            gift.Pixelate(5),
	"colorize":            gift.Colorize(240, 50, 100),
	"grayscale":           gift.Grayscale(),
	"sepia":               gift.Sepia(100),
	"invert":              gift.Invert(),
	"mean":                gift.Mean(5, true),
	"median":              gift.Median(5, true),
	"minimum":             gift.Minimum(5, true),
	"maximum":             gift.Maximum(5, true),
	"hue_rotate":          gift.Hue(45),
	"color_balance":       gift.ColorBalance(10, -10, -10),
	"color_func": gift.ColorFunc(
		func(r0, g0, b0, a0 float32) (r, g, b, a float32) {
			r = 1 - r0
			g = g0 + 0.1
			b = 0
			a = a0
			return r, g, b, a
		},
	),
	"convolution_emboss": gift.Convolution(
		[]float32{
			-1, -1, 0,
			-1, 1, 1,
			0, 1, 1,
		},
		false, false, false, 0.0,
	),
}

// Stats is a snapshot of how the enhancement pipeline has been doing.
//
// It exists because a frame that cannot be decoded or encoded is passed through
// untouched rather than dropped, which is the right thing to do for a live video
// feed — a slightly wrong frame beats a gap — but it means a camera whose frames
// have silently stopped being enhanced looks exactly like a camera whose frames
// need no enhancement. These counters are the difference between the two.
type Stats struct {
	// Processed is the total number of frames handed to ProcessCamera.
	Processed uint64
	// DecodeFailed counts frames whose JPEG could not be decoded, and which
	// were therefore forwarded exactly as they arrived.
	DecodeFailed uint64
	// EncodeFailed counts frames that were enhanced successfully but could not
	// be re-encoded, and which were therefore forwarded in their original form.
	EncodeFailed uint64
}

// Enhancer applies the enhancement pipeline to raw JPEG frames.
// ProcessCamera is the entry point, and is safe for concurrent use.
type Enhancer struct {
	cfgs    CameraConfigs
	bufPool sync.Pool

	processed    atomic.Uint64
	decodeFailed atomic.Uint64
	encodeFailed atomic.Uint64
}

// New returns an Enhancer driven by cfgs.
func New(cfgs CameraConfigs) *Enhancer {
	return &Enhancer{
		cfgs:    cfgs,
		bufPool: sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}
}

// ProcessCamera looks up the per-camera config (falling back to Default) and
// enhances jpeg, calling push with the result. A frame that cannot be decoded
// or re-encoded is passed through unchanged and counted in Stats.
func (e *Enhancer) ProcessCamera(cameraID string, jpeg []byte, push func([]byte)) {
	e.processed.Add(1)
	push(e.process(jpeg, e.configFor(cameraID)))
}

// Stats returns a snapshot of the counters. The three values are read
// independently, so a snapshot taken while frames are in flight may show
// slightly inconsistent totals; that is fine for reporting and avoids putting a
// lock on the per-frame path.
func (e *Enhancer) Stats() Stats {
	return Stats{
		Processed:    e.processed.Load(),
		DecodeFailed: e.decodeFailed.Load(),
		EncodeFailed: e.encodeFailed.Load(),
	}
}

func (e *Enhancer) configFor(cameraID string) Config {
	if cfg, ok := e.cfgs.Cameras[cameraID]; ok {
		return cfg
	}
	return e.cfgs.Default
}

func (e *Enhancer) process(jpeg []byte, cfg Config) []byte {
	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		e.decodeFailed.Add(1)
		return jpeg
	}

	img = applyDarkCorrection(img, cfg)
	img = applyFilters(img, cfg.Filters)

	buf := e.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer e.bufPool.Put(buf)

	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(cfg.JPEGQuality)); err != nil {
		e.encodeFailed.Add(1)
		return jpeg
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out
}

// applyDarkCorrection brightens a frame that came out too dark, by an amount
// that depends on how dark it is: a frame at half the threshold luminance gets
// half of BrightnessMax and half of ContrastMax, and a frame at or above the
// threshold is returned untouched. A DarkThreshold of zero turns the pass off.
func applyDarkCorrection(img image.Image, cfg Config) image.Image {
	if cfg.DarkThreshold > 0 {
		lum := avgLuminance32(img)
		if lum < cfg.DarkThreshold {
			factor := (cfg.DarkThreshold - lum) / cfg.DarkThreshold
			if cfg.BrightnessMax > 0 {
				img = imaging.AdjustBrightness(img, factor*cfg.BrightnessMax)
			}
			if cfg.ContrastMax > 0 {
				img = imaging.AdjustContrast(img, factor*cfg.ContrastMax)
			}
		}
	}
	return img
}

// applyFilters runs img through the named filters from filterRegistry, in the
// order given. Names that are not in the registry are skipped, so a typo in a
// config file costs that one filter rather than the whole frame. When no name
// matches, img is returned unchanged.
func applyFilters(img image.Image, keys []string) image.Image {
	var filters []gift.Filter
	for _, key := range keys {
		if f, ok := filterRegistry[key]; ok {
			filters = append(filters, f)
		}
	}
	if len(filters) > 0 {
		g := gift.New(filters...)
		dst := image.NewNRGBA(g.Bounds(img.Bounds()))
		g.Draw(dst, img)
		img = dst
	}
	return img
}

// avgLuminance32 computes the average BT.601 luminance (0–1) by downsampling
// to a 32-pixel-wide thumbnail using the fast Box filter.
func avgLuminance32(img image.Image) float64 {
	thumb := imaging.Resize(img, 32, 0, imaging.Box)
	pix := thumb.Pix
	n := len(pix) / 4
	if n == 0 {
		return 0
	}
	var sum uint64
	for i := 0; i < len(pix); i += 4 {
		r, g, b := uint64(pix[i]), uint64(pix[i+1]), uint64(pix[i+2])
		sum += 299*r + 587*g + 114*b
	}
	return float64(sum) / float64(n) / 1000.0 / 255.0
}
