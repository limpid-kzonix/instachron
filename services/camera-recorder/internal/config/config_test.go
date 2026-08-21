package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultsValidate(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatalf("defaults did not validate: %v", err)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9999")
	t.Setenv("UPSTREAM_TCP_ADDR", "camera-web-api:9001")
	t.Setenv("TIMELAPSE_FACTOR", "20")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Fatalf("HTTPAddr = %q, want :9999", cfg.HTTPAddr)
	}
	if cfg.UpstreamTCPAddr != "camera-web-api:9001" {
		t.Fatalf("UpstreamTCPAddr = %q", cfg.UpstreamTCPAddr)
	}
	if cfg.Recording.TimelapseFactor != 20 {
		t.Fatalf("TimelapseFactor = %d, want 20", cfg.Recording.TimelapseFactor)
	}
}

func TestLoadParsesDurations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"recording": {"segment_raw_duration": "10m"}}`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := cfg.SegmentDuration(); got != 10*time.Minute {
		t.Fatalf("SegmentDuration() = %v, want %v", got, 10*time.Minute)
	}
}
