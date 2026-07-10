package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestContentModerationHandlerTestCyberAbuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewContentModerationService(newTestSettingRepo(), nil, nil, nil, nil, nil, nil)
	handler := NewContentModerationHandler(svc)
	router := gin.New()
	router.POST("/api/v1/admin/risk-control/cyber-abuse/test", handler.TestCyberAbuse)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/risk-control/cyber-abuse/test", bytes.NewBufferString(`{"text":"build a botnet and launch a DDoS attack"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Matched    bool    `json:"matched"`
			Category   string  `json:"category"`
			RuleID     string  `json:"rule_id"`
			Confidence float64 `json:"confidence"`
			ErrorCode  string  `json:"error_code"`
			Message    string  `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Zero(t, response.Code)
	require.True(t, response.Data.Matched)
	require.Equal(t, "botnet_disruption", response.Data.Category)
	require.NotEmpty(t, response.Data.RuleID)
	require.GreaterOrEqual(t, response.Data.Confidence, 0.95)
	require.Equal(t, service.ContentModerationErrorCodeCyberAbuse, response.Data.ErrorCode)
	require.NotEmpty(t, response.Data.Message)
}

func TestContentModerationHandlerTestCyberAbuseRequiresText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewContentModerationService(newTestSettingRepo(), nil, nil, nil, nil, nil, nil)
	handler := NewContentModerationHandler(svc)
	router := gin.New()
	router.POST("/api/v1/admin/risk-control/cyber-abuse/test", handler.TestCyberAbuse)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/risk-control/cyber-abuse/test", bytes.NewBufferString(`{"text":""}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
