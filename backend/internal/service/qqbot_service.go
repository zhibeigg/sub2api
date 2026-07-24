package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/mail"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultQQBotFirstBindBonus         = 5.0
	defaultQQBotLinkTTLMinutes         = 15
	QQBotCommandCooldownDefaultSeconds = 60
	QQBotCommandCooldownMinSeconds     = 10
	QQBotCommandCooldownMaxSeconds     = 3600
	qqBotDeliveryCallbackTTL           = 10 * time.Second
	qqBotDefaultHelpMessage            = "欢迎使用 PokeAPI 账户助手。\n\n绑定账户：请私聊发送 /bind 你的邮箱\n查看渠道状态：发送 /check\n查看帮助：发送 /help\n\n验证链接只会发送到 Sub2API 账户邮箱。数字 QQ 仅作为展示信息，实际身份以机器人 OpenID 为准。"
	qqBotDefaultWelcomeMessage         = "欢迎 {user} 加入 {site}！\n\n可用指令：\n绑定账户（请私聊机器人）：{bind_command}\n查看渠道状态：/check\n查看帮助：/help\n\n安全提示：请勿向任何人提供密码、验证码或 API 密钥；账户绑定链接只会发送到你的站点账户邮箱。"
)

func ProvideQQBotUserLookup(repo UserRepository) QQBotUserLookup {
	return repo
}

type QQBotService struct {
	repo            QQBotBindingRepository
	userRepo        QQBotUserLookup
	settingRepo     SettingRepository
	emailQueue      *EmailQueueService
	billingCache    BillingCache
	publicBaseURLMu sync.RWMutex
	publicBaseURL   string
	transportMu     sync.RWMutex
	transport       QQBotProactiveC2CTransport
	now             func() time.Time
}

func NewQQBotService(
	repo QQBotBindingRepository,
	userRepo QQBotUserLookup,
	settingRepo SettingRepository,
	emailQueue *EmailQueueService,
	billingCache BillingCache,
	_ *NotificationEmailService,
	cfg *config.Config,
) *QQBotService {
	publicBaseURL := ""
	if cfg != nil {
		publicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.QQBotIntegration.PublicBaseURL), "/")
	}
	return &QQBotService{
		repo:          repo,
		userRepo:      userRepo,
		settingRepo:   settingRepo,
		emailQueue:    emailQueue,
		billingCache:  billingCache,
		publicBaseURL: publicBaseURL,
		now:           time.Now,
	}
}

func (s *QQBotService) SetPublicBaseURL(value string) {
	if s == nil {
		return
	}
	s.publicBaseURLMu.Lock()
	s.publicBaseURL = strings.TrimRight(strings.TrimSpace(value), "/")
	s.publicBaseURLMu.Unlock()
}

func (s *QQBotService) getPublicBaseURL() string {
	if s == nil {
		return ""
	}
	s.publicBaseURLMu.RLock()
	defer s.publicBaseURLMu.RUnlock()
	return s.publicBaseURL
}

func (s *QQBotService) SetProactiveC2CTransport(transport QQBotProactiveC2CTransport) {
	if s == nil {
		return
	}
	s.transportMu.Lock()
	s.transport = transport
	s.transportMu.Unlock()
}

func (s *QQBotService) proactiveC2CTransport() QQBotProactiveC2CTransport {
	if s == nil {
		return nil
	}
	s.transportMu.RLock()
	defer s.transportMu.RUnlock()
	return s.transport
}

func (s *QQBotService) ActiveQQBotAppID() (string, bool) {
	transport := s.proactiveC2CTransport()
	if transport == nil {
		return "", false
	}
	appID, active := transport.ActiveAppID()
	appID = strings.TrimSpace(appID)
	return appID, active && appID != ""
}

func (s *QQBotService) SendAdminProactiveAlert(ctx context.Context, identityChannelID int64, content string) error {
	return s.SendProactiveC2CToIdentityChannel(ctx, identityChannelID, content)
}

