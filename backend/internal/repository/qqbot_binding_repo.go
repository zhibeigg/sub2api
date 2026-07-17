package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type qqBotBindingRepository struct {
	db *sql.DB
}

func NewQQBotBindingRepository(db *sql.DB) service.QQBotBindingRepository {
	return &qqBotBindingRepository{db: db}
}

type qqBotRowScanner interface {
	Scan(dest ...any) error
}

const (
	qqBotUpdateEmailDeliveryStatusSQL = `
UPDATE qqbot_binding_challenges
SET email_status = $1::varchar,
    status = CASE WHEN $1::varchar = 'failed' AND status = 'pending' THEN 'failed' ELSE status END,
    failure_code = CASE WHEN $1::varchar = 'failed' AND $2::varchar <> '' THEN $2::varchar ELSE failure_code END,
    updated_at = NOW()
WHERE id = $3::bigint`

	qqBotUpdateNotificationDeliveryStatusSQL = `
UPDATE qqbot_binding_challenges
SET notification_status = $1::varchar,
    updated_at = NOW()
WHERE id = $2::bigint`

	qqBotInsertDeliveryAuditSQL = `
INSERT INTO qqbot_binding_audit_logs (challenge_id, action, status, actor_type, reason, metadata)
VALUES ($1::bigint, $2::varchar, $3::varchar, 'system', $4::text, $5::jsonb)`
)

