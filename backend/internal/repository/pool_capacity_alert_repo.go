package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type poolCapacityAlertRepository struct{ db *sql.DB }

func NewPoolCapacityAlertRepository(db *sql.DB) service.PoolCapacityAlertRepository {
	return &poolCapacityAlertRepository{db: db}
}

func (r *poolCapacityAlertRepository) EvaluateAndMaybeCreateEvent(ctx context.Context, evaluation service.PoolCapacityEvaluation, now time.Time) (*service.PoolCapacityAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("pool capacity alert repository is not configured")
	}
	if evaluation.GroupID <= 0 || evaluation.AccountID <= 0 || evaluation.APIKeyID <= 0 || evaluation.UserID <= 0 {
		return nil, errors.New("invalid pool capacity alert scope")
	}
	evaluation.AlertMetric = strings.TrimSpace(evaluation.AlertMetric)
	if evaluation.AlertMetric == "" {
		evaluation.AlertMetric = service.PoolCapacityAlertMetricPredictedRequests
	}
	if evaluation.ThresholdRequests == nil || *evaluation.ThresholdRequests <= 0 {
		threshold := service.DefaultPoolCapacityAlertThresholdRequests
		evaluation.ThresholdRequests = &threshold
	}
	switch evaluation.AlertMetric {
	case service.PoolCapacityAlertMetricPredictedRequests:
	case service.PoolCapacityAlertMetricRemainingBalanceUSD:
		if evaluation.ThresholdUSD == nil || *evaluation.ThresholdUSD <= 0 {
			return nil, errors.New("invalid pool capacity alert USD threshold")
		}
	default:
		return nil, errors.New("invalid pool capacity alert metric")
	}
	if evaluation.ReminderCooldown <= 0 {
		evaluation.ReminderCooldown = 24 * time.Hour
	}
	if evaluation.DeliveryMaxAttempts < 1 {
		evaluation.DeliveryMaxAttempts = 6
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentGroupName string
	err = tx.QueryRowContext(ctx, `
		SELECT name FROM groups
		WHERE id=$1 AND status=$2
		  AND pool_capacity_alert_enabled=TRUE
		  AND pool_capacity_alert_generation=$3`,
		evaluation.GroupID, service.StatusActive, evaluation.GroupGeneration,
	).Scan(&currentGroupName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(evaluation.GroupName) == "" {
		evaluation.GroupName = currentGroupName
	}

	scope := fmt.Sprintf("pool-capacity:%d:%d:%d:%d:%d:%d", evaluation.GroupID, evaluation.GroupGeneration, evaluation.AccountID, evaluation.APIKeyID, evaluation.UserID, evaluation.BillingType)
	var ignored string
	if err := tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))::text`, scope).Scan(&ignored); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pool_capacity_alert_states (
			group_id,group_generation,account_id,api_key_id,user_id,billing_type,status,episode,
			alert_metric,predicted_requests,remaining_balance_usd,threshold_requests,threshold_usd,
			account_requests,api_key_requests,wallet_requests,avg_account_cost,avg_actual_cost,
			sample_count,bottleneck,last_evaluated_at,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'healthy',0,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$19,$19)
		ON CONFLICT (group_id,group_generation,account_id,api_key_id,user_id,billing_type) DO NOTHING`,
		evaluation.GroupID, evaluation.GroupGeneration, evaluation.AccountID, evaluation.APIKeyID, evaluation.UserID, evaluation.BillingType,
		evaluation.AlertMetric, evaluation.PredictedRequests, evaluation.RemainingBalanceUSD, evaluation.ThresholdRequests, evaluation.ThresholdUSD,
		evaluation.AccountRequests, evaluation.APIKeyRequests, evaluation.WalletRequests, evaluation.AverageAccountCost, evaluation.AverageActualCost,
		evaluation.SampleCount, evaluation.Bottleneck, now,
	); err != nil {
		return nil, err
	}

	var stateID, episode int64
	var oldStatus string
	var lastAlerted sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT id,status,episode,last_alerted_at
		FROM pool_capacity_alert_states
		WHERE group_id=$1 AND group_generation=$2 AND account_id=$3 AND api_key_id=$4 AND user_id=$5 AND billing_type=$6
		FOR UPDATE`, evaluation.GroupID, evaluation.GroupGeneration, evaluation.AccountID, evaluation.APIKeyID, evaluation.UserID, evaluation.BillingType,
	).Scan(&stateID, &oldStatus, &episode, &lastAlerted); err != nil {
		return nil, err
	}

	isLow := poolCapacityEvaluationBelowThreshold(evaluation)
	if !isLow {
		_, err = tx.ExecContext(ctx, `
			UPDATE pool_capacity_alert_states SET
				status='healthy',alert_metric=$1,predicted_requests=$2,remaining_balance_usd=$3,
				threshold_requests=$4,threshold_usd=$5,account_requests=$6,api_key_requests=$7,wallet_requests=$8,
				avg_account_cost=$9,avg_actual_cost=$10,sample_count=$11,bottleneck=$12,last_evaluated_at=$13,updated_at=$13
			WHERE id=$14`, evaluation.AlertMetric, evaluation.PredictedRequests, evaluation.RemainingBalanceUSD,
			evaluation.ThresholdRequests, evaluation.ThresholdUSD, evaluation.AccountRequests, evaluation.APIKeyRequests, evaluation.WalletRequests,
			evaluation.AverageAccountCost, evaluation.AverageActualCost, evaluation.SampleCount, evaluation.Bottleneck, now, stateID)
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit()
	}

	alertDue := oldStatus != service.PoolCapacityAlertStatusLow || !lastAlerted.Valid || !lastAlerted.Time.Add(evaluation.ReminderCooldown).After(now)
	if alertDue {
		episode++
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE pool_capacity_alert_states SET
			status='low',episode=$1,alert_metric=$2,predicted_requests=$3,remaining_balance_usd=$4,
			threshold_requests=$5,threshold_usd=$6,account_requests=$7,api_key_requests=$8,wallet_requests=$9,
			avg_account_cost=$10,avg_actual_cost=$11,sample_count=$12,bottleneck=$13,last_evaluated_at=$14,
			last_alerted_at=CASE WHEN $15 THEN $14 ELSE last_alerted_at END,updated_at=$14
		WHERE id=$16`, episode, evaluation.AlertMetric, evaluation.PredictedRequests, evaluation.RemainingBalanceUSD,
		evaluation.ThresholdRequests, evaluation.ThresholdUSD, evaluation.AccountRequests, evaluation.APIKeyRequests, evaluation.WalletRequests,
		evaluation.AverageAccountCost, evaluation.AverageActualCost, evaluation.SampleCount, evaluation.Bottleneck, now, alertDue, stateID)
	if err != nil {
		return nil, err
	}
	if !alertDue {
		return nil, tx.Commit()
	}

	eventThresholdRequests := poolCapacityEventThresholdRequests(evaluation)
	event := &service.PoolCapacityAlertEvent{
		StateID:             stateID,
		Episode:             episode,
		GroupID:             evaluation.GroupID,
		GroupGeneration:     evaluation.GroupGeneration,
		AccountID:           evaluation.AccountID,
		APIKeyID:            evaluation.APIKeyID,
		UserID:              evaluation.UserID,
		BillingType:         evaluation.BillingType,
		GroupName:           evaluation.GroupName,
		AccountName:         evaluation.AccountName,
		APIKeyName:          evaluation.APIKeyName,
		UserEmail:           evaluation.UserEmail,
		AlertMetric:         evaluation.AlertMetric,
		PredictedRequests:   evaluation.PredictedRequests,
		RemainingBalanceUSD: evaluation.RemainingBalanceUSD,
		ThresholdRequests:   eventThresholdRequests,
		ThresholdUSD:        evaluation.ThresholdUSD,
		AccountRequests:     evaluation.AccountRequests,
		APIKeyRequests:      evaluation.APIKeyRequests,
		WalletRequests:      evaluation.WalletRequests,
		AverageAccountCost:  evaluation.AverageAccountCost,
		AverageActualCost:   evaluation.AverageActualCost,
		AccountRemaining:    evaluation.AccountRemaining,
		APIKeyRemaining:     evaluation.APIKeyRemaining,
		WalletRemaining:     evaluation.WalletRemaining,
		SampleCount:         evaluation.SampleCount,
		Bottleneck:          evaluation.Bottleneck,
		QQBotAppID:          evaluation.QQBotAppID,
		CreatedAt:           now,
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO pool_capacity_alert_events (
			state_id,episode,group_id,group_generation,account_id,api_key_id,user_id,billing_type,
			group_name,account_name,api_key_name,user_email,alert_metric,predicted_requests,remaining_balance_usd,
			threshold_requests,threshold_usd,account_requests,api_key_requests,wallet_requests,avg_account_cost,avg_actual_cost,
			account_remaining,api_key_remaining,wallet_remaining,sample_count,bottleneck,qqbot_app_id,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
		ON CONFLICT (state_id,episode) DO UPDATE SET state_id=EXCLUDED.state_id
		RETURNING id`,
		event.StateID, event.Episode, event.GroupID, event.GroupGeneration, event.AccountID, event.APIKeyID, event.UserID, event.BillingType,
		event.GroupName, event.AccountName, event.APIKeyName, event.UserEmail, event.AlertMetric, event.PredictedRequests, event.RemainingBalanceUSD,
		event.ThresholdRequests, event.ThresholdUSD, event.AccountRequests, event.APIKeyRequests, event.WalletRequests, event.AverageAccountCost, event.AverageActualCost,
		event.AccountRemaining, event.APIKeyRemaining, event.WalletRemaining, event.SampleCount, event.Bottleneck, event.QQBotAppID, event.CreatedAt,
	).Scan(&event.ID); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pool_capacity_alert_deliveries (
			event_id,channel,recipient_user_id,identity_channel_id,recipient_email,recipient_name,locale,
			status,attempt_count,max_attempts,next_attempt_at,created_at,updated_at
		)
		SELECT $1,'email',u.id,0,u.email,
		       COALESCE(NULLIF(u.username,''),split_part(u.email,'@',1)),'',
		       'pending',0,$2,$3,$3,$3
		FROM (
			SELECT DISTINCT ON (LOWER(BTRIM(candidate.email)))
			       candidate.id,BTRIM(candidate.email) AS email,BTRIM(candidate.username) AS username
			FROM users candidate
			WHERE candidate.role=$4 AND candidate.status=$5 AND candidate.deleted_at IS NULL
			  AND BTRIM(candidate.email) <> ''
			  AND BTRIM(candidate.email) ~* '^[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}$'
			  AND LOWER(BTRIM(candidate.email)) NOT LIKE '%-connect.invalid'
			ORDER BY LOWER(BTRIM(candidate.email)),candidate.id
		) u
		ON CONFLICT (event_id,channel,recipient_user_id,identity_channel_id) DO NOTHING`,
		event.ID, evaluation.DeliveryMaxAttempts, now, service.RoleAdmin, service.StatusActive,
	); err != nil {
		return nil, err
	}

	if strings.TrimSpace(evaluation.QQBotAppID) != "" {
		providerKey := "qqbot:" + strings.TrimSpace(evaluation.QQBotAppID)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pool_capacity_alert_deliveries (
				event_id,channel,recipient_user_id,identity_channel_id,recipient_email,recipient_name,locale,
				status,attempt_count,max_attempts,next_attempt_at,created_at,updated_at
			)
			SELECT DISTINCT $1,'qqbot',u.id,aic.id,'',
			       COALESCE(NULLIF(BTRIM(u.username),''),split_part(BTRIM(u.email),'@',1)),'zh',
			       'pending',0,$2,$3,$3,$3
			FROM users u
			JOIN auth_identities ai ON ai.user_id=u.id
			JOIN auth_identity_channels aic ON aic.identity_id=ai.id
			WHERE u.role=$4 AND u.status=$5 AND u.deleted_at IS NULL
			  AND ai.provider_type='qqbot' AND ai.provider_key=$6 AND ai.verified_at IS NOT NULL
			  AND ai.provider_subject=aic.channel_subject
			  AND aic.provider_type='qqbot' AND aic.provider_key=$6
			  AND aic.channel='c2c' AND aic.channel_app_id=$7
			ON CONFLICT (event_id,channel,recipient_user_id,identity_channel_id) DO NOTHING`,
			event.ID, evaluation.DeliveryMaxAttempts, now, service.RoleAdmin, service.StatusActive, providerKey, evaluation.QQBotAppID,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return event, nil
}

func (r *poolCapacityAlertRepository) ClaimDeliveries(ctx context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]service.PoolCapacityAlertDelivery, error) {
	if r == nil || r.db == nil || limit <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE pool_capacity_alert_deliveries d SET status='cancelled',last_error_class='cancelled',last_error='group alert disabled or generation changed',lease_owner=NULL,lease_expires_at=NULL,updated_at=$1
		FROM pool_capacity_alert_events e
		WHERE d.event_id=e.id
		  AND (d.status IN ('pending','retry') OR (d.status='sending' AND d.lease_expires_at < $1))
		  AND NOT EXISTS (
			SELECT 1 FROM groups g WHERE g.id=e.group_id AND g.status=$2
			  AND g.pool_capacity_alert_enabled=TRUE AND g.pool_capacity_alert_generation=e.group_generation
		  )`, now, service.StatusActive); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE pool_capacity_alert_deliveries d SET status='cancelled',last_error_class='cancelled',last_error='administrator recipient inactive',lease_owner=NULL,lease_expires_at=NULL,updated_at=$1
		WHERE (d.status IN ('pending','retry') OR (d.status='sending' AND d.lease_expires_at < $1))
		  AND NOT EXISTS (
			SELECT 1 FROM users u
			WHERE u.role=$2 AND u.status=$3 AND u.deleted_at IS NULL
			  AND ((d.channel='email' AND LOWER(BTRIM(u.email))=LOWER(BTRIM(d.recipient_email)))
			       OR (d.channel<>'email' AND u.id=d.recipient_user_id))
		  )`,
		now, service.RoleAdmin, service.StatusActive); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE pool_capacity_alert_deliveries SET status='dead',last_error_class='lease_expired',
		last_error='delivery lease expired after final attempt',lease_owner=NULL,lease_expires_at=NULL,updated_at=$1
		WHERE status='sending' AND lease_expires_at < $1 AND attempt_count >= max_attempts`, now); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id FROM pool_capacity_alert_deliveries
			WHERE ((status IN ('pending','retry') AND next_attempt_at <= $1)
			    OR (status='sending' AND lease_expires_at < $1))
			  AND attempt_count < max_attempts
			ORDER BY next_attempt_at,id
			FOR UPDATE SKIP LOCKED LIMIT $2
		), claimed AS (
			UPDATE pool_capacity_alert_deliveries d SET
				status='sending',lease_owner=$3,lease_expires_at=$4,attempt_count=d.attempt_count+1,updated_at=$1
			FROM candidates c WHERE d.id=c.id
			RETURNING d.id,d.event_id,d.channel,d.recipient_user_id,d.identity_channel_id,d.recipient_email,d.recipient_name,d.locale,d.attempt_count,d.max_attempts
		)
		SELECT c.id,c.channel,c.recipient_user_id,c.identity_channel_id,c.recipient_email,c.recipient_name,c.locale,c.attempt_count,c.max_attempts,
		       e.id,e.state_id,e.episode,e.group_id,e.group_generation,e.account_id,e.api_key_id,e.user_id,e.billing_type,
		       e.group_name,e.account_name,e.api_key_name,e.user_email,e.alert_metric,e.predicted_requests,e.remaining_balance_usd,
		       e.threshold_requests,e.threshold_usd,e.account_requests,e.api_key_requests,e.wallet_requests,e.avg_account_cost,e.avg_actual_cost,
		       e.account_remaining,e.api_key_remaining,e.wallet_remaining,e.sample_count,e.bottleneck,e.qqbot_app_id,e.created_at
		FROM claimed c JOIN pool_capacity_alert_events e ON e.id=c.event_id
		ORDER BY c.id`, now, limit, owner, now.Add(lease))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.PoolCapacityAlertDelivery, 0, limit)
	for rows.Next() {
		var delivery service.PoolCapacityAlertDelivery
		var predictedRequests, thresholdRequests sql.NullInt64
		var accountRequests, apiKeyRequests, walletRequests sql.NullInt64
		var remainingBalanceUSD, thresholdUSD sql.NullFloat64
		var accountRemaining, apiKeyRemaining, walletRemaining sql.NullFloat64
		if err := rows.Scan(
			&delivery.ID, &delivery.Channel, &delivery.RecipientUserID, &delivery.IdentityChannelID,
			&delivery.RecipientEmail, &delivery.RecipientName, &delivery.Locale, &delivery.AttemptCount, &delivery.MaxAttempts,
			&delivery.Event.ID, &delivery.Event.StateID, &delivery.Event.Episode, &delivery.Event.GroupID, &delivery.Event.GroupGeneration,
			&delivery.Event.AccountID, &delivery.Event.APIKeyID, &delivery.Event.UserID, &delivery.Event.BillingType,
			&delivery.Event.GroupName, &delivery.Event.AccountName, &delivery.Event.APIKeyName, &delivery.Event.UserEmail,
			&delivery.Event.AlertMetric, &predictedRequests, &remainingBalanceUSD, &thresholdRequests, &thresholdUSD,
			&accountRequests, &apiKeyRequests, &walletRequests, &delivery.Event.AverageAccountCost, &delivery.Event.AverageActualCost,
			&accountRemaining, &apiKeyRemaining, &walletRemaining, &delivery.Event.SampleCount, &delivery.Event.Bottleneck,
			&delivery.Event.QQBotAppID, &delivery.Event.CreatedAt,
		); err != nil {
			return nil, err
		}
		delivery.Event.PredictedRequests = poolCapacityNullableInt64Ptr(predictedRequests)
		delivery.Event.RemainingBalanceUSD = poolCapacityNullableFloat64Ptr(remainingBalanceUSD)
		delivery.Event.ThresholdRequests = poolCapacityNullableInt64Ptr(thresholdRequests)
		delivery.Event.ThresholdUSD = poolCapacityNullableFloat64Ptr(thresholdUSD)
		delivery.Event.AccountRequests = poolCapacityNullableInt64Ptr(accountRequests)
		delivery.Event.APIKeyRequests = poolCapacityNullableInt64Ptr(apiKeyRequests)
		delivery.Event.WalletRequests = poolCapacityNullableInt64Ptr(walletRequests)
		delivery.Event.AccountRemaining = poolCapacityNullableFloat64Ptr(accountRemaining)
		delivery.Event.APIKeyRemaining = poolCapacityNullableFloat64Ptr(apiKeyRemaining)
		delivery.Event.WalletRemaining = poolCapacityNullableFloat64Ptr(walletRemaining)
		out = append(out, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *poolCapacityAlertRepository) IsDeliveryCurrent(ctx context.Context, deliveryID int64, owner string) (bool, error) {
	if r == nil || r.db == nil || deliveryID <= 0 || strings.TrimSpace(owner) == "" {
		return false, nil
	}
	var current bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pool_capacity_alert_deliveries d
			JOIN pool_capacity_alert_events e ON e.id=d.event_id
			JOIN groups g ON g.id=e.group_id
			WHERE d.id=$1 AND d.status='sending' AND d.lease_owner=$2 AND d.lease_expires_at > NOW()
			  AND g.status=$3 AND g.pool_capacity_alert_enabled=TRUE
			  AND g.pool_capacity_alert_generation=e.group_generation
			  AND EXISTS (
				SELECT 1 FROM users u
				WHERE u.role=$4 AND u.status=$3 AND u.deleted_at IS NULL
				  AND ((d.channel='email' AND LOWER(BTRIM(u.email))=LOWER(BTRIM(d.recipient_email)))
				       OR (d.channel<>'email' AND u.id=d.recipient_user_id))
			  )
		)`, deliveryID, owner, service.StatusActive, service.RoleAdmin).Scan(&current)
	return current, err
}

