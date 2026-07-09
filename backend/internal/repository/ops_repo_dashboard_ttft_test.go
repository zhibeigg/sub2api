package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQueryExactTTFTOrFallback_UsesExactWindowPercentiles(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 7, 9, 3, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filter := &service.OpsDashboardFilter{Platform: " Kiro "}

	mock.ExpectQuery(`(?s)percentile_cont\(0\.50\).*COUNT\(first_token_ms\).*FROM usage_logs ul`).
		WithArgs(start, end, "kiro").
		WillReturnRows(sqlmock.NewRows([]string{
			"ttft_p50",
			"ttft_p90",
			"ttft_p95",
			"ttft_p99",
			"ttft_avg",
			"ttft_max",
			"ttft_sample_count",
		}).AddRow(6100.4, 13012.6, 22000.2, 33314.1, 7686.3, int64(53003), int64(905)))

	fallbackP99 := 54425
	fallback := service.OpsPercentiles{P99: &fallbackP99}
	ttft, approximate, err := repo.queryExactTTFTOrFallback(context.Background(), filter, start, end, fallback)
	require.NoError(t, err)
	require.False(t, approximate)
	require.NotNil(t, ttft.P50)
	require.Equal(t, 6100, *ttft.P50)
	require.NotNil(t, ttft.P99)
	require.Equal(t, 33314, *ttft.P99)
	require.NotNil(t, ttft.Max)
	require.Equal(t, 53003, *ttft.Max)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryExactTTFTOrFallback_MarksTimeoutFallbackApproximate(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 7, 9, 3, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	fallbackP99 := 54425
	fallback := service.OpsPercentiles{P99: &fallbackP99}

	mock.ExpectQuery(`(?s)percentile_cont\(0\.50\).*COUNT\(first_token_ms\).*FROM usage_logs ul`).
		WithArgs(start, end).
		WillReturnError(context.DeadlineExceeded)

	ttft, approximate, err := repo.queryExactTTFTOrFallback(context.Background(), &service.OpsDashboardFilter{}, start, end, fallback)
	require.NoError(t, err)
	require.True(t, approximate)
	require.NotNil(t, ttft.P99)
	require.Equal(t, fallbackP99, *ttft.P99)
	require.NoError(t, mock.ExpectationsWereMet())
}