func (r *qqBotBindingRepository) CreateChallenge(ctx context.Context, input service.QQBotChallengeCreateInput) (service.QQBotBindingRecord, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.QQBotBindingRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO qqbot_binding_challenges (
    event_id, message_id, challenge_token_hash, bot_app_id, scene, provider_subject,
    source_id, channel_id, display_name, user_id, email_hash, masked_email,
    status, failure_code, email_status, bonus_amount, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (event_id) DO NOTHING
RETURNING id`,
		input.EventID,
		input.MessageID,
		input.TokenHash,
		input.BotAppID,
		input.Scene,
		input.ProviderSubject,
		input.SourceID,
		input.ChannelID,
		input.DisplayName,
		input.UserID,
		input.EmailHash,
		input.MaskedEmail,
		input.Status,
		input.FailureCode,
		input.EmailStatus,
		input.BonusAmount,
		input.ExpiresAt,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		record, _, getErr := r.getChallengeByEvent(ctx, input.EventID)
		return record, false, getErr
	}
	if err != nil {
		return service.QQBotBindingRecord{}, false, err
	}

	metadata, _ := json.Marshal(map[string]any{
		"scene":        input.Scene,
		"source_id":    input.SourceID,
		"channel_id":   input.ChannelID,
		"email_status": input.EmailStatus,
		"failure_code": input.FailureCode,
	})
	if _, err := tx.ExecContext(ctx, `
INSERT INTO qqbot_binding_audit_logs (
    challenge_id, action, status, actor_type, actor_subject, user_id,
    bot_app_id, provider_subject_hash, masked_email, reason, metadata
) VALUES ($1, 'prepare', $2, 'qq_user', $3, $4, $5, $6, $7, $8, $9::jsonb)`,
		id,
		input.Status,
		input.ProviderSubject,
		input.UserID,
		input.BotAppID,
		qqBotFingerprint(input.ProviderSubject),
		input.MaskedEmail,
		input.FailureCode,
		string(metadata),
	); err != nil {
		return service.QQBotBindingRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return service.QQBotBindingRecord{}, false, err
	}
	record, _, err := r.getChallengeByID(ctx, id)
	return record, true, err
}

func (r *qqBotBindingRepository) GetChallengeByToken(ctx context.Context, token string) (service.QQBotBindingRecord, string, error) {
	record, email, err := r.getChallengeByToken(ctx, token)
	if err != nil {
		return service.QQBotBindingRecord{}, "", err
	}
	if record.Status == service.QQBotBindingStatusPending && !record.ExpiresAt.After(time.Now().UTC()) {
		if err := r.expireChallenge(ctx, parseQQBotRecordID(record.ID), time.Now().UTC()); err != nil {
			return service.QQBotBindingRecord{}, "", err
		}
		record.Status = service.QQBotBindingStatusExpired
	}
	return record, email, nil
}

func (r *qqBotBindingRepository) UpdateEmailStatus(ctx context.Context, id int64, status, failureCode string) error {
	return r.updateDeliveryStatus(ctx, id, "email_status", "email", status, failureCode)
}

func (r *qqBotBindingRepository) UpdateNotificationStatus(ctx context.Context, id int64, status, failureCode string) error {
	return r.updateDeliveryStatus(ctx, id, "notification_status", "notify", status, failureCode)
}

func (r *qqBotBindingRepository) updateDeliveryStatus(ctx context.Context, id int64, column, action, status, failureCode string) error {
	if column != "email_status" && column != "notification_status" {
		return fmt.Errorf("unsupported qqbot delivery status column %q", column)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if column == "email_status" {
		if _, err := tx.ExecContext(ctx, qqBotUpdateEmailDeliveryStatusSQL, status, failureCode, id); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, qqBotUpdateNotificationDeliveryStatusSQL, status, id); err != nil {
			return err
		}
	}
	metadata, _ := json.Marshal(map[string]string{"delivery_status": status, "failure_code": failureCode})
	if _, err := tx.ExecContext(ctx, qqBotInsertDeliveryAuditSQL, id, action, status, failureCode, string(metadata)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *qqBotBindingRepository) CompleteBinding(ctx context.Context, input service.QQBotCompleteRepositoryInput) (service.QQBotCompleteRepositoryResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	record, recipientEmail, err := r.getChallengeByTokenTx(ctx, tx, input.Token, true)
	if err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}
	if record.Status == service.QQBotBindingStatusCompleted {
		return service.QQBotCompleteRepositoryResult{Record: record, Granted: record.BonusAmount > 0, RecipientEmail: recipientEmail}, tx.Commit()
	}
	if record.Status == service.QQBotBindingStatusRevoked {
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotBindingRevoked
	}
	if record.Status == service.QQBotBindingStatusFailed {
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotBindingFailed
	}
	if record.Status == service.QQBotBindingStatusExpired || !record.ExpiresAt.After(input.Now) {
		if err := r.failChallengeTx(ctx, tx, record, service.QQBotBindingStatusExpired, "BINDING_EXPIRED", "expire", input.Now); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotBindingExpired
	}
	if record.UserID == nil || *record.UserID <= 0 {
		if err := r.failChallengeTx(ctx, tx, record, service.QQBotBindingStatusFailed, "ACCOUNT_NOT_AVAILABLE", "complete", input.Now); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotBindingFailed
	}

	identityLock := "qqbot:identity:" + record.BotAppID + ":" + record.ProviderSubject
	userLock := "qqbot:user:" + strconv.FormatInt(*record.UserID, 10)
	for _, key := range []string{identityLock, userLock} {
		var ignored string
		if err := tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))::text`, key).Scan(&ignored); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
	}

	var userStatus string
	var currentBalance float64
	if err := tx.QueryRowContext(ctx, `SELECT email, status, balance FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, *record.UserID).
		Scan(&recipientEmail, &userStatus, &currentBalance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if failErr := r.failChallengeTx(ctx, tx, record, service.QQBotBindingStatusFailed, "ACCOUNT_NOT_AVAILABLE", "complete", input.Now); failErr != nil {
				return service.QQBotCompleteRepositoryResult{}, failErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return service.QQBotCompleteRepositoryResult{}, commitErr
			}
			return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotBindingFailed
		}
		return service.QQBotCompleteRepositoryResult{}, err
	}
	if userStatus != service.StatusActive {
		if err := r.failChallengeTx(ctx, tx, record, service.QQBotBindingStatusFailed, "ACCOUNT_DISABLED", "complete", input.Now); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotBindingFailed
	}

	providerKey := "qqbot:" + record.BotAppID
	identityID, ownerID, err := r.lookupQQBotIdentityTx(ctx, tx, providerKey, record.ProviderSubject)
	if err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}
	if identityID != 0 && ownerID != *record.UserID {
		if err := r.failChallengeTx(ctx, tx, record, service.QQBotBindingStatusFailed, "QQ_IDENTITY_CONFLICT", "complete", input.Now); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotIdentityOwned
	}

	identityMetadata, _ := json.Marshal(map[string]string{
		"scene":              record.Scene,
		"source_id":          record.SourceID,
		"channel_id":         record.ChannelID,
		"display_name":       record.DisplayName,
		"declared_qq_number": input.QQNumber,
	})
	if identityID == 0 {
		if err := tx.QueryRowContext(ctx, `
INSERT INTO auth_identities (user_id, provider_type, provider_key, provider_subject, verified_at, metadata, created_at, updated_at)
VALUES ($1, 'qqbot', $2, $3, $4, $5::jsonb, $4, $4)
RETURNING id`, *record.UserID, providerKey, record.ProviderSubject, input.Now, string(identityMetadata)).Scan(&identityID); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE auth_identities SET verified_at = $1, metadata = $2::jsonb, updated_at = $1 WHERE id = $3`, input.Now, string(identityMetadata), identityID); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
	}

	channelMetadata, _ := json.Marshal(map[string]string{
		"source_id":          record.SourceID,
		"channel_id":         record.ChannelID,
		"declared_qq_number": input.QQNumber,
	})
	var existingChannelIdentity sql.NullInt64
	err = tx.QueryRowContext(ctx, `
SELECT identity_id FROM auth_identity_channels
WHERE provider_type = 'qqbot' AND provider_key = $1 AND channel = $2
  AND channel_app_id = $3 AND channel_subject = $4
FOR UPDATE`, providerKey, record.Scene, record.BotAppID, record.ProviderSubject).Scan(&existingChannelIdentity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return service.QQBotCompleteRepositoryResult{}, err
	}
	if existingChannelIdentity.Valid && existingChannelIdentity.Int64 != identityID {
		return service.QQBotCompleteRepositoryResult{}, service.ErrQQBotIdentityOwned
	}
	if existingChannelIdentity.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE auth_identity_channels SET metadata = $1::jsonb, updated_at = $2 WHERE identity_id = $3 AND provider_type = 'qqbot' AND provider_key = $4 AND channel = $5 AND channel_app_id = $6 AND channel_subject = $7`, string(channelMetadata), input.Now, identityID, providerKey, record.Scene, record.BotAppID, record.ProviderSubject); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO auth_identity_channels (identity_id, provider_type, provider_key, channel, channel_app_id, channel_subject, metadata, created_at, updated_at)
VALUES ($1, 'qqbot', $2, $3, $4, $5, $6::jsonb, $7, $7)`, identityID, providerKey, record.Scene, record.BotAppID, record.ProviderSubject, string(channelMetadata), input.Now); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
	}

	var grantID int64
	grantInserted := true
	err = tx.QueryRowContext(ctx, `
INSERT INTO user_provider_default_grants (user_id, provider_type, grant_reason, granted_at, created_at)
VALUES ($1, 'qqbot', 'first_bind', $2, $2)
ON CONFLICT (user_id, provider_type, grant_reason) DO NOTHING
RETURNING id`, *record.UserID, input.Now).Scan(&grantID)
	if errors.Is(err, sql.ErrNoRows) {
		grantInserted = false
	} else if err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}

	balanceBefore := currentBalance
	balanceAfter := currentBalance
	granted := grantInserted && input.Bonus > 0
	if granted {
		if err := tx.QueryRowContext(ctx, `UPDATE users SET balance = balance + $1, updated_at = $2 WHERE id = $3 RETURNING balance`, input.Bonus, input.Now, *record.UserID).Scan(&balanceAfter); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, notes, created_at, validity_days)
