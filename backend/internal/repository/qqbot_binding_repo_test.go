package repository

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestQQBotBindingRepositoryUpdateEmailStatusUsesTypedParameters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(qqBotUpdateEmailDeliveryStatusSQL)).
		WithArgs("sent", "", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(qqBotInsertDeliveryAuditSQL)).
		WithArgs(int64(1), "email", "sent", "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := &qqBotBindingRepository{db: db}
	err = repo.UpdateEmailStatus(context.Background(), 1, "sent", "")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQQBotBindingRepositoryUpdateNotificationStatusUsesContiguousParameters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(qqBotUpdateNotificationDeliveryStatusSQL)).
		WithArgs("failed", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(qqBotInsertDeliveryAuditSQL)).
		WithArgs(int64(2), "notify", "failed", "EMAIL_DELIVERY_FAILED", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := &qqBotBindingRepository{db: db}
	err = repo.UpdateNotificationStatus(context.Background(), 2, "failed", "EMAIL_DELIVERY_FAILED")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
