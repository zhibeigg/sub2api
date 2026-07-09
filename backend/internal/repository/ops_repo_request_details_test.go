package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListRequestDetails_TTFTSort(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 7, 10, 2, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Kind:      "success",
		Sort:      "ttft_desc",
		Page:      1,
		PageSize:  10,
	}

	mock.ExpectQuery(`SELECT COUNT\(1\) FROM combined WHERE kind = \$3 AND first_token_ms IS NOT NULL`).
		WithArgs(start, end, "success").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	createdAt := start.Add(30 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"kind",
		"created_at",
		"request_id",
		"platform",
		"model",
		"duration_ms",
		"first_token_ms",
		"status_code",
		"error_id",
		"phase",
		"severity",
		"message",
		"user_id",
		"api_key_id",
		"account_id",
		"group_id",
		"stream",
	}).AddRow(
		"success",
		createdAt,
		"client:req-1",
		"kiro",
		"claude-opus-4-8",
		45000,
		32000,
		nil,
		nil,
		nil,
		nil,
		nil,
		int64(4),
		int64(19),
		int64(133),
		int64(7),
		true,
	)

	mock.ExpectQuery(`ORDER BY first_token_ms DESC, created_at DESC\s+LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, "success", 10, 0).
		WillReturnRows(rows)

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "client:req-1", items[0].RequestID)
	require.NotNil(t, items[0].FirstTokenMs)
	require.Equal(t, 32000, *items[0].FirstTokenMs)
	require.NotNil(t, items[0].DurationMs)
	require.Equal(t, 45000, *items[0].DurationMs)
	require.True(t, items[0].Stream)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryListRequestDetails_InvalidSortFailsBeforeQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	_, _, err := repo.ListRequestDetails(context.Background(), &service.OpsRequestDetailFilter{Sort: "not-a-sort"})
	require.EqualError(t, err, "invalid sort")
	require.NoError(t, mock.ExpectationsWereMet())
}
