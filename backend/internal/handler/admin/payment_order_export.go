package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const maxAdminOrderExportRows = 100000

// ExportOrders exports all filtered order rows or signup promo attribution groups as CSV.
// GET /api/v1/admin/payment/orders/export?mode=orders|attribution
func (h *PaymentHandler) ExportOrders(c *gin.Context) {
	params, err := parseAdminOrderFilters(c, 1, 100)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	mode := strings.TrimSpace(c.DefaultQuery("mode", "orders"))
	if mode != "orders" && mode != "attribution" {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_EXPORT_MODE", "mode must be orders or attribution"))
		return
	}

	if mode == "attribution" {
		h.exportOrderAttribution(c, params)
		return
	}
	h.exportOrderRows(c, params)
}

func (h *PaymentHandler) exportOrderRows(c *gin.Context, params service.OrderListParams) {
	total, err := h.paymentService.AdminCountOrders(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if total > maxAdminOrderExportRows {
		response.ErrorFrom(c, infraerrors.BadRequest("EXPORT_LIMIT_EXCEEDED", "filtered orders exceed the 100000 row export limit"))
		return
	}

	writer := beginAdminOrderCSV(c, "payment_orders")
	if err := writer.Write([]string{
		"order_id", "out_trade_no", "user_id", "user_email", "user_name",
		"signup_promo_attribution", "signup_promo_code_id", "signup_promo_code",
		"order_type", "payment_type", "status", "currency", "credited_amount",
		"paid_amount", "refund_amount", "net_recharge_amount", "created_at",
		"paid_at", "completed_at", "refund_at",
	}); err != nil {
		return
	}

	err = h.paymentService.AdminIterateOrders(c.Request.Context(), params, 1000, func(orders []*dbent.PaymentOrder) error {
		for _, order := range orders {
			if err := writer.Write(adminOrderCSVRecord(order)); err != nil {
				return err
			}
		}
		writer.Flush()
		return writer.Error()
	})
	writer.Flush()
	if err != nil || writer.Error() != nil {
		return
	}
}

func (h *PaymentHandler) exportOrderAttribution(c *gin.Context, params service.OrderListParams) {
	summary, err := h.paymentService.GetAdminOrderSummary(c.Request.Context(), params, 1, maxAdminOrderExportRows)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if summary.GroupTotal > maxAdminOrderExportRows {
		response.ErrorFrom(c, infraerrors.BadRequest("EXPORT_LIMIT_EXCEEDED", "attribution groups exceed the 100000 row export limit"))
		return
	}

	writer := beginAdminOrderCSV(c, "payment_order_attribution")
	if err := writer.Write([]string{
		"signup_promo_attribution", "signup_promo_code_id", "signup_promo_code",
		"order_user_count", "recharged_user_count", "successful_order_count",
		"gross_recharge_amount", "refunded_amount", "net_recharge_amount",
	}); err != nil {
		return
	}
	for _, group := range summary.Groups {
		promoCodeID := ""
		if group.PromoCodeID != nil {
			promoCodeID = strconv.FormatInt(*group.PromoCodeID, 10)
		}
		if err := writer.Write([]string{
			safeCSVText(group.PromoAttribution),
			promoCodeID,
			safeCSVText(group.PromoCode),
			strconv.FormatInt(group.OrderUserCount, 10),
			strconv.FormatInt(group.RechargedUserCount, 10),
			strconv.FormatInt(group.SuccessfulOrderCount, 10),
			group.GrossRechargeAmount,
			group.RefundedAmount,
			group.NetRechargeAmount,
		}); err != nil {
			return
		}
	}
	writer.Flush()
}

func beginAdminOrderCSV(c *gin.Context, prefix string) *csv.Writer {
	filename := fmt.Sprintf("%s_%s.csv", prefix, time.Now().UTC().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	return csv.NewWriter(c.Writer)
}

func adminOrderCSVRecord(order *dbent.PaymentOrder) []string {
	if order == nil {
		return nil
	}
	promoCodeID := ""
	if order.SignupPromoCodeID != nil {
		promoCodeID = strconv.FormatInt(*order.SignupPromoCodeID, 10)
	}
	promoCode := ""
	if order.SignupPromoCode != nil {
		promoCode = *order.SignupPromoCode
	}
	return []string{
		strconv.FormatInt(order.ID, 10),
		safeCSVText(order.OutTradeNo),
		strconv.FormatInt(order.UserID, 10),
		safeCSVText(order.UserEmail),
		safeCSVText(order.UserName),
		safeCSVText(order.SignupPromoAttribution),
		promoCodeID,
		safeCSVText(promoCode),
		safeCSVText(order.OrderType),
		safeCSVText(order.PaymentType),
		safeCSVText(order.Status),
		safeCSVText(service.PaymentOrderCurrency(order)),
		formatCSVAmount(order.Amount),
		formatCSVAmount(order.PayAmount),
		formatCSVAmount(order.RefundAmount),
		formatCSVAmount(adminOrderNetRechargeAmount(order)),
		formatCSVTime(&order.CreatedAt),
		formatCSVTime(order.PaidAt),
		formatCSVTime(order.CompletedAt),
		formatCSVTime(order.RefundAt),
	}
}

func formatCSVAmount(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatCSVTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func safeCSVText(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	if value == "" {
		return value
	}
	if value[0] == '\t' || value[0] == '\r' {
		return "'" + value
	}
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
