package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type announcementEmailRepository struct{ db *sql.DB }

func NewAnnouncementEmailRepository(db *sql.DB) service.AnnouncementEmailRepository {
	return &announcementEmailRepository{db: db}
}

func (r *announcementEmailRepository) GetByAnnouncementID(ctx context.Context, announcementID int64) (*service.AnnouncementEmailNotification, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, announcement_id, status, scheduled_at, recipient_count,
		pending_count, sending_count, sent_count, failed_count, ambiguous_count, skipped_count, attempt_count,
		created_by, last_error_code, preparation_cursor_id, recipient_cutoff_id, last_error, created_at, updated_at, started_at, finished_at
		FROM announcement_email_jobs WHERE announcement_id=$1`, announcementID)
	out, err := scanAnnouncementEmailSummary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAnnouncementEmailNotFound
	}
	return out, err
}

func (r *announcementEmailRepository) CountEligibleRecipients(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("announcement email repository is not configured")
	}
	var count int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+eligibleAnnouncementEmailRecipientsSQL).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanAnnouncementEmailSummary(row interface{ Scan(...any) error }) (*service.AnnouncementEmailNotification, error) {
	var out service.AnnouncementEmailNotification
	var last, lastCode sql.NullString
	var createdBy sql.NullInt64
	var started, finished sql.NullTime
	if err := row.Scan(&out.JobID, &out.AnnouncementID, &out.Status, &out.ScheduledAt, &out.RecipientCount,
		&out.PendingCount, &out.SendingCount, &out.SentCount, &out.FailedCount, &out.AmbiguousCount,
		&out.SkippedCount, &out.AttemptCount, &createdBy, &lastCode, &out.PreparationCursorID, &out.RecipientCutoffID, &last, &out.CreatedAt, &out.UpdatedAt, &started, &finished); err != nil {
		return nil, err
	}
	if last.Valid {
		out.LastError = &last.String
	}
	if lastCode.Valid {
		out.LastErrorCode = &lastCode.String
	}
	if createdBy.Valid {
		out.CreatedBy = &createdBy.Int64
	}
	if started.Valid {
		out.StartedAt = &started.Time
	}
	if finished.Valid {
		out.FinishedAt = &finished.Time
	}
	return &out, nil
}

func (r *announcementEmailRepository) ClaimJob(ctx context.Context, owner string, now time.Time, lease time.Duration) (*service.AnnouncementEmailJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE announcement_email_jobs j SET status='cancelled',finished_at=$1,lease_owner=NULL,lease_expires_at=NULL,updated_at=$1
		WHERE j.scheduled_at <= $1 AND j.status IN ('pending','preparing','sending')
		AND NOT EXISTS (SELECT 1 FROM announcements a WHERE a.id=j.announcement_id AND a.status='active' AND (a.ends_at IS NULL OR a.ends_at>$1))`, now); err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, `SELECT j.id FROM announcement_email_jobs j
		JOIN announcements a ON a.id=j.announcement_id
		WHERE j.scheduled_at <= $1 AND j.status IN ('pending','preparing','sending')
		AND a.status='active' AND (a.starts_at IS NULL OR a.starts_at <= $1) AND (a.ends_at IS NULL OR a.ends_at > $1)
		AND (j.lease_expires_at IS NULL OR j.lease_expires_at < $1 OR j.lease_owner=$2)
		ORDER BY j.scheduled_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1`, now, owner)
	var id int64
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	leaseUntil := now.Add(lease)
	jobRow := tx.QueryRowContext(ctx, `UPDATE announcement_email_jobs SET
		status=CASE WHEN status='pending' THEN 'preparing' ELSE status END,
		lease_owner=$2, lease_expires_at=$3::timestamptz, attempt_count=attempt_count+1, started_at=COALESCE(started_at,$1), updated_at=$1
		WHERE id=$4 RETURNING id, announcement_id, status, scheduled_at, recipient_count, pending_count,
		sending_count, sent_count, failed_count, ambiguous_count, skipped_count, attempt_count, created_by, last_error_code, preparation_cursor_id, recipient_cutoff_id,
		last_error, created_at, updated_at, started_at, finished_at, announcement_title, announcement_content, announcement_starts_at`, now, owner, leaseUntil, id)
	job, err := scanAnnouncementEmailJob(jobRow)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

