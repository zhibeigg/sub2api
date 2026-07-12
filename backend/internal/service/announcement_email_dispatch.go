package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type AnnouncementEmailService struct {
	repo          AnnouncementEmailRepository
	email         *EmailService
	notifications *NotificationEmailService
	cfg           *config.Config
}

func NewAnnouncementEmailService(repo AnnouncementEmailRepository, email *EmailService, notifications *NotificationEmailService, cfg *config.Config) *AnnouncementEmailService {
	return &AnnouncementEmailService{repo: repo, email: email, notifications: notifications, cfg: cfg}
}

func (s *AnnouncementEmailService) Get(ctx context.Context, announcementID int64) (*AnnouncementEmailNotification, error) {
	if s == nil || s.repo == nil {
		return nil, ErrAnnouncementEmailNotFound
	}
	return s.repo.GetByAnnouncementID(ctx, announcementID)
}

func (s *AnnouncementEmailService) Retry(ctx context.Context, announcementID int64, includeAmbiguous bool) (*AnnouncementEmailNotification, error) {
	if s == nil || s.repo == nil {
		return nil, ErrAnnouncementEmailNotFound
	}
	return s.repo.Retry(ctx, announcementID, includeAmbiguous, time.Now().UTC())
}

func (s *AnnouncementEmailService) Capability(ctx context.Context) (AnnouncementEmailCapability, error) {
	capability := AnnouncementEmailCapability{}
	if s == nil || s.cfg == nil {
		return capability, nil
	}
	capability.Enabled = s.cfg.AnnouncementEmail.Enabled
	if s.email != nil {
		_, err := s.email.GetSMTPConfig(ctx)
		capability.SMTPConfigured = err == nil
	}
	if s.repo != nil {
		count, err := s.repo.CountEligibleRecipients(ctx)
		if err != nil {
			return AnnouncementEmailCapability{}, fmt.Errorf("count eligible announcement email recipients: %w", err)
		}
		capability.EligibleCount = count
	}
	return capability, nil
}

func (s *AnnouncementEmailService) ValidatePublication(ctx context.Context, title, content string) error {
	if s == nil || s.cfg == nil || !s.cfg.AnnouncementEmail.Enabled {
		return ErrAnnouncementEmailDisabled
	}
	if s.email == nil {
		return ErrAnnouncementEmailSMTPUnavailable
	}
	if _, err := s.email.GetSMTPConfig(ctx); err != nil {
		return ErrAnnouncementEmailSMTPUnavailable.WithCause(err)
	}
	if s.notifications == nil {
		return ErrAnnouncementEmailRenderUnsafe
	}
	renderer, err := s.notifications.PrepareBatchRenderer(ctx, NotificationEmailEventAnnouncementPublished)
	if err != nil {
		return ErrAnnouncementEmailRenderUnsafe.WithCause(err)
	}
	variables := map[string]string{
		"announcement_title":     title,
		"announcement_content":   content,
		"announcement_starts_at": time.Now().UTC().Format(time.RFC3339),
	}
	for _, locale := range s.notifications.SupportedLocales() {
		if _, err := renderer.Render(locale, "recipient@example.com", "Recipient", variables); err != nil {
			return ErrAnnouncementEmailRenderUnsafe.WithCause(err)
		}
	}
	return nil
}

type AnnouncementEmailDispatchRuntime struct {
	repo          AnnouncementEmailRepository
	email         *EmailService
	notifications *NotificationEmailService
	cfg           *config.Config
	owner         string
	mu            sync.Mutex
	cancel        context.CancelFunc
	done          chan struct{}
}