VALUES ($1, 'balance', $2, 'used', $3, $4, $5, $4, 0)`, input.RedeemCode, input.Bonus, *record.UserID, input.Now, "QQBot first-bind bonus; challenge="+record.ID); err != nil {
			return service.QQBotCompleteRepositoryResult{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE qqbot_binding_challenges
SET status = 'completed', declared_qq_number = $1, bonus_amount = $2,
    balance_before = $3, balance_after = $4, failure_code = '',
    completed_at = $5, updated_at = $5
WHERE id = $6`, input.QQNumber, func() float64 {
		if granted {
			return input.Bonus
		}
		return 0
	}(), balanceBefore, balanceAfter, input.Now, parseQQBotRecordID(record.ID)); err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}

	metadata, _ := json.Marshal(map[string]any{
		"declared_qq_number": input.QQNumber,
		"granted":            granted,
		"bonus_amount": func() float64 {
			if granted {
				return input.Bonus
			}
			return 0
		}(),
		"balance_before": balanceBefore,
		"balance_after":  balanceAfter,
	})
	if _, err := tx.ExecContext(ctx, `
INSERT INTO qqbot_binding_audit_logs (
    challenge_id, action, status, actor_type, actor_subject, user_id,
    bot_app_id, provider_subject_hash, masked_email, metadata
) VALUES ($1, 'complete', 'completed', 'qq_user', $2, $3, $4, $5, $6, $7::jsonb)`,
		parseQQBotRecordID(record.ID), record.ProviderSubject, *record.UserID, record.BotAppID,
		qqBotFingerprint(record.ProviderSubject), record.MaskedEmail, string(metadata)); err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return service.QQBotCompleteRepositoryResult{}, err
	}
	bonusAmount := 0.0
	if granted {
		bonusAmount = input.Bonus
	}
	record.Status = service.QQBotBindingStatusCompleted
	record.DeclaredQQ = input.QQNumber
	record.BonusAmount = bonusAmount
	record.BalanceBefore = float64Pointer(balanceBefore)
	record.BalanceAfter = float64Pointer(balanceAfter)
	record.CompletedAt = timePointer(input.Now)
	return service.QQBotCompleteRepositoryResult{
		Record:         record,
		Granted:        granted,
		NewlyCompleted: true,
		RecipientEmail: recipientEmail,
	}, nil
}

