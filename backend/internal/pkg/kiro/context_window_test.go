package kiro

import "testing"

func TestGetContextWindowSize(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		// Newer models (>= 4.6) use a 1M window.
		{"claude-sonnet-4.6", 1_000_000},
		{"claude-opus-4-6", 1_000_000},
		{"claude-opus-4.8", 1_000_000},
		{"claude-sonnet-5", 200_000}, // no minor version -> regex miss, no 5.x tag -> 200K fallback
		{"claude-opus-5.0", 1_000_000},
		// Older models use a 200K window.
		{"claude-sonnet-4.5", 200_000},
		{"claude-opus-4-5", 200_000},
		{"claude-haiku-4.5", 200_000},
		{"claude-3-5-sonnet-20241022", 200_000},
		{"unknown-model", 200_000},
	}
	for _, c := range cases {
		if got := GetContextWindowSize(c.model); got != c.want {
			t.Errorf("GetContextWindowSize(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}
