package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/shopspring/decimal"
)

const (
	AdminOrderTimeFieldCreatedAt = "created_at"
	AdminOrderTimeFieldPaidAt    = "paid_at"
)

type AdminOrderPaidAmount struct {
	Currency   string `json:"currency"`
	OrderCount int64  `json:"order_count"`
	Amount     string `json:"amount"`
}

type AdminOrderAmountSummary struct {
	FilteredOrderCount   int64                  `json:"filtered_order_count"`
	PaidOrderCount       int64                  `json:"paid_order_count"`
	PaidAmounts          []AdminOrderPaidAmount `json:"paid_amounts"`
	SuccessfulOrderCount int64                  `json:"successful_order_count"`
	RechargedUserCount   int64                  `json:"recharged_user_count"`
	GrossRechargeAmount  string                 `json:"gross_recharge_amount"`
	RefundedAmount       string                 `json:"refunded_amount"`
	NetRechargeAmount    string                 `json:"net_recharge_amount"`
}

type AdminOrderAttributionGroup struct {
	PromoAttribution     string `json:"promo_attribution"`
	PromoCodeID          *int64 `json:"promo_code_id,omitempty"`
	PromoCode            string `json:"promo_code,omitempty"`
	OrderUserCount       int64  `json:"order_user_count"`
	RechargedUserCount   int64  `json:"recharged_user_count"`
	SuccessfulOrderCount int64  `json:"successful_order_count"`
	GrossRechargeAmount  string `json:"gross_recharge_amount"`
	RefundedAmount       string `json:"refunded_amount"`
	NetRechargeAmount    string `json:"net_recharge_amount"`
}

type AdminOrderSummary struct {
	Totals     AdminOrderAmountSummary      `json:"totals"`
	Groups     []AdminOrderAttributionGroup `json:"groups"`
	GroupPage  int                          `json:"group_page"`
	GroupSize  int                          `json:"group_page_size"`
	GroupTotal int64                        `json:"group_total"`
}

type AdminOrderPromoCodeOption struct {
	PromoAttribution string `json:"promo_attribution"`
	PromoCodeID      *int64 `json:"promo_code_id,omitempty"`
	PromoCode        string `json:"promo_code,omitempty"`
	Status           string `json:"status,omitempty"`
	Historical       bool   `json:"historical,omitempty"`
}

func normalizeAdminOrderParams(p OrderListParams) OrderListParams {
	p.Keyword = strings.TrimSpace(p.Keyword)
	p.Status = strings.TrimSpace(p.Status)
	p.OrderType = strings.TrimSpace(p.OrderType)
	p.PaymentType = strings.TrimSpace(p.PaymentType)
	p.PromoAttribution = strings.TrimSpace(p.PromoAttribution)
	if p.TimeField == "" {
		p.TimeField = AdminOrderTimeFieldCreatedAt
	}
	return p
}

func (s *PaymentService) adminOrderQuery(p OrderListParams) *dbent.PaymentOrderQuery {
	p = normalizeAdminOrderParams(p)
	q := s.entClient.PaymentOrder.Query()
	if p.UserID > 0 {
		q = q.Where(paymentorder.UserIDEQ(p.UserID))
	}
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	if p.Keyword != "" {
		q = q.Where(paymentorder.Or(
			paymentorder.OutTradeNoContainsFold(p.Keyword),
			paymentorder.UserEmailContainsFold(p.Keyword),
			paymentorder.UserNameContainsFold(p.Keyword),
			paymentorder.SignupPromoCodeContainsFold(p.Keyword),
		))
	}
	if p.PromoCodeID != nil {
		q = q.Where(
			paymentorder.SignupPromoAttributionEQ(PromoAttributionAttributed),
			paymentorder.SignupPromoCodeIDEQ(*p.PromoCodeID),
		)
	} else {
		switch p.PromoAttribution {
		case PromoAttributionAttributed, PromoAttributionNone, PromoAttributionLegacyUnknown:
			q = q.Where(paymentorder.SignupPromoAttributionEQ(p.PromoAttribution))
		}
	}
	if p.TimeField == AdminOrderTimeFieldPaidAt {
		if p.StartTime != nil {
			q = q.Where(paymentorder.PaidAtGTE(*p.StartTime))
		}
		if p.EndTime != nil {
			q = q.Where(paymentorder.PaidAtLT(*p.EndTime))
		}
	} else {
		if p.StartTime != nil {
			q = q.Where(paymentorder.CreatedAtGTE(*p.StartTime))
		}
		if p.EndTime != nil {
			q = q.Where(paymentorder.CreatedAtLT(*p.EndTime))
		}
	}
	return q
}