func (s *QQBotService) ListProactiveAdminRecipients(ctx context.Context) ([]QQBotAdminRecipient, error) {
	if s == nil || s.repo == nil {
		return nil, ErrQQBotNotConfigured
	}
	transport := s.proactiveC2CTransport()
	if transport == nil {
		return nil, ErrQQBotNotConfigured
	}
	botAppID, active := transport.ActiveAppID()
	botAppID = strings.TrimSpace(botAppID)
	if !active || botAppID == "" {
		return nil, ErrQQBotNotConfigured
	}
	recipients, err := s.repo.ListActiveAdminC2CRecipients(ctx, botAppID)
	if err != nil {
		return nil, err
	}
	result := make([]QQBotAdminRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient.IdentityID <= 0 || recipient.IdentityChannelID <= 0 {
			continue
		}
		if _, ok := qqBotC2COpenID(recipient.ChannelSubject); !ok {
			continue
		}
		recipient.ChannelSubject = ""
		result = append(result, recipient)
	}
	return result, nil
}

func (s *QQBotService) SendProactiveC2CToIdentityChannel(ctx context.Context, identityChannelID int64, content string) error {
	content = strings.TrimSpace(content)
	if s == nil || s.repo == nil || identityChannelID <= 0 || content == "" {
		return ErrQQBotInvalidInput
	}
	transport := s.proactiveC2CTransport()
	if transport == nil {
		return ErrQQBotNotConfigured
	}
	botAppID, active := transport.ActiveAppID()
	botAppID = strings.TrimSpace(botAppID)
	if !active || botAppID == "" {
		return ErrQQBotNotConfigured
	}
	recipient, found, err := s.repo.GetActiveAdminC2CRecipient(ctx, botAppID, identityChannelID)
	if err != nil {
		return err
	}
	if !found || recipient.IdentityID <= 0 || recipient.IdentityChannelID != identityChannelID {
		return ErrQQBotRecipientUnavailable
	}
	openID, ok := qqBotC2COpenID(recipient.ChannelSubject)
	if !ok {
		return ErrQQBotRecipientUnavailable
	}
	return transport.SendProactiveC2C(ctx, botAppID, openID, content)
}

func qqBotC2COpenID(channelSubject string) (string, bool) {
	const prefix = "c2c:"
	channelSubject = strings.TrimSpace(channelSubject)
	if !strings.HasPrefix(channelSubject, prefix) {
		return "", false
	}
	openID := strings.TrimSpace(strings.TrimPrefix(channelSubject, prefix))
	return openID, openID != ""
}

func (s *QQBotService) HasActiveBoundIdentity(ctx context.Context, botAppID, providerSubject string) (bool, error) {
	if s == nil || s.repo == nil {
		return false, ErrQQBotNotConfigured
	}
	return s.repo.HasActiveBoundIdentity(ctx, strings.TrimSpace(botAppID), strings.TrimSpace(providerSubject))
}

