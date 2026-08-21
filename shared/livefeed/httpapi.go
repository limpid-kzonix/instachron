package livefeed

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/w0rxbend/instachron/shared/mjpeg"
	"github.com/w0rxbend/instachron/shared/webui"
)

// Response bodies returned on the two error paths that are not plain 404s.
const (
	msgNoFrameYet         = "no frame received yet"
	msgStreamingUnsupport = "streaming not supported"
)

// APIOptions adjusts the parts of the camera API that genuinely differ between
// the services that serve it. Everything not listed here — the routes, the
// headers, the status codes, the JSON shape — is identical everywhere on
// purpose, because it is the project's published HTTP contract.
//
// The zero value is what the four restream proxies want, so they pass nothing.
type APIOptions struct {
	// Rotation reports the display rotation in degrees for a camera ID, for the
	// /cameras listing. Leave it nil when the service does not rotate frames,
	// and every camera is reported with a rotation of 0.
	Rotation func(id string) int

	// CreateOnSubscribe changes what /cameras/{id}/stream does for a camera
	// that has not sent a frame yet. False — the default — returns 404, which
	// is what a proxy wants: it only ever knows about cameras it has seen
	// frames for, so an unknown ID is a client mistake. True attaches to the
	// camera anyway and streams nothing until frames start arriving, which is
	// what the service talking to the cameras directly wants, so that a viewer
	// can open a stream before the camera has finished connecting.
	CreateOnSubscribe bool

	// LogSubscribers logs a line when a stream subscriber attaches and another
	// when it detaches. Useful on the service humans point their browsers at;
	// noise on a proxy whose only client is another proxy.
	LogSubscribers bool
}

// NewCameraAPI returns the HTTP handler that every restream proxy serves:
// the bundled dashboard at /, the camera list at /cameras, and per-camera
// snapshot and MJPEG stream endpoints. This is the project's published HTTP
// contract — the bundled web UI and external clients depend on these exact
// routes, headers and status codes.
//
// Pass the zero APIOptions for the standard proxy behaviour.
func NewCameraAPI(m *Registry, logger *log.Logger, opts APIOptions) http.Handler {
	s := &apiServer{registry: m, logger: logger, opts: opts}
	return s.routes()
}

type apiServer struct {
	registry *Registry
	logger   *log.Logger
	opts     APIOptions
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /cameras", s.handleCameras)
	mux.HandleFunc("GET /cameras/{id}/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /cameras/{id}/stream", s.handleStream)
	return mux
}

func (s *apiServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(webui.IndexHTML)
}

// handleCameras returns a JSON array of cameras.Info for every camera seen
// since startup, including cameras that are currently offline.
func (s *apiServer) handleCameras(w http.ResponseWriter, r *http.Request) {
	// A nil Rotation reports every camera with a rotation of 0, which is right
	// for the proxies: they republish frames without rotating them.
	cams := s.registry.KnownCameras(s.opts.Rotation)
	if s.opts.LogSubscribers {
		s.logger.Printf("GET /cameras -> %d camera(s)", len(cams))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cams)
}

func (s *apiServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h := s.registry.Lookup(id)
	if h == nil {
		http.NotFound(w, r)
		return
	}

	f := h.LatestFrame()
	if f == nil {
		http.Error(w, msgNoFrameYet, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	_, _ = w.Write(f)
}

// hubForStream finds the hub a stream request should attach to, or nil when
// there is nothing to attach to and the request should 404. See
// APIOptions.CreateOnSubscribe for why the two answers differ by service.
func (s *apiServer) hubForStream(id string) *Hub {
	if s.opts.CreateOnSubscribe {
		return s.registry.Hub(id)
	}
	return s.registry.Lookup(id)
}

func (s *apiServer) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, msgStreamingUnsupport, http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	h := s.hubForStream(id)
	if h == nil {
		http.NotFound(w, r)
		return
	}
	if s.opts.LogSubscribers {
		s.logger.Printf("GET /cameras/%s/stream -> subscriber attached", id)
		defer s.logger.Printf("GET /cameras/%s/stream -> subscriber detached", id)
	}

	w.Header().Set("Content-Type", mjpeg.ContentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case f, ok := <-ch:
			if !ok {
				return
			}
			if err := mjpeg.WriteFrame(w, f); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
