package metrics

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	activeEncoders atomic.Int64
	storageBytes   atomic.Int64
	encoderErrors  atomic.Int64
	segmentsDone   atomic.Int64

	mu      sync.Mutex
	cameras map[string]*Camera
}

type Camera struct {
	framesReceived int64
	framesRecorded int64
	framesDropped  int64
	segmentsDone   int64
}

func New() *Metrics {
	return &Metrics{cameras: make(map[string]*Camera)}
}

func (m *Metrics) IncFramesReceived(cameraID string) {
	m.mu.Lock()
	m.ensureCamera(cameraID).framesReceived++
	m.mu.Unlock()
}

func (m *Metrics) IncFramesRecorded(cameraID string) {
	m.mu.Lock()
	m.ensureCamera(cameraID).framesRecorded++
	m.mu.Unlock()
}

func (m *Metrics) IncFramesDropped(cameraID string) {
	m.mu.Lock()
	m.ensureCamera(cameraID).framesDropped++
	m.mu.Unlock()
}

func (m *Metrics) IncSegmentCompleted(cameraID string) {
	m.segmentsDone.Add(1)
	m.mu.Lock()
	m.ensureCamera(cameraID).segmentsDone++
	m.mu.Unlock()
}

func (m *Metrics) IncEncoderError()        { m.encoderErrors.Add(1) }
func (m *Metrics) IncActiveEncoders()      { m.activeEncoders.Add(1) }
func (m *Metrics) DecActiveEncoders()      { m.activeEncoders.Add(-1) }
func (m *Metrics) SetStorageBytes(n int64) { m.storageBytes.Store(n) }

func (m *Metrics) ActiveCameras() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.cameras)
}

func (m *Metrics) WritePrometheus(w io.Writer) {
	snapshot := m.snapshot()
	writeGauge(w, "camera_recorder_active_cameras", "Number of cameras seen by the recorder.", float64(len(snapshot.cameras)))
	writeGauge(w, "camera_recorder_active_encoders", "Number of currently running ffmpeg encoders.", float64(m.activeEncoders.Load()))
	writeGauge(w, "camera_recorder_storage_bytes", "Total completed MP4 bytes under recorder storage.", float64(m.storageBytes.Load()))
	writeCounter(w, "camera_recorder_encoder_errors_total", "Total ffmpeg encoder errors.", float64(m.encoderErrors.Load()))
	writeCounter(w, "camera_recorder_segments_completed_total", "Total completed video segments.", float64(m.segmentsDone.Load()))
	writeMetricHeader(w, "camera_recorder_frames_received_total", "Total frames received from upstream.", "counter")
	for _, id := range snapshot.ids {
		c := snapshot.cameras[id]
		writeSampleLabel(w, "camera_recorder_frames_received_total", id, float64(c.framesReceived))
	}
	writeMetricHeader(w, "camera_recorder_frames_recorded_total", "Total frames selected for timelapse recording.", "counter")
	for _, id := range snapshot.ids {
		c := snapshot.cameras[id]
		writeSampleLabel(w, "camera_recorder_frames_recorded_total", id, float64(c.framesRecorded))
	}
	writeMetricHeader(w, "camera_recorder_frames_dropped_total", "Total frames dropped by recorder queues or timelapse sampling.", "counter")
	for _, id := range snapshot.ids {
		c := snapshot.cameras[id]
		writeSampleLabel(w, "camera_recorder_frames_dropped_total", id, float64(c.framesDropped))
	}
	writeMetricHeader(w, "camera_recorder_camera_segments_completed_total", "Completed video segments per camera.", "counter")
	for _, id := range snapshot.ids {
		c := snapshot.cameras[id]
		writeSampleLabel(w, "camera_recorder_camera_segments_completed_total", id, float64(c.segmentsDone))
	}
}

type snap struct {
	ids     []string
	cameras map[string]Camera
}

func (m *Metrics) snapshot() snap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := snap{cameras: make(map[string]Camera, len(m.cameras))}
	for id, c := range m.cameras {
		out.ids = append(out.ids, id)
		out.cameras[id] = *c
	}
	sort.Strings(out.ids)
	return out
}

func (m *Metrics) ensureCamera(id string) *Camera {
	c := m.cameras[id]
	if c == nil {
		c = &Camera{}
		m.cameras[id] = c
	}
	return c
}

func writeGauge(w io.Writer, name, help string, value float64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n", name, help, name, name, value)
}

func writeCounter(w io.Writer, name, help string, value float64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %g\n", name, help, name, name, value)
}

func writeMetricHeader(w io.Writer, name, help, metricType string) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, metricType)
}

func writeSampleLabel(w io.Writer, name, cameraID string, value float64) {
	_, _ = fmt.Fprintf(w, "%s{camera_id=%q} %g\n", name, cameraID, value)
}
