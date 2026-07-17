package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestQQBotHMACMiddlewareVerifiesBodyAndRejectsReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{QQBotIntegration: config.QQBotIntegrationConfig{
		Enabled: true, KeyID: "primary", HMACSecret: strings.Repeat("s", 32),
		TimestampToleranceSeconds: 300, NonceTTLSeconds: 600,
	}}
	router := gin.New()
	router.Use(NewQQBotHMACMiddleware(cfg, rdb))
	router.POST("/api/v1/integrations/qqbot/test", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"value":"ok"}`, string(body))
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})

	body := []byte(`{"value":"ok"}`)
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := "0123456789abcdef0123456789abcdef"
	first := signedQQBotRequest(http.MethodPost, "/api/v1/integrations/qqbot/test", body, cfg.QQBotIntegration, timestamp, nonce)
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, first)
	require.Equal(t, http.StatusOK, firstResponse.Code)

	replay := signedQQBotRequest(http.MethodPost, "/api/v1/integrations/qqbot/test", body, cfg.QQBotIntegration, timestamp, nonce)
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replay)
	require.Equal(t, http.StatusConflict, replayResponse.Code)
	require.Contains(t, replayResponse.Body.String(), "QQBOT_REQUEST_REPLAYED")
}

func TestQQBotHMACMiddlewareRejectsInvalidSignatureAndExpiredTimestamp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{QQBotIntegration: config.QQBotIntegrationConfig{
		Enabled: true, KeyID: "primary", HMACSecret: strings.Repeat("s", 32),
		TimestampToleranceSeconds: 60, NonceTTLSeconds: 120,
	}}
	router := gin.New()
	router.Use(NewQQBotHMACMiddleware(cfg, rdb))
	router.POST("/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	invalid := signedQQBotRequest(http.MethodPost, "/test", nil, cfg.QQBotIntegration, strconv.FormatInt(time.Now().Unix(), 10), "aaaaaaaaaaaaaaaa")
	invalid.Header.Set(QQBotHeaderSignature, strings.Repeat("0", 64))
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalid)
	require.Equal(t, http.StatusUnauthorized, invalidResponse.Code)
	require.Contains(t, invalidResponse.Body.String(), "QQBOT_SIGNATURE_INVALID")

	expiredTimestamp := strconv.FormatInt(time.Now().Add(-2*time.Minute).Unix(), 10)
	expired := signedQQBotRequest(http.MethodPost, "/test", nil, cfg.QQBotIntegration, expiredTimestamp, "bbbbbbbbbbbbbbbb")
	expiredResponse := httptest.NewRecorder()
	router.ServeHTTP(expiredResponse, expired)
	require.Equal(t, http.StatusUnauthorized, expiredResponse.Code)
	require.Contains(t, expiredResponse.Body.String(), "QQBOT_TIMESTAMP_EXPIRED")
}

func signedQQBotRequest(method, requestURI string, body []byte, cfg config.QQBotIntegrationConfig, timestamp, nonce string) *http.Request {
	request := httptest.NewRequest(method, requestURI, bytes.NewReader(body))
	request.Header.Set(QQBotHeaderKeyID, cfg.KeyID)
	request.Header.Set(QQBotHeaderTimestamp, timestamp)
	request.Header.Set(QQBotHeaderNonce, nonce)
	request.Header.Set(QQBotHeaderSignature, qqBotTestSignature(method, request.URL.RequestURI(), timestamp, nonce, body, cfg.HMACSecret))
	return request
}

func qqBotTestSignature(method, requestURI, timestamp, nonce string, body []byte, secret string) string {
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{strings.ToUpper(method), requestURI, timestamp, nonce, hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}
