// Package config assembles the detector's settings from the two places they can
// come from: a JSON config file, and environment variables that override it.
//
// This lives outside the detect package on purpose. detect is about running a
// model on an image; where the settings came from — a file, a variable, a
// built-in default — is not its concern, and keeping the two apart means detect
// can be exercised in a test without a config file existing anywhere.
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/w0rxbend/instachron/services/camera-web-restream-detector-api/internal/detect"
	"github.com/w0rxbend/instachron/shared/envconf"
)

// DefaultPath is the config file read when CONFIG_FILE is unset.
const DefaultPath = "config.json"

// Load reads the detector config from path and applies environment overrides.
//
// A missing or unreadable file is not an error the caller has to handle: Load
// returns the built-in defaults along with a non-nil err describing what went
// wrong, so the caller can log the reason and carry on with a usable config.
// That is deliberate — a detector that starts with defaults and passes frames
// through is far more useful than one that refuses to start because nobody
// wrote a config file.
func Load(path string) (detect.Config, error) {
	cfg := detect.DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return applyEnv(cfg), fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return applyEnv(detect.DefaultConfig()), fmt.Errorf("parse config %q: %w", path, err)
	}
	return applyEnv(cfg), nil
}

// applyEnv overlays the environment variables that may override the file.
//
// Only ORT_LIB_PATH is overridable today. It gets this treatment because it is
// a machine-specific filesystem path: the same config.json is shared between
// developers and containers, while libonnxruntime.so sits in a different place
// on each of them, so pointing at a locally downloaded copy should not mean
// editing a file that is under version control.
func applyEnv(cfg detect.Config) detect.Config {
	cfg.OrtLibPath = envconf.String("ORT_LIB_PATH", cfg.OrtLibPath)
	return cfg
}