func (s *PaymentService) AdminCountOrders(ctx context.Context, p OrderListParams) (int, error) {
	total, err := s.adminOrderQuery(p).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count admin orders: %w", err)
	}
	return total, nil
}

func (s *PaymentService) AdminIterateOrders(ctx context.Context, p OrderListParams, batchSize int, visit func([]*dbent.PaymentOrder) error) error {
	if batchSize <= 0 || batchSize > 5000 {
		batchSize = 1000
	}
	var beforeID int64
	for {
		q := s.adminOrderQuery(p)
		if beforeID > 0 {
			q = q.Where(paymentorder.IDLT(beforeID))
		}
		orders, err := q.Order(dbent.Desc(paymentorder.FieldID)).Limit(batchSize).All(ctx)
		if err != nil {
			return fmt.Errorf("iterate admin orders: %w", err)
		}
		if len(orders) == 0 {
			return nil
		}
		if err := visit(orders); err != nil {
			return err
		}
		beforeID = orders[len(orders)-1].ID
	}
}

func (s *PaymentService) GetAdminOrderSummary(ctx context.Context, p OrderListParams, groupPage, groupSize int) (*AdminOrderSummary, error) {
	p = normalizeAdminOrderParams(p)
	if groupPage <= 0 {
		groupPage = 1
	}
	if groupSize <= 0 {
		groupSize = 50
	}
	if groupSize > 100000 {
		groupSize = 100000
	}
	where, args := adminOrderSQLWhere(p)

	result := &AdminOrderSummary{GroupPage: groupPage, GroupSize: groupSize, Groups: []AdminOrderAttributionGroup{}}
	var gross, refunded decimal.Decimal
	totalsSQL := `
SELECT COUNT(*)::bigint,
       COUNT(*) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL)::bigint,
       COUNT(DISTINCT user_id) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL)::bigint,
       COALESCE(SUM(amount) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL), 0),
       COALESCE(SUM(refund_amount) FILTER (
           WHERE order_type = 'balance'
             AND completed_at IS NOT NULL
             AND refund_at IS NOT NULL
             AND status IN ('PARTIALLY_REFUNDED', 'REFUNDED')
       ), 0)
FROM payment_orders` + where
	rows, err := s.entClient.QueryContext(ctx, totalsSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin order totals: %w", err)
	}
	if rows.Next() {
		err = rows.Scan(
			&result.Totals.FilteredOrderCount,
			&result.Totals.SuccessfulOrderCount,
			&result.Totals.RechargedUserCount,
			&gross,
			&refunded,
		)
	}
	closeErr := rows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan admin order totals: %w", err)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	result.Totals.GrossRechargeAmount = gross.StringFixed(2)
	result.Totals.RefundedAmount = refunded.StringFixed(2)
	result.Totals.NetRechargeAmount = nonNegativeAdminOrderAmount(gross.Sub(refunded)).StringFixed(2)

	paidAmounts, paidOrderCount, err := s.listAdminOrderPaidAmounts(ctx, p)
	if err != nil {
		return nil, err
	}
	result.Totals.PaidOrderCount = paidOrderCount
	result.Totals.PaidAmounts = paidAmounts

	countSQL := `SELECT COUNT(*)::bigint FROM (
SELECT 1
FROM payment_orders` + where + `
GROUP BY signup_promo_attribution, signup_promo_code_id
) AS grouped_orders`
	countRows, err := s.entClient.QueryContext(ctx, countSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("count admin order attribution groups: %w", err)
	}
	if countRows.Next() {
		err = countRows.Scan(&result.GroupTotal)
	}
	closeErr = countRows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan admin order attribution group count: %w", err)
	}
	if closeErr != nil {
		return nil, closeErr
	}

	groupArgs := append([]any{}, args...)
	limitPlaceholder := fmt.Sprintf("$%d", len(groupArgs)+1)
	groupArgs = append(groupArgs, groupSize)
	offsetPlaceholder := fmt.Sprintf("$%d", len(groupArgs)+1)
	groupArgs = append(groupArgs, (groupPage-1)*groupSize)
	groupsSQL := `
SELECT signup_promo_attribution,
       signup_promo_code_id,
       COALESCE(MAX(signup_promo_code), ''),
       COUNT(DISTINCT user_id)::bigint,
       COUNT(DISTINCT user_id) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL)::bigint,
       COUNT(*) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL)::bigint,
       COALESCE(SUM(amount) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL), 0),
       COALESCE(SUM(refund_amount) FILTER (
           WHERE order_type = 'balance'
             AND completed_at IS NOT NULL
             AND refund_at IS NOT NULL
             AND status IN ('PARTIALLY_REFUNDED', 'REFUNDED')
       ), 0)
FROM payment_orders` + where + `
GROUP BY signup_promo_attribution, signup_promo_code_id
ORDER BY (
    COALESCE(SUM(amount) FILTER (WHERE order_type = 'balance' AND completed_at IS NOT NULL), 0)
    - COALESCE(SUM(refund_amount) FILTER (
        WHERE order_type = 'balance'
          AND completed_at IS NOT NULL
          AND refund_at IS NOT NULL
          AND status IN ('PARTIALLY_REFUNDED', 'REFUNDED')
      ), 0)
) DESC,
signup_promo_code_id ASC NULLS LAST
LIMIT ` + limitPlaceholder + ` OFFSET ` + offsetPlaceholder
	groupRows, err := s.entClient.QueryContext(ctx, groupsSQL, groupArgs...)
	if err != nil {
		return nil, fmt.Errorf("query admin order attribution groups: %w", err)
	}
	defer func() { _ = groupRows.Close() }()
	for groupRows.Next() {
		var group AdminOrderAttributionGroup
		var promoCodeID sql.NullInt64
		var groupGross, groupRefunded decimal.Decimal
		if err := groupRows.Scan(
			&group.PromoAttribution,
			&promoCodeID,
			&group.PromoCode,
			&group.OrderUserCount,
			&group.RechargedUserCount,
			&group.SuccessfulOrderCount,
			&groupGross,
			&groupRefunded,
		); err != nil {
			return nil, fmt.Errorf("scan admin order attribution group: %w", err)
		}
		if promoCodeID.Valid {
			id := promoCodeID.Int64
			group.PromoCodeID = &id
		}
		group.GrossRechargeAmount = groupGross.StringFixed(2)
		group.RefundedAmount = groupRefunded.StringFixed(2)
		group.NetRechargeAmount = nonNegativeAdminOrderAmount(groupGross.Sub(groupRefunded)).StringFixed(2)
		result.Groups = append(result.Groups, group)
	}
	if err := groupRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin order attribution groups: %w", err)
	}
	return result, nil
}

