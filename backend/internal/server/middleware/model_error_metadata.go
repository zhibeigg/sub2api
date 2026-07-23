package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelerror"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelErrorMetadata runs after endpoint normalization and before gateway auth.
// It exposes a PokeAPI-owned request ID early enough for SSE flushes and
// WebSocket upgrades without adding error-only language/code headers to success.
func ModelErrorMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil {
			if _, ok := service.EndpointProtocolFromContext(c.Request.Context()); ok {
				modelerror.ApplyRequestIDHeader(c.Request.Context(), c.Writer.Header())
			}
		}
		c.Next()
	}
}
