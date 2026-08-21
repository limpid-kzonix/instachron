// Package detect runs YOLOv8 object detection on JPEG frames using ONNX Runtime.
package detect

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disintegration/imaging"
	ort "github.com/yalue/onnxruntime_go"
)

// Config holds detector parameters loaded from config.json.
type Config struct {
	ModelPath      string   `json:"model_path"`
	OrtLibPath     string   `json:"ort_lib_path"`    // path to libonnxruntime.so; "" = default "onnxruntime.so"
	InputName      string   `json:"input_name"`      // default "images"
	OutputName     string   `json:"output_name"`     // default "output0"
	ConfThreshold  float32  `json:"conf_threshold"`  // default 0.25
	NMSThreshold   float32  `json:"nms_threshold"`   // default 0.45
	InputWidth     int      `json:"input_width"`     // default 640
	InputHeight    int      `json:"input_height"`    // default 640
	NumClasses     int      `json:"num_classes"`     // default 80
	JPEGQuality    int      `json:"jpeg_quality"`    // default 85
	AllowedClasses []string `json:"allowed_classes"` // if non-empty, only these class names are kept
	Debug          bool     `json:"debug"`
}

func DefaultConfig() Config {
	return Config{
		ModelPath:     "models/yolov8n.onnx",
		OrtLibPath:    "libonnxruntime.so",
		InputName:     "images",
		OutputName:    "output0",
		ConfThreshold: 0.25,
		NMSThreshold:  0.45,
		InputWidth:    640,
		InputHeight:   640,
		NumClasses:    80,
		JPEGQuality:   85,
	}
}

// OutputLayout describes how the model's flat output array is indexed.
type OutputLayout struct {
	NumBoxes    int
	NumChannels int
	Transposed  bool // true = [1, numBoxes, numChannels]; false = [1, numChannels, numBoxes]
}

// Detector runs YOLOv8 inference.
// Inference runs every detectEveryN frames; every frame is annotated with the
// most recently computed detections so bounding boxes are visible on all frames.
type Detector struct {
	cfg          Config
	session      *ort.AdvancedSession
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
	layout       OutputLayout
	allowedIDs   map[int]struct{} // nil = all classes allowed
	logger       *log.Logger
	mu           sync.Mutex // guards ORT session
	bufPool      sync.Pool

	lastDetsMu sync.RWMutex
	lastDets   []Detection // most recent inference result

	// periodic stats (logged every ~10s)
	frameIn    atomic.Int64
	frameDet   atomic.Int64
	lastLog    atomic.Int64 // UnixNano of last stats log
	frameCount atomic.Int64 // monotonic counter for decimation
}

// ONNX Runtime keeps one environment per process, so initialisation has to
// happen exactly once no matter how many detectors are built. ortOnce enforces
// that, and ortInitErr remembers the outcome: without it a second caller would
// see a nil error and go on to use an environment that was never initialised.
var (
	ortOnce    sync.Once
	ortInitErr error
)

// initORT initialises the process-wide ONNX Runtime environment on the first
// call and returns the result of that first call on every later one.
//
// Only the first caller's cfg.OrtLibPath has any effect, because the shared
// library is loaded once. That is not a limitation in practice — this service
// builds a single detector — but it is why the path is read here rather than
// stored on the Detector.
func initORT(cfg Config) error {
	ortOnce.Do(func() {
		if cfg.OrtLibPath != "" {
			ort.SetSharedLibraryPath(cfg.OrtLibPath)
		}
		ortInitErr = ort.InitializeEnvironment(ort.WithLogLevelError())
	})
	return ortInitErr
}

// newTensors allocates the fixed input and output buffers that every inference
// reuses, sized from cfg and the probed output layout.
//
// These are allocated by the C library rather than by Go, so the garbage
// collector will not reclaim them: each one has to be handed back with
// Destroy. That is why the failure path below destroys the input tensor before
// returning — an error after a successful allocation would otherwise leak it,
// and the caller has no handle to free something it never received.
func newTensors(cfg Config, layout OutputLayout) (input, output *ort.Tensor[float32], err error) {
	// The input is always one RGB image at the model's configured size, in
	// channel-first order: [batch=1, channels=3, height, width].
	inputShape := ort.NewShape(1, 3, int64(cfg.InputHeight), int64(cfg.InputWidth))
	input, err = ort.NewEmptyTensor[float32](inputShape)
	if err != nil {
		return nil, nil, fmt.Errorf("input tensor: %w", err)
	}

	// The output shape depends on how the model was exported; both orderings
	// hold the same numbers, so only the axis order differs.
	outputShape := ort.NewShape(1, int64(layout.NumChannels), int64(layout.NumBoxes))
	if layout.Transposed {
		outputShape = ort.NewShape(1, int64(layout.NumBoxes), int64(layout.NumChannels))
	}
	output, err = ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		_ = input.Destroy()
		return nil, nil, fmt.Errorf("output tensor: %w", err)
	}
	return input, output, nil
}