func (r *qqBotBindingRepository) ListBindings(ctx context.Context, filter service.QQBotBindingListFilter) (service.QQBotBindingPage, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	conditions := []string{"1=1"}
	args := make([]any, 0, 8)
	addArg := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	if status != "" {
		placeholder := addArg(status)
		if status == service.QQBotBindingStatusExpired {
			conditions = append(conditions, "(q.status = 'expired' OR (q.status = 'pending' AND q.expires_at <= NOW()))")
		} else if status == service.QQBotBindingStatusPending {
			conditions = append(conditions, "q.status = 'pending' AND q.expires_at > NOW()")
		} else {
			conditions = append(conditions, "q.status = "+placeholder)
		}
	}
	if scene := strings.ToLower(strings.TrimSpace(filter.Scene)); scene != "" {
		conditions = append(conditions, "q.scene = "+addArg(scene))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		placeholder := addArg("%" + search + "%")
		conditions = append(conditions, "(q.masked_email ILIKE "+placeholder+" OR q.declared_qq_number ILIKE "+placeholder+" OR q.source_id ILIKE "+placeholder+" OR q.channel_id ILIKE "+placeholder+")")
	}
	if filter.From != nil {
		conditions = append(conditions, "q.created_at >= "+addArg(*filter.From))
	}
	if filter.To != nil {
		conditions = append(conditions, "q.created_at <= "+addArg(*filter.To))
	}
	where := strings.Join(conditions, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM qqbot_binding_challenges q WHERE "+where, args...).Scan(&total); err != nil {
		return service.QQBotBindingPage{}, err
	}
	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	limitArg := "$" + strconv.Itoa(len(queryArgs)-1)
	offsetArg := "$" + strconv.Itoa(len(queryArgs))
	query := `
SELECT q.id,
       CASE WHEN q.status = 'pending' AND q.expires_at <= NOW() THEN 'expired' ELSE q.status END,
       q.masked_email, q.scene, q.source_id, q.channel_id, q.declared_qq_number,
       q.bonus_amount, q.balance_before, q.balance_after, q.failure_code,
       q.email_status, q.notification_status, q.created_at, q.expires_at,
       q.completed_at, q.revoked_at, q.event_id, q.message_id, q.bot_app_id,
       q.provider_subject, q.display_name, q.user_id, COALESCE(u.email, '')
FROM qqbot_binding_challenges q
LEFT JOIN users u ON u.id = q.user_id
WHERE ` + where + `
ORDER BY q.created_at DESC, q.id DESC
LIMIT ` + limitArg + ` OFFSET ` + offsetArg
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return service.QQBotBindingPage{}, err
	}
	defer rows.Close()
	items := make([]service.QQBotBindingRecord, 0, pageSize)
	for rows.Next() {
		record, _, err := scanQQBotRecord(rows)
		if err != nil {
			return service.QQBotBindingPage{}, err
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return service.QQBotBindingPage{}, err
	}
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	return service.QQBotBindingPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *qqBotBindingRepository) Stats(ctx context.Context, now time.Time) (service.QQBotStats, error) {
	startOfDay := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	var stats service.QQBotStats
	err := r.db.QueryRowContext(ctx, `
SELECT
    COUNT(*) FILTER (WHERE created_at >= $1),
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'completed'),
    COUNT(*) FILTER (WHERE status = 'pending' AND expires_at > $2),
    COUNT(*) FILTER (WHERE status = 'expired' OR (status = 'pending' AND expires_at <= $2)),
    COUNT(*) FILTER (WHERE status = 'failed'),
    COUNT(*) FILTER (WHERE status = 'revoked'),
    COALESCE(SUM(bonus_amount) FILTER (WHERE status = 'completed'), 0),
    COALESCE(SUM(bonus_amount) FILTER (WHERE status = 'completed' AND completed_at >= $1), 0)
FROM qqbot_binding_challenges`, startOfDay, now).Scan(
		&stats.TodayRequests,
		&stats.TotalRequests,
		&stats.Completed,
		&stats.Pending,
		&stats.Expired,
		&stats.Failed,
		&stats.Revoked,
		&stats.GrantedTotal,
		&stats.TodayGrantedTotal,
	)
	if err != nil {
		return service.QQBotStats{}, err
	}
	terminal := stats.Completed + stats.Expired + stats.Failed + stats.Revoked
	if terminal > 0 {
		stats.CompletionRate = float64(stats.Completed) / float64(terminal)
	}
	return stats, nil
}

