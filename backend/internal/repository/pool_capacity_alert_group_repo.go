package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

func (r *poolCapacityAlertRepository) EvaluateGroupBalanceAndMaybeCreateEvent(ctx context.Context, evaluation service.PoolCapacityGroupBalanceEvaluation, now time.Time) (*service.PoolCapacityAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("pool capacity alert repository is not configured")
	}
	if evaluation.GroupID <= 0 || evaluation.GroupGeneration < 0 {
		return nil, errors.New("invalid group balance alert scope")
	}
	if evaluation.ThresholdUSD <= 0 {
		return nil, errors.New("invalid group balance alert USD threshold")
	}
	if evaluation.PoolAuthoritativeBalanceUSD < 0 || evaluation.NormalEstimatedBalanceUSD < 0 ||
		math.IsNaN(evaluation.PoolAuthoritativeBalanceUSD) || math.IsInf(evaluation.PoolAuthoritativeBalanceUSD, 0) ||
		math.IsNaN(evaluation.NormalEstimatedBalanceUSD) || math.IsInf(evaluation.NormalEstimatedBalanceUSD, 0) {
		return nil, errors.New("invalid group balance alert amount")
	}
	if !evaluation.Unlimited {
		if evaluation.RemainingBalanceUSD == nil || *evaluation.RemainingBalanceUSD < 0 || math.IsNaN(*evaluation.RemainingBalanceUSD) || math.IsInf(*evaluation.RemainingBalanceUSD, 0) {
			return nil, errors.New("invalid group balance alert amount")
		}
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
		  AND pool_capacity_alert_metric=$3
		  AND pool_capacity_alert_generation=$4`,
		evaluation.GroupID, service.StatusActive, service.PoolCapacityAlertMetricRemainingBalanceUSD, evaluation.GroupGeneration,
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

	scope := fmt.Sprintf("pool-capacity:group:%d:%d", evaluation.GroupID, evaluation.GroupGeneration)
	var ignored string
	if err := tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))::text`, scope).Scan(&ignored); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pool_capacity_alert_states (
			group_id,group_generation,scope_type,account_id,api_key_id,user_id,billing_type,status,episode,
			alert_metric,predicted_requests,remaining_balance_usd,pool_authoritative_balance_usd,normal_estimated_balance_usd,
			pool_account_count,normal_account_count,skipped_account_count,unknown_account_count,stale_account_count,incompatible_unit_account_count,
			threshold_requests,threshold_usd,account_requests,api_key_requests,wallet_requests,avg_account_cost,avg_actual_cost,
			sample_count,bottleneck,last_evaluated_at,created_at,updated_at
		) VALUES ($1,$2,'group',NULL,NULL,NULL,NULL,'healthy',0,$3,NULL,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULL,NULL,NULL,0,0,$15,'group_predicted_balance',$16,$16,$16)
		ON CONFLICT (group_id,group_generation) WHERE scope_type='group' DO NOTHING`,
		evaluation.GroupID, evaluation.GroupGeneration, service.PoolCapacityAlertMetricRemainingBalanceUSD,
		evaluation.RemainingBalanceUSD, evaluation.PoolAuthoritativeBalanceUSD, evaluation.NormalEstimatedBalanceUSD,
		evaluation.PoolAccountCount, evaluation.NormalAccountCount, evaluation.SkippedAccountCount, evaluation.UnknownAccountCount,
		evaluation.StaleAccountCount, evaluation.IncompatibleUnitAccountCount, service.DefaultPoolCapacityAlertThresholdRequests,
		evaluation.ThresholdUSD, evaluation.PoolAccountCount+evaluation.NormalAccountCount, now,
	); err != nil {
		return nil, err
	}

	var stateID, episode int64
	var oldStatus string
	var lastAlerted sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT id,status,episode,last_alerted_at
		FROM pool_capacity_alert_states
		WHERE group_id=$1 AND group_generation=$2 AND scope_type='group'
		FOR UPDATE`, evaluation.GroupID, evaluation.GroupGeneration,
	).Scan(&stateID, &oldStatus, &episode, &lastAlerted); err != nil {
		return nil, err
	}

	isLow := !evaluation.Unlimited && evaluation.RemainingBalanceUSD != nil &&
		decimal.NewFromFloat(*evaluation.RemainingBalanceUSD).LessThan(decimal.NewFromFloat(evaluation.ThresholdUSD))
	if !isLow {
		_, err = tx.ExecContext(ctx, `
			UPDATE pool_capacity_alert_states SET
				status='healthy',alert_metric=$1,predicted_requests=NULL,remaining_balance_usd=$2,
				pool_authoritative_balance_usd=$3,normal_estimated_balance_usd=$4,pool_account_count=$5,normal_account_count=$6,
				skipped_account_count=$7,unknown_account_count=$8,stale_account_count=$9,incompatible_unit_account_count=$10,
				threshold_requests=$11,threshold_usd=$12,account_requests=NULL,api_key_requests=NULL,wallet_requests=NULL,
				avg_account_cost=0,avg_actual_cost=0,sample_count=$13,bottleneck='group_predicted_balance',last_evaluated_at=$14,updated_at=$14
			WHERE id=$15`, service.PoolCapacityAlertMetricRemainingBalanceUSD, evaluation.RemainingBalanceUSD,
			evaluation.PoolAuthoritativeBalanceUSD, evaluation.NormalEstimatedBalanceUSD, evaluation.PoolAccountCount, evaluation.NormalAccountCount,
			evaluation.SkippedAccountCount, evaluation.UnknownAccountCount, evaluation.StaleAccountCount, evaluation.IncompatibleUnitAccountCount,
			service.DefaultPoolCapacityAlertThresholdRequests, evaluation.ThresholdUSD, evaluation.PoolAccountCount+evaluation.NormalAccountCount, now, stateID)
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
			status='low',episode=$1,alert_metric=$2,predicted_requests=NULL,remaining_balance_usd=$3,
			pool_authoritative_balance_usd=$4,normal_estimated_balance_usd=$5,pool_account_count=$6,normal_account_count=$7,
			skipped_account_count=$8,unknown_account_count=$9,stale_account_count=$10,incompatible_unit_account_count=$11,
			threshold_requests=$12,threshold_usd=$13,account_requests=NULL,api_key_requests=NULL,wallet_requests=NULL,
			avg_account_cost=0,avg_actual_cost=0,sample_count=$14,bottleneck='group_predicted_balance',last_evaluated_at=$15,
			last_alerted_at=CASE WHEN $16 THEN $15 ELSE last_alerted_at END,updated_at=$15
		WHERE id=$17`, episode, service.PoolCapacityAlertMetricRemainingBalanceUSD, evaluation.RemainingBalanceUSD,
		evaluation.PoolAuthoritativeBalanceUSD, evaluation.NormalEstimatedBalanceUSD, evaluation.PoolAccountCount, evaluation.NormalAccountCount,
		evaluation.SkippedAccountCount, evaluation.UnknownAccountCount, evaluation.StaleAccountCount, evaluation.IncompatibleUnitAccountCount,
		service.DefaultPoolCapacityAlertThresholdRequests, evaluation.ThresholdUSD, evaluation.PoolAccountCount+evaluation.NormalAccountCount,
		now, alertDue, stateID)
	if err != nil {
		return nil, err
	}
	if !alertDue {
		return nil, tx.Commit()
	}

	remaining := *evaluation.RemainingBalanceUSD
	poolSubtotal := evaluation.PoolAuthoritativeBalanceUSD
	normalSubtotal := evaluation.NormalEstimatedBalanceUSD
	threshold := evaluation.ThresholdUSD
	event := &service.PoolCapacityAlertEvent{
		StateID:                      stateID,
		Episode:                      episode,
		GroupID:                      evaluation.GroupID,
		GroupGeneration:              evaluation.GroupGeneration,
		ScopeType:                    service.PoolCapacityAlertScopeGroup,
		GroupName:                    evaluation.GroupName,
		AlertMetric:                  service.PoolCapacityAlertMetricRemainingBalanceUSD,
		RemainingBalanceUSD:          &remaining,
		PoolAuthoritativeBalanceUSD:  &poolSubtotal,
		NormalEstimatedBalanceUSD:    &normalSubtotal,
		PoolAccountCount:             evaluation.PoolAccountCount,
		NormalAccountCount:           evaluation.NormalAccountCount,
		SkippedAccountCount:          evaluation.SkippedAccountCount,
		UnknownAccountCount:          evaluation.UnknownAccountCount,
		StaleAccountCount:            evaluation.StaleAccountCount,
		IncompatibleUnitAccountCount: evaluation.IncompatibleUnitAccountCount,
		ThresholdUSD:                 &threshold,
		SampleCount:                  evaluation.PoolAccountCount + evaluation.NormalAccountCount,
		Bottleneck:                   "group_predicted_balance",
		QQBotAppID:                   evaluation.QQBotAppID,
		CreatedAt:                    now,
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO pool_capacity_alert_events (
			state_id,episode,group_id,group_generation,scope_type,account_id,api_key_id,user_id,billing_type,
			group_name,account_name,api_key_name,user_email,alert_metric,predicted_requests,remaining_balance_usd,
			pool_authoritative_balance_usd,normal_estimated_balance_usd,pool_account_count,normal_account_count,
			skipped_account_count,unknown_account_count,stale_account_count,incompatible_unit_account_count,
			threshold_requests,threshold_usd,account_requests,api_key_requests,wallet_requests,avg_account_cost,avg_actual_cost,
			account_remaining,api_key_remaining,wallet_remaining,sample_count,bottleneck,qqbot_app_id,created_at
		) VALUES ($1,$2,$3,$4,'group',NULL,NULL,NULL,NULL,$5,'','','',$6,NULL,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULL,$16,NULL,NULL,NULL,0,0,NULL,NULL,NULL,$17,$18,$19,$20)
		ON CONFLICT (state_id,episode) DO UPDATE SET state_id=EXCLUDED.state_id
		RETURNING id`,
		event.StateID, event.Episode, event.GroupID, event.GroupGeneration, event.GroupName, event.AlertMetric,
		event.RemainingBalanceUSD, event.PoolAuthoritativeBalanceUSD, event.NormalEstimatedBalanceUSD,
		event.PoolAccountCount, event.NormalAccountCount, event.SkippedAccountCount, event.UnknownAccountCount,
		event.StaleAccountCount, event.IncompatibleUnitAccountCount, event.ThresholdUSD, event.SampleCount,
		event.Bottleneck, event.QQBotAppID, event.CreatedAt,
	).Scan(&event.ID); err != nil {
		return nil, err
	}

	if err := enqueuePoolCapacityAlertDeliveriesTx(ctx, tx, event.ID, evaluation.QQBotAppID, evaluation.DeliveryMaxAttempts, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return event, nil
}

func enqueuePoolCapacityAlertDeliveriesTx(ctx context.Context, tx *sql.Tx, eventID int64, qqbotAppID string, maxAttempts int, now time.Time) error {
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
		eventID, maxAttempts, now, service.RoleAdmin, service.StatusActive,
	); err != nil {
		return err
	}

	qqbotAppID = strings.TrimSpace(qqbotAppID)
	if qqbotAppID == "" {
		return nil
	}
	providerKey := "qqbot:" + qqbotAppID
	_, err := tx.ExecContext(ctx, `
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
		eventID, maxAttempts, now, service.RoleAdmin, service.StatusActive, providerKey, qqbotAppID,
	)
	return err
}
