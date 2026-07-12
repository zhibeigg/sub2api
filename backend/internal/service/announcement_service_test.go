package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type announcementRepoStub struct {
	item             *Announcement
	createdWithEmail bool
	updatedWithEmail bool
	emailScheduledAt time.Time
}

func (s *announcementRepoStub) Create(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
}
func (s *announcementRepoStub) CreateWithEmailJob(_ context.Context, a *Announcement, scheduledAt time.Time) error {
	s.item = a
	s.createdWithEmail = true
	s.emailScheduledAt = scheduledAt
	return nil
}

func (s *announcementRepoStub) GetByID(_ context.Context, _ int64) (*Announcement, error) {
	if s.item == nil {
		return nil, ErrAnnouncementNotFound
	}
	return s.item, nil
}

func (s *announcementRepoStub) Update(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
}
func (s *announcementRepoStub) UpdateWithEmailJob(_ context.Context, a *Announcement, scheduledAt time.Time) error {
	s.item = a
	s.updatedWithEmail = true
	s.emailScheduledAt = scheduledAt
	return nil
}

func (*announcementRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (*announcementRepoStub) List(context.Context, pagination.PaginationParams, AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (*announcementRepoStub) ListActive(context.Context, time.Time) ([]Announcement, error) {
	return nil, nil
}

func TestAnnouncementServiceCreateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	now := time.Unix(1776790020, 0)

	_, err := svc.Create(context.Background(), &CreateAnnouncementInput{
		Title:      "公告",
		Content:    "内容",
		Status:     AnnouncementStatusActive,
		NotifyMode: AnnouncementNotifyModePopup,
		StartsAt:   &now,
		EndsAt:     &now,
	})
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
}

func TestAnnouncementServiceUpdateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{
		item: &Announcement{
			ID:         1,
			Title:      "公告",
			Content:    "内容",
			Status:     AnnouncementStatusActive,
			NotifyMode: AnnouncementNotifyModePopup,
		},
	}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	now := time.Unix(1776790020, 0)
	startsAt := &now
	endsAt := &now

	_, err := svc.Update(context.Background(), 1, &UpdateAnnouncementInput{
		StartsAt: &startsAt,
		EndsAt:   &endsAt,
	})
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
}

type announcementEmailValidatorStub struct {
	calls int
	err   error
}

func (s *announcementEmailValidatorStub) ValidatePublication(context.Context, string, string) error {
	s.calls++
	return s.err
}

func TestAnnouncementServiceCreateEmailRequiresActive(t *testing.T) {
	svc := NewAnnouncementService(&announcementRepoStub{}, nil, nil, nil)
	_, err := svc.Create(context.Background(), &CreateAnnouncementInput{
		Title: "公告", Content: "内容", Status: AnnouncementStatusDraft, SendEmail: true,
	})
	require.ErrorIs(t, err, ErrAnnouncementEmailRequiresActive)
}

func TestAnnouncementServiceCreateEmailValidatesAndSchedulesFutureStart(t *testing.T) {
	repo := &announcementRepoStub{}
	validator := &announcementEmailValidatorStub{}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	svc.SetEmailPublicationValidator(validator)
	startsAt := time.Now().UTC().Add(time.Hour)

	_, err := svc.Create(context.Background(), &CreateAnnouncementInput{
		Title: "公告", Content: "内容", Status: AnnouncementStatusActive, StartsAt: &startsAt, SendEmail: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, validator.calls)
	require.True(t, repo.createdWithEmail)
	require.Equal(t, startsAt, repo.emailScheduledAt)
}

func TestAnnouncementServiceUpdateExistingEmailJobDoesNotRevalidateOrResend(t *testing.T) {
	repo := &announcementRepoStub{item: &Announcement{
		ID: 1, Title: "公告", Content: "内容", Status: AnnouncementStatusActive,
		EmailNotification: &AnnouncementEmailNotification{JobID: 7, Status: AnnouncementEmailJobCompleted},
	}}
	validator := &announcementEmailValidatorStub{err: ErrAnnouncementEmailDisabled}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	svc.SetEmailPublicationValidator(validator)

	updated, err := svc.Update(context.Background(), 1, &UpdateAnnouncementInput{SendEmail: true})
	require.NoError(t, err)
	require.Zero(t, validator.calls)
	require.True(t, repo.updatedWithEmail)
	require.Equal(t, int64(7), updated.EmailNotification.JobID)
}