func (r *qqBotBindingRepository) RecordSettingsAudit(ctx context.Context, adminSubject string, metadata map[string]any) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO qqbot_binding_audit_logs (action, status, actor_type, actor_subject, metadata)
VALUES ('settings', 'completed', 'admin', $1, $2::jsonb)`, strings.TrimSpace(adminSubject), string(payload))
	return err
}

func (r *qqBotBindingRepository) Unbind(ctx context.Context, id int64, reason, adminSubject string, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	record, _, err := r.getChallengeByIDTx(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if record.Status == service.QQBotBindingStatusRevoked {
		return tx.Commit()
	}
	if record.Status != service.QQBotBindingStatusCompleted || record.UserID == nil {
		return service.ErrQQBotBindingFailed
	}
	providerKey := "qqbot:" + record.BotAppID
	if _, err := tx.ExecContext(ctx, `
DELETE FROM auth_identities
WHERE user_id = $1 AND provider_type = 'qqbot' AND provider_key = $2 AND provider_subject = $3`, *record.UserID, providerKey, record.ProviderSubject); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE qqbot_binding_challenges
SET status = 'revoked', revoked_at = $1, updated_at = $1
WHERE user_id = $2 AND bot_app_id = $3 AND provider_subject = $4 AND status = 'completed'`, now, *record.UserID, record.BotAppID, record.ProviderSubject); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{"reason": reason, "admin_subject": adminSubject})
	if _, err := tx.ExecContext(ctx, `
INSERT INTO qqbot_binding_audit_logs (
    challenge_id, action, status, actor_type, actor_subject, user_id,
    bot_app_id, provider_subject_hash, masked_email, reason, metadata
) VALUES ($1, 'unbind', 'revoked', 'admin', $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		id, adminSubject, *record.UserID, record.BotAppID, qqBotFingerprint(record.ProviderSubject), record.MaskedEmail, reason, string(metadata)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *qqBotBindingRepository) lookupQQBotIdentityTx(ctx context.Context, tx *sql.Tx, providerKey, providerSubject string) (identityID, ownerID int64, err error) {
	err = tx.QueryRowContext(ctx, `
SELECT id, user_id FROM auth_identities
WHERE provider_type = 'qqbot' AND provider_key = $1 AND provider_subject = $2
FOR UPDATE`, providerKey, providerSubject).Scan(&identityID, &ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return identityID, ownerID, err
}

func (r *qqBotBindingRepository) failChallengeTx(ctx context.Context, tx *sql.Tx, record service.QQBotBindingRecord, status, code, action string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE qqbot_binding_challenges
SET status = $1, failure_code = $2, updated_at = $3
WHERE id = $4`, status, code, now, parseQQBotRecordID(record.ID)); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{"failure_code": code})
	_, err := tx.ExecContext(ctx, `
INSERT INTO qqbot_binding_audit_logs (
    challenge_id, action, status, actor_type, actor_subject, user_id,
    bot_app_id, provider_subject_hash, masked_email, reason, metadata
) VALUES ($1, $2, $3, 'system', $4, $5, $6, $7, $8, $9, $10::jsonb)`,
		parseQQBotRecordID(record.ID), action, status, record.ProviderSubject, record.UserID,
		record.BotAppID, qqBotFingerprint(record.ProviderSubject), record.MaskedEmail, code, string(metadata))
	return err
}