// allowedClassIDs turns the configured list of class names into the set of
// numeric class IDs the model uses, so filtering a detection later is a map
// lookup rather than a string comparison.
//
// An empty list returns nil, which every caller reads as "no filter, keep
// everything" — that is the difference between an empty set and no set at all,
// and it is why the return value is nil rather than an empty map. A name that
// is not a known class is reported and skipped, so one typo in a config file
// costs that one class rather than silently filtering everything away.
func allowedClassIDs(names []string, logger *log.Logger) map[int]struct{} {
	if len(names) == 0 {
		return nil
	}
	nameToID := make(map[string]int, len(CocoClasses))
	for i, name := range CocoClasses {
		nameToID[name] = i
	}
	allowed := make(map[int]struct{}, len(names))
	for _, name := range names {
		id, ok := nameToID[name]
		if !ok {
			logger.Printf("allowed_classes: unknown class name %q (ignored)", name)
			continue
		}
		allowed[id] = struct{}{}
	}
	return allowed
}

// New initialises the ONNX Runtime environment (once per process) and loads the model.
// logger is used for debug output when cfg.Debug is true; pass nil to use the default logger.
func New(cfg Config, logger *log.Logger) (*Detector, error) {
	if logger == nil {
		logger = log.Default()
	}
	if err := initORT(cfg); err != nil {
		return nil, fmt.Errorf("ort init: %w", err)
	}

	// Probe the model to discover actual tensor names and output shape.
	// This handles any YOLOv8 export variant without manual name configuration.
	probe, err := probeModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("probe model: %w", err)
	}
	logger.Printf("model probe: input=%q output=%q shape=%+v", probe.InputName, probe.OutputName, probe.Layout)

	inputTensor, outputTensor, err := newTensors(cfg, probe.Layout)
	if err != nil {
		return nil, err
	}

	session, err := ort.NewAdvancedSession(
		cfg.ModelPath,
		[]string{probe.InputName},
		[]string{probe.OutputName},
		[]ort.Value{inputTensor},
		[]ort.Value{outputTensor},
		nil,
	)
	if err != nil {
		// Same reasoning as in newTensors: nothing else will free these.
		_ = inputTensor.Destroy()
		_ = outputTensor.Destroy()
		return nil, fmt.Errorf("session: %w", err)
	}

	allowed := allowedClassIDs(cfg.AllowedClasses, logger)
	if allowed != nil {
		logger.Printf("class filter: only showing %v", cfg.AllowedClasses)
	}

	d := &Detector{
		cfg:          cfg,
		session:      session,
		inputTensor:  inputTensor,
		outputTensor: outputTensor,
		layout:       probe.Layout,
		allowedIDs:   allowed,
		logger:       logger,
		bufPool:      sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}
	d.lastLog.Store(time.Now().UnixNano())
	return d, nil
}

type modelProbe struct {
	InputName  string
	OutputName string
	Layout     OutputLayout
}

// probeModel inspects the ONNX model file to discover the actual input/output
// tensor names and output shape, so the session works regardless of export naming.
func probeModel(cfg Config) (modelProbe, error) {
	inputs, outputs, err := ort.GetInputOutputInfo(cfg.ModelPath)
	if err != nil {
		return modelProbe{}, fmt.Errorf("GetInputOutputInfo: %w", err)
	}
	if len(inputs) == 0 {
		return modelProbe{}, fmt.Errorf("model has no inputs")
	}
	if len(outputs) == 0 {
		return modelProbe{}, fmt.Errorf("model has no outputs")
	}

	inputName := inputs[0].Name
	outputName := outputs[0].Name

	// Override with config values if explicitly set (non-default)
	if cfg.InputName != "" && cfg.InputName != "images" {
		inputName = cfg.InputName
	}
	if cfg.OutputName != "" && cfg.OutputName != "output0" {
		outputName = cfg.OutputName
	}

	dims := outputs[0].Dimensions
	if len(dims) != 3 {
		return modelProbe{}, fmt.Errorf("expected 3-D output, got %v", dims)
	}
	// dims = [batch, A, B]; batch is always 1
	a, b := int(dims[1]), int(dims[2])
	numChannels := 4 + cfg.NumClasses

	var layout OutputLayout
	switch {
	case a == numChannels: // [1, 84, 8400] — channel-first
		layout = OutputLayout{NumBoxes: b, NumChannels: a, Transposed: false}
	case b == numChannels: // [1, 8400, 84] — transposed / boxes-first
		layout = OutputLayout{NumBoxes: a, NumChannels: b, Transposed: true}
	default:
		return modelProbe{}, fmt.Errorf(
			"output shape %v doesn't match expected numChannels=%d", dims, numChannels)
	}

	return modelProbe{InputName: inputName, OutputName: outputName, Layout: layout}, nil
}

// Layout returns the probed output layout (useful for logging).
func (d *Detector) Layout() OutputLayout { return d.layout }

// String renders the layout in the order the numbers actually appear in the
// model's output tensor, so a log line shows both the shape and its meaning.
func (l OutputLayout) String() string {
	if l.Transposed {
		return fmt.Sprintf("transposed [1,%d,%d]", l.NumBoxes, l.NumChannels)
	}
	return fmt.Sprintf("channel-first [1,%d,%d]", l.NumChannels, l.NumBoxes)
}

