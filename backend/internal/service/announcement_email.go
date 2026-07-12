package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AnnouncementEmailJobPending               = "pending"
	AnnouncementEmailJobPreparing             = "preparing"
	AnnouncementEmailJobSending               = "sending"
	AnnouncementEmailJobCompleted             = "completed"
	AnnouncementEmailJobCompletedWithFailures = "completed_with_failures"
	AnnouncementEmailJobFailed                = "failed"
	AnnouncementEmailJobCancelled             = "cancelled"

	AnnouncementEmailDeliveryPending   = "pending"
	AnnouncementEmailDeliverySending   = "sending"
	AnnouncementEmailDeliverySent      = "sent"
	AnnouncementEmailDeliveryFailed    = "failed"
	AnnouncementEmailDeliveryAmbiguous = "ambiguous"
	AnnouncementEmailDeliverySkipped   = "skipped"
)

var (
	ErrAnnouncementEmailRequiresActive  = infraerrors.BadRequest("ANNOUNCEMENT_EMAIL_REQUIRES_ACTIVE", "email notification can only be requested for an active announcement")
	ErrAnnouncementEmailDisabled        = infraerrors.Conflict("ANNOUNCEMENT_EMAIL_DISABLED", "announcement email notification is disabled")
	ErrAnnouncementEmailSMTPUnavailable = infraerrors.Conflict("ANNOUNCEMENT_EMAIL_SMTP_UNAVAILABLE", "SMTP must be configured before publishing an announcement email")
	ErrAnnouncementEmailRenderUnsafe    = infraerrors.BadRequest("ANNOUNCEMENT_EMAIL_RENDER_UNSAFE", "announcement email content cannot be rendered safely")
	ErrAnnouncementEmailNotFound        = infraerrors.NotFound("ANNOUNCEMENT_EMAIL_JOB_NOT_FOUND", "announcement email notification not found")
	ErrAnnouncementEmailRetryEmpty      = infraerrors.Conflict("ANNOUNCEMENT_EMAIL_NOT_RETRYABLE", "announcement email notification has no retryable deliveries")
)

type AnnouncementEmailNotification = domain.AnnouncementEmailNotification

type AnnouncementEmailRecipient struct {
	UserID   int64
	Email    string
	Username string
}

type AnnouncementEmailJob struct {
	AnnouncementEmailNotification
	AnnouncementTitle    string
	AnnouncementContent  string
	AnnouncementStartsAt *time.Time
}

type AnnouncementEmailDelivery struct {
	ID             int64
	JobID          int64
	UserID         int64
	RecipientEmail string
	RecipientName  string
	Locale         string
	AttemptCount   int
	MaxAttempts    int
}

type AnnouncementEmailRepository interface {
	GetByAnnouncementID(ctx context.Context, announcementID int64) (*AnnouncementEmailNotification, error)
	CountEligibleRecipients(ctx context.Context) (int64, error)
	ClaimJob(ctx context.Context, owner string, now time.Time, lease time.Duration) (*AnnouncementEmailJob, error)
	PrepareRecipients(ctx context.Context, job *AnnouncementEmailJob, batchSize, maxAttempts int) (bool, error)
	ClaimDeliveries(ctx context.Context, jobID int64, owner string, now time.Time, lease time.Duration, limit int) ([]AnnouncementEmailDelivery, error)
	MarkDeliverySent(ctx context.Context, deliveryID int64, owner string, sentAt time.Time) error
	MarkDeliveryFailed(ctx context.Context, deliveryID int64, owner, class, message string, nextAttemptAt *time.Time) error
	RefreshJob(ctx context.Context, jobID int64, owner string, now time.Time, lease time.Duration) (*AnnouncementEmailJob, error)
	MarkJobFailed(ctx context.Context, jobID int64, owner, code, message string, now time.Time) error
	Retry(ctx context.Context, announcementID int64, includeAmbiguous bool, now time.Time) (*AnnouncementEmailNotification, error)
}

type AnnouncementEmailCapability struct {
	Enabled        bool  `json:"enabled"`
	SMTPConfigured bool  `json:"smtp_configured"`
	EligibleCount  int64 `json:"eligible_count"`
}

type AnnouncementEmailPublicationValidator interface {
	ValidatePublication(ctx context.Context, title, content string) error
}
