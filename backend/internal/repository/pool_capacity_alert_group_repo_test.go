package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPoolCapacityGroupBalanceCreatesSingleGroupScopeEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	now := time.Date(2026, time.July, 22, 5, 0, 0, 0, time.UTC)
	remaining := 9.25
	repo := &poolCapacityAlertRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT name FROM groups.*pool_capacity_alert_metric=\$3.*pool_capacity_alert_generation=\$4`).
		WithArgs(int64(17), service.StatusActive, service.PoolCapacityAlertMetricRemainingBalanceUSD, int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("production"))
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock`).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(""))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_states.*'group'.*ON CONFLICT \(group_id,group_generation\) WHERE scope_type='group' DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id,status,episode,last_alerted_at.*scope_type='group'.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "episode", "last_alerted_at"}).
			AddRow(5, service.PoolCapacityAlertStatusHealthy, 0, nil))
	mock.ExpectExec(`(?s)UPDATE pool_capacity_alert_states SET.*status='low'.*pool_authoritative_balance_usd`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO pool_capacity_alert_events.*'group'.*RETURNING id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_deliveries.*SELECT \$1,'email'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	event, err := repo.EvaluateGroupBalanceAndMaybeCreateEvent(context.Background(), service.PoolCapacityGroupBalanceEvaluation{
		GroupID:                      17,
		GroupGeneration:              4,
		RemainingBalanceUSD:          &remaining,
		PoolAuthoritativeBalanceUSD:  5.25,
		NormalEstimatedBalanceUSD:    4,
		PoolAccountCount:             2,
		NormalAccountCount:           3,
		SkippedAccountCount:          1,
		UnknownAccountCount:          0,
		StaleAccountCount:            0,
		IncompatibleUnitAccountCount: 0,
		ThresholdUSD:                 10,
		ReminderCooldown:             time.Hour,
		DeliveryMaxAttempts:          3,
	}, now)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, int64(9), event.ID)
	require.Equal(t, service.PoolCapacityAlertScopeGroup, event.ScopeType)
	require.Zero(t, event.AccountID)
	require.InDelta(t, remaining, *event.RemainingBalanceUSD, 1e-12)
	require.InDelta(t, 5.25, *event.PoolAuthoritativeBalanceUSD, 1e-12)
	require.InDelta(t, 4, *event.NormalEstimatedBalanceUSD, 1e-12)
	require.Equal(t, 2, event.PoolAccountCount)
	require.Equal(t, 3, event.NormalAccountCount)
	require.Equal(t, 1, event.SkippedAccountCount)
}

func TestPoolCapacityGroupBalanceEqualThresholdRecoversHealthyWithoutEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	now := time.Date(2026, time.July, 22, 5, 0, 0, 0, time.UTC)
	remaining := 10.0
	repo := &poolCapacityAlertRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT name FROM groups`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("production"))
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock`).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(""))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_states`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id,status,episode,last_alerted_at.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "episode", "last_alerted_at"}).
			AddRow(5, service.PoolCapacityAlertStatusLow, 2, now.Add(-time.Minute)))
	mock.ExpectExec(`(?s)UPDATE pool_capacity_alert_states SET.*status='healthy'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	event, err := repo.EvaluateGroupBalanceAndMaybeCreateEvent(context.Background(), service.PoolCapacityGroupBalanceEvaluation{
		GroupID:             17,
		GroupGeneration:     4,
		RemainingBalanceUSD: &remaining,
		PoolAccountCount:    1,
		NormalAccountCount:  1,
		ThresholdUSD:        10,
		ReminderCooldown:    time.Hour,
		DeliveryMaxAttempts: 3,
	}, now)
	require.NoError(t, err)
	require.Nil(t, event, "strict comparison must not alert at equality")
}

func TestPoolCapacityGroupBalanceUnlimitedRecoversHealthy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	now := time.Date(2026, time.July, 22, 5, 0, 0, 0, time.UTC)
	repo := &poolCapacityAlertRepository{db: db}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT name FROM groups`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("production"))
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock`).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(""))
	mock.ExpectExec(`(?s)INSERT INTO pool_capacity_alert_states`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id,status,episode,last_alerted_at.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "episode", "last_alerted_at"}).
			AddRow(5, service.PoolCapacityAlertStatusLow, 2, now.Add(-time.Minute)))
	mock.ExpectExec(`(?s)UPDATE pool_capacity_alert_states SET.*status='healthy'.*remaining_balance_usd=\$2`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	event, err := repo.EvaluateGroupBalanceAndMaybeCreateEvent(context.Background(), service.PoolCapacityGroupBalanceEvaluation{
		GroupID:             17,
		GroupGeneration:     4,
		Unlimited:           true,
		PoolAccountCount:    1,
		ThresholdUSD:        10,
		ReminderCooldown:    time.Hour,
		DeliveryMaxAttempts: 3,
	}, now)
	require.NoError(t, err)
	require.Nil(t, event)
}
