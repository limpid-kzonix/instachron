// Package config reads the enhancer's per-camera settings from a JSON file.
//
// It sits outside the enhance package on purpose: enhance is about applying
// image filters, and where its parameters came from is a separate question.
// Keeping the file handling here means enhance can be tested with a config
// value built in code, without a file existing anywhere on disk.
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/w0rxbend/instachron/services/camera-web-restream-enhancer-api/internal/enhance"
)

// DefaultPath is the config file read when CONFIG_FILE is unset.
const DefaultPath = "config.json"

// Load reads per-camera enhancement settings from path.
//
// A missing or malformed file is not fatal: Load returns the built-in defaults
// together with a non-nil err explaining what went wrong, so the service can
// report the reason and keep running with sensible settings rather than
// refusing to start because nobody wrote a config file.
func Load(path string) (enhance.CameraConfigs, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return enhance.DefaultCameraConfigs(), fmt.Errorf("read config %q: %w", path, err)
	}

	cfgs := enhance.DefaultCameraConfigs()
	if err := json.Unmarshal(data, &cfgs); err != nil {
		return enhance.DefaultCameraConfigs(), fmt.Errorf("parse config %q: %w", path, err)
	}
	// A file that lists no per-camera overrides leaves the map nil, and every
	// camera then falls back to Default. Allocating it here means callers can
	// read from it without a nil check.
	if cfgs.Cameras == nil {
		cfgs.Cameras = make(map[string]enhance.Config)
	}
	return cfgs, nil
}