func (s *QQBotService) PrepareBinding(ctx context.Context, input QQBotPrepareBindingRequest) (QQBotPrepareBindingResponse, error) {
	input.EventID = strings.TrimSpace(input.EventID)
	input.MessageID = strings.TrimSpace(input.MessageID)
	input.BotAppID = strings.TrimSpace(input.BotAppID)
	input.Scene = strings.ToLower(strings.TrimSpace(input.Scene))
	input.ProviderSubject = strings.TrimSpace(input.ProviderSubject)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	email, err := normalizeQQBotEmail(input.Email)
	if err != nil || !validQQBotPrepareInput(input) {
		return QQBotPrepareBindingResponse{}, ErrQQBotInvalidInput
	}

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return QQBotPrepareBindingResponse{}, err
	}
	if !settings.BindingEnabled {
		return QQBotPrepareBindingResponse{}, ErrQQBotBindingDisabled
	}
	if s.getPublicBaseURL() == "" {
		return QQBotPrepareBindingResponse{}, ErrQQBotNotConfigured
	}

	boundEmail, alreadyBound, err := s.repo.FindBoundEmail(ctx, input.BotAppID, input.ProviderSubject)
	if err != nil {
		return QQBotPrepareBindingResponse{}, err
	}
	if alreadyBound {
		return QQBotPrepareBindingResponse{
			Accepted:     true,
			AlreadyBound: true,
			MaskedEmail:  maskQQBotEmail(boundEmail),
		}, nil
	}

	token, err := newQQBotToken(32)
	if err != nil {
		return QQBotPrepareBindingResponse{}, err
	}
	now := s.now().UTC()
	maskedEmail := maskQQBotEmail(email)
	challenge := QQBotChallengeCreateInput{
		EventID:         input.EventID,
		MessageID:       input.MessageID,
		TokenHash:       hashQQBotToken(token),
		BotAppID:        input.BotAppID,
		Scene:           input.Scene,
		ProviderSubject: input.ProviderSubject,
		SourceID:        input.SourceID,
		ChannelID:       input.ChannelID,
		DisplayName:     input.DisplayName,
		EmailHash:       hashQQBotEmail(email),
		MaskedEmail:     maskedEmail,
		Status:          QQBotBindingStatusPending,
		EmailStatus:     QQBotDeliveryStatusQueued,
		BonusAmount:     settings.FirstBindBonus,
		ExpiresAt:       now.Add(time.Duration(settings.LinkTTLMinutes) * time.Minute),
	}

	user, userErr := s.userRepo.GetByEmail(ctx, email)
	switch {
	case userErr == nil && user != nil && user.Status == StatusActive:
		challenge.UserID = &user.ID
	case errors.Is(userErr, ErrUserNotFound):
		challenge.Status = QQBotBindingStatusFailed
		challenge.FailureCode = "ACCOUNT_NOT_FOUND"
		challenge.EmailStatus = QQBotDeliveryStatusSkipped
	case userErr == nil:
		challenge.Status = QQBotBindingStatusFailed
		challenge.FailureCode = "ACCOUNT_DISABLED"
		challenge.EmailStatus = QQBotDeliveryStatusSkipped
	default:
		return QQBotPrepareBindingResponse{}, userErr
	}

	record, created, err := s.repo.CreateChallenge(ctx, challenge)
	if err != nil {
		return QQBotPrepareBindingResponse{}, err
	}
	response := QQBotPrepareBindingResponse{Accepted: true, MaskedEmail: maskedEmail}
	if !created || challenge.UserID == nil {
		return response, nil
	}

	bindingURL, err := s.bindingURL(token)
	if err != nil {
		_ = s.repo.UpdateEmailStatus(ctx, parseQQBotServiceRecordID(record.ID), QQBotDeliveryStatusFailed, "BINDING_URL_INVALID")
		return QQBotPrepareBindingResponse{}, ErrQQBotNotConfigured
	}
	challengeID := parseQQBotServiceRecordID(record.ID)
	mailInput := NotificationEmailSendInput{
		Event:          NotificationEmailEventQQBotBindingLink,
		Locale:         "zh",
		RecipientEmail: email,
		RecipientName:  qqBotRecipientName(user, email),
		UserID:         user.ID,
		SourceType:     "qqbot_binding",
		SourceID:       record.ID,
		ReminderKey:    "verification_link",
		Variables: map[string]string{
			"binding_url":        bindingURL,
			"expires_in_minutes": strconv.Itoa(settings.LinkTTLMinutes),
			"masked_email":       maskedEmail,
			"bonus_amount":       formatQQBotAmount(settings.FirstBindBonus),
		},
	}
	if s.emailQueue == nil {
		_ = s.repo.UpdateEmailStatus(ctx, challengeID, QQBotDeliveryStatusFailed, "EMAIL_QUEUE_UNAVAILABLE")
		return QQBotPrepareBindingResponse{}, ErrQQBotEmailQueueFull
	}
	if err := s.emailQueue.EnqueueNotification(mailInput, s.deliveryCallback(challengeID, true)); err != nil {
		_ = s.repo.UpdateEmailStatus(ctx, challengeID, QQBotDeliveryStatusFailed, "EMAIL_QUEUE_FULL")
		return QQBotPrepareBindingResponse{}, ErrQQBotEmailQueueFull
	}
	return response, nil
}

