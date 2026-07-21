package service

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type qqBotRepoStub struct {
	boundEmail        string
	alreadyBound      bool
	findBoundErr      error
	activeBound       bool
	activeBoundErr    error
	createInput       QQBotChallengeCreateInput
	createRecord      QQBotBindingRecord
	createCreated     bool
	inspectRecord     QQBotBindingRecord
	inspectEmail      string
	completeInput     QQBotCompleteRepositoryInput
	completeResult    QQBotCompleteRepositoryResult
	emailStatus       string
	notificationState string
	settingsAudit     map[string]any
}

func (s *qqBotRepoStub) FindBoundEmail(context.Context, string, string) (string, bool, error) {
	return s.boundEmail, s.alreadyBound, s.findBoundErr
}
func (s *qqBotRepoStub) HasActiveBoundIdentity(context.Context, string, string) (bool, error) {
	return s.activeBound, s.activeBoundErr
}
func (s *qqBotRepoStub) CreateChallenge(_ context.Context, input QQBotChallengeCreateInput) (QQBotBindingRecord, bool, error) {
	s.createInput = input
	return s.createRecord, s.createCreated, nil
}
func (s *qqBotRepoStub) GetChallengeByToken(context.Context, string) (QQBotBindingRecord, string, error) {
	return s.inspectRecord, s.inspectEmail, nil
}
func (s *qqBotRepoStub) UpdateEmailStatus(_ context.Context, _ int64, status, _ string) error {
	s.emailStatus = status
	return nil
}
func (s *qqBotRepoStub) UpdateNotificationStatus(_ context.Context, _ int64, status, _ string) error {
	s.notificationState = status
	return nil
}
func (s *qqBotRepoStub) CompleteBinding(_ context.Context, input QQBotCompleteRepositoryInput) (QQBotCompleteRepositoryResult, error) {
	s.completeInput = input
	return s.completeResult, nil
}
func (s *qqBotRepoStub) ListBindings(context.Context, QQBotBindingListFilter) (QQBotBindingPage, error) {
	return QQBotBindingPage{}, nil
}
func (s *qqBotRepoStub) Stats(context.Context, time.Time) (QQBotStats, error) {
	return QQBotStats{}, nil
}
func (s *qqBotRepoStub) Unbind(context.Context, int64, string, string, time.Time) error {
	return nil
}
func (s *qqBotRepoStub) RecordSettingsAudit(_ context.Context, _ string, metadata map[string]any) error {
	s.settingsAudit = metadata
	return nil
}

type qqBotUserLookupStub struct {
	user *User
	err  error
}

func (s qqBotUserLookupStub) GetByEmail(context.Context, string) (*User, error) {
	return s.user, s.err
}

type qqBotSettingRepoStub struct {
	values  map[string]string
	updates map[string]string
}