// Destroy releases ORT resources. Call once when the detector is no longer needed.
func (d *Detector) Destroy() {
	_ = d.session.Destroy()
	_ = d.inputTensor.Destroy()
	_ = d.outputTensor.Destroy()
}

// Detect runs inference on jpeg, returning the detections and the annotated JPEG.
// It blocks until inference completes (unlike Process which drops busy frames).
func (d *Detector) Detect(jpeg []byte) ([]Detection, []byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.infer(jpeg)
}

const detectEveryN = 15

// Process runs inference and annotation for one frame.
// Inference runs on every Nth frame to refresh detections; every frame is
// annotated with the most recent detections so bounding boxes are always visible.
func (d *Detector) Process(jpeg []byte, push func([]byte)) {
	n := d.frameCount.Add(1)
	d.frameIn.Add(1)

	// Refresh detections on every Nth frame (non-blocking — skip if already running).
	if n%detectEveryN == 0 && d.mu.TryLock() {
		dets, err := d.inferenceOnly(jpeg)
		d.mu.Unlock()
		if err != nil {
			d.logger.Printf("detect: inference error: %v", err)
		} else {
			if len(dets) > 0 {
				d.frameDet.Add(1)
			}
			d.lastDetsMu.Lock()
			d.lastDets = dets
			d.lastDetsMu.Unlock()
		}
	}

	// Annotate every frame with the latest known detections.
	d.lastDetsMu.RLock()
	dets := d.lastDets
	d.lastDetsMu.RUnlock()

	annotated, err := d.annotateJPEG(jpeg, dets)
	if err != nil {
		d.logger.Printf("detect: annotate error: %v", err)
		push(jpeg)
	} else {
		push(annotated)
	}
	d.logStats()
}

// logStats prints a summary line every 10 seconds.
func (d *Detector) logStats() {
	now := time.Now().UnixNano()
	last := d.lastLog.Load()
	if now-last < int64(10*time.Second) {
		return
	}
	if !d.lastLog.CompareAndSwap(last, now) {
		return
	}
	in := d.frameIn.Load()
	det := d.frameDet.Load()
	d.logger.Printf("detect stats: %d frames annotated, %d inference runs with detections (every %dth frame, conf_threshold=%.2f)",
		in, det, detectEveryN, d.cfg.ConfThreshold)
}

// inferenceOnly runs the ONNX model and returns detections. Caller must hold d.mu.
func (d *Detector) inferenceOnly(jpeg []byte) ([]Detection, error) {
	src, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	lb := letterbox(src, d.cfg.InputWidth, d.cfg.InputHeight)
	toTensor(lb.img, d.inputTensor.GetData())

	if err := d.session.Run(); err != nil {
		return nil, fmt.Errorf("inference: %w", err)
	}

	raw := d.outputTensor.GetData()

	if d.cfg.Debug {
		maxScore := float32(0)
		for _, v := range raw {
			if v > maxScore {
				maxScore = v
			}
		}
		d.logger.Printf("detect debug: output max_value=%.4f layout=%+v", maxScore, d.layout)
	}

	dets := parseOutput(raw, parseParams{
		Layout:        d.layout,
		ConfThreshold: d.cfg.ConfThreshold,
		NMSThreshold:  d.cfg.NMSThreshold,
		Letterbox:     lb,
		OrigW:         src.Bounds().Dx(),
		OrigH:         src.Bounds().Dy(),
	})

	if d.allowedIDs != nil {
		filtered := dets[:0]
		for _, det := range dets {
			if _, ok := d.allowedIDs[det.ClassID]; ok {
				filtered = append(filtered, det)
			}
		}
		dets = filtered
	}

	if d.cfg.Debug {
		d.logger.Printf("detect debug: %d detection(s) (conf_threshold=%.2f)", len(dets), d.cfg.ConfThreshold)
		for _, det := range dets {
			d.logger.Printf("  [%s] conf=%.3f box=(%.0f,%.0f)-(%.0f,%.0f)",
				det.ClassName, det.Confidence, det.X1, det.Y1, det.X2, det.Y2)
		}
	}

	return dets, nil
}

// annotateJPEG draws dets onto jpeg and returns the encoded result.
func (d *Detector) annotateJPEG(jpeg []byte, dets []Detection) ([]byte, error) {
	src, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	nrgba := toNRGBA(src)
	annotated := Annotate(nrgba, dets)

	buf := d.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer d.bufPool.Put(buf)

	if err := imaging.Encode(buf, annotated, imaging.JPEG, imaging.JPEGQuality(d.cfg.JPEGQuality)); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

// infer is used by Detect (full pipeline, caller must hold d.mu).
func (d *Detector) infer(jpeg []byte) ([]Detection, []byte, error) {
	dets, err := d.inferenceOnly(jpeg)
	if err != nil {
		return nil, nil, err
	}
	annotated, err := d.annotateJPEG(jpeg, dets)
	return dets, annotated, err
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	dst := image.NewNRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}