func (s *PaymentService) listAdminOrderPaidAmounts(ctx context.Context, p OrderListParams) ([]AdminOrderPaidAmount, int64, error) {
	query, args := adminOrderPaidAmountsSQL(p)
	rows, err := s.entClient.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin paid order amounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	amounts := []AdminOrderPaidAmount{}
	var totalCount int64
	for rows.Next() {
		var item AdminOrderPaidAmount
		var amount decimal.Decimal
		if err := rows.Scan(&item.Currency, &item.OrderCount, &amount); err != nil {
			return nil, 0, fmt.Errorf("scan admin paid order amount: %w", err)
		}
		item.Amount = amount.StringFixed(2)
		totalCount += item.OrderCount
		amounts = append(amounts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin paid order amounts: %w", err)
	}
	return amounts, totalCount, nil
}

func adminOrderPaidAmountsSQL(p OrderListParams) (string, []any) {
	where, args := adminOrderSQLWhere(p)
	if where == "" {
		where = " WHERE paid_at IS NOT NULL"
	} else {
		where += " AND paid_at IS NOT NULL"
	}
	query := `
SELECT CASE
           WHEN UPPER(COALESCE(provider_snapshot->>'currency', '')) ~ '^[A-Z]{3}$'
               THEN UPPER(provider_snapshot->>'currency')
           ELSE 'CNY'
       END AS currency,
       COUNT(*)::bigint,
       COALESCE(SUM(pay_amount), 0)
FROM payment_orders` + where + `
GROUP BY 1
ORDER BY 1`
	return query, args
}

func (s *PaymentService) ListAdminOrderPromoCodeOptions(ctx context.Context, search string, limit int) ([]AdminOrderPromoCodeOption, error) {
	search = strings.TrimSpace(search)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	args := []any{}
	searchWhere := ""
	if search != "" {
		args = append(args, "%"+search+"%")
		searchWhere = "WHERE option_code ILIKE $1 OR promo_code_id::text ILIKE $1"
	}
	args = append(args, limit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))
	query := `
WITH promo_options AS (
    SELECT id AS promo_code_id, code AS option_code, status, FALSE AS historical
    FROM promo_codes
    UNION ALL
    SELECT po.signup_promo_code_id AS promo_code_id,
           COALESCE(MAX(po.signup_promo_code), '') AS option_code,
           'historical' AS status,
           TRUE AS historical
    FROM payment_orders AS po
    WHERE po.signup_promo_attribution = 'attributed'
      AND po.signup_promo_code_id IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM promo_codes AS pc WHERE pc.id = po.signup_promo_code_id)
    GROUP BY po.signup_promo_code_id
)
SELECT promo_code_id, option_code, status, historical
FROM promo_options
` + searchWhere + `
ORDER BY option_code ASC, promo_code_id ASC
LIMIT ` + limitPlaceholder
	rows, err := s.entClient.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin order promo options: %w", err)
	}
	defer func() { _ = rows.Close() }()
	options := []AdminOrderPromoCodeOption{
		{PromoAttribution: PromoAttributionNone},
		{PromoAttribution: PromoAttributionLegacyUnknown},
	}
	for rows.Next() {
		var option AdminOrderPromoCodeOption
		var promoCodeID int64
		if err := rows.Scan(&promoCodeID, &option.PromoCode, &option.Status, &option.Historical); err != nil {
			return nil, fmt.Errorf("scan admin order promo option: %w", err)
		}
		option.PromoAttribution = PromoAttributionAttributed
		option.PromoCodeID = &promoCodeID
		options = append(options, option)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin order promo options: %w", err)
	}
	return options, nil
}