func (s *qqBotSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (s *qqBotSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return "", ErrSettingNotFound
}
func (s *qqBotSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}
func (s *qqBotSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (s *qqBotSettingRepoStub) SetMultiple(_ context.Context, updates map[string]string) error {
	s.updates = make(map[string]string, len(updates))
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range updates {
		s.updates[key] = value
		s.values[key] = value
	}
	return nil
}
func (s *qqBotSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *qqBotSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestQQBotPrepareBindingHashesTokenAndQueuesVerificationEmail(t *testing.T) {
	now := time.Date(2026, 7, 17, 4, 0, 0, 0, time.UTC)
	repo := &qqBotRepoStub{
		createCreated: true,
		createRecord: QQBotBindingRecord{
			ID: "41",
		},
	}
	settings := &qqBotSettingRepoStub{values: map[string]string{}}
	queue := &EmailQueueService{taskChan: make(chan EmailTask, 1)}
	svc := NewQQBotService(
		repo,
		qqBotUserLookupStub{user: &User{ID: 9, Email: "user@example.com", Username: "User", Status: StatusActive}},
		settings,
		queue,
		nil,
		nil,
		&config.Config{QQBotIntegration: config.QQBotIntegrationConfig{PublicBaseURL: "https://qqbot.example.com"}},
	)
	svc.now = func() time.Time { return now }

	result, err := svc.PrepareBinding(context.Background(), QQBotPrepareBindingRequest{
		EventID:         "event-1",
		BotAppID:        "app-1",
		Scene:           "c2c",
		ProviderSubject: "c2c:openid-1",
		Email:           "User@Example.com",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Equal(t, "u***r@example.com", result.MaskedEmail)
	require.Len(t, repo.createInput.TokenHash, 64)
	require.Equal(t, hashQQBotEmail("user@example.com"), repo.createInput.EmailHash)
	require.Equal(t, int64(9), *repo.createInput.UserID)
	require.Equal(t, now.Add(15*time.Minute), repo.createInput.ExpiresAt)
	require.Equal(t, QQBotDeliveryStatusQueued, repo.createInput.EmailStatus)

	task := <-queue.taskChan
	require.Equal(t, TaskTypeNotificationMail, task.TaskType)
	require.NotNil(t, task.NotificationInput)
	require.Equal(t, NotificationEmailEventQQBotBindingLink, task.NotificationInput.Event)
	bindingURL, parseErr := url.Parse(task.NotificationInput.Variables["binding_url"])
	require.NoError(t, parseErr)
	token := bindingURL.Query().Get("token")
	require.NotEmpty(t, token)
	require.NotEqual(t, repo.createInput.TokenHash, token)
	require.Equal(t, repo.createInput.TokenHash, hashQQBotToken(token))
}

func TestQQBotPrepareBindingReturnsExistingBindingWithoutCreatingChallenge(t *testing.T) {
	repo := &qqBotRepoStub{boundEmail: "785740487@qq.com", alreadyBound: true}
	queue := &EmailQueueService{taskChan: make(chan EmailTask, 1)}
	svc := NewQQBotService(
		repo,
		qqBotUserLookupStub{err: ErrUserNotFound},
		&qqBotSettingRepoStub{values: map[string]string{}},
		queue,
		nil,
		nil,
		&config.Config{QQBotIntegration: config.QQBotIntegrationConfig{PublicBaseURL: "https://qqbot.example.com"}},
	)

	result, err := svc.PrepareBinding(context.Background(), QQBotPrepareBindingRequest{
		EventID:         "event-bound",
		BotAppID:        "app-1",
		Scene:           "c2c",
		ProviderSubject: "c2c:openid-bound",
		Email:           "different@example.com",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.True(t, result.AlreadyBound)
	require.Equal(t, "7***7@qq.com", result.MaskedEmail)
	require.Empty(t, repo.createInput.EventID)
	require.Empty(t, queue.taskChan)
}

func TestQQBotPrepareBindingDoesNotRevealMissingAccount(t *testing.T) {
	repo := &qqBotRepoStub{createCreated: true, createRecord: QQBotBindingRecord{ID: "42"}}
	queue := &EmailQueueService{taskChan: make(chan EmailTask, 1)}
	svc := NewQQBotService(
		repo,
		qqBotUserLookupStub{err: ErrUserNotFound},
		&qqBotSettingRepoStub{values: map[string]string{}},
		queue,
		nil,
		nil,
		&config.Config{QQBotIntegration: config.QQBotIntegrationConfig{PublicBaseURL: "https://qqbot.example.com"}},
	)

	result, err := svc.PrepareBinding(context.Background(), QQBotPrepareBindingRequest{
		EventID: "event-2", BotAppID: "app-1", Scene: "group", ProviderSubject: "group:openid-2", Email: "missing@example.com",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Equal(t, QQBotBindingStatusFailed, repo.createInput.Status)
	require.Equal(t, "ACCOUNT_NOT_FOUND", repo.createInput.FailureCode)
	require.Equal(t, QQBotDeliveryStatusSkipped, repo.createInput.EmailStatus)
	require.Nil(t, repo.createInput.UserID)
	require.Empty(t, queue.taskChan)
}

func TestQQBotInspectReturnsServiceDisabledState(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute)
	repo := &qqBotRepoStub{inspectRecord: QQBotBindingRecord{Status: QQBotBindingStatusPending, ExpiresAt: expiresAt}}
	settings := &qqBotSettingRepoStub{values: map[string]string{SettingKeyQQBotBindingEnabled: "false"}}
	svc := NewQQBotService(repo, qqBotUserLookupStub{}, settings, nil, nil, nil, &config.Config{})

	result, err := svc.InspectBinding(context.Background(), strings.Repeat("x", 24))
	require.NoError(t, err)
	require.Equal(t, "service_disabled", result.Status)
}

func TestQQBotUpdateSettingsNormalizesIDsAndAudits(t *testing.T) {
	repo := &qqBotRepoStub{}
	settings := &qqBotSettingRepoStub{values: map[string]string{}}
	svc := NewQQBotService(repo, qqBotUserLookupStub{}, settings, nil, nil, nil, &config.Config{})
	now := time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	groups := []string{" group-2 ", "group-1", "group-1", ""}
	guilds := []string{"guild-1"}
	channels := map[string]string{" guild-1 ": " channel-1 "}
	bonus := 8.5
	ttl := 30
	channelCheckEnabled := false

	result, err := svc.UpdateSettings(context.Background(), QQBotSettingsUpdate{
		FirstBindBonus:       &bonus,
		LinkTTLMinutes:       &ttl,
		ChannelCheckEnabled:  &channelCheckEnabled,
		AllowedGroupIDs:      &groups,
		AllowedGuildIDs:      &guilds,
		GuildWelcomeChannels: &channels,
		AdminSubject:         "admin",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"group-1", "group-2"}, result.AllowedGroupIDs)
	require.Equal(t, map[string]string{"guild-1": "channel-1"}, result.GuildWelcomeChannels)
	require.Equal(t, &now, result.UpdatedAt)
	require.Equal(t, "8.5", settings.updates[SettingKeyQQBotFirstBindBonus])
	require.Equal(t, "false", settings.updates[SettingKeyQQBotChannelCheckEnabled])
	require.False(t, result.ChannelCheckEnabled)
	require.Equal(t, float64(8.5), repo.settingsAudit["first_bind_bonus"])
	require.Equal(t, false, repo.settingsAudit["channel_check_enabled"])
}
