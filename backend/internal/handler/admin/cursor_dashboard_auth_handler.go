package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CursorDashboardAuthHandler struct {
	auth *service.CursorDashboardAuthService
}

func NewCursorDashboardAuthHandler(auth *service.CursorDashboardAuthService) *CursorDashboardAuthHandler {
	return &CursorDashboardAuthHandler{auth: auth}
}

type cursorDashboardAuthStartRequest struct {
	AccountID int64 `json:"account_id" binding:"required,gt=0"`
}

func (h *CursorDashboardAuthHandler) Start(c *gin.Context) {
	var req cursorDashboardAuthStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.auth.StartLogin(c.Request.Context(), req.AccountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type cursorDashboardAuthPollRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

func (h *CursorDashboardAuthHandler) Poll(c *gin.Context) {
	var req cursorDashboardAuthPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.auth.PollLogin(c.Request.Context(), req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
