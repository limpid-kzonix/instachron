package detect

import (
	"reflect"
	"sort"
	"testing"
)

// These tests exercise the pure decoding maths only. Nothing here loads the
// ONNX Runtime shared library — that happens in New, which none of this
// touches — so they run on any machine.

// numChannels is what a YOLOv8 model trained on the 80 COCO classes emits per
// box: 4 box coordinates (centre x, centre y, width, height) followed by one
// score per class.
const numChannels = 4 + 80

// rawBox is one prediction as the model would emit it, before decoding.
type rawBox struct {
	cx, cy, w, h float32
	scores       map[int]float32 // class index -> score; classes left out score 0
}

// encodeBoxes flattens boxes into the single float32 array an ONNX session
// hands back. YOLOv8 exports disagree about the ordering of that array, and
// parseOutput has to cope with both:
//
//	transposed == false: [1, numChannels, numBoxes] — every box's centre-x,
//	                     then every box's centre-y, and so on channel by channel.
//	transposed == true:  [1, numBoxes, numChannels] — all 84 numbers for box 0,
//	                     then all 84 numbers for box 1.
//
// Reading one layout as if it were the other does not fail loudly: it produces
// plausible-looking boxes in the wrong places. Encoding the same detection both
// ways and demanding the same result is what pins that down.
func encodeBoxes(boxes []rawBox, transposed bool) []float32 {
	data := make([]float32, len(boxes)*numChannels)
	set := func(ch, box int, v float32) {
		if transposed {
			data[box*numChannels+ch] = v
		} else {
			data[ch*len(boxes)+box] = v
		}
	}
	for i, b := range boxes {
		set(0, i, b.cx)
		set(1, i, b.cy)
		set(2, i, b.w)
		set(3, i, b.h)
		for class, score := range b.scores {
			set(4+class, i, score)
		}
	}
	return data
}

// identityLetterbox describes a letterboxing that did nothing: no scaling and
// no padding, so letterboxed coordinates are already original-image ones.
var identityLetterbox = letterboxResult{scale: 1, padLeft: 0, padTop: 0}

func approxEqual(a, b, tol float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func detectionsEqual(a, b Detection, tol float32) bool {
	return a.ClassID == b.ClassID &&
		a.ClassName == b.ClassName &&
		approxEqual(a.Confidence, b.Confidence, tol) &&
		approxEqual(a.X1, b.X1, tol) &&
		approxEqual(a.Y1, b.Y1, tol) &&
		approxEqual(a.X2, b.X2, tol) &&
		approxEqual(a.Y2, b.Y2, tol)
}

// TestParseOutputLayoutsAgree feeds parseOutput the same logical detection in
// both storage layouts and requires byte-identical results.
// squareParams builds the parse settings used by the cases that feed the model
// a frame that was already the model's own input size, so no letterbox mapping
// is involved and detections come back in the coordinates they went in with.
func squareParams(layout OutputLayout, lb letterboxResult) parseParams {
	return parseParams{
		Layout:        layout,
		ConfThreshold: 0.5,
		NMSThreshold:  0.5,
		Letterbox:     lb,
		OrigW:         640,
		OrigH:         640,
	}
}

func TestParseOutputLayoutsAgree(t *testing.T) {
	boxes := []rawBox{
		// A "car" (COCO class 2) centred at (100, 100), 50 wide and 40 tall.
		{cx: 100, cy: 100, w: 50, h: 40, scores: map[int]float32{2: 0.9}},
		// A second, empty slot: score 0 keeps it under any threshold.
		{},
	}

	layout := OutputLayout{NumBoxes: len(boxes), NumChannels: numChannels}

	layout.Transposed = false
	channelFirst := parseOutput(encodeBoxes(boxes, false), squareParams(layout, identityLetterbox))

	layout.Transposed = true
	boxesFirst := parseOutput(encodeBoxes(boxes, true), squareParams(layout, identityLetterbox))

	if !reflect.DeepEqual(channelFirst, boxesFirst) {
		t.Fatalf("layouts disagree:\n channel-first: %+v\n boxes-first:   %+v", channelFirst, boxesFirst)
	}

	// Both must also decode to the box we encoded: centre (100, 100) with size
	// 50x40 spans x from 75 to 125 and y from 80 to 120.
	want := Detection{ClassID: 2, ClassName: CocoClasses[2], Confidence: 0.9, X1: 75, Y1: 80, X2: 125, Y2: 120}
	if len(channelFirst) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(channelFirst), channelFirst)
	}
	if !detectionsEqual(channelFirst[0], want, 1e-3) {
		t.Errorf("got %+v, want %+v", channelFirst[0], want)
	}
}