func (s *QQBotService) InspectBinding(ctx context.Context, token string) (QQBotBindingInspection, error) {
	token = strings.TrimSpace(token)
	if len(token) < 20 || len(token) > 256 {
		return QQBotBindingInspection{}, ErrQQBotBindingNotFound
	}
	record, _, err := s.repo.GetChallengeByToken(ctx, token)
	if err != nil {
		return QQBotBindingInspection{}, err
	}
	status := record.Status
	settings, settingsErr := s.GetSettings(ctx)
	if settingsErr != nil {
		return QQBotBindingInspection{}, settingsErr
	}
	if !settings.BindingEnabled && status == QQBotBindingStatusPending {
		status = "service_disabled"
	}
	return QQBotBindingInspection{
		Status:       status,
		MaskedEmail:  record.MaskedEmail,
		Scene:        record.Scene,
		BonusAmount:  record.BonusAmount,
		ExpiresAt:    &record.ExpiresAt,
		CompletedAt:  record.CompletedAt,
		BalanceAfter: record.BalanceAfter,
		DeclaredQQ:   record.DeclaredQQ,
	}, nil
}

func (s *QQBotService) CompleteBinding(ctx context.Context, input QQBotCompleteBindingRequest) (QQBotCompleteBindingResponse, error) {
	input.Token = strings.TrimSpace(input.Token)
	input.QQNumber = strings.TrimSpace(input.QQNumber)
	if len(input.Token) < 20 || len(input.Token) > 256 {
		return QQBotCompleteBindingResponse{}, ErrQQBotBindingNotFound
	}
	if !validQQBotNumber(input.QQNumber) {
		return QQBotCompleteBindingResponse{}, ErrQQBotInvalidQQNumber
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return QQBotCompleteBindingResponse{}, err
	}
	if !settings.BindingEnabled {
		return QQBotCompleteBindingResponse{}, ErrQQBotBindingDisabled
	}
	redeemCode, err := newQQBotRedeemCode()
	if err != nil {
		return QQBotCompleteBindingResponse{}, err
	}
	result, err := s.repo.CompleteBinding(ctx, QQBotCompleteRepositoryInput{
		Token:      input.Token,
		QQNumber:   input.QQNumber,
		Bonus:      settings.FirstBindBonus,
		RedeemCode: redeemCode,
		Now:        s.now().UTC(),
	})
	if err != nil {
		return QQBotCompleteBindingResponse{}, err
	}
	if result.NewlyCompleted && result.Record.UserID != nil && s.billingCache != nil {
		userID := *result.Record.UserID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), qqBotDeliveryCallbackTTL)
			defer cancel()
			if err := s.billingCache.InvalidateUserBalance(cacheCtx, userID); err != nil {
				slog.Warn("invalidate QQBot binding balance cache failed", "user_id", userID, "error", err)
			}
		}()
	}
	if result.NewlyCompleted && strings.TrimSpace(result.RecipientEmail) != "" {
		s.enqueueCompletionEmail(result)
	}
	return QQBotCompleteBindingResponse{
		Status:       result.Record.Status,
		Granted:      result.Granted,
		BonusAmount:  result.Record.BonusAmount,
		BalanceAfter: result.Record.BalanceAfter,
		MaskedEmail:  result.Record.MaskedEmail,
		DeclaredQQ:   result.Record.DeclaredQQ,
	}, nil
}

