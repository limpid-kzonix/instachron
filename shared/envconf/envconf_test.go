package envconf

import (
	"testing"
	"time"
)

// The tests below all follow the same shape: set the variable (or leave it
// unset), call the reader, and check the result. t.Setenv restores the previous
// value when the test ends, so the cases cannot affect each other.

func TestString(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  string
	}{
		{name: "unset falls back", set: false, want: "fallback"},
		{name: "empty falls back", set: true, value: "", want: "fallback"},
		{name: "set overrides", set: true, value: "custom", want: "custom"},
		{name: "whitespace is a value", set: true, value: " ", want: " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_STRING", tt.value)
			}
			if got := String("INSTACHRON_TEST_STRING", "fallback"); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLookupString(t *testing.T) {
	t.Run("unset reports absent", func(t *testing.T) {
		got, ok := LookupString("INSTACHRON_TEST_LOOKUP")
		if ok || got != "" {
			t.Errorf("LookupString() = %q, %v; want \"\", false", got, ok)
		}
	})
	t.Run("empty reports absent", func(t *testing.T) {
		t.Setenv("INSTACHRON_TEST_LOOKUP", "")
		if _, ok := LookupString("INSTACHRON_TEST_LOOKUP"); ok {
			t.Error("LookupString() reported an empty variable as present")
		}
	})
	t.Run("set reports present", func(t *testing.T) {
		t.Setenv("INSTACHRON_TEST_LOOKUP", "v")
		got, ok := LookupString("INSTACHRON_TEST_LOOKUP")
		if !ok || got != "v" {
			t.Errorf("LookupString() = %q, %v; want \"v\", true", got, ok)
		}
	})
}

func TestInt(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  int
	}{
		{name: "unset falls back", set: false, want: 7},
		{name: "empty falls back", set: true, value: "", want: 7},
		{name: "valid number wins", set: true, value: "42", want: 42},
		{name: "negative number wins", set: true, value: "-3", want: -3},
		{name: "zero is a real value", set: true, value: "0", want: 0},
		{name: "unparseable falls back", set: true, value: "twelve", want: 7},
		{name: "float falls back", set: true, value: "1.5", want: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_INT", tt.value)
			}
			if got := Int("INSTACHRON_TEST_INT", 7); got != tt.want {
				t.Errorf("Int() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestInt64(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  int64
	}{
		{name: "unset falls back", set: false, want: 5},
		{name: "valid number wins", set: true, value: "9000000000", want: 9000000000},
		{name: "unparseable falls back", set: true, value: "big", want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_INT64", tt.value)
			}
			if got := Int64("INSTACHRON_TEST_INT64", 5); got != tt.want {
				t.Errorf("Int64() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUint32(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  uint32
	}{
		{name: "unset falls back", set: false, want: 1},
		{name: "valid number wins", set: true, value: "4096", want: 4096},
		{name: "negative falls back", set: true, value: "-1", want: 1},
		{name: "overflow falls back", set: true, value: "4294967296", want: 1},
		{name: "maximum is accepted", set: true, value: "4294967295", want: 4294967295},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_UINT32", tt.value)
			}
			if got := Uint32("INSTACHRON_TEST_UINT32", 1); got != tt.want {
				t.Errorf("Uint32() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback bool
		want     bool
	}{
		{name: "unset falls back to true", set: false, fallback: true, want: true},
		{name: "unset falls back to false", set: false, fallback: false, want: false},
		{name: "true", set: true, value: "true", want: true},
		{name: "one", set: true, value: "1", want: true},
		{name: "yes", set: true, value: "yes", want: true},
		{name: "on", set: true, value: "on", want: true},
		{name: "uppercase TRUE", set: true, value: "TRUE", want: true},
		{name: "false overrides a true default", set: true, value: "false", fallback: true, want: false},
		{name: "zero", set: true, value: "0", fallback: true, want: false},
		{name: "no", set: true, value: "no", fallback: true, want: false},
		{name: "off", set: true, value: "off", fallback: true, want: false},
		{name: "unrecognised falls back", set: true, value: "maybe", fallback: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_BOOL", tt.value)
			}
			if got := Bool("INSTACHRON_TEST_BOOL", tt.fallback); got != tt.want {
				t.Errorf("Bool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  time.Duration
	}{
		{name: "unset falls back", set: false, want: time.Second},
		{name: "milliseconds", set: true, value: "500ms", want: 500 * time.Millisecond},
		{name: "compound", set: true, value: "2m30s", want: 150 * time.Second},
		{name: "bare number has no unit and falls back", set: true, value: "10", want: time.Second},
		{name: "nonsense falls back", set: true, value: "soon", want: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_DURATION", tt.value)
			}
			if got := Duration("INSTACHRON_TEST_DURATION", time.Second); got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeconds(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  time.Duration
	}{
		{name: "unset falls back", set: false, want: 60 * time.Second},
		{name: "whole seconds", set: true, value: "30", want: 30 * time.Second},
		{name: "zero disables", set: true, value: "0", want: 0},
		{name: "duration syntax is not accepted here", set: true, value: "30s", want: 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("INSTACHRON_TEST_SECONDS", tt.value)
			}
			if got := Seconds("INSTACHRON_TEST_SECONDS", 60*time.Second); got != tt.want {
				t.Errorf("Seconds() = %v, want %v", got, tt.want)
			}
		})
	}
}
