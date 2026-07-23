package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/gin-gonic/gin"
)

// RegisterQQBotRoutes exposes the embedded BotGo webhook/public binding API and
// retains the legacy HMAC bridge during the rollback window.
func RegisterQQBotRoutes(root *gin.Engine, v1 *gin.RouterGroup, h *handler.QQBotHandler, hmacMiddleware gin.HandlerFunc) {
	root.POST("/webhooks/qq", h.Webhook)
	root.GET("/webhooks/qq/onebot", h.OneBotWebhook)
	root.GET(qqbot.ChannelCheckImagePath, h.ChannelStatusImage)

	public := v1.Group("/public/bindings")
	public.POST("/inspect", h.PublicInspectBinding)
	public.POST("/complete", h.PublicCompleteBinding)

	legacy := v1.Group("/integrations/qqbot")
	legacy.Use(hmacMiddleware)
	legacy.POST("/bindings/prepare", h.PrepareBinding)
	legacy.POST("/bindings/inspect", h.InspectBinding)
	legacy.POST("/bindings/complete", h.CompleteBinding)
	legacy.GET("/bindings", h.ListBindings)
	legacy.POST("/bindings/:id/unbind", h.Unbind)
	legacy.GET("/stats", h.Stats)
	legacy.GET("/settings", h.GetSettings)
	legacy.PATCH("/settings", h.UpdateSettings)
}