func (s *QQBotService) GetSettings(ctx context.Context) (QQBotSettings, error) {
	defaults := defaultQQBotSettings()
	if s.settingRepo == nil {
		return defaults, nil
	}
	keys := []string{
		SettingKeyQQBotBindingEnabled,
		SettingKeyQQBotFirstBindBonus,
		SettingKeyQQBotLinkTTLMinutes,
		SettingKeyQQBotCommandCooldownSeconds,
		SettingKeyQQBotWelcomeEnabled,
		SettingKeyQQBotWelcomeMessage,
		SettingKeyQQBotFirstInteractionEnabled,
		SettingKeyQQBotChannelCheckEnabled,
		SettingKeyQQBotHelpMessage,
		SettingKeyQQBotAllowedGroupIDs,
		SettingKeyQQBotAllowedGuildIDs,
		SettingKeyQQBotGuildWelcomeChannels,
	}
	values, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return QQBotSettings{}, err
	}
	settings := defaults
	settings.BindingEnabled = parseQQBotBool(values[SettingKeyQQBotBindingEnabled], defaults.BindingEnabled)
	settings.FirstBindBonus = parseQQBotFloat(values[SettingKeyQQBotFirstBindBonus], defaults.FirstBindBonus)
	settings.LinkTTLMinutes = parseQQBotInt(values[SettingKeyQQBotLinkTTLMinutes], defaults.LinkTTLMinutes)
	settings.CommandCooldownSeconds = parseQQBotInt(values[SettingKeyQQBotCommandCooldownSeconds], defaults.CommandCooldownSeconds)
	settings.WelcomeEnabled = parseQQBotBool(values[SettingKeyQQBotWelcomeEnabled], defaults.WelcomeEnabled)
	if value, ok := values[SettingKeyQQBotWelcomeMessage]; ok {
		settings.WelcomeMessage = value
	}
	settings.FirstInteractionEnabled = parseQQBotBool(values[SettingKeyQQBotFirstInteractionEnabled], defaults.FirstInteractionEnabled)
	settings.ChannelCheckEnabled = parseQQBotBool(values[SettingKeyQQBotChannelCheckEnabled], defaults.ChannelCheckEnabled)
	if value, ok := values[SettingKeyQQBotHelpMessage]; ok {
		settings.HelpMessage = value
	}
	parseQQBotJSON(values[SettingKeyQQBotAllowedGroupIDs], &settings.AllowedGroupIDs)
	parseQQBotJSON(values[SettingKeyQQBotAllowedGuildIDs], &settings.AllowedGuildIDs)
	parseQQBotJSON(values[SettingKeyQQBotGuildWelcomeChannels], &settings.GuildWelcomeChannels)
	settings.AllowedGroupIDs = normalizeQQBotIDs(settings.AllowedGroupIDs)
	settings.AllowedGuildIDs = normalizeQQBotIDs(settings.AllowedGuildIDs)
	settings.GuildWelcomeChannels = normalizeQQBotChannelMap(settings.GuildWelcomeChannels)
	if settings.FirstBindBonus < 0 {
		settings.FirstBindBonus = defaults.FirstBindBonus
	}
	if settings.LinkTTLMinutes < 5 || settings.LinkTTLMinutes > 1440 {
		settings.LinkTTLMinutes = defaults.LinkTTLMinutes
	}
	if settings.CommandCooldownSeconds < QQBotCommandCooldownMinSeconds || settings.CommandCooldownSeconds > QQBotCommandCooldownMaxSeconds {
		settings.CommandCooldownSeconds = defaults.CommandCooldownSeconds
	}
	return settings, nil
}