func ProvideAnnouncementEmailDispatchRuntime(repo AnnouncementEmailRepository, email *EmailService, notifications *NotificationEmailService, cfg *config.Config) *AnnouncementEmailDispatchRuntime {
	host, _ := os.Hostname()
	r := &AnnouncementEmailDispatchRuntime{repo: repo, email: email, notifications: notifications, cfg: cfg, owner: fmt.Sprintf("%s:%d:%d", host, os.Getpid(), time.Now().UnixNano())}
	r.Start()
	return r
}
func (r *AnnouncementEmailDispatchRuntime) Start() {
	if r == nil || r.cfg == nil || !r.cfg.AnnouncementEmail.Enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() { defer close(r.done); r.run(ctx) }()
}
func (r *AnnouncementEmailDispatchRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
func (r *AnnouncementEmailDispatchRuntime) Running() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancel != nil
}
func (r *AnnouncementEmailDispatchRuntime) run(ctx context.Context) {
	interval := time.Duration(r.cfg.AnnouncementEmail.PollIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.processOne(ctx); err != nil && ctx.Err() == nil {
			slog.Error("announcement email dispatch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (r *AnnouncementEmailDispatchRuntime) processOne(ctx context.Context) error {
	c := r.cfg.AnnouncementEmail
	lease := time.Duration(c.LeaseSeconds) * time.Second
	now := time.Now().UTC()
	job, err := r.repo.ClaimJob(ctx, r.owner, now, lease)
	if err != nil || job == nil {
		return err
	}
	if job.Status == AnnouncementEmailJobPreparing {
		for {
			done, e := r.repo.PrepareRecipients(ctx, job, c.RecipientBatchSize, c.MaxAttempts)
			if e != nil {
				return e
			}
			if done {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	if r.email == nil {
		err = fmt.Errorf("email service is not configured")
		_ = r.repo.MarkJobFailed(ctx, job.JobID, r.owner, "smtp_config", err.Error(), time.Now().UTC())
		return err
	}
	if r.notifications == nil {
		err = fmt.Errorf("notification email service is not configured")
		_ = r.repo.MarkJobFailed(ctx, job.JobID, r.owner, "template", err.Error(), time.Now().UTC())
		return err
	}
	smtpConfig, err := r.email.GetSMTPConfig(ctx)
	if err != nil {
		_ = r.repo.MarkJobFailed(ctx, job.JobID, r.owner, "smtp_config", err.Error(), time.Now().UTC())
		return err
	}
	renderer, err := r.notifications.PrepareBatchRenderer(ctx, NotificationEmailEventAnnouncementPublished)
	if err != nil {
		_ = r.repo.MarkJobFailed(ctx, job.JobID, r.owner, "template", err.Error(), time.Now().UTC())
		return err
	}
	startsAt := job.ScheduledAt
	if job.AnnouncementStartsAt != nil {
		startsAt = *job.AnnouncementStartsAt
	}
	vars := map[string]string{"announcement_title": job.AnnouncementTitle, "announcement_content": job.AnnouncementContent, "announcement_starts_at": startsAt.Format("2006-01-02 15:04:05 MST")}
	for {
		deliveries, e := r.repo.ClaimDeliveries(ctx, job.JobID, r.owner, time.Now().UTC(), lease, c.DeliveryBatchSize)
		if e != nil {
			return e
		}
		if len(deliveries) == 0 {
			_, e = r.repo.RefreshJob(ctx, job.JobID, r.owner, time.Now().UTC(), lease)
			return e
		}
		sem := make(chan struct{}, c.DeliveryWorkerCount)
		var wg sync.WaitGroup
		for i := range deliveries {
			d := deliveries[i]
			wg.Add(1)
			sem <- struct{}{}
			go func() { defer wg.Done(); defer func() { <-sem }(); r.sendDelivery(ctx, smtpConfig, renderer, vars, d) }()
		}
		wg.Wait()
		if _, e = r.repo.RefreshJob(ctx, job.JobID, r.owner, time.Now().UTC(), lease); e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
func (r *AnnouncementEmailDispatchRuntime) sendDelivery(parent context.Context, smtpConfig *SMTPConfig, renderer *NotificationEmailBatchRenderer, vars map[string]string, d AnnouncementEmailDelivery) {
	c := r.cfg.AnnouncementEmail
	ctx, cancel := context.WithTimeout(parent, time.Duration(c.SendTimeoutSeconds)*time.Second)
	defer cancel()
	locale := d.Locale
	if r.notifications != nil {
		locale = r.notifications.ResolveRecipientLocale(ctx, d.UserID, d.RecipientEmail)
	}
	rendered, err := renderer.Render(locale, d.RecipientEmail, d.RecipientName, vars)
	if err == nil {
		err = r.email.SendEmailWithConfigContext(ctx, smtpConfig, d.RecipientEmail, rendered.Subject, rendered.HTML)
	}
	if err == nil {
		if markErr := r.repo.MarkDeliverySent(parent, d.ID, r.owner, time.Now().UTC()); markErr != nil {
			slog.Error("mark announcement email sent failed", "delivery_id", d.ID, "error", markErr)
		}
		return
	}
	class := ClassifySMTPError(err)
	var next *time.Time
	if class == SMTPErrorTemporary && d.AttemptCount < d.MaxAttempts {
		delay := time.Duration(float64(c.RetryBaseSeconds)*math.Pow(2, float64(d.AttemptCount-1))) * time.Second
		max := time.Duration(c.MaxRetrySeconds) * time.Second
		if delay > max {
			delay = max
		}
		t := time.Now().UTC().Add(delay)
		next = &t
	}
	if class == "" {
		class = SMTPErrorPermanent
	}
	if markErr := r.repo.MarkDeliveryFailed(parent, d.ID, r.owner, string(class), truncateAnnouncementEmailError(err), next); markErr != nil {
		slog.Error("mark announcement email failed", "delivery_id", d.ID, "error", markErr)
	}
}
func truncateAnnouncementEmailError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

var _ = strconv.Itoa