func poolCapacityEventThresholdRequests(evaluation service.PoolCapacityEvaluation) *int64 {
	if evaluation.AlertMetric == service.PoolCapacityAlertMetricRemainingBalanceUSD {
		return nil
	}
	return evaluation.ThresholdRequests
}

func poolCapacityEvaluationBelowThreshold(evaluation service.PoolCapacityEvaluation) bool {
	switch evaluation.AlertMetric {
	case service.PoolCapacityAlertMetricPredictedRequests:
		return evaluation.PredictedRequests != nil && evaluation.ThresholdRequests != nil && *evaluation.PredictedRequests < *evaluation.ThresholdRequests
	case service.PoolCapacityAlertMetricRemainingBalanceUSD:
		return evaluation.RemainingBalanceUSD != nil && evaluation.ThresholdUSD != nil &&
			decimal.NewFromFloat(*evaluation.RemainingBalanceUSD).LessThan(decimal.NewFromFloat(*evaluation.ThresholdUSD))
	default:
		return false
	}
}

func poolCapacityNullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}

func poolCapacityNullableFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
	return &out
}

func (r *poolCapacityAlertRepository) MarkDeliverySent(ctx context.Context, deliveryID int64, owner string, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE pool_capacity_alert_deliveries SET status='sent',sent_at=$1,lease_owner=NULL,lease_expires_at=NULL,
		last_error_class=NULL,last_error=NULL,updated_at=$1
		WHERE id=$2 AND status='sending' AND lease_owner=$3`, sentAt, deliveryID, owner)
	return err
}

func (r *poolCapacityAlertRepository) MarkDeliveryFailed(ctx context.Context, deliveryID int64, owner, class, message string, nextAttemptAt *time.Time) error {
	status := service.PoolCapacityAlertDeliveryDead
	if nextAttemptAt != nil {
		status = service.PoolCapacityAlertDeliveryRetry
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE pool_capacity_alert_deliveries SET status=$1,next_attempt_at=COALESCE($2::timestamptz,next_attempt_at),
		lease_owner=NULL,lease_expires_at=NULL,last_error_class=$3,last_error=$4,updated_at=NOW()
		WHERE id=$5 AND status='sending' AND lease_owner=$6`, status, nextAttemptAt, class, message, deliveryID, owner)
	return err
}

func (r *poolCapacityAlertRepository) MarkDeliveryCancelled(ctx context.Context, deliveryID int64, owner, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE pool_capacity_alert_deliveries SET status='cancelled',lease_owner=NULL,lease_expires_at=NULL,
		last_error_class='cancelled',last_error=$1,updated_at=NOW()
		WHERE id=$2 AND status='sending' AND lease_owner=$3`, reason, deliveryID, owner)
	return err
}
