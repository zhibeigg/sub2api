package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestQQBotBindingRepositoryFindBoundEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta(qqBotFindBoundEmailSQL)).
		WithArgs("qqbot:app-1", "c2c:openid-1").
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("785740487@qq.com"))

	repo := &qqBotBindingRepository{db: db}
	email, found, err := repo.FindBoundEmail(context.Background(), " app-1 ", " c2c:openid-1 ")

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "785740487@qq.com", email)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQQBotBindingRepositoryFindBoundEmailReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta(qqBotFindBoundEmailSQL)).
		WithArgs("qqbot:app-1", "group:openid-2").
		WillReturnError(sql.ErrNoRows)

	repo := &qqBotBindingRepository{db: db}
	email, found, err := repo.FindBoundEmail(context.Background(), "app-1", "group:openid-2")

	require.NoError(t, err)
	require.False(t, found)
	require.Empty(t, email)
	require.NoError(t, mock.ExpectationsWereMet())
}

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
