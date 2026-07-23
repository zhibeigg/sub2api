package qqbot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	SettingKeyRuntimeConfig         = "qqbot_runtime_config"
	ConfigInvalidationChannel       = "sub2api:qqbot:config:invalidate"
	DefaultWorkerCount              = 4
	DefaultQueueCapacity            = 4096
	DefaultAPITimeoutMS             = 10000
	MaxWebhookBodyBytes       int64 = 1 << 20
)

var (
	ErrConfigConflict     = infraerrors.Conflict("QQBOT_CONFIG_CONFLICT", "QQBot 配置已被其他管理员更新")
	ErrInvalidConfig      = infraerrors.BadRequest("QQBOT_INVALID_CONFIG", "QQBot 配置无效")
	ErrProbeRequired      = infraerrors.Conflict("QQBOT_PROBE_REQUIRED", "请先使用当前凭据完成 QQBot 连接探测")
	ErrRuntimeDisabled    = infraerrors.ServiceUnavailable("QQBOT_RUNTIME_DISABLED", "QQBot runtime is disabled")
	ErrRuntimeUnavailable = infraerrors.ServiceUnavailable("QQBOT_RUNTIME_UNAVAILABLE", "QQBot runtime is unavailable")
)

type Scene string

const (
	SceneGroup Scene = "group"
	SceneC2C   Scene = "c2c"
	SceneGuild Scene = "guild"
)

type InboundEvent struct {
	EventID         string `json:"event_id"`
	MessageID       string `json:"message_id,omitempty"`
	Scene           Scene  `json:"scene"`
	Content         string `json:"content,omitempty"`
	ProviderSubject string `json:"provider_subject"`
	SourceID        string `json:"source_id,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
	GuildID         string `json:"guild_id,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	MemberJoined    bool   `json:"member_joined,omitempty"`
	EnterAIO        bool   `json:"enter_aio,omitempty"`
}

type storageConfig struct {
	Enabled                 bool      `json:"enabled"`
	AppID                   string    `json:"app_id"`
	AppSecretCiphertext     string    `json:"app_secret_ciphertext,omitempty"`
	WebhookSecretCiphertext string    `json:"webhook_secret_ciphertext,omitempty"`
	Sandbox                 bool      `json:"sandbox"`
	PublicBaseURL           string    `json:"public_base_url"`
	WorkerCount             int       `json:"worker_count"`
	QueueCapacity           int       `json:"queue_capacity"`
	APITimeoutMS            int       `json:"api_timeout_ms"`
	ConfigVersion           int64     `json:"config_version"`
	UpdatedAt               time.Time `json:"updated_at"`
	UpdatedBy               int64     `json:"updated_by"`
	ChangeSummary           string    `json:"change_summary"`
}

type ActiveConfig struct {
	Enabled       bool
	AppID         string
	AppSecret     string
	WebhookSecret string
	Sandbox       bool
	PublicBaseURL string
	WorkerCount   int
	QueueCapacity int
	APITimeoutMS  int
	ConfigVersion int64
	UpdatedAt     time.Time
	UpdatedBy     int64
}

type PublicConfig struct {
	Enabled                 bool              `json:"enabled"`
	AppID                   string            `json:"app_id"`
	AppSecretConfigured     bool              `json:"app_secret_configured"`
	WebhookSecretConfigured bool              `json:"webhook_secret_configured"`
	Sandbox                 bool              `json:"sandbox"`
	PublicBaseURL           string            `json:"public_base_url"`
	WorkerCount             int               `json:"worker_count"`
	QueueCapacity           int               `json:"queue_capacity"`
	APITimeoutMS            int               `json:"api_timeout_ms"`
	BindingEnabled          bool              `json:"binding_enabled"`
	FirstBindBonus          float64           `json:"first_bind_bonus"`
	LinkTTLMinutes          int               `json:"link_ttl_minutes"`
	WelcomeEnabled          bool              `json:"welcome_enabled"`
	WelcomeMessage          string            `json:"welcome_message"`
	FirstInteractionEnabled bool              `json:"first_interaction_enabled"`
	ChannelCheckEnabled     bool              `json:"channel_check_enabled"`
	HelpMessage             string            `json:"help_message"`
	AllowedGroupIDs         []string          `json:"allowed_group_ids"`
	AllowedGuildIDs         []string          `json:"allowed_guild_ids"`
	GuildWelcomeChannels    map[string]string `json:"guild_welcome_channels"`
	ConfigVersion           int64             `json:"config_version"`
	UpdatedAt               time.Time         `json:"updated_at"`
	UpdatedBy               int64             `json:"updated_by"`
	ChangeSummary           string            `json:"change_summary"`
}

