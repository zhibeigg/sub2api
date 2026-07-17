package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	QQBotProviderType = "qqbot"

	QQBotBindingStatusPending   = "pending"
	QQBotBindingStatusCompleted = "completed"
	QQBotBindingStatusExpired   = "expired"
	QQBotBindingStatusFailed    = "failed"
	QQBotBindingStatusRevoked   = "revoked"

	QQBotDeliveryStatusPending = "pending"
	QQBotDeliveryStatusQueued  = "queued"
	QQBotDeliveryStatusSent    = "sent"
	QQBotDeliveryStatusFailed  = "failed"
	QQBotDeliveryStatusSkipped = "skipped"
)

var (
	ErrQQBotBindingDisabled = infraerrors.Forbidden("BINDING_DISABLED", "QQ account binding is currently disabled")
	ErrQQBotBindingNotFound = infraerrors.NotFound("INVALID_BINDING_TOKEN", "binding link is invalid or no longer available")
	ErrQQBotBindingExpired  = infraerrors.Conflict("BINDING_EXPIRED", "binding link has expired")
	ErrQQBotBindingRevoked  = infraerrors.Conflict("BINDING_REVOKED", "binding has been revoked")
	ErrQQBotBindingFailed   = infraerrors.Conflict("BINDING_FAILED", "binding request cannot be completed")
	ErrQQBotIdentityOwned   = infraerrors.Conflict("QQ_IDENTITY_CONFLICT", "this QQ identity is already bound to another account")
	ErrQQBotInvalidInput    = infraerrors.BadRequest("INVALID_QQBOT_BINDING_INPUT", "invalid QQBot binding request")
	ErrQQBotInvalidQQNumber = infraerrors.BadRequest("INVALID_QQ_NUMBER", "QQ number must be 5 to 12 digits and cannot start with zero")
	ErrQQBotEmailQueueFull  = infraerrors.ServiceUnavailable("QQBOT_EMAIL_QUEUE_FULL", "verification email queue is temporarily unavailable")
	ErrQQBotNotConfigured   = infraerrors.ServiceUnavailable("QQBOT_INTEGRATION_NOT_CONFIGURED", "QQBot integration is not configured")
)

const (
	SettingKeyQQBotBindingEnabled          = "qqbot_binding_enabled"
	SettingKeyQQBotFirstBindBonus          = "qqbot_first_bind_bonus"
	SettingKeyQQBotLinkTTLMinutes          = "qqbot_link_ttl_minutes"
	SettingKeyQQBotWelcomeEnabled          = "qqbot_welcome_enabled"
	SettingKeyQQBotFirstInteractionEnabled = "qqbot_first_interaction_enabled"
	SettingKeyQQBotHelpMessage             = "qqbot_help_message"
	SettingKeyQQBotAllowedGroupIDs         = "qqbot_allowed_group_ids"
	SettingKeyQQBotAllowedGuildIDs         = "qqbot_allowed_guild_ids"
	SettingKeyQQBotGuildWelcomeChannels    = "qqbot_guild_welcome_channels"
)

const (
	NotificationEmailEventQQBotBindingLink = "auth.qqbot_binding_link"
	NotificationEmailEventQQBotBound       = "auth.qqbot_bound"
)

type QQBotSettings struct {
	BindingEnabled          bool              `json:"binding_enabled"`
	FirstBindBonus          float64           `json:"first_bind_bonus"`
	LinkTTLMinutes          int               `json:"link_ttl_minutes"`
	WelcomeEnabled          bool              `json:"welcome_enabled"`
	FirstInteractionEnabled bool              `json:"first_interaction_enabled"`
	HelpMessage             string            `json:"help_message"`
	AllowedGroupIDs         []string          `json:"allowed_group_ids"`
	AllowedGuildIDs         []string          `json:"allowed_guild_ids"`
	GuildWelcomeChannels    map[string]string `json:"guild_welcome_channels"`
	UpdatedAt               *time.Time        `json:"updated_at,omitempty"`
}

type QQBotSettingsUpdate struct {
	BindingEnabled          *bool              `json:"binding_enabled,omitempty"`
	FirstBindBonus          *float64           `json:"first_bind_bonus,omitempty"`
	LinkTTLMinutes          *int               `json:"link_ttl_minutes,omitempty"`
	WelcomeEnabled          *bool              `json:"welcome_enabled,omitempty"`
	FirstInteractionEnabled *bool              `json:"first_interaction_enabled,omitempty"`
	HelpMessage             *string            `json:"help_message,omitempty"`
	AllowedGroupIDs         *[]string          `json:"allowed_group_ids,omitempty"`
	AllowedGuildIDs         *[]string          `json:"allowed_guild_ids,omitempty"`
	GuildWelcomeChannels    *map[string]string `json:"guild_welcome_channels,omitempty"`
	AdminSubject            string             `json:"admin_subject,omitempty"`
}

type QQBotPrepareBindingRequest struct {
	EventID         string `json:"event_id"`
	MessageID       string `json:"message_id,omitempty"`
	BotAppID        string `json:"bot_app_id"`
	Scene           string `json:"scene"`
	ProviderSubject string `json:"provider_subject"`
	SourceID        string `json:"source_id,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name,omitempty"`
}

