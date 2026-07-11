package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCursorDashboardAuthRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerCursorDashboardAuthRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{
		CursorDashboardAuth: &adminhandler.CursorDashboardAuthHandler{},
	}})

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	require.True(t, routes[http.MethodPost+" /api/v1/admin/cursor/dashboard-auth/start"])
	require.True(t, routes[http.MethodPost+" /api/v1/admin/cursor/dashboard-auth/poll"])
}
