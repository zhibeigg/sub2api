package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type playgroundAPIKeyReaderStub struct {
	key *APIKey
	err error
}

func (s playgroundAPIKeyReaderStub) GetByID(context.Context, int64) (*APIKey, error) {
	return s.key, s.err
}

type playgroundAPIKeyAccessReaderStub struct {
	playgroundAPIKeyReaderStub
	allowed map[int64]bool
}

func (s playgroundAPIKeyAccessReaderStub) CanUserBindGroup(_ context.Context, _ *User, group *Group) bool {
	return group != nil && s.allowed[group.ID]
}

type playgroundModelListerStub struct {
	byGroup map[int64][]string
	calls   []int64
}

type playgroundRoutableModelListerStub struct {
	playgroundModelListerStub
	routable map[int64]bool
}

func (s *playgroundRoutableModelListerStub) GetAvailablePlaygroundModels(_ context.Context, groupID *int64, _ string) ([]string, bool) {
	if groupID == nil {
		return nil, false
	}
	s.calls = append(s.calls, *groupID)
	return append([]string(nil), s.byGroup[*groupID]...), s.routable[*groupID]
}

func (s *playgroundModelListerStub) GetAvailableModels(_ context.Context, groupID *int64, _ string) []string {
	if groupID == nil {
		return nil
	}
	s.calls = append(s.calls, *groupID)
	return append([]string(nil), s.byGroup[*groupID]...)
}

func TestPlaygroundServiceGetModelOptionsAggregatesBindingsByPriority(t *testing.T) {
	first := &Group{ID: 20, Name: "media", Platform: PlatformGrok, Status: StatusActive, AllowImageGeneration: true, Hydrated: true}
	second := &Group{ID: 10, Name: "chat", Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true, ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{"claude-sonnet-custom"}}}
	models := &playgroundModelListerStub{byGroup: map[int64][]string{
		20: {"grok-4.5", "grok-imagine-image", "grok-imagine-video-1.5"},
		10: {"claude-sonnet-custom"},
	}}
	svc := NewPlaygroundService(playgroundAPIKeyReaderStub{key: &APIKey{
		ID: 8, UserID: 7,
		GroupBindings: []APIKeyGroupBinding{
			{GroupID: 10, Priority: 5, Group: second},
			{GroupID: 20, Priority: 1, Group: first},
		},
	}}, models)

	options, err := svc.GetModelOptions(context.Background(), 7, 8)
	require.NoError(t, err)
	require.Equal(t, []int64{20, 10}, models.calls)
	require.Equal(t, []PlaygroundModelOption{
		{ID: "20::grok-4.5", GroupID: 20, GroupName: "media", GroupPriority: 1, Model: "grok-4.5", Platform: PlatformGrok, Capabilities: []string{"chat"}},
		{ID: "20::grok-imagine-image", GroupID: 20, GroupName: "media", GroupPriority: 1, Model: "grok-imagine-image", Platform: PlatformGrok, Capabilities: []string{"image"}},
		{ID: "20::grok-imagine-video-1.5", GroupID: 20, GroupName: "media", GroupPriority: 1, Model: "grok-imagine-video-1.5", Platform: PlatformGrok, Capabilities: []string{"video"}},
		{ID: "10::claude-sonnet-custom", GroupID: 10, GroupName: "chat", GroupPriority: 5, Model: "claude-sonnet-custom", Platform: PlatformAnthropic, Capabilities: []string{"chat"}},
	}, options)
}

func TestPlaygroundServiceGetModelOptionsSupportsLegacyGroupAndCustomList(t *testing.T) {
	groupID := int64(9)
	group := &Group{
		ID: groupID, Name: "custom", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
		AllowImageGeneration: true,
		ModelsListConfig:     GroupModelsListConfig{Enabled: true, Models: []string{"gpt-image-2", "missing-model", "gpt-5.4"}},
	}
	svc := NewPlaygroundService(playgroundAPIKeyReaderStub{key: &APIKey{ID: 3, UserID: 2, GroupID: &groupID, Group: group}}, &playgroundModelListerStub{})

	options, err := svc.GetModelOptions(context.Background(), 2, 3)
	require.NoError(t, err)
	require.Equal(t, []PlaygroundModelOption{
		{ID: "9::gpt-image-2", GroupID: groupID, GroupName: "custom", GroupPriority: 0, Model: "gpt-image-2", Platform: PlatformOpenAI, Capabilities: []string{"image"}},
		{ID: "9::gpt-5.4", GroupID: groupID, GroupName: "custom", GroupPriority: 0, Model: "gpt-5.4", Platform: PlatformOpenAI, Capabilities: []string{"chat"}},
	}, options)
}