func (r *qqBotBindingRepository) expireChallenge(ctx context.Context, id int64, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	record, _, err := r.getChallengeByIDTx(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if record.Status != service.QQBotBindingStatusPending || record.ExpiresAt.After(now) {
		return tx.Commit()
	}
	if err := r.failChallengeTx(ctx, tx, record, service.QQBotBindingStatusExpired, "BINDING_EXPIRED", "expire", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *qqBotBindingRepository) getChallengeByEvent(ctx context.Context, eventID string) (service.QQBotBindingRecord, string, error) {
	return r.getOne(ctx, `
SELECT q.id, q.status, q.masked_email, q.scene, q.source_id, q.channel_id, q.declared_qq_number,
       q.bonus_amount, q.balance_before, q.balance_after, q.failure_code,
       q.email_status, q.notification_status, q.created_at, q.expires_at,
       q.completed_at, q.revoked_at, q.event_id, q.message_id, q.bot_app_id,
       q.provider_subject, q.display_name, q.user_id, COALESCE(u.email, '')
FROM qqbot_binding_challenges q LEFT JOIN users u ON u.id = q.user_id
WHERE q.event_id = $1`, eventID)
}

func (r *qqBotBindingRepository) getChallengeByToken(ctx context.Context, token string) (service.QQBotBindingRecord, string, error) {
	return r.getOne(ctx, `
SELECT q.id, q.status, q.masked_email, q.scene, q.source_id, q.channel_id, q.declared_qq_number,
       q.bonus_amount, q.balance_before, q.balance_after, q.failure_code,
       q.email_status, q.notification_status, q.created_at, q.expires_at,
       q.completed_at, q.revoked_at, q.event_id, q.message_id, q.bot_app_id,
       q.provider_subject, q.display_name, q.user_id, COALESCE(u.email, '')
FROM qqbot_binding_challenges q LEFT JOIN users u ON u.id = q.user_id
WHERE q.challenge_token_hash = $1`, qqBotTokenHash(token))
}

func (r *qqBotBindingRepository) getChallengeByID(ctx context.Context, id int64) (service.QQBotBindingRecord, string, error) {
	return r.getOne(ctx, `
SELECT q.id, q.status, q.masked_email, q.scene, q.source_id, q.channel_id, q.declared_qq_number,
       q.bonus_amount, q.balance_before, q.balance_after, q.failure_code,
       q.email_status, q.notification_status, q.created_at, q.expires_at,
       q.completed_at, q.revoked_at, q.event_id, q.message_id, q.bot_app_id,
       q.provider_subject, q.display_name, q.user_id, COALESCE(u.email, '')
FROM qqbot_binding_challenges q LEFT JOIN users u ON u.id = q.user_id
WHERE q.id = $1`, id)
}

func (r *qqBotBindingRepository) getOne(ctx context.Context, query string, arg any) (service.QQBotBindingRecord, string, error) {
	record, email, err := scanQQBotRecord(r.db.QueryRowContext(ctx, query, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return service.QQBotBindingRecord{}, "", service.ErrQQBotBindingNotFound
	}
	return record, email, err
}

func (r *qqBotBindingRepository) getChallengeByTokenTx(ctx context.Context, tx *sql.Tx, token string, lock bool) (service.QQBotBindingRecord, string, error) {
	query := `
SELECT q.id, q.status, q.masked_email, q.scene, q.source_id, q.channel_id, q.declared_qq_number,
       q.bonus_amount, q.balance_before, q.balance_after, q.failure_code,
       q.email_status, q.notification_status, q.created_at, q.expires_at,
       q.completed_at, q.revoked_at, q.event_id, q.message_id, q.bot_app_id,
       q.provider_subject, q.display_name, q.user_id, COALESCE(u.email, '')
FROM qqbot_binding_challenges q LEFT JOIN users u ON u.id = q.user_id
WHERE q.challenge_token_hash = $1`
	if lock {
		query += " FOR UPDATE OF q"
	}
	record, email, err := scanQQBotRecord(tx.QueryRowContext(ctx, query, qqBotTokenHash(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return service.QQBotBindingRecord{}, "", service.ErrQQBotBindingNotFound
	}
	return record, email, err
}

func (r *qqBotBindingRepository) getChallengeByIDTx(ctx context.Context, tx *sql.Tx, id int64, lock bool) (service.QQBotBindingRecord, string, error) {
	query := `
SELECT q.id, q.status, q.masked_email, q.scene, q.source_id, q.channel_id, q.declared_qq_number,
       q.bonus_amount, q.balance_before, q.balance_after, q.failure_code,
       q.email_status, q.notification_status, q.created_at, q.expires_at,
       q.completed_at, q.revoked_at, q.event_id, q.message_id, q.bot_app_id,
       q.provider_subject, q.display_name, q.user_id, COALESCE(u.email, '')
FROM qqbot_binding_challenges q LEFT JOIN users u ON u.id = q.user_id
WHERE q.id = $1`
	if lock {
		query += " FOR UPDATE OF q"
	}
	record, email, err := scanQQBotRecord(tx.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return service.QQBotBindingRecord{}, "", service.ErrQQBotBindingNotFound
	}
	return record, email, err
}

func scanQQBotRecord(scanner qqBotRowScanner) (service.QQBotBindingRecord, string, error) {
	var (
		id             int64
		balanceBefore  sql.NullFloat64
		balanceAfter   sql.NullFloat64
		completedAt    sql.NullTime
		revokedAt      sql.NullTime
		userID         sql.NullInt64
		recipientEmail string
		record         service.QQBotBindingRecord
	)
	if err := scanner.Scan(
		&id,
		&record.Status,
		&record.MaskedEmail,
		&record.Scene,
		&record.SourceID,
		&record.ChannelID,
		&record.DeclaredQQ,
		&record.BonusAmount,
		&balanceBefore,
		&balanceAfter,
		&record.FailureCode,
		&record.EmailStatus,
		&record.NotificationStatus,
		&record.CreatedAt,
		&record.ExpiresAt,
		&completedAt,
		&revokedAt,
		&record.EventID,
		&record.MessageID,
		&record.BotAppID,
		&record.ProviderSubject,
		&record.DisplayName,
		&userID,
		&recipientEmail,
	); err != nil {
		return service.QQBotBindingRecord{}, "", err
	}
	record.ID = strconv.FormatInt(id, 10)
	record.OpenIDFingerprint = qqBotFingerprint(record.ProviderSubject)
	if balanceBefore.Valid {
		record.BalanceBefore = float64Pointer(balanceBefore.Float64)
	}
	if balanceAfter.Valid {
		record.BalanceAfter = float64Pointer(balanceAfter.Float64)
	}
	if completedAt.Valid {
		record.CompletedAt = timePointer(completedAt.Time)
	}
	if revokedAt.Valid {
		record.RevokedAt = timePointer(revokedAt.Time)
	}
	if userID.Valid {
		record.UserID = int64Pointer(userID.Int64)
	}
	return record, recipientEmail, nil
}

func qqBotFingerprint(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:8])
}

func qqBotTokenHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func parseQQBotRecordID(value string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return id
}

func float64Pointer(value float64) *float64  { return &value }
func int64Pointer(value int64) *int64        { return &value }
func timePointer(value time.Time) *time.Time { return &value }