type QQBotPrepareBindingResponse struct {
	Accepted     bool   `json:"accepted"`
	AlreadyBound bool   `json:"already_bound,omitempty"`
	MaskedEmail  string `json:"masked_email,omitempty"`
}

type QQBotInspectBindingRequest struct {
	Token string `json:"token"`
}

type QQBotBindingInspection struct {
	Status       string     `json:"status"`
	MaskedEmail  string     `json:"masked_email,omitempty"`
	Scene        string     `json:"scene,omitempty"`
	BonusAmount  float64    `json:"bonus_amount"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	BalanceAfter *float64   `json:"balance_after,omitempty"`
	DeclaredQQ   string     `json:"declared_qq_number,omitempty"`
}

type QQBotCompleteBindingRequest struct {
	Token    string `json:"token"`
	QQNumber string `json:"qq_number"`
}

type QQBotCompleteBindingResponse struct {
	Status       string   `json:"status"`
	Granted      bool     `json:"granted"`
	BonusAmount  float64  `json:"bonus_amount"`
	BalanceAfter *float64 `json:"balance_after,omitempty"`
	MaskedEmail  string   `json:"masked_email,omitempty"`
	DeclaredQQ   string   `json:"declared_qq_number,omitempty"`
}

type QQBotBindingRecord struct {
	ID                 string     `json:"id"`
	Status             string     `json:"status"`
	MaskedEmail        string     `json:"masked_email"`
	OpenIDFingerprint  string     `json:"openid_fingerprint"`
	Scene              string     `json:"scene"`
	SourceID           string     `json:"source_id,omitempty"`
	ChannelID          string     `json:"channel_id,omitempty"`
	DeclaredQQ         string     `json:"declared_qq_number,omitempty"`
	BonusAmount        float64    `json:"bonus_amount"`
	BalanceBefore      *float64   `json:"balance_before,omitempty"`
	BalanceAfter       *float64   `json:"balance_after,omitempty"`
	FailureCode        string     `json:"failure_code,omitempty"`
	EmailStatus        string     `json:"email_status,omitempty"`
	NotificationStatus string     `json:"notification_status,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	ExpiresAt          time.Time  `json:"expires_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`

	EventID         string `json:"-"`
	MessageID       string `json:"-"`
	BotAppID        string `json:"-"`
	ProviderSubject string `json:"-"`
	DisplayName     string `json:"-"`
	UserID          *int64 `json:"-"`
}

type QQBotBindingPage struct {
	Items    []QQBotBindingRecord `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Pages    int                  `json:"pages"`
}

type QQBotBindingListFilter struct {
	Page     int
	PageSize int
	Status   string
	Scene    string
	Search   string
	From     *time.Time
	To       *time.Time
}

type QQBotStats struct {
	TodayRequests     int64   `json:"today_requests"`
	TotalRequests     int64   `json:"total_requests"`
	Completed         int64   `json:"completed"`
	Pending           int64   `json:"pending"`
	Expired           int64   `json:"expired"`
	Failed            int64   `json:"failed"`
	Revoked           int64   `json:"revoked"`
	GrantedTotal      float64 `json:"granted_total"`
	TodayGrantedTotal float64 `json:"today_granted_total"`
	CompletionRate    float64 `json:"completion_rate"`
}

type QQBotUnbindRequest struct {
	Reason       string `json:"reason"`
	AdminSubject string `json:"admin_subject"`
}

type QQBotUnbindResponse struct {
	Status string `json:"status"`
}

type QQBotChallengeCreateInput struct {
	EventID         string
	MessageID       string
	TokenHash       string
	BotAppID        string
	Scene           string
	ProviderSubject string
	SourceID        string
	ChannelID       string
	DisplayName     string
	UserID          *int64
	EmailHash       string
	MaskedEmail     string
	Status          string
	FailureCode     string
	EmailStatus     string
	BonusAmount     float64
	ExpiresAt       time.Time
}

type QQBotCompleteRepositoryInput struct {
	Token      string
	QQNumber   string
	Bonus      float64
	RedeemCode string
	Now        time.Time
}

type QQBotCompleteRepositoryResult struct {
	Record         QQBotBindingRecord
	Granted        bool
	NewlyCompleted bool
	RecipientEmail string
}

type QQBotUserLookup interface {
	GetByEmail(ctx context.Context, email string) (*User, error)
}

type QQBotBindingRepository interface {
	FindBoundEmail(ctx context.Context, botAppID, providerSubject string) (string, bool, error)
	CreateChallenge(ctx context.Context, input QQBotChallengeCreateInput) (QQBotBindingRecord, bool, error)
	GetChallengeByToken(ctx context.Context, token string) (QQBotBindingRecord, string, error)
	UpdateEmailStatus(ctx context.Context, id int64, status, failureCode string) error
	UpdateNotificationStatus(ctx context.Context, id int64, status, failureCode string) error
	CompleteBinding(ctx context.Context, input QQBotCompleteRepositoryInput) (QQBotCompleteRepositoryResult, error)
	ListBindings(ctx context.Context, filter QQBotBindingListFilter) (QQBotBindingPage, error)
	Stats(ctx context.Context, now time.Time) (QQBotStats, error)
	Unbind(ctx context.Context, id int64, reason, adminSubject string, now time.Time) error
	RecordSettingsAudit(ctx context.Context, adminSubject string, metadata map[string]any) error
}