func (s *QQBotService) UpdateSettings(ctx context.Context, update QQBotSettingsUpdate) (QQBotSettings, error) {
	current, err := s.GetSettings(ctx)
	if err != nil {
		return QQBotSettings{}, err
	}
	if update.BindingEnabled != nil {
		current.BindingEnabled = *update.BindingEnabled
	}
	if update.FirstBindBonus != nil {
		if *update.FirstBindBonus < 0 || *update.FirstBindBonus > 1_000_000 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
		current.FirstBindBonus = *update.FirstBindBonus
	}
	if update.LinkTTLMinutes != nil {
		if *update.LinkTTLMinutes < 5 || *update.LinkTTLMinutes > 1440 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
		current.LinkTTLMinutes = *update.LinkTTLMinutes
	}
	if update.CommandCooldownSeconds != nil {
		if *update.CommandCooldownSeconds < QQBotCommandCooldownMinSeconds || *update.CommandCooldownSeconds > QQBotCommandCooldownMaxSeconds {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
		current.CommandCooldownSeconds = *update.CommandCooldownSeconds
	}
	if update.WelcomeEnabled != nil {
		current.WelcomeEnabled = *update.WelcomeEnabled
	}
	if update.WelcomeMessage != nil {
		if len([]rune(*update.WelcomeMessage)) > 4000 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
		current.WelcomeMessage = strings.TrimSpace(*update.WelcomeMessage)
	}
	if update.FirstInteractionEnabled != nil {
		current.FirstInteractionEnabled = *update.FirstInteractionEnabled
	}
	if update.ChannelCheckEnabled != nil {
		current.ChannelCheckEnabled = *update.ChannelCheckEnabled
	}
	if update.HelpMessage != nil {
		if len([]rune(*update.HelpMessage)) > 4000 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
		current.HelpMessage = strings.TrimSpace(*update.HelpMessage)
	}
	if update.AllowedGroupIDs != nil {
		current.AllowedGroupIDs = normalizeQQBotIDs(*update.AllowedGroupIDs)
		if len(current.AllowedGroupIDs) > 500 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
	}
	if update.AllowedGuildIDs != nil {
		current.AllowedGuildIDs = normalizeQQBotIDs(*update.AllowedGuildIDs)
		if len(current.AllowedGuildIDs) > 500 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
	}
	if update.GuildWelcomeChannels != nil {
		current.GuildWelcomeChannels = normalizeQQBotChannelMap(*update.GuildWelcomeChannels)
		if len(current.GuildWelcomeChannels) > 500 {
			return QQBotSettings{}, ErrQQBotInvalidInput
		}
	}
	groupJSON, _ := json.Marshal(current.AllowedGroupIDs)
	guildJSON, _ := json.Marshal(current.AllowedGuildIDs)
	channelsJSON, _ := json.Marshal(current.GuildWelcomeChannels)
	updates := map[string]string{
		SettingKeyQQBotBindingEnabled:          strconv.FormatBool(current.BindingEnabled),
		SettingKeyQQBotFirstBindBonus:          strconv.FormatFloat(current.FirstBindBonus, 'f', -1, 64),
		SettingKeyQQBotLinkTTLMinutes:          strconv.Itoa(current.LinkTTLMinutes),
		SettingKeyQQBotCommandCooldownSeconds:  strconv.Itoa(current.CommandCooldownSeconds),
		SettingKeyQQBotWelcomeEnabled:          strconv.FormatBool(current.WelcomeEnabled),
		SettingKeyQQBotWelcomeMessage:          current.WelcomeMessage,
		SettingKeyQQBotFirstInteractionEnabled: strconv.FormatBool(current.FirstInteractionEnabled),
		SettingKeyQQBotChannelCheckEnabled:     strconv.FormatBool(current.ChannelCheckEnabled),
		SettingKeyQQBotHelpMessage:             current.HelpMessage,
		SettingKeyQQBotAllowedGroupIDs:         string(groupJSON),
		SettingKeyQQBotAllowedGuildIDs:         string(guildJSON),
		SettingKeyQQBotGuildWelcomeChannels:    string(channelsJSON),
	}
	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return QQBotSettings{}, err
	}
	now := s.now().UTC()
	current.UpdatedAt = &now
	if s.repo != nil {
		if err := s.repo.RecordSettingsAudit(ctx, strings.TrimSpace(update.AdminSubject), map[string]any{
			"binding_enabled":           current.BindingEnabled,
			"first_bind_bonus":          current.FirstBindBonus,
			"link_ttl_minutes":          current.LinkTTLMinutes,
			"command_cooldown_seconds":  current.CommandCooldownSeconds,
			"welcome_enabled":           current.WelcomeEnabled,
			"first_interaction_enabled": current.FirstInteractionEnabled,
			"channel_check_enabled":     current.ChannelCheckEnabled,
			"allowed_group_count":       len(current.AllowedGroupIDs),
			"allowed_guild_count":       len(current.AllowedGuildIDs),
		}); err != nil {
			return QQBotSettings{}, err
		}
	}
	return current, nil
}

