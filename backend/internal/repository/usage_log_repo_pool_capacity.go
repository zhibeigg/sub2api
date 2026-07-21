package repository

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

// GetRecentPoolCapacityCostSummary returns the newest successfully applied
// billing samples before the caller-provided current request. The current
// request is supplied in-memory by the service because usage logs are batched.
func (r *usageLogRepository) GetRecentPoolCapacityCostSummary(
	ctx context.Context,
	groupID int64,
	excludeRequestID string,
	excludeAPIKeyID int64,
	limit int,
) (*service.PoolCapacityCostSummary, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("usage log repository is not configured")
	}
	if groupID <= 0 || limit <= 0 {
		return &service.PoolCapacityCostSummary{}, nil
	}
	const query = `
		SELECT COUNT(*),
		       COALESCE(SUM(sample.account_cost), 0)::text,
		       COALESCE(SUM(sample.actual_cost), 0)::text
		FROM (
			SELECT
				COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1) AS account_cost,
				ul.actual_cost
			FROM usage_logs ul
			WHERE ul.group_id = $1
			  AND ul.actual_cost > 0
			  AND COALESCE(ul.request_type, 0) <> $4
			  AND COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1) > 0
			  AND ($2 = '' OR ul.request_id <> $2 OR ul.api_key_id <> $3)
			  AND (
				EXISTS (
					SELECT 1 FROM usage_billing_dedup ubd
					WHERE ubd.request_id = ul.request_id AND ubd.api_key_id = ul.api_key_id
				)
				OR EXISTS (
					SELECT 1 FROM usage_billing_dedup_archive uba
					WHERE uba.request_id = ul.request_id AND uba.api_key_id = ul.api_key_id
				)
			  )
			ORDER BY ul.created_at DESC, ul.id DESC
			LIMIT $5
		) sample`
	rows, err := r.sql.QueryContext(ctx, query, groupID, excludeRequestID, excludeAPIKeyID, int16(service.RequestTypeCyberBlocked), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := &service.PoolCapacityCostSummary{}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	var accountCostSum, actualCostSum string
	if err := rows.Scan(&out.Count, &accountCostSum, &actualCostSum); err != nil {
		return nil, err
	}
	out.AccountCostSum, err = decimal.NewFromString(accountCostSum)
	if err != nil {
		return nil, err
	}
	out.ActualCostSum, err = decimal.NewFromString(actualCostSum)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
