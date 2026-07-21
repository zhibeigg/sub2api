package service

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeModelRateMultipliers(t *testing.T) {
	t.Run("normalizes patterns", func(t *testing.T) {
		got, err := NormalizeModelRateMultipliers(map[string]float64{
			" GPT-* ": 0.65,
			"GROK-*":  0.6,
		})
		require.NoError(t, err)
		require.Equal(t, map[string]float64{"gpt-*": 0.65, "grok-*": 0.6}, got)
	})

	t.Run("rejects duplicate normalized patterns", func(t *testing.T) {
		_, err := NormalizeModelRateMultipliers(map[string]float64{
			"gpt-*":  0.65,
			" GPT-*": 0.7,
		})
		require.ErrorContains(t, err, "duplicate")
	})

	for name, input := range map[string]map[string]float64{
		"empty pattern": {" ": 0.65},
		"zero":          {"gpt-*": 0},
		"negative":      {"gpt-*": -1},
		"nan":           {"gpt-*": math.NaN()},
		"infinity":      {"gpt-*": math.Inf(1)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeModelRateMultipliers(input)
			require.Error(t, err)
		})
	}

	t.Run("rejects too many rules", func(t *testing.T) {
		input := make(map[string]float64, maxGroupModelRateMultiplierRules+1)
		for i := 0; i <= maxGroupModelRateMultiplierRules; i++ {
			input[fmt.Sprintf("model-%d", i)] = 1
		}
		_, err := NormalizeModelRateMultipliers(input)
		require.ErrorContains(t, err, "at most")
	})
}

func TestGroupRateMultiplierForModel(t *testing.T) {
	group := &Group{
		RateMultiplier: 0.6,
		ModelRateMultipliers: map[string]float64{
			"grok-*":            0.6,
			"gpt-*":             0.65,
			"gpt-*-fast*":       0.7,
			"gpt-*-max*":        0.7,
			"claude-*":          0.65,
			"claude-*-fast*":    0.7,
			"claude-fable-5":    0.7,
			"gemini-*":          0.65,
			"invalid-free-rate": 0,
		},
	}

	tests := []struct {
		model string
		want  float64
	}{
		{"grok-4.5", 0.6},
		{"GPT-5.4", 0.65},
		{"gpt-5-fast", 0.7},
		{"gpt-5.1-codex-max-high", 0.7},
		{"claude-sonnet-4-6", 0.65},
		{"claude-opus-4-7-fast-mode", 0.7},
		{"claude-fable-5", 0.7},
		{"gemini-3.1-pro", 0.65},
		{"composer-2.5", 0.6},
		{"invalid-free-rate", 0.6},
		{"", 0.6},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			require.InDelta(t, tt.want, group.RateMultiplierForModel(tt.model), 1e-12)
		})
	}
}

func TestModelRatePatternMatches(t *testing.T) {
	require.True(t, modelRatePatternMatches("gpt-*-fast*", "gpt-5-fast"))
	require.True(t, modelRatePatternMatches("gpt-*-max*", "gpt-5.1-codex-max-high"))
	require.True(t, modelRatePatternMatches("*", "anything"))
	require.False(t, modelRatePatternMatches("gpt-*-fast", "gpt-5.1-codex-max"))
	require.False(t, modelRatePatternMatches("claude-*", "gpt-5.4"))
}

func TestGroupModelRateMultipliersAuthCacheVersion(t *testing.T) {
	require.Equal(t, 20, apiKeyAuthSnapshotVersion)
}

func TestGroupModelRateMultipliersAuthSnapshotRoundTrip(t *testing.T) {
	group := &Group{
		ID:                   26,
		Name:                 "cursor",
		Platform:             PlatformAnthropic,
		Status:               StatusActive,
		RateMultiplier:       0.6,
		ModelRateMultipliers: map[string]float64{"gpt-*": 0.65, "grok-*": 0.6},
	}

	snapshot := groupToAuthSnapshot(group)
	require.Equal(t, group.ModelRateMultipliers, snapshot.ModelRateMultipliers)

	restored := groupFromAuthSnapshot(snapshot)
	require.True(t, restored.Hydrated)
	require.Equal(t, group.ModelRateMultipliers, restored.ModelRateMultipliers)
	require.InDelta(t, 0.65, restored.RateMultiplierForModel("gpt-5.4"), 1e-12)
}
