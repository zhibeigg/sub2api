package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type PlaygroundHandler struct {
	service *service.PlaygroundService
}

func NewPlaygroundHandler(playgroundService *service.PlaygroundService) *PlaygroundHandler {
	return &PlaygroundHandler{service: playgroundService}
}

// GetModelOptions returns credential-free, group-aware model choices for one owned API key.
func (h *PlaygroundHandler) GetModelOptions(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	apiKeyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || apiKeyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	options, err := h.service.GetModelOptions(c.Request.Context(), subject.UserID, apiKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, options)
}
