package app

import (
	"testing"
)

func TestLoadConfigParsesCameraIDFlag(t *testing.T) {
	t.Setenv("STREAM_URL", "rtmp://example/live/key")
	t.Setenv("RTMP_URL", "")
	t.Setenv("TWITCH_STREAM_KEY", "")
	t.Setenv("YOUTUBE_STREAM_KEY", "")

	got, err := loadConfig([]string{"--camera-id", "42"})
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if got.cameraID != 42 {
		t.Fatalf("cameraID = %d, want 42", got.cameraID)
	}
}

func TestStreamURLFromEnv(t *testing.T) {
	t.Setenv("STREAM_URL", "")
	t.Setenv("RTMP_URL", "")
	t.Setenv("TWITCH_STREAM_KEY", "twitch-key")
	t.Setenv("YOUTUBE_STREAM_KEY", "")

	got, err := streamURLFromEnv()
	if err != nil {
		t.Fatalf("streamURLFromEnv returned error: %v", err)
	}

	want := "rtmp://live.twitch.tv/app/twitch-key"
	if got != want {
		t.Fatalf("stream URL = %s, want %s", got, want)
	}
}
