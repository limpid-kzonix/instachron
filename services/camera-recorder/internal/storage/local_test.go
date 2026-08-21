package storage

import (
	"context"
	"testing"
	"time"
)

func TestLocalCompleteListAndOpen(t *testing.T) {
	store := NewLocal(t.TempDir())
	ctx := context.Background()
	start := time.Unix(100, 0).UTC()

	seg, err := store.BeginSegment(ctx, "7", start, 10, 10)
	if err != nil {
		t.Fatalf("BeginSegment returned error: %v", err)
	}
	if _, err := seg.Writer.Write([]byte("mp4")); err != nil {
		t.Fatalf("write segment: %v", err)
	}
	if err := seg.Writer.Close(); err != nil {
		t.Fatalf("close segment: %v", err)
	}
	info, err := store.CompleteSegment(ctx, seg, start.Add(time.Minute))
	if err != nil {
		t.Fatalf("CompleteSegment returned error: %v", err)
	}

	files, err := store.List(ctx, ListFilter{CameraID: "7"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(files) != 1 || files[0].FileName != info.FileName {
		t.Fatalf("files = %#v, want completed file", files)
	}

	f, _, err := store.Open(ctx, "7", info.FileName)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() { _ = f.Close() }()
}
