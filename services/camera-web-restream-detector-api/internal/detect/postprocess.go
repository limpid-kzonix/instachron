package detect

import "sort"

// Detection holds a single object detection result in original-image pixel space.
type Detection struct {
	ClassID    int
	ClassName  string
	Confidence float32
	X1, Y1     float32 // top-left
	X2, Y2     float32 // bottom-right
}

// parseParams groups everything parseOutput needs besides the raw model output.
// These values travel together and are all either thresholds or facts about the
// frame the output belongs to, so passing them as one value keeps the call site
// readable: with seven positional parameters, two of which are floats and two
// of which are ints, a transposed pair of arguments would still compile and
// would silently produce wrong boxes.
type parseParams struct {
	// Layout says how the model's output tensor is arranged in memory.
	Layout OutputLayout
	// ConfThreshold is the minimum class score for a box to be considered.
	ConfThreshold float32
	// NMSThreshold is the overlap above which two boxes of the same class are
	// treated as the same object, and the lower-scoring one is discarded.
	NMSThreshold float32
	// Letterbox describes how the frame was scaled and padded to reach the
	// model's input size, and knows how to undo that mapping.
	Letterbox letterboxResult
	// OrigW and OrigH are the dimensions of the frame before letterboxing, and
	// therefore the bounds that detections are clamped to.
	OrigW, OrigH int
}

// parseOutput decodes YOLOv8 ONNX output into filtered, NMS-applied detections
// mapped to original image space. It handles both common export layouts:
//
//	[1, 4+classes, boxes] — channel-first  (p.Layout.Transposed == false)
//	[1, boxes, 4+classes] — boxes-first    (p.Layout.Transposed == true)
func parseOutput(data []float32, p parseParams) []Detection {
	layout := p.Layout
	numBoxes := layout.NumBoxes
	numClasses := layout.NumChannels - 4

	// at returns data[channel, box] regardless of storage layout
	var at func(ch, box int) float32
	if layout.Transposed {
		// data[box * numChannels + ch]
		nc := layout.NumChannels
		at = func(ch, box int) float32 { return data[box*nc+ch] }
	} else {
		// data[ch * numBoxes + box]
		at = func(ch, box int) float32 { return data[ch*numBoxes+box] }
	}

	var candidates []Detection

	for i := 0; i < numBoxes; i++ {
		// find max class score
		bestScore := float32(0)
		bestClass := 0
		for c := 0; c < numClasses; c++ {
			s := at(4+c, i)
			if s > bestScore {
				bestScore = s
				bestClass = c
			}
		}
		if bestScore < p.ConfThreshold {
			continue
		}

		cx := at(0, i)
		cy := at(1, i)
		w := at(2, i)
		h := at(3, i)

		// The model reports each box as a centre point with a width and a
		// height; the rest of this package works in corners, so convert.
		bx1 := cx - w/2
		by1 := cy - h/2
		bx2 := cx + w/2
		by2 := cy + h/2

		// Those corners are in the letterboxed square the model was fed, so map
		// both of them back to where they belong in the original frame.
		ox1, oy1 := p.Letterbox.toOriginal(bx1, by1, p.OrigW, p.OrigH)
		ox2, oy2 := p.Letterbox.toOriginal(bx2, by2, p.OrigW, p.OrigH)

		if ox2 <= ox1 || oy2 <= oy1 {
			continue
		}

		className := "unknown"
		if bestClass < len(CocoClasses) {
			className = CocoClasses[bestClass]
		}

		candidates = append(candidates, Detection{
			ClassID:    bestClass,
			ClassName:  className,
			Confidence: bestScore,
			X1:         ox1,
			Y1:         oy1,
			X2:         ox2,
			Y2:         oy2,
		})
	}

	return nms(candidates, p.NMSThreshold)
}

// nms applies per-class non-maximum suppression.
func nms(dets []Detection, iouThresh float32) []Detection {
	byClass := make(map[int][]Detection)
	for _, d := range dets {
		byClass[d.ClassID] = append(byClass[d.ClassID], d)
	}

	var result []Detection
	for _, group := range byClass {
		sort.Slice(group, func(i, j int) bool {
			return group[i].Confidence > group[j].Confidence
		})
		suppressed := make([]bool, len(group))
		for i := range group {
			if suppressed[i] {
				continue
			}
			result = append(result, group[i])
			for j := i + 1; j < len(group); j++ {
				if !suppressed[j] && boxIoU(group[i], group[j]) > iouThresh {
					suppressed[j] = true
				}
			}
		}
	}
	return result
}

func boxIoU(a, b Detection) float32 {
	ix1 := max(a.X1, b.X1)
	iy1 := max(a.Y1, b.Y1)
	ix2 := min(a.X2, b.X2)
	iy2 := min(a.Y2, b.Y2)
	inter := max(0, ix2-ix1) * max(0, iy2-iy1)
	aArea := (a.X2 - a.X1) * (a.Y2 - a.Y1)
	bArea := (b.X2 - b.X1) * (b.Y2 - b.Y1)
	return inter / (aArea + bArea - inter + 1e-6)
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
