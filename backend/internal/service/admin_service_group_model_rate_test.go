//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminServiceCreateGroupNormalizesModelRateMultipliers(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	created, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "model-rate-create",
		Platform:       PlatformOpenAI,
		RateMultiplier: 0.6,
		ModelRateMultipliers: map[string]float64{
			" GPT-* ": 0.65,
			"GROK-*":  0.6,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, repo.created)
	require.Equal(t, map[string]float64{"gpt-*": 0.65, "grok-*": 0.6}, repo.created.ModelRateMultipliers)
}

func TestAdminServiceCreateGroupRejectsInvalidModelRateMultipliers(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                 "model-rate-invalid",
		Platform:             PlatformOpenAI,
		RateMultiplier:       0.6,
		ModelRateMultipliers: map[string]float64{"gpt-*": 0},
	})

	require.ErrorContains(t, err, "must be finite and > 0")
	require.Nil(t, repo.created)
}

func TestAdminServiceUpdateGroupReplacesAndClearsModelRateMultipliers(t *testing.T) {
	for name, rules := range map[string]map[string]float64{
		"replace": {" CLAUDE-* ": 0.65},
		"clear":   {},
	} {
		t.Run(name, func(t *testing.T) {
			existing := &Group{
				ID:                   26,
				Name:                 "cursor",
				Platform:             PlatformAnthropic,
				Status:               StatusActive,
				RateMultiplier:       0.6,
				ModelRateMultipliers: map[string]float64{"gpt-*": 0.7},
			}
			repo := &groupRepoStubForAdmin{getByID: existing}
			svc := &adminServiceImpl{groupRepo: repo}

			updated, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
				ModelRateMultipliers: &rules,
			})

			require.NoError(t, err)
			require.NotNil(t, updated)
			require.NotNil(t, repo.updated)
			if name == "replace" {
				require.Equal(t, map[string]float64{"claude-*": 0.65}, repo.updated.ModelRateMultipliers)
			} else {
				require.Empty(t, repo.updated.ModelRateMultipliers)
			}
		})
	}
}
