package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterQQBotRoutes exposes the HMAC-protected API consumed by sub2api-qqbot.
func RegisterQQBotRoutes(v1 *gin.RouterGroup, h *handler.QQBotHandler, hmacMiddleware gin.HandlerFunc) {
	group := v1.Group("/integrations/qqbot")
	group.Use(hmacMiddleware)
	group.POST("/bindings/prepare", h.PrepareBinding)
	group.POST("/bindings/inspect", h.InspectBinding)
	group.POST("/bindings/complete", h.CompleteBinding)
	group.GET("/bindings", h.ListBindings)
	group.POST("/bindings/:id/unbind", h.Unbind)
	group.GET("/stats", h.Stats)
	group.GET("/settings", h.GetSettings)
	group.PATCH("/settings", h.UpdateSettings)
}