type UpdateConfigRequest struct {
	ExpectedConfigVersion   int64             `json:"expected_config_version" binding:"required"`
	Enabled                 bool              `json:"enabled"`
	AppID                   string            `json:"app_id"`
	AppSecret               string            `json:"app_secret,omitempty"`
	WebhookSecret           string            `json:"webhook_secret,omitempty"`
	Sandbox                 bool              `json:"sandbox"`
	PublicBaseURL           string            `json:"public_base_url"`
	WorkerCount             int               `json:"worker_count"`
	QueueCapacity           int               `json:"queue_capacity"`
	APITimeoutMS            int               `json:"api_timeout_ms"`
	BindingEnabled          bool              `json:"binding_enabled"`
	FirstBindBonus          float64           `json:"first_bind_bonus"`
	LinkTTLMinutes          int               `json:"link_ttl_minutes"`
	WelcomeEnabled          bool              `json:"welcome_enabled"`
	WelcomeMessage          string            `json:"welcome_message"`
	FirstInteractionEnabled bool              `json:"first_interaction_enabled"`
	ChannelCheckEnabled     bool              `json:"channel_check_enabled"`
	HelpMessage             string            `json:"help_message"`
	AllowedGroupIDs         []string          `json:"allowed_group_ids"`
	AllowedGuildIDs         []string          `json:"allowed_guild_ids"`
	GuildWelcomeChannels    map[string]string `json:"guild_welcome_channels"`
}

func (r UpdateConfigRequest) businessUpdate() service.QQBotSettingsUpdate {
	return service.QQBotSettingsUpdate{
		BindingEnabled:          &r.BindingEnabled,
		FirstBindBonus:          &r.FirstBindBonus,
		LinkTTLMinutes:          &r.LinkTTLMinutes,
		WelcomeEnabled:          &r.WelcomeEnabled,
		WelcomeMessage:          &r.WelcomeMessage,
		FirstInteractionEnabled: &r.FirstInteractionEnabled,
		ChannelCheckEnabled:     &r.ChannelCheckEnabled,
		HelpMessage:             &r.HelpMessage,
		AllowedGroupIDs:         &r.AllowedGroupIDs,
		AllowedGuildIDs:         &r.AllowedGuildIDs,
		GuildWelcomeChannels:    &r.GuildWelcomeChannels,
	}
}

type RuntimeStatus string

const (
	RuntimeDisabled  RuntimeStatus = "disabled"
	RuntimeStarting  RuntimeStatus = "starting"
	RuntimeRunning   RuntimeStatus = "running"
	RuntimeReloading RuntimeStatus = "reloading"
	RuntimeDegraded  RuntimeStatus = "degraded"
)

type RuntimeState struct {
	DesiredConfigVersion int64         `json:"desired_config_version"`
	ActiveConfigVersion  int64         `json:"active_config_version"`
	ProcessStatus        RuntimeStatus `json:"process_status"`
	WorkerTotal          int           `json:"worker_total"`
	WorkerActive         int           `json:"worker_active"`
	StreamBacklog        int64         `json:"stream_backlog"`
	StreamPending        int64         `json:"stream_pending"`
	DeadLetterTotal      int64         `json:"dead_letter_total"`
	LastWebhookAt        *time.Time    `json:"last_webhook_at,omitempty"`
	LastEventAt          *time.Time    `json:"last_event_at,omitempty"`
	LastSendAt           *time.Time    `json:"last_send_at,omitempty"`
	LastErrorCode        string        `json:"last_error_code,omitempty"`
	LastErrorMessage     string        `json:"last_error_message,omitempty"`
	LastErrorAt          *time.Time    `json:"last_error_at,omitempty"`
}

type ProbeRequest struct {
	AppID               string `json:"app_id"`
	AppSecret           string `json:"app_secret,omitempty"`
	WebhookSecret       string `json:"webhook_secret,omitempty"`
	Sandbox             bool   `json:"sandbox"`
	PublicBaseURL       string `json:"public_base_url"`
	APITimeoutMS        int    `json:"api_timeout_ms"`
	ChannelCheckEnabled *bool  `json:"channel_check_enabled,omitempty"`
}

type ProbeResult struct {
	OK               bool      `json:"ok"`
	Status           string    `json:"status"`
	Message          string    `json:"message"`
	ErrorCode        string    `json:"error_code,omitempty"`
	LatencyMS        int64     `json:"latency_ms"`
	BotIDFingerprint string    `json:"bot_id_fingerprint,omitempty"`
	CheckedAt        time.Time `json:"checked_at"`
}

