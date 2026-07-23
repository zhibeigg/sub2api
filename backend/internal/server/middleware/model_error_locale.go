package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelerror"
	"github.com/gin-gonic/gin"
)

// ModelErrorLocale parses Accept-Language once and stores the resolved locale in
// request context. It deliberately does not write response headers so ordinary
// APIs and successful responses are not marked as localized errors.
func ModelErrorLocale(cfg *config.Config) gin.HandlerFunc {
	fallback := "en"
	if cfg != nil && cfg.Gateway.ModelErrorDefaultLocale != "" {
		fallback = cfg.Gateway.ModelErrorDefaultLocale
	}
	return func(c *gin.Context) {
		if c.Request != nil {
			locale := modelerror.ResolveAcceptLanguage(c.GetHeader("Accept-Language"), fallback)
			ctx := modelerror.WithLocale(c.Request.Context(), locale, fallback)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
