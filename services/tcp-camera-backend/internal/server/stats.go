package server

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

const frameStatsInterval = 5 * time.Second

type frameStats struct {
	logger   *log.Logger
	addr     string
	interval time.Duration
	done     chan struct{}
	once     sync.Once

	mu       sync.Mutex
	byCamera map[streamproto.CameraID]uint64
}

func newFrameStats(logger *log.Logger, addr string, interval time.Duration) *frameStats {
	return &frameStats{
		logger:   logger,
		addr:     addr,
		interval: interval,
		done:     make(chan struct{}),
		byCamera: make(map[streamproto.CameraID]uint64),
	}
}

func (s *frameStats) Record(cameraID streamproto.CameraID) {
	s.mu.Lock()
	s.byCamera[cameraID]++
	s.mu.Unlock()
}

func (s *frameStats) Run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.logAndReset()
		case <-s.done:
			return
		}
	}
}

func (s *frameStats) Stop() {
	s.once.Do(func() {
		close(s.done)
	})
}

func (s *frameStats) logAndReset() {
	s.mu.Lock()
	if len(s.byCamera) == 0 {
		s.mu.Unlock()
		return
	}

	counts := make(map[streamproto.CameraID]uint64, len(s.byCamera))
	var total uint64
	for cameraID, frames := range s.byCamera {
		counts[cameraID] = frames
		total += frames
	}
	clear(s.byCamera)
	s.mu.Unlock()

	s.logger.Printf("frame stats: addr=%s interval=%s frames=%d camera_frames=%s",
		s.addr, s.interval, total, formatCameraCounts(counts))
}

func formatCameraCounts(counts map[streamproto.CameraID]uint64) string {
	cameraIDs := make([]streamproto.CameraID, 0, len(counts))
	for cameraID := range counts {
		cameraIDs = append(cameraIDs, cameraID)
	}
	sort.Slice(cameraIDs, func(i, j int) bool {
		return cameraIDs[i] < cameraIDs[j]
	})

	parts := make([]string, 0, len(cameraIDs))
	for _, cameraID := range cameraIDs {
		parts = append(parts, fmt.Sprintf("camera=%d frames=%d", cameraID, counts[cameraID]))
	}
	return strings.Join(parts, ", ")
}
