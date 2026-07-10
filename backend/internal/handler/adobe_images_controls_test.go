package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseAdobeImageRequest_StrictControls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct{ name, body string }{
		{"n", `{"model":"nano-banana","prompt":"x","n":2}`},
		{"stream", `{"model":"nano-banana","prompt":"x","stream":false}`},
		{"mask", `{"model":"nano-banana","prompt":"x","mask":"x"}`},
		{"output", `{"model":"nano-banana","prompt":"x","output_format":"jpeg"}`},
		{"response", `{"model":"nano-banana","prompt":"x","response_format":"binary"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			_, err := parseAdobeImageRequest(c)
			require.Error(t, err)
		})
	}
}

func TestParseAdobeImageRequest_AcceptsURLAndB64Format(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"nano-banana","prompt":"x","n":1,"quality":"4k","response_format":"b64_json","image":{"image_url":"https://example.com/ref.png"}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	req, err := parseAdobeImageRequest(c)
	require.NoError(t, err)
	require.Equal(t, "b64_json", req.ResponseFormat)
	require.Len(t, req.References, 1)
}