func (s *QQBotService) Stats(ctx context.Context) (QQBotStats, error) {
	return s.repo.Stats(ctx, s.now().UTC())
}

func (s *QQBotService) ListBindings(ctx context.Context, filter QQBotBindingListFilter) (QQBotBindingPage, error) {
	return s.repo.ListBindings(ctx, filter)
}

func (s *QQBotService) Unbind(ctx context.Context, id int64, input QQBotUnbindRequest) (QQBotUnbindResponse, error) {
	reason := strings.TrimSpace(input.Reason)
	adminSubject := strings.TrimSpace(input.AdminSubject)
	if id <= 0 || len([]rune(reason)) < 3 || len([]rune(reason)) > 300 || adminSubject == "" {
		return QQBotUnbindResponse{}, ErrQQBotInvalidInput
	}
	if err := s.repo.Unbind(ctx, id, reason, adminSubject, s.now().UTC()); err != nil {
		return QQBotUnbindResponse{}, err
	}
	return QQBotUnbindResponse{Status: QQBotBindingStatusRevoked}, nil
}

func (s *QQBotService) enqueueCompletionEmail(result QQBotCompleteRepositoryResult) {
	challengeID := parseQQBotServiceRecordID(result.Record.ID)
	if err := s.repo.UpdateNotificationStatus(context.Background(), challengeID, QQBotDeliveryStatusQueued, ""); err != nil {
		slog.Warn("mark QQBot completion email queued failed", "challenge_id", challengeID, "error", err)
	}
	balance := 0.0
	if result.Record.BalanceAfter != nil {
		balance = *result.Record.BalanceAfter
	}
	boundAt := s.now().UTC()
	if result.Record.CompletedAt != nil {
		boundAt = result.Record.CompletedAt.UTC()
	}
	input := NotificationEmailSendInput{
		Event:          NotificationEmailEventQQBotBound,
		Locale:         "zh",
		RecipientEmail: result.RecipientEmail,
		RecipientName:  qqBotEmailLocalPart(result.RecipientEmail),
		SourceType:     "qqbot_binding",
		SourceID:       result.Record.ID,
		ReminderKey:    "completed",
		Variables: map[string]string{
			"qq_number":       result.Record.DeclaredQQ,
			"bonus_amount":    formatQQBotAmount(result.Record.BonusAmount),
			"current_balance": formatQQBotAmount(balance),
			"bound_at":        boundAt.Format(time.RFC3339),
		},
	}
	if result.Record.UserID != nil {
		input.UserID = *result.Record.UserID
	}
	if s.emailQueue == nil {
		ctx, cancel := context.WithTimeout(context.Background(), qqBotDeliveryCallbackTTL)
		defer cancel()
		_ = s.repo.UpdateNotificationStatus(ctx, challengeID, QQBotDeliveryStatusFailed, "EMAIL_QUEUE_UNAVAILABLE")
		return
	}
	if err := s.emailQueue.EnqueueNotification(input, s.deliveryCallback(challengeID, false)); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), qqBotDeliveryCallbackTTL)
		defer cancel()
		_ = s.repo.UpdateNotificationStatus(ctx, challengeID, QQBotDeliveryStatusFailed, "EMAIL_QUEUE_FULL")
	}
}

func (s *QQBotService) deliveryCallback(challengeID int64, verification bool) func(error) {
	return func(sendErr error) {
		ctx, cancel := context.WithTimeout(context.Background(), qqBotDeliveryCallbackTTL)
		defer cancel()
		status := QQBotDeliveryStatusSent
		failureCode := ""
		if sendErr != nil {
			status = QQBotDeliveryStatusFailed
			failureCode = "EMAIL_DELIVERY_FAILED"
		}
		var err error
		if verification {
			err = s.repo.UpdateEmailStatus(ctx, challengeID, status, failureCode)
		} else {
			err = s.repo.UpdateNotificationStatus(ctx, challengeID, status, failureCode)
		}
		if err != nil {
			slog.Warn("update QQBot email delivery status failed", "challenge_id", challengeID, "verification", verification, "error", err)
		}
	}
}