func scanAnnouncementEmailJob(row interface{ Scan(...any) error }) (*service.AnnouncementEmailJob, error) {
	var out service.AnnouncementEmailJob
	var last, lastCode sql.NullString
	var createdBy sql.NullInt64
	var started, finished, annStarts sql.NullTime
	if err := row.Scan(&out.JobID, &out.AnnouncementID, &out.Status, &out.ScheduledAt, &out.RecipientCount,
		&out.PendingCount, &out.SendingCount, &out.SentCount, &out.FailedCount, &out.AmbiguousCount,
		&out.SkippedCount, &out.AttemptCount, &createdBy, &lastCode, &out.PreparationCursorID, &out.RecipientCutoffID, &last, &out.CreatedAt, &out.UpdatedAt,
		&started, &finished, &out.AnnouncementTitle, &out.AnnouncementContent, &annStarts); err != nil {
		return nil, err
	}
	if last.Valid {
		out.LastError = &last.String
	}
	if lastCode.Valid {
		out.LastErrorCode = &lastCode.String
	}
	if createdBy.Valid {
		out.CreatedBy = &createdBy.Int64
	}
	if started.Valid {
		out.StartedAt = &started.Time
	}
	if finished.Valid {
		out.FinishedAt = &finished.Time
	}
	if annStarts.Valid {
		out.AnnouncementStartsAt = &annStarts.Time
	}
	return &out, nil
}

const eligibleAnnouncementEmailRecipientsSQL = `FROM users u WHERE u.status='active' AND u.deleted_at IS NULL
	AND BTRIM(u.email) <> '' AND BTRIM(u.email) ~* '^[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}$'
	AND LOWER(BTRIM(u.email)) NOT LIKE '%@linuxdo-connect.invalid'
	AND LOWER(BTRIM(u.email)) NOT LIKE '%@oidc-connect.invalid'
	AND LOWER(BTRIM(u.email)) NOT LIKE '%@wechat-connect.invalid'
	AND LOWER(BTRIM(u.email)) NOT LIKE '%@dingtalk-connect.invalid'
	AND EXISTS (SELECT 1 FROM auth_identities ai WHERE ai.user_id=u.id AND ai.provider_type='email'
		AND ai.provider_key='email' AND ai.verified_at IS NOT NULL
		AND LOWER(BTRIM(ai.provider_subject))=LOWER(BTRIM(u.email)))`

