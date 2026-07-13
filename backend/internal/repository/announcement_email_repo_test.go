package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestEligibleAnnouncementEmailRecipientsSQLUsesSinglePostgresEscapes(t *testing.T) {
	require.Contains(t, eligibleAnnouncementEmailRecipientsSQL, `~* '^[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}$'`)
	require.NotContains(t, eligibleAnnouncementEmailRecipientsSQL, `\\-`)
	require.NotContains(t, eligibleAnnouncementEmailRecipientsSQL, `\\.`)
	require.True(t, strings.Contains(eligibleAnnouncementEmailRecipientsSQL, "verified_at IS NOT NULL"))
}

func TestAnnouncementEmailRepositoryRefreshJobCastsLeaseExpiryParameter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)
	owner := "test-owner"
	jobID := int64(3)
	queryError := errors.New("refresh query reached sqlmock")
	expectation := mock.ExpectQuery(regexp.QuoteMeta("lease_expires_at=CASE WHEN NOT EXISTS(SELECT 1 FROM announcement_email_deliveries WHERE job_id=j.id AND status IN('pending','sending')) THEN NULL::timestamptz ELSE $3::timestamptz END"))
	expectation.WithArgs(now, owner, now.Add(time.Minute), jobID)
	expectation.WillReturnError(queryError)

	repo := &announcementEmailRepository{db: db}
	_, err = repo.RefreshJob(context.Background(), jobID, owner, now, time.Minute)

	require.ErrorIs(t, err, queryError)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnnouncementEmailRepositoryMarkDeliveryFailedCastsNextAttemptParameter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectation := mock.ExpectExec(regexp.QuoteMeta("next_attempt_at=COALESCE($2::timestamptz,next_attempt_at)"))
	expectation.WithArgs("failed", nil, "temporary", "retry later", int64(10), "test-owner")
	expectation.WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &announcementEmailRepository{db: db}
	err = repo.MarkDeliveryFailed(context.Background(), 10, "test-owner", "temporary", "retry later", nil)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
