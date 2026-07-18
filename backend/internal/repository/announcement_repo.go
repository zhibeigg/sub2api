package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/announcement"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type announcementRepository struct {
	client *dbent.Client
}

func NewAnnouncementRepository(client *dbent.Client) service.AnnouncementRepository {
	return &announcementRepository{client: client}
}

func (r *announcementRepository) CreateWithEmailJob(ctx context.Context, a *service.Announcement, scheduledAt time.Time) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := r.Create(txCtx, a); err != nil {
		return err
	}
	summary, err := createAnnouncementEmailJob(txCtx, tx.Client(), a, scheduledAt)
	if err != nil {
		return err
	}
	a.EmailNotification = summary
	return tx.Commit()
}

func (r *announcementRepository) Create(ctx context.Context, a *service.Announcement) error {
	client := clientFromContext(ctx, r.client)
	builder := client.Announcement.Create().
		SetTitle(a.Title).
		SetContent(a.Content).
		SetStatus(a.Status).
		SetNotifyMode(a.NotifyMode).
		SetTargeting(a.Targeting)

	if a.StartsAt != nil {
		builder.SetStartsAt(*a.StartsAt)
	}
	if a.EndsAt != nil {
		builder.SetEndsAt(*a.EndsAt)
	}
	if a.CreatedBy != nil {
		builder.SetCreatedBy(*a.CreatedBy)
	}
	if a.UpdatedBy != nil {
		builder.SetUpdatedBy(*a.UpdatedBy)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	applyAnnouncementEntityToService(a, created)
	return nil
}

func (r *announcementRepository) GetByID(ctx context.Context, id int64) (*service.Announcement, error) {
	m, err := r.client.Announcement.Query().
		Where(announcement.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAnnouncementNotFound, nil)
	}
	out := announcementEntityToService(m)
	if summary, summaryErr := queryAnnouncementEmailSummary(ctx, r.client, id); summaryErr == nil {
		out.EmailNotification = summary
	}
	return out, nil
}

func (r *announcementRepository) UpdateWithEmailJob(ctx context.Context, a *service.Announcement, scheduledAt time.Time) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := r.Update(txCtx, a); err != nil {
		return err
	}
	summary, err := createAnnouncementEmailJob(txCtx, tx.Client(), a, scheduledAt)
	if err != nil {
		return err
	}
	a.EmailNotification = summary
	return tx.Commit()
}

func (r *announcementRepository) Update(ctx context.Context, a *service.Announcement) error {
	client := clientFromContext(ctx, r.client)
	builder := client.Announcement.UpdateOneID(a.ID).
		SetTitle(a.Title).
		SetContent(a.Content).
		SetStatus(a.Status).
		SetNotifyMode(a.NotifyMode).
		SetTargeting(a.Targeting)

	if a.StartsAt != nil {
		builder.SetStartsAt(*a.StartsAt)
	} else {
		builder.ClearStartsAt()
	}
	if a.EndsAt != nil {
		builder.SetEndsAt(*a.EndsAt)
	} else {
		builder.ClearEndsAt()
	}
	if a.CreatedBy != nil {
		builder.SetCreatedBy(*a.CreatedBy)
	} else {
		builder.ClearCreatedBy()
	}
	if a.UpdatedBy != nil {
		builder.SetUpdatedBy(*a.UpdatedBy)
	} else {
		builder.ClearUpdatedBy()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAnnouncementNotFound, nil)
	}

	a.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *announcementRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.Announcement.Delete().Where(announcement.IDEQ(id)).Exec(ctx)
	return err
}

