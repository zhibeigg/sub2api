package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type QQBotHandler struct {
	service *service.QQBotService
	config  *qqbot.ConfigManager
	runtime *qqbot.Runtime
	queue   *qqbot.ReliableQueue
}

func NewQQBotHandler(qqBotService *service.QQBotService, configManager *qqbot.ConfigManager, runtime *qqbot.Runtime, queue *qqbot.ReliableQueue) *QQBotHandler {
	return &QQBotHandler{service: qqBotService, config: configManager, runtime: runtime, queue: queue}
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

func (h *QQBotHandler) Webhook(c *gin.Context) {
	if h.runtime == nil {
		response.ErrorFrom(c, qqbot.ErrRuntimeUnavailable)
		return
	}
	h.runtime.ServeWebhook(c.Writer, c.Request)
}

func (h *QQBotHandler) PublicInspectBinding(c *gin.Context) {
	var input service.QQBotInspectBindingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	if !h.allowPublic(c, "inspect:"+middleware2.SecurityClientIP(c), 30, time.Minute) {
		return
	}
	result, err := h.service.InspectBinding(c.Request.Context(), input.Token)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) PublicCompleteBinding(c *gin.Context) {
	var input service.QQBotCompleteBindingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, service.ErrQQBotInvalidInput)
		return
	}
	if !h.allowPublic(c, "complete:"+middleware2.SecurityClientIP(c), 10, 10*time.Minute) {
		return
	}
	result, err := h.service.CompleteBinding(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) allowPublic(c *gin.Context, scope string, limit int64, window time.Duration) bool {
	if h.queue == nil {
		response.ErrorFrom(c, qqbot.ErrRuntimeUnavailable)
		return false
	}
	allowed, _, err := h.queue.Allow(c.Request.Context(), "public:"+scope, limit, window)
	if err != nil {
		response.ErrorFrom(c, qqbot.ErrRuntimeUnavailable)
		return false
	}
	if !allowed {
		response.Error(c, http.StatusTooManyRequests, "too many requests")
		return false
	}
	return true
}

func (h *QQBotHandler) GetConfig(c *gin.Context) {
	if h.config == nil {
		response.ErrorFrom(c, qqbot.ErrRuntimeUnavailable)
		return
	}
	response.Success(c, h.config.Public())
}

func (h *QQBotHandler) UpdateConfig(c *gin.Context) {
	var input qqbot.UpdateConfigRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ErrorFrom(c, qqbot.ErrInvalidConfig)
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	result, err := h.config.Save(c.Request.Context(), input, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *QQBotHandler) Probe(c *gin.Context) {
	if h.runtime == nil || h.config == nil {
		response.ErrorFrom(c, qqbot.ErrRuntimeUnavailable)
		return
	}
	var input qqbot.ProbeRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		response.ErrorFrom(c, qqbot.ErrInvalidConfig)
		return
	}
	candidate, err := h.config.ResolveProbeConfig(input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	timeout := time.Duration(candidate.APITimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	result := h.runtime.ProbeConfig(ctx, candidate)
	if result.OK {
		if err := h.config.RecordSuccessfulProbe(c.Request.Context(), candidate); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	response.Success(c, result)
}

func (h *QQBotHandler) GetRuntime(c *gin.Context) {
	if h.runtime == nil {
		response.ErrorFrom(c, qqbot.ErrRuntimeUnavailable)
		return
	}
	response.Success(c, h.runtime.State(c.Request.Context()))
}

func (h *QQBotHandler) AdminUnbind(c *gin.Context) {
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
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	input.AdminSubject = strconv.FormatInt(subject.UserID, 10)
	result, err := h.service.Unbind(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

var qqBotVerificationPath = regexp.MustCompile(`^/(\d+)\.json$`)

func (h *QQBotHandler) AppIDVerificationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || h.config == nil {
			c.Next()
			return
		}
		matches := qqBotVerificationPath.FindStringSubmatch(c.Request.URL.Path)
		if len(matches) != 2 || matches[1] != h.config.Public().AppID {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"bot_appid": matches[1]})
		c.Abort()
	}
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