func defaultStorageConfig(publicBaseURL string) storageConfig {
	return storageConfig{PublicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"), WorkerCount: DefaultWorkerCount, QueueCapacity: DefaultQueueCapacity, APITimeoutMS: DefaultAPITimeoutMS, ConfigVersion: 1}
}

func parseStorageConfig(raw, publicBaseURL string) (storageConfig, error) {
	fallbackPublicURL := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	cfg := defaultStorageConfig(fallbackPublicURL)
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return storageConfig{}, fmt.Errorf("decode qqbot runtime config: %w", err)
		}
	}
	normalizeStorageConfig(&cfg)
	if cfg.PublicBaseURL == "" {
		cfg.PublicBaseURL = fallbackPublicURL
	}
	if err := validateStorageConfig(cfg, false); err != nil {
		return storageConfig{}, err
	}
	return cfg, nil
}

func normalizeStorageConfig(cfg *storageConfig) {
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.PublicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = DefaultWorkerCount
	}
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = DefaultQueueCapacity
	}
	if cfg.APITimeoutMS == 0 {
		cfg.APITimeoutMS = DefaultAPITimeoutMS
	}
	if cfg.ConfigVersion < 1 {
		cfg.ConfigVersion = 1
	}
}

func validAppID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validateStorageConfig(cfg storageConfig, requireEnabledSecrets bool) error {
	if cfg.WorkerCount < 1 || cfg.WorkerCount > 64 || cfg.QueueCapacity < 16 || cfg.QueueCapacity > 100000 || cfg.APITimeoutMS < 100 || cfg.APITimeoutMS > 30000 {
		return ErrInvalidConfig
	}
	if cfg.AppID != "" && !validAppID(cfg.AppID) {
		return ErrInvalidConfig
	}
	if cfg.PublicBaseURL != "" {
		u, err := url.Parse(cfg.PublicBaseURL)
		if err != nil || !u.IsAbs() || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ErrInvalidConfig
		}
	}
	if cfg.Enabled || requireEnabledSecrets {
		if cfg.AppID == "" || cfg.PublicBaseURL == "" || cfg.AppSecretCiphertext == "" {
			return ErrInvalidConfig
		}
	}
	return nil
}

func publicFromStorage(cfg storageConfig, settings service.QQBotSettings) PublicConfig {
	return PublicConfig{
		Enabled:                 cfg.Enabled,
		AppID:                   cfg.AppID,
		AppSecretConfigured:     cfg.AppSecretCiphertext != "",
		WebhookSecretConfigured: cfg.WebhookSecretCiphertext != "" || cfg.AppSecretCiphertext != "",
		Sandbox:                 cfg.Sandbox,
		PublicBaseURL:           cfg.PublicBaseURL,
		WorkerCount:             cfg.WorkerCount,
		QueueCapacity:           cfg.QueueCapacity,
		APITimeoutMS:            cfg.APITimeoutMS,
		BindingEnabled:          settings.BindingEnabled,
		FirstBindBonus:          settings.FirstBindBonus,
		LinkTTLMinutes:          settings.LinkTTLMinutes,
		WelcomeEnabled:          settings.WelcomeEnabled,
		WelcomeMessage:          settings.WelcomeMessage,
		FirstInteractionEnabled: settings.FirstInteractionEnabled,
		ChannelCheckEnabled:     settings.ChannelCheckEnabled,
		HelpMessage:             settings.HelpMessage,
		AllowedGroupIDs:         append([]string{}, settings.AllowedGroupIDs...),
		AllowedGuildIDs:         append([]string{}, settings.AllowedGuildIDs...),
		GuildWelcomeChannels:    cloneStringMap(settings.GuildWelcomeChannels),
		ConfigVersion:           cfg.ConfigVersion,
		UpdatedAt:               cfg.UpdatedAt,
		UpdatedBy:               cfg.UpdatedBy,
		ChangeSummary:           cfg.ChangeSummary,
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func configChangeSummary(cfg storageConfig, settings service.QQBotSettings, changedSecrets []string) string {
	ids := append(append([]string{}, settings.AllowedGroupIDs...), settings.AllowedGuildIDs...)
	sort.Strings(ids)
	digest := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	appDigest := sha256.Sum256([]byte(cfg.AppID))
	payload := map[string]any{"enabled": cfg.Enabled, "sandbox": cfg.Sandbox, "worker_count": cfg.WorkerCount, "queue_capacity": cfg.QueueCapacity, "api_timeout_ms": cfg.APITimeoutMS, "channel_check_enabled": settings.ChannelCheckEnabled, "app_id_hash": hex.EncodeToString(appDigest[:8]), "allowlist_count": len(ids), "allowlist_hash": hex.EncodeToString(digest[:8]), "changed_secrets": changedSecrets}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func Fingerprint(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:8])
}

func credentialProbeFingerprint(cfg ActiveConfig) string {
	payload := strings.Join([]string{
		strings.TrimSpace(cfg.AppID),
		strings.TrimSpace(cfg.AppSecret),
		strings.TrimSpace(cfg.WebhookSecret),
		fmt.Sprintf("%t", cfg.Sandbox),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:4] + "…" + value[len(value)-4:]
}
