package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestPoolCapacityEvaluationBelowThresholdIsStrict(t *testing.T) {
	fortyNine := int64(49)
	fifty := int64(50)
	fiftyOne := int64(51)
	thresholdRequests := int64(50)

	require.True(t, poolCapacityEvaluationBelowThreshold(service.PoolCapacityEvaluation{
		AlertMetric:       service.PoolCapacityAlertMetricPredictedRequests,
		PredictedRequests: &fortyNine,
		ThresholdRequests: &thresholdRequests,
	}))
	require.False(t, poolCapacityEvaluationBelowThreshold(service.PoolCapacityEvaluation{
		AlertMetric:       service.PoolCapacityAlertMetricPredictedRequests,
		PredictedRequests: &fifty,
		ThresholdRequests: &thresholdRequests,
	}), "exactly 50 requests must not alert")
	require.False(t, poolCapacityEvaluationBelowThreshold(service.PoolCapacityEvaluation{
		AlertMetric:       service.PoolCapacityAlertMetricPredictedRequests,
		PredictedRequests: &fiftyOne,
		ThresholdRequests: &thresholdRequests,
	}))

	belowUSD := 9.99
	equalUSD := 10.0
	thresholdUSD := 10.0
	require.True(t, poolCapacityEvaluationBelowThreshold(service.PoolCapacityEvaluation{
		AlertMetric:         service.PoolCapacityAlertMetricRemainingBalanceUSD,
		RemainingBalanceUSD: &belowUSD,
		ThresholdUSD:        &thresholdUSD,
	}))
	require.False(t, poolCapacityEvaluationBelowThreshold(service.PoolCapacityEvaluation{
		AlertMetric:         service.PoolCapacityAlertMetricRemainingBalanceUSD,
		RemainingBalanceUSD: &equalUSD,
		ThresholdUSD:        &thresholdUSD,
	}), "exactly equal USD balance must not alert")

	require.Nil(t, poolCapacityEventThresholdRequests(service.PoolCapacityEvaluation{
		AlertMetric:       service.PoolCapacityAlertMetricRemainingBalanceUSD,
		ThresholdRequests: &thresholdRequests,
	}), "amount events must keep legacy request-threshold placeholders at N/A")
	require.Equal(t, &thresholdRequests, poolCapacityEventThresholdRequests(service.PoolCapacityEvaluation{
		AlertMetric:       service.PoolCapacityAlertMetricPredictedRequests,
		ThresholdRequests: &thresholdRequests,
	}))
}

func TestGetRecentPoolCapacityCostSummaryUsesBoundedSuccessfulBillingHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*usage_billing_dedup.*usage_billing_dedup_archive.*ORDER BY ul\.created_at DESC, ul\.id DESC.*LIMIT \$5`).
		WithArgs(int64(12), "request-current", int64(34), int16(service.RequestTypeCyberBlocked), 49).
		WillReturnRows(sqlmock.NewRows([]string{"count", "account_cost_sum", "actual_cost_sum"}).AddRow(49, "4.9000000000", "9.8000000000"))
	mock.ExpectClose()

	summary, err := repo.GetRecentPoolCapacityCostSummary(context.Background(), 12, "request-current", 34, 49)
	require.NoError(t, err)
	require.Equal(t, 49, summary.Count)
	require.True(t, summary.AccountCostSum.Equal(decimal.RequireFromString("4.9000000000")))
	require.True(t, summary.ActualCostSum.Equal(decimal.RequireFromString("9.8000000000")))
}

func TestPoolCapacityAlertEventDeduplicatesAdministratorEmails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	predicted := int64(49)
	repo := &poolCapacityAlertRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT name FROM groups.*pool_capacity_alert_generation=\$3.*pool_capacity_alert_metric=\$4`).
		WithArgs(int64(1), service.StatusActive, int64(3), service.PoolCapacityAlertMetricPredictedRequests).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("pool-group"))
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock`).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(""))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_states`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id,status,episode,last_alerted_at.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "episode", "last_alerted_at"}).
			AddRow(5, service.PoolCapacityAlertStatusHealthy, 0, nil))
	mock.ExpectExec(`(?s)UPDATE pool_capacity_alert_states SET.*status='low'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO pool_capacity_alert_events.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_deliveries.*SELECT DISTINCT ON \(LOWER\(BTRIM\(candidate\.email\)\)\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_deliveries.*ai\.provider_subject=aic\.channel_subject`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	thresholdRequests := service.DefaultPoolCapacityAlertThresholdRequests
	event, err := repo.EvaluateAndMaybeCreateEvent(context.Background(), service.PoolCapacityEvaluation{
		GroupID:             1,
		GroupGeneration:     3,
		AccountID:           2,
		APIKeyID:            3,
		UserID:              4,
		BillingType:         service.BillingTypeBalance,
		AlertMetric:         service.PoolCapacityAlertMetricPredictedRequests,
		PredictedRequests:   &predicted,
		AverageAccountCost:  1,
		AverageActualCost:   1,
		SampleCount:         service.PoolCapacityAlertSampleSize,
		QQBotAppID:          "app-1",
		ThresholdRequests:   &thresholdRequests,
		ReminderCooldown:    time.Hour,
		DeliveryMaxAttempts: 3,
	}, now)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, int64(9), event.ID)
}

func TestPoolCapacityAlertDeliveryRevalidationChecksGenerationAndPrimaryEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	repo := &poolCapacityAlertRepository{db: db}
	mock.ExpectQuery(`(?s)SELECT EXISTS.*s\.status=\$5 AND s\.episode=e\.episode.*pool_capacity_alert_generation=e\.group_generation.*pool_capacity_alert_metric=e\.alert_metric.*LOWER\(BTRIM\(u\.email\)\)=LOWER\(BTRIM\(d\.recipient_email\)\)`).
		WithArgs(int64(7), "worker-1", service.StatusActive, service.RoleAdmin, service.PoolCapacityAlertStatusLow).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectClose()

	current, err := repo.IsDeliveryCurrent(context.Background(), 7, "worker-1")
	require.NoError(t, err)
	require.True(t, current)
}