func (r *announcementEmailRepository) PrepareRecipients(ctx context.Context, job *service.AnnouncementEmailJob, batchSize, maxAttempts int) (done bool, err error) {
	if job == nil {
		return false, errors.New("nil announcement email job")
	}
	if batchSize < 1 {
		batchSize = 200
	}
	if maxAttempts < 1 {
		maxAttempts = 5
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	cutoff := job.RecipientCutoffID
	if cutoff == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(u.id),0) `+eligibleAnnouncementEmailRecipientsSQL).Scan(&cutoff); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE announcement_email_jobs SET recipient_cutoff_id=$1,updated_at=NOW() WHERE id=$2`, cutoff, job.JobID); err != nil {
			return false, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT u.id,BTRIM(u.email),COALESCE(NULLIF(BTRIM(u.username),''),split_part(BTRIM(u.email),'@',1)) `+eligibleAnnouncementEmailRecipientsSQL+` AND u.id>$1 AND u.id<=$2 ORDER BY u.id LIMIT $3`, job.PreparationCursorID, cutoff, batchSize)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	last := job.PreparationCursorID
	recipients := make([]service.AnnouncementEmailRecipient, 0, batchSize)
	for rows.Next() {
		var rec service.AnnouncementEmailRecipient
		if err = rows.Scan(&rec.UserID, &rec.Email, &rec.Username); err != nil {
			return false, err
		}
		last = rec.UserID
		recipients = append(recipients, rec)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if err = rows.Close(); err != nil {
		return false, err
	}
	inserted := 0
	for _, rec := range recipients {
		res, e := tx.ExecContext(ctx, `INSERT INTO announcement_email_deliveries(job_id,user_id,recipient_email,recipient_name,status,max_attempts,next_attempt_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,'pending',$5,NOW(),NOW(),NOW()) ON CONFLICT(job_id,user_id) DO NOTHING`, job.JobID, rec.UserID, rec.Email, rec.Username, maxAttempts)
		if e != nil {
			return false, e
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	done = last >= cutoff || inserted < batchSize
	status := "preparing"
	if done {
		status = "sending"
	}
	_, err = tx.ExecContext(ctx, `UPDATE announcement_email_jobs SET preparation_cursor_id=$1,status=$2,
		recipient_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=$3),
		pending_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=$3 AND status='pending'),updated_at=NOW() WHERE id=$3`, last, status, job.JobID)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	job.PreparationCursorID = last
	job.RecipientCutoffID = cutoff
	job.Status = status
	return done, nil
}

func (r *announcementEmailRepository) ClaimDeliveries(ctx context.Context, jobID int64, owner string, now time.Time, lease time.Duration, limit int) (deliveries []service.AnnouncementEmailDelivery, err error) {
	if limit < 1 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `WITH candidates AS (SELECT id FROM announcement_email_deliveries WHERE job_id=$1
		AND ((status='pending' AND next_attempt_at<=$2) OR (status='sending' AND lease_expires_at<$2))
		AND attempt_count<max_attempts ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT $3)
		UPDATE announcement_email_deliveries d SET status='sending',lease_owner=$4,lease_expires_at=$5::timestamptz,
		attempt_count=d.attempt_count+1,updated_at=$2 FROM candidates c WHERE d.id=c.id
		RETURNING d.id,d.job_id,d.user_id,d.recipient_email,d.recipient_name,d.locale,d.attempt_count,d.max_attempts`, jobID, now, limit, owner, now.Add(lease))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	out := make([]service.AnnouncementEmailDelivery, 0, limit)
	for rows.Next() {
		var d service.AnnouncementEmailDelivery
		if err = rows.Scan(&d.ID, &d.JobID, &d.UserID, &d.RecipientEmail, &d.RecipientName, &d.Locale, &d.AttemptCount, &d.MaxAttempts); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *announcementEmailRepository) MarkDeliverySent(ctx context.Context, id int64, owner string, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE announcement_email_deliveries SET status='sent',sent_at=$1,lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,last_error_class=NULL,updated_at=$1 WHERE id=$2 AND lease_owner=$3 AND status='sending'`, sentAt, id, owner)
	return err
}
func (r *announcementEmailRepository) MarkDeliveryFailed(ctx context.Context, id int64, owner, class, message string, next *time.Time) error {
	status := "failed"
	if class == "ambiguous" {
		status = "ambiguous"
	}
	if next != nil {
		status = "pending"
	}
	_, err := r.db.ExecContext(ctx, `UPDATE announcement_email_deliveries SET status=$1,next_attempt_at=COALESCE($2::timestamptz,next_attempt_at),lease_owner=NULL,lease_expires_at=NULL,last_error_class=$3,last_error=$4,updated_at=NOW() WHERE id=$5 AND lease_owner=$6 AND status='sending'`, status, next, class, message, id, owner)
	return err
}
func (r *announcementEmailRepository) RefreshJob(ctx context.Context, jobID int64, owner string, now time.Time, lease time.Duration) (*service.AnnouncementEmailJob, error) {
	row := r.db.QueryRowContext(ctx, `UPDATE announcement_email_jobs j SET
		recipient_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=j.id),
		pending_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=j.id AND status='pending'),
		sending_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=j.id AND status='sending'),
		sent_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=j.id AND status='sent'),
		failed_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=j.id AND status='failed'),
		ambiguous_count=(SELECT COUNT(*) FROM announcement_email_deliveries WHERE job_id=j.id AND status='ambiguous'),
		status=CASE WHEN NOT EXISTS(SELECT 1 FROM announcement_email_deliveries WHERE job_id=j.id AND status IN('pending','sending'))
			THEN CASE WHEN EXISTS(SELECT 1 FROM announcement_email_deliveries WHERE job_id=j.id AND status IN('failed','ambiguous')) THEN 'completed_with_failures' ELSE 'completed' END
			ELSE 'sending' END,
		finished_at=CASE WHEN NOT EXISTS(SELECT 1 FROM announcement_email_deliveries WHERE job_id=j.id AND status IN('pending','sending')) THEN COALESCE(finished_at,$1) ELSE NULL END,
		lease_owner=CASE WHEN NOT EXISTS(SELECT 1 FROM announcement_email_deliveries WHERE job_id=j.id AND status IN('pending','sending')) THEN NULL ELSE $2 END,
		lease_expires_at=CASE WHEN NOT EXISTS(SELECT 1 FROM announcement_email_deliveries WHERE job_id=j.id AND status IN('pending','sending')) THEN NULL::timestamptz ELSE $3::timestamptz END,updated_at=$1
		WHERE j.id=$4 AND (j.lease_owner=$2 OR j.lease_expires_at<$1)
		RETURNING id,announcement_id,status,scheduled_at,recipient_count,pending_count,sending_count,sent_count,failed_count,ambiguous_count,skipped_count,attempt_count,created_by,last_error_code,preparation_cursor_id,recipient_cutoff_id,last_error,created_at,updated_at,started_at,finished_at,announcement_title,announcement_content,announcement_starts_at`, now, owner, now.Add(lease), jobID)
	return scanAnnouncementEmailJob(row)
}
func (r *announcementEmailRepository) MarkJobFailed(ctx context.Context, jobID int64, owner, code, message string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE announcement_email_jobs SET status='failed',last_error_code=$1,last_error=$2,finished_at=$3,lease_owner=NULL,lease_expires_at=NULL,updated_at=$3 WHERE id=$4 AND lease_owner=$5`, code, message, now, jobID, owner)
	return err
}

func (r *announcementEmailRepository) Retry(ctx context.Context, announcementID int64, includeAmbiguous bool, now time.Time) (*service.AnnouncementEmailNotification, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var jobID int64
	var jobStatus string
	if err := tx.QueryRowContext(ctx, `SELECT id,status FROM announcement_email_jobs WHERE announcement_id=$1 FOR UPDATE`, announcementID).Scan(&jobID, &jobStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrAnnouncementEmailNotFound
		}
		return nil, err
	}
	if jobStatus != service.AnnouncementEmailJobFailed && jobStatus != service.AnnouncementEmailJobCompletedWithFailures {
		return nil, service.ErrAnnouncementEmailRetryEmpty
	}

	condition := "status='failed'"
	if includeAmbiguous {
		condition = "status IN ('failed','ambiguous')"
	}
	res, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE announcement_email_deliveries SET status='pending',next_attempt_at=$1,attempt_count=0,lease_owner=NULL,lease_expires_at=NULL,updated_at=$1 WHERE job_id=$2 AND %s`, condition), now, jobID)
	if err != nil {
		return nil, err
	}
	retried, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if retried == 0 && jobStatus != service.AnnouncementEmailJobFailed {
		return nil, service.ErrAnnouncementEmailRetryEmpty
	}
	if _, err = tx.ExecContext(ctx, `UPDATE announcement_email_jobs SET status='sending',finished_at=NULL,lease_owner=NULL,lease_expires_at=NULL,last_error_code=NULL,last_error=NULL,updated_at=$1 WHERE id=$2`, now, jobID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByAnnouncementID(ctx, announcementID)
}