func nonNegativeAdminOrderAmount(amount decimal.Decimal) decimal.Decimal {
	if amount.IsNegative() {
		return decimal.Zero
	}
	return amount
}

func adminOrderSQLWhere(p OrderListParams) (string, []any) {
	p = normalizeAdminOrderParams(p)
	clauses := make([]string, 0, 10)
	args := make([]any, 0, 10)
	add := func(format string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if p.UserID > 0 {
		add("user_id = $%d", p.UserID)
	}
	if p.Status != "" {
		add("status = $%d", p.Status)
	}
	if p.OrderType != "" {
		add("order_type = $%d", p.OrderType)
	}
	if p.PaymentType != "" {
		add("payment_type = $%d", p.PaymentType)
	}
	if p.Keyword != "" {
		args = append(args, "%"+p.Keyword+"%")
		placeholder := len(args)
		clauses = append(clauses, fmt.Sprintf("(out_trade_no ILIKE $%d OR user_email ILIKE $%d OR user_name ILIKE $%d OR signup_promo_code ILIKE $%d)", placeholder, placeholder, placeholder, placeholder))
	}
	if p.PromoCodeID != nil {
		add("signup_promo_code_id = $%d", *p.PromoCodeID)
		clauses = append(clauses, "signup_promo_attribution = 'attributed'")
	} else {
		switch p.PromoAttribution {
		case PromoAttributionAttributed, PromoAttributionNone, PromoAttributionLegacyUnknown:
			add("signup_promo_attribution = $%d", p.PromoAttribution)
		}
	}
	timeColumn := "created_at"
	if p.TimeField == AdminOrderTimeFieldPaidAt {
		timeColumn = "paid_at"
	}
	if p.StartTime != nil {
		add(timeColumn+" >= $%d", *p.StartTime)
	}
	if p.EndTime != nil {
		add(timeColumn+" < $%d", *p.EndTime)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
