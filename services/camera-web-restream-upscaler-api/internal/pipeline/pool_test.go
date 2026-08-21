package pipeline

import (
	"slices"
	"testing"
)

// TestStepsFor pins down how an upscale factor is broken into resize passes.
// Powers of two become repeated 2× passes because resizing twice by 2 keeps
// more detail than resizing once by 4; anything else is done in one pass.
func TestStepsFor(t *testing.T) {
	tests := []struct {
		name  string
		scale int
		want  []int
	}{
		{name: "scale 1 needs no resize at all", scale: 1, want: nil},
		{name: "scale 2 is a single doubling", scale: 2, want: []int{2}},
		{name: "scale 4 is two doublings", scale: 4, want: []int{2, 2}},
		{name: "scale 8 is three doublings", scale: 8, want: []int{2, 2, 2}},
		{name: "scale 3 is not a power of two, so one pass", scale: 3, want: []int{3}},
		{name: "scale 6 is not a power of two, so one pass", scale: 6, want: []int{6}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stepsFor(tt.scale)
			if !slices.Equal(got, tt.want) {
				t.Errorf("stepsFor(%d) = %v, want %v", tt.scale, got, tt.want)
			}
		})
	}
}