func TestPlaygroundServiceGetModelOptionsHidesUnsupportedCompatibilityModels(t *testing.T) {
	groupID := int64(4)
	group := &Group{ID: groupID, Name: "gemini", Platform: PlatformGemini, Status: StatusActive, Hydrated: true, AllowImageGeneration: true}
	models := &playgroundModelListerStub{byGroup: map[int64][]string{4: {"gemini-2.5-flash", "gemini-2.5-flash-image"}}}
	svc := NewPlaygroundService(playgroundAPIKeyReaderStub{key: &APIKey{ID: 5, UserID: 6, GroupID: &groupID, Group: group}}, models)

	options, err := svc.GetModelOptions(context.Background(), 6, 5)
	require.NoError(t, err)
	require.Equal(t, []PlaygroundModelOption{
		{ID: "4::gemini-2.5-flash", GroupID: groupID, GroupName: "gemini", GroupPriority: 0, Model: "gemini-2.5-flash", Platform: PlatformGemini, Capabilities: []string{"chat"}},
	}, options)
}

func TestPlaygroundServiceGetModelOptionsSkipsBindingsWithoutRoutableAccounts(t *testing.T) {
	routableGroup := &Group{ID: 1, Name: "routable", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	unavailableGroup := &Group{ID: 2, Name: "unavailable", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	models := &playgroundRoutableModelListerStub{
		playgroundModelListerStub: playgroundModelListerStub{byGroup: map[int64][]string{1: {"gpt-5.4"}}},
		routable:                  map[int64]bool{1: true, 2: false},
	}
	key := &APIKey{ID: 5, UserID: 6, GroupBindings: []APIKeyGroupBinding{
		{GroupID: 1, Priority: 0, Group: routableGroup},
		{GroupID: 2, Priority: 1, Group: unavailableGroup},
	}}

	options, err := NewPlaygroundService(playgroundAPIKeyReaderStub{key: key}, models).GetModelOptions(context.Background(), 6, 5)
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, int64(1), options[0].GroupID)
	require.Equal(t, []int64{1, 2}, models.calls)
}

func TestPlaygroundServiceGetModelOptionsSkipsGroupsWithoutCurrentAccess(t *testing.T) {
	allowedGroup := &Group{ID: 1, Name: "allowed", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	deniedGroup := &Group{ID: 2, Name: "denied", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
	models := &playgroundModelListerStub{byGroup: map[int64][]string{1: {"gpt-5.4"}, 2: {"gpt-5.4"}}}
	reader := playgroundAPIKeyAccessReaderStub{
		playgroundAPIKeyReaderStub: playgroundAPIKeyReaderStub{key: &APIKey{
			ID: 5, UserID: 6, User: &User{ID: 6},
			GroupBindings: []APIKeyGroupBinding{
				{GroupID: 1, Priority: 0, Group: allowedGroup},
				{GroupID: 2, Priority: 1, Group: deniedGroup},
			},
		}},
		allowed: map[int64]bool{1: true},
	}

	options, err := NewPlaygroundService(reader, models).GetModelOptions(context.Background(), 6, 5)
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, int64(1), options[0].GroupID)
	require.Equal(t, []int64{1}, models.calls)
}

func TestPlaygroundServiceGetModelOptionsRejectsForeignKey(t *testing.T) {
	svc := NewPlaygroundService(playgroundAPIKeyReaderStub{key: &APIKey{ID: 5, UserID: 99}}, &playgroundModelListerStub{})
	options, err := svc.GetModelOptions(context.Background(), 6, 5)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Nil(t, options)
}
