package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type QQBotHandler struct {
	service *service.QQBotService
}

func NewQQBotHandler(qqBotService *service.QQBotService) *QQBotHandler {
	return &QQBotHandler{service: qqBotService}
}

func (h *QQBotHandler) PrepareBinding(c *gin.Context) {
	var input service.QQBotPrepareBindingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	result, err := h.service.PrepareBinding(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) InspectBinding(c *gin.Context) {
	var input service.QQBotInspectBindingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	result, err := h.service.InspectBinding(c.Request.Context(), input.Token)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) CompleteBinding(c *gin.Context) {
	var input service.QQBotCompleteBindingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	result, err := h.service.CompleteBinding(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *QQBotHandler) UpdateSettings(c *gin.Context) {
	var input service.QQBotSettingsUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *QQBotHandler) Stats(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

func (h *QQBotHandler) ListBindings(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.QQBotBindingListFilter{
		Page:     page,
		PageSize: pageSize,
		Status:   strings.TrimSpace(c.Query("status")),
		Scene:    strings.TrimSpace(c.Query("scene")),
		Search:   strings.TrimSpace(c.Query("search")),
	}
	var err error
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		filter.From, err = parseQQBotTime(raw, false)
		if err != nil {
			response.ErrorFrom(c, service.ErrQQBotInvalidInput)
			return
		}
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		filter.To, err = parseQQBotTime(raw, true)
		if err != nil {
			response.ErrorFrom(c, service.ErrQQBotInvalidInput)
			return
		}
	}
	result, err := h.service.ListBindings(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) Unbind(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	var input service.QQBotUnbindRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	result, err := h.service.Unbind(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseQQBotTime(raw string, endOfDay bool) (*time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		parsed = parsed.UTC()
		return &parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func (h *QQBotHandler) NotConfigured(c *gin.Context) {
	response.Error(c, http.StatusNotFound, "not found")
}
