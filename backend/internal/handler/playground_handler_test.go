package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type playgroundHandlerKeyReader struct{ key *service.APIKey }

func (s playgroundHandlerKeyReader) GetByID(context.Context, int64) (*service.APIKey, error) {
	return s.key, nil
}

type playgroundHandlerModelLister struct{ models []string }

func (s playgroundHandlerModelLister) GetAvailableModels(context.Context, *int64, string) []string {
	return s.models
}

func TestPlaygroundHandlerFetchURLValidatesBatchContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	playgroundService := service.NewPlaygroundService(playgroundHandlerKeyReader{}, playgroundHandlerModelLister{})
	router := gin.New()
	router.Use(withUserSubject(11))
	router.POST("/api/v1/playground/fetch-url", NewPlaygroundHandler(playgroundService).FetchURL)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/playground/fetch-url", strings.NewReader(`{"urls":["http://example.com/1","http://example.com/2","http://example.com/3","http://example.com/4"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"reason":"PLAYGROUND_FETCH_TOO_MANY_URLS"`)
}

func TestPlaygroundHandlerFetchURLAcceptsSingleURLContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	playgroundService := service.NewPlaygroundService(playgroundHandlerKeyReader{}, playgroundHandlerModelLister{})
	router := gin.New()
	router.Use(withUserSubject(11))
	router.POST("/api/v1/playground/fetch-url", NewPlaygroundHandler(playgroundService).FetchURL)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/playground/fetch-url", strings.NewReader(`{"url":"http://localhost/private"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"reason":"PLAYGROUND_FETCH_INVALID_URL"`)
}

func TestPlaygroundHandlerGetModelOptionsReturnsCredentialFreeOwnedOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(3)
	playgroundService := service.NewPlaygroundService(
		playgroundHandlerKeyReader{key: &service.APIKey{
			ID: 7, UserID: 11, Key: "secret-must-not-leak", GroupID: &groupID,
			Group: &service.Group{ID: groupID, Name: "chat", Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true},
		}},
		playgroundHandlerModelLister{models: []string{"gpt-5.4"}},
	)
	router := gin.New()
	router.Use(withUserSubject(11))
	router.GET("/api/v1/playground/api-keys/:id/model-options", NewPlaygroundHandler(playgroundService).GetModelOptions)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/playground/api-keys/7/model-options", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"group_id":3`)
	require.Contains(t, recorder.Body.String(), `"model":"gpt-5.4"`)
	require.Contains(t, recorder.Body.String(), `"features":{"image_input":true,"responses":true,"web_search":true,"code_execution":true,"web_fetch":true}`)
	require.NotContains(t, recorder.Body.String(), "secret-must-not-leak")
	require.False(t, strings.Contains(strings.ToLower(recorder.Body.String()), "credential"))
}
