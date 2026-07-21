package handler

import (
	"strconv"
	"strings"

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

// FetchURL retrieves public textual URLs for authenticated playground sessions.
func (h *PlaygroundHandler) FetchURL(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req service.PlaygroundFetchURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	urls := req.RequestedURLs()
	results, err := h.service.FetchURLsWithLimit(c.Request.Context(), urls, req.ResponseLimit())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if strings.TrimSpace(req.URL) != "" && len(req.URLs) == 0 {
		response.Success(c, results[0])
		return
	}
	response.Success(c, gin.H{"results": results})
}

// GetModelOptions returns the credential-free, deduplicated model union for one owned API key.
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
