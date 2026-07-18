package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountValidateCredentialsRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAccountRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{
		Account:     &adminhandler.AccountHandler{},
		OAuth:       &adminhandler.OAuthHandler{},
		OpenAIOAuth: &adminhandler.OpenAIOAuthHandler{},
	}}, nil)

	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/v1/admin/accounts/validate-credentials" {
			return
		}
	}
	require.Fail(t, "validate credentials route was not registered under account routes")
}
