package detect

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// allowedClassIDs is the part of detector construction that has no dependency
// on ONNX Runtime, which is what makes it testable here: the rest of New needs
// libonnxruntime.so and a model file, and cannot run in a plain unit test.
func TestAllowedClassIDs(t *testing.T) {
	tests := []struct {
		name      string
		classes   []string
		wantIDs   []int
		wantNil   bool
		wantWarn  string
		wantCount int
	}{
		{
			name:    "no filter configured means keep everything",
			classes: nil,
			wantNil: true,
		},
		{
			name:    "an empty list also means keep everything",
			classes: []string{},
			wantNil: true,
		},
		{
			name:      "a single known class",
			classes:   []string{"person"},
			wantIDs:   []int{0},
			wantCount: 1,
		},
		{
			name:      "several known classes",
			classes:   []string{"person", "car"},
			wantIDs:   []int{0, 2},
			wantCount: 2,
		},
		{
			name:      "an unknown class is reported and skipped",
			classes:   []string{"person", "hovercraft"},
			wantIDs:   []int{0},
			wantCount: 1,
			wantWarn:  "hovercraft",
		},
		{
			name:      "a list of only unknown classes filters everything away",
			classes:   []string{"hovercraft"},
			wantCount: 0,
			wantWarn:  "hovercraft",
		},
		{
			name:      "a repeated class is counted once",
			classes:   []string{"person", "person"},
			wantIDs:   []int{0},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			logger := log.New(&logged, "", 0)

			got := allowedClassIDs(tt.classes, logger)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("allowedClassIDs() = %v, want nil to mean \"no filter\"", got)
				}
				return
			}
			if got == nil {
				t.Fatal("allowedClassIDs() = nil, but a configured list must produce a filter even when nothing in it is valid")
			}
			if len(got) != tt.wantCount {
				t.Errorf("allowedClassIDs() has %d entries, want %d", len(got), tt.wantCount)
			}
			for _, id := range tt.wantIDs {
				if _, ok := got[id]; !ok {
					t.Errorf("class ID %d missing from the filter", id)
				}
			}
			if tt.wantWarn != "" && !strings.Contains(logged.String(), tt.wantWarn) {
				t.Errorf("expected a warning mentioning %q, got %q", tt.wantWarn, logged.String())
			}
			if tt.wantWarn == "" && logged.Len() != 0 {
				t.Errorf("expected no warning, got %q", logged.String())
			}
		})
	}
}

// TestAllowedClassIDsMatchesCocoOrder guards the assumption the filter relies
// on: that a class's position in CocoClasses is the numeric ID the model emits.
func TestAllowedClassIDsMatchesCocoOrder(t *testing.T) {
	logger := log.New(bytes.NewBuffer(nil), "", 0)
	last := len(CocoClasses) - 1
	got := allowedClassIDs([]string{CocoClasses[last]}, logger)
	if _, ok := got[last]; !ok {
		t.Errorf("the last COCO class %q did not map to ID %d", CocoClasses[last], last)
	}
}
