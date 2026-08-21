package recorder

import (
	"testing"
	"time"

	"github.com/w0rxbend/instachron/shared/imageutil"
)

func TestShouldKeepAppliesTimelapseInterval(t *testing.T) {
	var next time.Time
	start := time.Unix(100, 0)
	interval := time.Second

	if !shouldKeep(start, &next, interval) {
		t.Fatal("first frame should be kept")
	}
	if shouldKeep(start.Add(500*time.Millisecond), &next, interval) {
		t.Fatal("frame before next interval should be skipped")
	}
	if !shouldKeep(start.Add(time.Second), &next, interval) {
		t.Fatal("frame at next interval should be kept")
	}
}

func TestLooksLikeJPEG(t *testing.T) {
	if !imageutil.LooksLikeJPEG([]byte{0xFF, 0xD8, 0x00, 0xFF, 0xD9}) {
		t.Fatal("valid JPEG markers were rejected")
	}
	if imageutil.LooksLikeJPEG([]byte{0x00, 0xD8, 0x00, 0xFF, 0x00}) {
		t.Fatal("invalid JPEG markers were accepted")
	}
}
