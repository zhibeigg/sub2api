package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AnnouncementHandler handles admin announcement management
type AnnouncementHandler struct {
	announcementService      *service.AnnouncementService
	announcementEmailService *service.AnnouncementEmailService
}

// NewAnnouncementHandler creates a new admin announcement handler
func NewAnnouncementHandler(announcementService *service.AnnouncementService) *AnnouncementHandler {
	return &AnnouncementHandler{announcementService: announcementService}
}

func ProvideAnnouncementHandler(announcementService *service.AnnouncementService, emailService *service.AnnouncementEmailService) *AnnouncementHandler {
	if announcementService != nil {
		announcementService.SetEmailPublicationValidator(emailService)
	}
	return &AnnouncementHandler{announcementService: announcementService, announcementEmailService: emailService}
}

type CreateAnnouncementRequest struct {
	Title      string                        `json:"title" binding:"required"`
	Content    string                        `json:"content" binding:"required"`
	Status     string                        `json:"status" binding:"omitempty,oneof=draft active archived"`
	NotifyMode string                        `json:"notify_mode" binding:"omitempty,oneof=silent popup"`
	Targeting  service.AnnouncementTargeting `json:"targeting"`
	StartsAt   *int64                        `json:"starts_at"` // Unix seconds, 0/empty = immediate
	EndsAt     *int64                        `json:"ends_at"`   // Unix seconds, 0/empty = never
	SendEmail  bool                          `json:"send_email"`
}

type UpdateAnnouncementRequest struct {
	Title      *string                        `json:"title"`
	Content    *string                        `json:"content"`
	Status     *string                        `json:"status" binding:"omitempty,oneof=draft active archived"`
	NotifyMode *string                        `json:"notify_mode" binding:"omitempty,oneof=silent popup"`
	Targeting  *service.AnnouncementTargeting `json:"targeting"`
	StartsAt   *int64                         `json:"starts_at"` // Unix seconds, 0 = clear
	EndsAt     *int64                         `json:"ends_at"`   // Unix seconds, 0 = clear
	SendEmail  bool                           `json:"send_email"`
}

// List handles listing announcements with filters
// GET /api/v1/admin/announcements
func (h *AnnouncementHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if len(search) > 200 {
		search = search[:200]
	}

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	items, paginationResult, err := h.announcementService.List(
		c.Request.Context(),
		params,
		service.AnnouncementListFilters{Status: status, Search: search},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.Announcement, 0, len(items))
	for i := range items {
		out = append(out, *dto.AnnouncementFromService(&items[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// GetByID handles getting an announcement by ID
// GET /api/v1/admin/announcements/:id
func (h *AnnouncementHandler) GetByID(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	item, err := h.announcementService.GetByID(c.Request.Context(), announcementID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AnnouncementFromService(item))
}

// Create handles creating a new announcement
// POST /api/v1/admin/announcements
func (h *AnnouncementHandler) Create(c *gin.Context) {
	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &service.CreateAnnouncementInput{
		Title:      req.Title,
		Content:    req.Content,
		Status:     req.Status,
		NotifyMode: req.NotifyMode,
		Targeting:  req.Targeting,
		ActorID:    &subject.UserID,
		SendEmail:  req.SendEmail,
	}

	if req.StartsAt != nil && *req.StartsAt > 0 {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.EndsAt != nil && *req.EndsAt > 0 {
		t := time.Unix(*req.EndsAt, 0)
		input.EndsAt = &t
	}

	if req.SendEmail {
		executeAdminStrictIdempotentJSON(c, "announcement.create.email", req, 24*time.Hour, func(ctx context.Context) (any, error) {
			created, err := h.announcementService.Create(ctx, input)
			if err != nil {
				return nil, err
			}
			return dto.AnnouncementFromService(created), nil
		})
		return
	}
	created, err := h.announcementService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AnnouncementFromService(created))
}

// Update handles updating an announcement
// PUT /api/v1/admin/announcements/:id
func (h *AnnouncementHandler) Update(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	var req UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &service.UpdateAnnouncementInput{
		Title:      req.Title,
		Content:    req.Content,
		Status:     req.Status,
		NotifyMode: req.NotifyMode,
		Targeting:  req.Targeting,
		ActorID:    &subject.UserID,
		SendEmail:  req.SendEmail,
	}

	if req.StartsAt != nil {
		if *req.StartsAt == 0 {
			var cleared *time.Time = nil
			input.StartsAt = &cleared
		} else {
			t := time.Unix(*req.StartsAt, 0)
			ptr := &t
			input.StartsAt = &ptr
		}
	}

	if req.EndsAt != nil {
		if *req.EndsAt == 0 {
			var cleared *time.Time = nil
			input.EndsAt = &cleared
		} else {
			t := time.Unix(*req.EndsAt, 0)
			ptr := &t
			input.EndsAt = &ptr
		}
	}

	if req.SendEmail {
		executeAdminStrictIdempotentJSON(c, "announcement.update.email", gin.H{"announcement_id": announcementID, "request": req}, 24*time.Hour, func(ctx context.Context) (any, error) {
			updated, err := h.announcementService.Update(ctx, announcementID, input)
			if err != nil {
				return nil, err
			}
			return dto.AnnouncementFromService(updated), nil
		})
		return
	}
	updated, err := h.announcementService.Update(c.Request.Context(), announcementID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AnnouncementFromService(updated))
}

// Delete handles deleting an announcement
// DELETE /api/v1/admin/announcements/:id
func (h *AnnouncementHandler) Delete(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	if err := h.announcementService.Delete(c.Request.Context(), announcementID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Announcement deleted successfully"})
}

// ListReadStatus handles listing users read status for an announcement
// GET /api/v1/admin/announcements/:id/read-status
func (h *AnnouncementHandler) EmailCapability(c *gin.Context) {
	if h.announcementEmailService == nil {
		response.Success(c, service.AnnouncementEmailCapability{})
		return
	}
	capability, err := h.announcementEmailService.Capability(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, capability)
}

func (h *AnnouncementHandler) GetEmailNotification(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}
	if h.announcementEmailService == nil {
		response.ErrorFrom(c, service.ErrAnnouncementEmailNotFound)
		return
	}
	item, err := h.announcementEmailService.Get(c.Request.Context(), announcementID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AnnouncementEmailNotificationFromService(item))
}

type RetryAnnouncementEmailRequest struct {
	IncludeAmbiguous bool `json:"include_ambiguous"`
}

func (h *AnnouncementHandler) RetryEmailNotification(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}
	var req RetryAnnouncementEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeAdminStrictIdempotentJSON(c, "announcement.email.retry", gin.H{"announcement_id": announcementID, "include_ambiguous": req.IncludeAmbiguous}, 24*time.Hour, func(ctx context.Context) (any, error) {
		if h.announcementEmailService == nil {
			return nil, service.ErrAnnouncementEmailNotFound
		}
		item, err := h.announcementEmailService.Retry(ctx, announcementID, req.IncludeAmbiguous)
		if err != nil {
			return nil, err
		}
		return gin.H{"email_notification": dto.AnnouncementEmailNotificationFromService(item)}, nil
	})
}

func (h *AnnouncementHandler) ListReadStatus(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "email"),
		SortOrder: c.DefaultQuery("sort_order", "asc"),
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 200 {
		search = search[:200]
	}

	items, paginationResult, err := h.announcementService.ListUserReadStatus(
		c.Request.Context(),
		announcementID,
		params,
		search,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}
