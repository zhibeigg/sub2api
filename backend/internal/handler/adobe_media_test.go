package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/firefly"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdobeModels_IntersectsCustomGroupListWithCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(8)
	apiKey := &service.APIKey{ID: 1, UserID: 2, GroupID: &groupID, Group: &service.Group{
		ID: groupID, Platform: service.PlatformAdobe,
		ModelsListConfig: service.GroupModelsListConfig{Enabled: true, Models: []string{"veo3", "unknown-hidden", "nano-banana"}},
	}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)

	NewAdobeMediaHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).Models(c)
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, []string{"nano-banana", "veo3"}, []string{body.Data[0].ID, body.Data[1].ID})
}

func TestAdobeUnsupported_ReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	NewAdobeMediaHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).Unsupported(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestParseAdobeVideoRequest_JSONReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := bytes.NewBufferString(`{
		"model":"veo3.1","prompt":"hello","duration":8,"resolution":"1080p",
		"generate_audio":false,"reference_mode":"image",
		"image":{"image_url":{"url":"https://cdn.example/a.png"}},
		"images":["https://cdn.example/b.png"]
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", body)
	c.Request.Header.Set("Content-Type", "application/json")

	req, err := parseAdobeVideoRequest(c)
	require.NoError(t, err)
	require.Equal(t, "veo3.1", req.Model)
	require.Equal(t, 8, req.Duration)
	require.NotNil(t, req.GenerateAudio)
	require.False(t, *req.GenerateAudio)
	require.Equal(t, []string{"https://cdn.example/a.png", "https://cdn.example/b.png"}, []string{req.References[0].URL, req.References[1].URL})
}

func TestParseAdobeVideoRequest_MultipartReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "sora"))
	require.NoError(t, writer.WriteField("prompt", "hello"))
	require.NoError(t, writer.WriteField("duration", "8"))
	require.NoError(t, writer.WriteField("generate_audio", "false"))
	require.NoError(t, writer.WriteField("image_url", "https://cdn.example/a.png"))
	part, err := writer.CreateFormFile("image", "reference.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-image-data"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	req, err := parseAdobeVideoRequest(c)
	require.NoError(t, err)
	require.Equal(t, "sora", req.Model)
	require.Equal(t, 8, req.Duration)
	require.Len(t, req.References, 2)
	require.Equal(t, "https://cdn.example/a.png", req.References[0].URL)
	require.Equal(t, "reference.png", req.References[1].Name)
	require.Equal(t, []byte("fake-image-data"), req.References[1].Data)
}

func TestParseAdobeVideoRequest_RejectsTooManyReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := bytes.NewBufferString(`{"prompt":"hello","images":["https://a.example/1.png","https://a.example/2.png","https://a.example/3.png","https://a.example/4.png"]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", body)
	c.Request.Header.Set("Content-Type", "application/json")
	_, err := parseAdobeVideoRequest(c)
	require.ErrorContains(t, err, "too many reference images")
}

func TestParseAdobeImageRequest_RequiresPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"model":"nano-banana"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	_, err := parseAdobeImageRequest(c)
	require.ErrorContains(t, err, "prompt is required")
}

func TestShouldFailoverAdobeError(t *testing.T) {
	require.True(t, shouldFailoverAdobeError(&firefly.ProviderError{Kind: firefly.ErrorAuth}))
	require.True(t, shouldFailoverAdobeError(&firefly.ProviderError{Kind: firefly.ErrorTemporary, Retryable: true}))
	require.False(t, shouldFailoverAdobeError(&firefly.ProviderError{Kind: firefly.ErrorRequest}))
	require.False(t, shouldFailoverAdobeError(&firefly.ProviderError{Kind: firefly.ErrorContentPolicy}))
}

func TestAdobeMediaAcquireAccountSlot_ReleasesSchedulerSlotOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	released := 0
	h := NewAdobeMediaHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	release, acquired := h.acquireAccountSlot(c, &service.AccountSelectionResult{
		Account:  &service.Account{ID: 99},
		Acquired: true,
		ReleaseFunc: func() {
			released++
		},
	})
	require.True(t, acquired)
	require.NotNil(t, release)
	release()
	release()
	require.Equal(t, 1, released)
}