func (r *announcementRepository) List(
	ctx context.Context,
	params pagination.PaginationParams,
	filters service.AnnouncementListFilters,
) ([]service.Announcement, *pagination.PaginationResult, error) {
	q := r.client.Announcement.Query()

	if filters.Status != "" {
		q = q.Where(announcement.StatusEQ(filters.Status))
	}
	if filters.Search != "" {
		q = q.Where(
			announcement.Or(
				announcement.TitleContainsFold(filters.Search),
				announcement.ContentContainsFold(filters.Search),
			),
		)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	itemsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range announcementListOrders(params) {
		itemsQuery = itemsQuery.Order(order)
	}

	items, err := itemsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := announcementEntitiesToService(items)
	for i := range out {
		if summary, summaryErr := queryAnnouncementEmailSummary(ctx, r.client, out[i].ID); summaryErr == nil {
			out[i].EmailNotification = summary
		}
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func announcementListOrder(params pagination.PaginationParams) (string, string) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	switch sortBy {
	case "title":
		return announcement.FieldTitle, sortOrder
	case "status":
		return announcement.FieldStatus, sortOrder
	case "notify_mode":
		return announcement.FieldNotifyMode, sortOrder
	case "starts_at":
		return announcement.FieldStartsAt, sortOrder
	case "ends_at":
		return announcement.FieldEndsAt, sortOrder
	case "id":
		return announcement.FieldID, sortOrder
	case "", "created_at":
		return announcement.FieldCreatedAt, sortOrder
	default:
		return announcement.FieldCreatedAt, pagination.SortOrderDesc
	}
}

func announcementListOrders(params pagination.PaginationParams) []func(*entsql.Selector) {
	field, sortOrder := announcementListOrder(params)

	if sortOrder == pagination.SortOrderAsc {
		if field == announcement.FieldID {
			return []func(*entsql.Selector){
				dbent.Asc(field),
			}
		}
		return []func(*entsql.Selector){
			dbent.Asc(field),
			dbent.Asc(announcement.FieldID),
		}
	}

	if field == announcement.FieldID {
		return []func(*entsql.Selector){
			dbent.Desc(field),
		}
	}
	return []func(*entsql.Selector){
		dbent.Desc(field),
		dbent.Desc(announcement.FieldID),
	}
}

func (r *announcementRepository) ListActive(ctx context.Context, now time.Time) ([]service.Announcement, error) {
	q := r.client.Announcement.Query().
		Where(
			announcement.StatusEQ(service.AnnouncementStatusActive),
			announcement.Or(announcement.StartsAtIsNil(), announcement.StartsAtLTE(now)),
			announcement.Or(announcement.EndsAtIsNil(), announcement.EndsAtGT(now)),
		).
		Order(dbent.Desc(announcement.FieldID)).
		Limit(200)

	items, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	return announcementEntitiesToService(items), nil
}

func applyAnnouncementEntityToService(dst *service.Announcement, src *dbent.Announcement) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}

func announcementEntityToService(m *dbent.Announcement) *service.Announcement {
	if m == nil {
		return nil
	}
	return &service.Announcement{
		ID:         m.ID,
		Title:      m.Title,
		Content:    m.Content,
		Status:     m.Status,
		NotifyMode: m.NotifyMode,
		Targeting:  m.Targeting,
		StartsAt:   m.StartsAt,
		EndsAt:     m.EndsAt,
		CreatedBy:  m.CreatedBy,
		UpdatedBy:  m.UpdatedBy,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func queryAnnouncementEmailSummary(ctx context.Context, client *dbent.Client, announcementID int64) (result *service.AnnouncementEmailNotification, err error) {
	rows, err := client.QueryContext(ctx, `SELECT id, announcement_id, status, scheduled_at,
		recipient_count, pending_count, sending_count, sent_count, failed_count, ambiguous_count,
		skipped_count, attempt_count, created_by, last_error_code,
		preparation_cursor_id, recipient_cutoff_id, last_error, created_at, updated_at, started_at, finished_at
		FROM announcement_email_jobs WHERE announcement_id = $1`, announcementID)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	var out service.AnnouncementEmailNotification
	var lastError, lastErrorCode sql.NullString
	var createdBy sql.NullInt64
	var startedAt, finishedAt sql.NullTime
	if err := rows.Scan(&out.JobID, &out.AnnouncementID, &out.Status, &out.ScheduledAt,
		&out.RecipientCount, &out.PendingCount, &out.SendingCount, &out.SentCount, &out.FailedCount, &out.AmbiguousCount,
		&out.SkippedCount, &out.AttemptCount, &createdBy, &lastErrorCode,
		&out.PreparationCursorID, &out.RecipientCutoffID, &lastError, &out.CreatedAt, &out.UpdatedAt, &startedAt, &finishedAt); err != nil {
		return nil, err
	}
	if lastError.Valid {
		out.LastError = &lastError.String
	}
	if lastErrorCode.Valid {
		out.LastErrorCode = &lastErrorCode.String
	}
	if createdBy.Valid {
		out.CreatedBy = &createdBy.Int64
	}
	if startedAt.Valid {
		out.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		out.FinishedAt = &finishedAt.Time
	}
	return &out, nil
}

func createAnnouncementEmailJob(ctx context.Context, client *dbent.Client, a *service.Announcement, scheduledAt time.Time) (*service.AnnouncementEmailNotification, error) {
	if _, err := client.ExecContext(ctx, `
		INSERT INTO announcement_email_jobs (
			announcement_id, status, scheduled_at, announcement_title, announcement_content,
			announcement_starts_at, created_by, created_at, updated_at
		) VALUES ($1, 'pending', $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (announcement_id) DO NOTHING`,
		a.ID, scheduledAt, a.Title, a.Content, a.StartsAt, a.UpdatedBy,
	); err != nil {
		return nil, err
	}
	return queryAnnouncementEmailSummary(ctx, client, a.ID)
}

func announcementEntitiesToService(models []*dbent.Announcement) []service.Announcement {
	out := make([]service.Announcement, 0, len(models))
	for i := range models {
		if s := announcementEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}
