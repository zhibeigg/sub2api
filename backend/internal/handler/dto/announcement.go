package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type Announcement struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	NotifyMode string `json:"notify_mode"`

	Targeting service.AnnouncementTargeting `json:"targeting"`

	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`

	CreatedBy *int64 `json:"created_by,omitempty"`
	UpdatedBy *int64 `json:"updated_by,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	EmailNotification *AnnouncementEmailNotification `json:"email_notification,omitempty"`
}

type AnnouncementEmailNotification struct {
	JobID            int64      `json:"job_id"`
	AnnouncementID   int64      `json:"announcement_id"`
	Status           string     `json:"status"`
	AvailableAt      time.Time  `json:"available_at"`
	TotalCount       int64      `json:"total_count"`
	PendingCount     int64      `json:"pending_count"`
	SendingCount     int64      `json:"sending_count"`
	SentCount        int64      `json:"sent_count"`
	FailedCount      int64      `json:"failed_count"`
	AmbiguousCount   int64      `json:"ambiguous_count"`
	SkippedCount     int64      `json:"skipped_count"`
	AttemptCount     int        `json:"attempt_count"`
	CreatedBy        *int64     `json:"created_by,omitempty"`
	LastErrorCode    *string    `json:"last_error_code,omitempty"`
	LastErrorMessage *string    `json:"last_error_message,omitempty"`
	CanRetry         bool       `json:"can_retry"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

type UserAnnouncement struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	NotifyMode string `json:"notify_mode"`

	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`

	ReadAt *time.Time `json:"read_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func AnnouncementFromService(a *service.Announcement) *Announcement {
	if a == nil {
		return nil
	}
	return &Announcement{
		ID:                a.ID,
		Title:             a.Title,
		Content:           a.Content,
		Status:            a.Status,
		NotifyMode:        a.NotifyMode,
		Targeting:         a.Targeting,
		StartsAt:          a.StartsAt,
		EndsAt:            a.EndsAt,
		CreatedBy:         a.CreatedBy,
		UpdatedBy:         a.UpdatedBy,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
		EmailNotification: AnnouncementEmailNotificationFromService(a.EmailNotification),
	}
}

func AnnouncementEmailNotificationFromService(n *service.AnnouncementEmailNotification) *AnnouncementEmailNotification {
	if n == nil {
		return nil
	}
	canRetry := n.Status == service.AnnouncementEmailJobFailed ||
		(n.Status == service.AnnouncementEmailJobCompletedWithFailures && (n.FailedCount > 0 || n.AmbiguousCount > 0))
	return &AnnouncementEmailNotification{
		JobID: n.JobID, AnnouncementID: n.AnnouncementID, Status: n.Status, AvailableAt: n.ScheduledAt,
		TotalCount: n.RecipientCount, PendingCount: n.PendingCount, SendingCount: n.SendingCount,
		SentCount: n.SentCount, FailedCount: n.FailedCount, AmbiguousCount: n.AmbiguousCount,
		SkippedCount: n.SkippedCount, AttemptCount: n.AttemptCount, CreatedBy: n.CreatedBy, LastErrorCode: n.LastErrorCode,
		LastErrorMessage: n.LastError, CanRetry: canRetry, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		StartedAt: n.StartedAt, FinishedAt: n.FinishedAt,
	}
}

func UserAnnouncementFromService(a *service.UserAnnouncement) *UserAnnouncement {
	if a == nil {
		return nil
	}
	return &UserAnnouncement{
		ID:         a.Announcement.ID,
		Title:      a.Announcement.Title,
		Content:    a.Announcement.Content,
		NotifyMode: a.Announcement.NotifyMode,
		StartsAt:   a.Announcement.StartsAt,
		EndsAt:     a.Announcement.EndsAt,
		ReadAt:     a.ReadAt,
		CreatedAt:  a.Announcement.CreatedAt,
		UpdatedAt:  a.Announcement.UpdatedAt,
	}
}
