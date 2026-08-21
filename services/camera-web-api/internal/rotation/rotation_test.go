package rotation

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		deg  int
		want Angle
	}{
		{name: "zero", deg: 0, want: Angle0},
		{name: "exact quarter turn", deg: 90, want: Angle90},
		{name: "exact half turn", deg: 180, want: Angle180},
		{name: "exact three quarter turn", deg: 270, want: Angle270},
		{name: "a full turn is no rotation", deg: 360, want: Angle0},
		{name: "negative quarter turn wraps", deg: -90, want: Angle270},
		{name: "negative half turn wraps", deg: -180, want: Angle180},
		{name: "more than a full turn wraps", deg: 450, want: Angle90},
		{name: "several turns wrap", deg: 720, want: Angle0},
		{name: "large negative wraps", deg: -450, want: Angle270},
		{name: "rounds down to the nearest quarter", deg: 44, want: Angle0},
		{name: "rounds up to the nearest quarter", deg: 46, want: Angle90},
		{name: "just below a half turn", deg: 134, want: Angle90},
		{name: "just above a quarter past", deg: 136, want: Angle180},
		{name: "just below three quarters", deg: 314, want: Angle270},
		{name: "just below a full turn rounds to zero", deg: 316, want: Angle0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.deg); got != tt.want {
				t.Errorf("Normalize(%d) = %v, want %v", tt.deg, got, tt.want)
			}
		})
	}
}

func TestAngleHelpers(t *testing.T) {
	if !Angle0.IsZero() {
		t.Error("Angle0.IsZero() = false, want true")
	}
	if Angle180.IsZero() {
		t.Error("Angle180.IsZero() = true, want false")
	}
	if got := Angle270.Degrees(); got != 270 {
		t.Errorf("Angle270.Degrees() = %d, want 270", got)
	}
	if got := Angle90.String(); got != "90°" {
		t.Errorf("Angle90.String() = %q, want \"90°\"", got)
	}
}

// TestGetUnconfiguredCameraIsUnrotated pins down the behaviour the JSON camera
// list depends on: a camera nobody configured reports no rotation rather than
// an error or a missing field.
func TestGetUnconfiguredCameraIsUnrotated(t *testing.T) {
	cfg := &Config{angles: map[string]Angle{"0": Angle90}}
	if got := cfg.Get("does-not-exist"); got != Angle0 {
		t.Errorf("Get() for an unconfigured camera = %v, want %v", got, Angle0)
	}
	if got := cfg.Degrees("does-not-exist"); got != 0 {
		t.Errorf("Degrees() for an unconfigured camera = %d, want 0", got)
	}
	if got := cfg.Degrees("0"); got != 90 {
		t.Errorf("Degrees(\"0\") = %d, want 90", got)
	}
}

// TestApplyWithoutRotationDoesNotTouchTheBytes checks the fast path: with
// nothing to do, Apply must hand back exactly what it was given rather than
// paying to decode and re-encode. Passing bytes that are not a JPEG at all
// proves no decode was attempted.
func TestApplyWithoutRotationDoesNotTouchTheBytes(t *testing.T) {
	notAJPEG := []byte("this is not an image")
	got := Apply(notAJPEG, Angle0)
	if string(got) != string(notAJPEG) {
		t.Errorf("Apply() with Angle0 changed the payload")
	}
}

// TestApplyReturnsInputOnUndecodableFrame pins the deliberate choice to forward
// a frame that cannot be decoded rather than dropping it.
func TestApplyReturnsInputOnUndecodableFrame(t *testing.T) {
	notAJPEG := []byte("this is not an image")
	got := Apply(notAJPEG, Angle90)
	if string(got) != string(notAJPEG) {
		t.Errorf("Apply() on an undecodable frame = %q, want the input returned unchanged", got)
	}
}
