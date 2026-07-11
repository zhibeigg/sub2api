package admin

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkgtimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func parseAdminOrderFilters(c *gin.Context, page, pageSize int) (service.OrderListParams, error) {
	params := service.OrderListParams{
		Page:             page,
		PageSize:         pageSize,
		Status:           strings.TrimSpace(c.Query("status")),
		OrderType:        strings.TrimSpace(c.Query("order_type")),
		PaymentType:      strings.TrimSpace(c.Query("payment_type")),
		Keyword:          strings.TrimSpace(c.Query("keyword")),
		PromoAttribution: strings.TrimSpace(c.Query("promo_attribution")),
		TimeField:        strings.TrimSpace(c.Query("time_field")),
	}
	if utf8.RuneCountInString(params.Keyword) > 100 {
		return params, infraerrors.BadRequest("INVALID_KEYWORD", "keyword must not exceed 100 characters")
	}
	if params.TimeField == "" {
		params.TimeField = service.AdminOrderTimeFieldCreatedAt
	}
	if params.TimeField != service.AdminOrderTimeFieldCreatedAt && params.TimeField != service.AdminOrderTimeFieldPaidAt {
		return params, infraerrors.BadRequest("INVALID_TIME_FIELD", "time_field must be created_at or paid_at")
	}
	switch params.PromoAttribution {
	case "", "all", service.PromoAttributionAttributed, service.PromoAttributionNone, service.PromoAttributionLegacyUnknown:
	default:
		return params, infraerrors.BadRequest("INVALID_PROMO_ATTRIBUTION", "invalid promo_attribution")
	}

	if value := strings.TrimSpace(c.Query("user_id")); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return params, infraerrors.BadRequest("INVALID_USER_ID", "user_id must be a positive integer")
		}
		params.UserID = id
	}
	if value := strings.TrimSpace(c.Query("promo_code_id")); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return params, infraerrors.BadRequest("INVALID_PROMO_CODE_ID", "promo_code_id must be a positive integer")
		}
		params.PromoCodeID = &id
		params.PromoAttribution = service.PromoAttributionAttributed
	}

	location := pkgtimezone.Location()
	if value := strings.TrimSpace(c.Query("timezone")); value != "" {
		parsed, err := time.LoadLocation(value)
		if err != nil {
			return params, infraerrors.BadRequest("INVALID_TIMEZONE", "timezone must be a valid IANA timezone")
		}
		location = parsed
	}
	if value := strings.TrimSpace(c.Query("start_date")); value != "" {
		parsed, err := time.ParseInLocation("2006-01-02", value, location)
		if err != nil {
			return params, infraerrors.BadRequest("INVALID_START_DATE", "start_date must use YYYY-MM-DD")
		}
		utc := parsed.UTC()
		params.StartTime = &utc
	}
	if value := strings.TrimSpace(c.Query("end_date")); value != "" {
		parsed, err := time.ParseInLocation("2006-01-02", value, location)
		if err != nil {
			return params, infraerrors.BadRequest("INVALID_END_DATE", "end_date must use YYYY-MM-DD")
		}
		utc := parsed.AddDate(0, 0, 1).UTC()
		params.EndTime = &utc
	}
	if params.StartTime != nil && params.EndTime != nil && !params.StartTime.Before(*params.EndTime) {
		return params, infraerrors.BadRequest("INVALID_DATE_RANGE", "start_date must not be after end_date")
	}
	return params, nil
}

func parsePositiveQueryInt(c *gin.Context, key string, fallback, maximum int) (int, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, infraerrors.BadRequest("INVALID_PAGINATION", key+" must be a positive integer")
	}
	if maximum > 0 && parsed > maximum {
		parsed = maximum
	}
	return parsed, nil
}