// TestParseOutputDrops covers the two reasons a decoded box never becomes a
// Detection: it scored too low, or it collapsed to zero area.
func TestParseOutputDrops(t *testing.T) {
	tests := []struct {
		name string
		box  rawBox
	}{
		{
			name: "score below threshold",
			box:  rawBox{cx: 100, cy: 100, w: 50, h: 40, scores: map[int]float32{2: 0.2}},
		},
		{
			name: "zero width leaves x2 <= x1",
			box:  rawBox{cx: 100, cy: 100, w: 0, h: 40, scores: map[int]float32{2: 0.9}},
		},
		{
			name: "zero height leaves y2 <= y1",
			box:  rawBox{cx: 100, cy: 100, w: 50, h: 0, scores: map[int]float32{2: 0.9}},
		},
		{
			name: "box entirely left of the image clamps to zero width",
			box:  rawBox{cx: -100, cy: 100, w: 50, h: 40, scores: map[int]float32{2: 0.9}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, transposed := range []bool{false, true} {
				layout := OutputLayout{NumBoxes: 1, NumChannels: numChannels, Transposed: transposed}
				got := parseOutput(encodeBoxes([]rawBox{tt.box}, transposed), squareParams(layout, identityLetterbox))
				if len(got) != 0 {
					t.Errorf("transposed=%v: got %+v, want no detections", transposed, got)
				}
			}
		})
	}
}

// TestParseOutputUnletterbox checks the coordinate mapping back to the original
// image. The detector feeds the model a square image built by shrinking the
// frame and padding the leftover space, so decoded boxes must have that padding
// subtracted and the shrink undone before they mean anything on the frame.
func TestParseOutputUnletterbox(t *testing.T) {
	// The frame was halved (scale 0.5) and then offset by 80 pixels to the
	// right and 140 pixels down inside the square model input.
	lb := letterboxResult{scale: 0.5, padLeft: 80, padTop: 140}

	// In model coordinates the box spans x 180..220 and y 230..250.
	box := rawBox{cx: 200, cy: 240, w: 40, h: 20, scores: map[int]float32{0: 0.8}}

	// Undoing that: (180-80)/0.5 = 200 and (230-140)/0.5 = 180.
	want := Detection{ClassID: 0, ClassName: CocoClasses[0], Confidence: 0.8, X1: 200, Y1: 180, X2: 280, Y2: 220}

	for _, transposed := range []bool{false, true} {
		layout := OutputLayout{NumBoxes: 1, NumChannels: numChannels, Transposed: transposed}
		got := parseOutput(encodeBoxes([]rawBox{box}, transposed), parseParams{
			Layout: layout, ConfThreshold: 0.5, NMSThreshold: 0.5,
			Letterbox: lb, OrigW: 1280, OrigH: 720,
		})
		if len(got) != 1 {
			t.Fatalf("transposed=%v: got %d detections, want 1", transposed, len(got))
		}
		if !detectionsEqual(got[0], want, 1e-3) {
			t.Errorf("transposed=%v: got %+v, want %+v", transposed, got[0], want)
		}
	}
}

// TestNMS covers non-maximum suppression: near-duplicate boxes for the same
// class are the same object seen twice, so only the most confident survives,
// but two different classes over the same pixels are two findings and both stay.
func TestNMS(t *testing.T) {
	// Same rectangle give or take two pixels: their intersection-over-union is
	// far above the 0.5 threshold used below.
	low := Detection{ClassID: 2, ClassName: "car", Confidence: 0.6, X1: 10, Y1: 10, X2: 110, Y2: 110}
	high := Detection{ClassID: 2, ClassName: "car", Confidence: 0.9, X1: 12, Y1: 12, X2: 112, Y2: 112}

	t.Run("same class collapses to the higher score", func(t *testing.T) {
		got := nms([]Detection{low, high}, 0.5)
		if len(got) != 1 {
			t.Fatalf("got %d detections, want 1: %+v", len(got), got)
		}
		if got[0] != high {
			t.Errorf("got %+v, want the higher-confidence %+v", got[0], high)
		}
	})

	t.Run("different classes both survive", func(t *testing.T) {
		other := low
		other.ClassID = 7
		other.ClassName = "truck"

		got := nms([]Detection{other, high}, 0.5)
		if len(got) != 2 {
			t.Fatalf("got %d detections, want 2: %+v", len(got), got)
		}
		// nms walks a map keyed by class, so the order it returns classes in is
		// not defined; sort before comparing.
		sort.Slice(got, func(i, j int) bool { return got[i].ClassID < got[j].ClassID })
		want := []Detection{high, other}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("distant boxes of one class both survive", func(t *testing.T) {
		far := low
		far.X1, far.Y1, far.X2, far.Y2 = 500, 500, 600, 600
		got := nms([]Detection{far, high}, 0.5)
		if len(got) != 2 {
			t.Errorf("got %d detections, want 2: %+v", len(got), got)
		}
	})
}
