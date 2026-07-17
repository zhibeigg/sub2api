package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	QQBotHeaderKeyID     = "X-QQBot-Key-Id"
	QQBotHeaderTimestamp = "X-QQBot-Timestamp"
	QQBotHeaderNonce     = "X-QQBot-Nonce"
	QQBotHeaderSignature = "X-QQBot-Signature"
	qqBotHMACMaxBody     = 1 << 20
)

// NewQQBotHMACMiddleware authenticates the private QQBot integration API and rejects nonce replay.
func NewQQBotHMACMiddleware(cfg *config.Config, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil || !cfg.QQBotIntegration.Enabled {
			abortQQBotHMAC(c, http.StatusNotFound, "QQBOT_INTEGRATION_DISABLED", "not found")
			return
		}
		keyID := strings.TrimSpace(c.GetHeader(QQBotHeaderKeyID))
		timestampRaw := strings.TrimSpace(c.GetHeader(QQBotHeaderTimestamp))
		nonce := strings.TrimSpace(c.GetHeader(QQBotHeaderNonce))
		signatureRaw := strings.TrimSpace(c.GetHeader(QQBotHeaderSignature))
		if keyID == "" || timestampRaw == "" || nonce == "" || signatureRaw == "" {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_SIGNATURE_REQUIRED", "QQBot signature headers are required")
			return
		}
		if subtle.ConstantTimeCompare([]byte(keyID), []byte(cfg.QQBotIntegration.KeyID)) != 1 {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_KEY_INVALID", "invalid QQBot key")
			return
		}
		if len(nonce) < 16 || len(nonce) > 128 || strings.ContainsAny(nonce, "\r\n\t ") {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_NONCE_INVALID", "invalid QQBot nonce")
			return
		}
		timestamp, err := strconv.ParseInt(timestampRaw, 10, 64)
		if err != nil {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_TIMESTAMP_INVALID", "invalid QQBot timestamp")
			return
		}
		now := time.Now().UTC()
		delta := now.Sub(time.Unix(timestamp, 0).UTC())
		if delta < 0 {
			delta = -delta
		}
		if delta > cfg.QQBotIntegration.TimestampTolerance() {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_TIMESTAMP_EXPIRED", "QQBot timestamp is outside the allowed window")
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, qqBotHMACMaxBody+1))
		if err != nil {
			abortQQBotHMAC(c, http.StatusBadRequest, "QQBOT_BODY_INVALID", "failed to read request body")
			return
		}
		if len(body) > qqBotHMACMaxBody {
			abortQQBotHMAC(c, http.StatusRequestEntityTooLarge, "QQBOT_BODY_TOO_LARGE", "request body is too large")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		providedSignature, err := hex.DecodeString(signatureRaw)
		if err != nil || len(providedSignature) != sha256.Size {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_SIGNATURE_INVALID", "invalid QQBot signature")
			return
		}
		bodyDigest := sha256.Sum256(body)
		canonical := strings.Join([]string{
			strings.ToUpper(c.Request.Method),
			c.Request.URL.RequestURI(),
			timestampRaw,
			nonce,
			hex.EncodeToString(bodyDigest[:]),
		}, "\n")
		mac := hmac.New(sha256.New, []byte(cfg.QQBotIntegration.HMACSecret))
		_, _ = mac.Write([]byte(canonical))
		expectedSignature := mac.Sum(nil)
		if subtle.ConstantTimeCompare(providedSignature, expectedSignature) != 1 {
			abortQQBotHMAC(c, http.StatusUnauthorized, "QQBOT_SIGNATURE_INVALID", "invalid QQBot signature")
			return
		}
		if redisClient == nil {
			abortQQBotHMAC(c, http.StatusServiceUnavailable, "QQBOT_REPLAY_GUARD_UNAVAILABLE", "QQBot replay guard is unavailable")
			return
		}
		nonceKey := "qqbot:hmac:nonce:" + keyID + ":" + nonce
		stored, err := redisClient.SetNX(c.Request.Context(), nonceKey, timestampRaw, cfg.QQBotIntegration.NonceTTL()).Result()
		if err != nil {
			abortQQBotHMAC(c, http.StatusServiceUnavailable, "QQBOT_REPLAY_GUARD_UNAVAILABLE", "QQBot replay guard is unavailable")
			return
		}
		if !stored {
			abortQQBotHMAC(c, http.StatusConflict, "QQBOT_REQUEST_REPLAYED", "QQBot request nonce has already been used")
			return
		}
		c.Set("qqbot_key_id", keyID)
		c.Next()
	}
}

func abortQQBotHMAC(c *gin.Context, status int, reason, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"code":    status,
		"message": message,
		"reason":  reason,
	})
}
