package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func TestQQBotAdminRoutesRequireAdminMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	registerQQBotAdminRoutes(admin, &handler.Handlers{QQBot: handler.NewQQBotHandler(nil, nil, nil, nil)})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qqbot/config", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", recorder.Code)
	}
}

func TestRegisterQQBotRoutesExposesWebhookAndPublicBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	h := handler.NewQQBotHandler(nil, nil, nil, nil)
	RegisterQQBotRoutes(router, v1, h, func(c *gin.Context) { c.Next() })
	for _, test := range []struct{ method, path string }{{http.MethodPost, "/webhooks/qq"}, {http.MethodPost, "/api/v1/public/bindings/inspect"}, {http.MethodPost, "/api/v1/public/bindings/complete"}} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusNotFound {
			t.Fatalf("route missing: %s %s", test.method, test.path)
		}
	}
}