func (s *QQBotService) bindingURL(token string) (string, error) {
	base, err := url.Parse(s.getPublicBaseURL())
	if err != nil || !base.IsAbs() || base.Host == "" {
		return "", ErrQQBotNotConfigured
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/bind"
	query := base.Query()
	query.Set("token", token)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func defaultQQBotSettings() QQBotSettings {
	return QQBotSettings{
		BindingEnabled:          true,
		FirstBindBonus:          defaultQQBotFirstBindBonus,
		LinkTTLMinutes:          defaultQQBotLinkTTLMinutes,
		CommandCooldownSeconds:  QQBotCommandCooldownDefaultSeconds,
		WelcomeEnabled:          true,
		WelcomeMessage:          qqBotDefaultWelcomeMessage,
		FirstInteractionEnabled: false,
		ChannelCheckEnabled:     false,
		HelpMessage:             qqBotDefaultHelpMessage,
		AllowedGroupIDs:         []string{},
		AllowedGuildIDs:         []string{},
		GuildWelcomeChannels:    map[string]string{},
	}
}

func normalizeQQBotEmail(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || len(raw) > 255 || strings.ContainsAny(raw, "\r\n\t") {
		return "", ErrQQBotInvalidInput
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil || !strings.EqualFold(parsed.Address, raw) {
		return "", ErrQQBotInvalidInput
	}
	parts := strings.Split(parsed.Address, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !strings.Contains(parts[1], ".") {
		return "", ErrQQBotInvalidInput
	}
	return strings.ToLower(parsed.Address), nil
}

func validQQBotPrepareInput(input QQBotPrepareBindingRequest) bool {
	if input.EventID == "" || len(input.EventID) > 255 || input.BotAppID == "" || len(input.BotAppID) > 128 {
		return false
	}
	if input.ProviderSubject == "" || len(input.ProviderSubject) > 512 || len(input.MessageID) > 255 {
		return false
	}
	if len(input.SourceID) > 255 || len(input.ChannelID) > 255 || len([]rune(input.DisplayName)) > 200 {
		return false
	}
	switch input.Scene {
	case "group", "c2c", "guild":
		return true
	default:
		return false
	}
}

func validQQBotNumber(value string) bool {
	if len(value) < 5 || len(value) > 12 || value[0] == '0' {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func newQQBotToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func newQQBotRedeemCode() (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "QQB" + strings.ToUpper(hex.EncodeToString(buffer)), nil
}

func hashQQBotEmail(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

func hashQQBotToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func maskQQBotEmail(email string) string {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := []rune(parts[0])
	var masked string
	switch len(local) {
	case 0:
		masked = "***"
	case 1:
		masked = string(local[0]) + "***"
	case 2:
		masked = string(local[0]) + "***" + string(local[1])
	default:
		masked = string(local[0]) + "***" + string(local[len(local)-1])
	}
	return masked + "@" + parts[1]
}

func qqBotRecipientName(user *User, email string) string {
	if user != nil && strings.TrimSpace(user.Username) != "" {
		return strings.TrimSpace(user.Username)
	}
	return qqBotEmailLocalPart(email)
}

func qqBotEmailLocalPart(email string) string {
	local, _, ok := strings.Cut(strings.TrimSpace(email), "@")
	if ok && local != "" {
		return local
	}
	return strings.TrimSpace(email)
}

func parseQQBotBool(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseQQBotFloat(value string, fallback float64) float64 {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseQQBotInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseQQBotJSON(raw string, target any) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	_ = json.Unmarshal([]byte(raw), target)
}

func normalizeQQBotIDs(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 255 {
			continue
		}
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeQQBotChannelMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for guildID, channelID := range values {
		guildID = strings.TrimSpace(guildID)
		channelID = strings.TrimSpace(channelID)
		if guildID == "" || channelID == "" || len(guildID) > 255 || len(channelID) > 255 {
			continue
		}
		out[guildID] = channelID
	}
	return out
}

func formatQQBotAmount(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func parseQQBotServiceRecordID(value string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return id
}
