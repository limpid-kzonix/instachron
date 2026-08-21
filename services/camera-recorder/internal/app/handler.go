package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/recorder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
)

type apiServer struct {
	store   storage.Store
	rec     *recorder.Sessions
	metrics *metrics.Metrics
	logger  *log.Logger
}

func newAPI(store storage.Store, rec *recorder.Sessions, m *metrics.Metrics, logger *log.Logger) *apiServer {
	return &apiServer{store: store, rec: rec, metrics: m, logger: logger}
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /cameras", s.handleCameras)
	mux.HandleFunc("GET /videos", s.handleVideos)
	mux.HandleFunc("GET /videos/{cameraID}/{fileName}", s.handleVideoFile)
	return mux
}

func (s *apiServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"service": "camera-recorder", "status": "ok"})
}

func (s *apiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if n, err := s.store.UsageBytes(r.Context()); err == nil {
		s.metrics.SetStorageBytes(n)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.metrics.WritePrometheus(w)
}

func (s *apiServer) handleCameras(w http.ResponseWriter, r *http.Request) {
	cameras, err := s.store.Cameras(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"cameras":        cameras,
		"active_cameras": s.rec.ActiveCameraIDs(),
	})
}

func (s *apiServer) handleVideos(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	files, err := s.store.List(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, files)
}

func (s *apiServer) handleVideoFile(w http.ResponseWriter, r *http.Request) {
	cameraID := r.PathValue("cameraID")
	fileName := r.PathValue("fileName")
	f, info, err := s.store.Open(r.Context(), cameraID, fileName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeContent(w, r, info.FileName, info.EndedAt, f)
}

func parseListFilter(r *http.Request) (storage.ListFilter, error) {
	q := r.URL.Query()
	filter := storage.ListFilter{
		CameraID: q.Get("camera_id"),
		Limit:    100,
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return filter, err
		}
		if n < 0 {
			return filter, fmt.Errorf("limit must be greater than or equal to 0")
		}
		if n > 1000 {
			n = 1000
		}
		filter.Limit = n
	}
	var err error
	if filter.From, err = parseTime(q.Get("from")); err != nil {
		return filter, err
	}
	if filter.To, err = parseTime(q.Get("to")); err != nil {
		return filter, err
	}
	return filter, nil
}

func parseTime(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(n, 0), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
