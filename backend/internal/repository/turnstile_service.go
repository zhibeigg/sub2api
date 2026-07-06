package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type turnstileVerifier struct {
	httpClient *http.Client
}

// NewTurnstileVerifier 创建 Cap siteverify 验证器。
func NewTurnstileVerifier() service.TurnstileVerifier {
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:            10 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		sharedClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &turnstileVerifier{
		httpClient: sharedClient,
	}
}

// capVerifyRequest Cap /siteverify 请求体（JSON）。
type capVerifyRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
	RemoteIP string `json:"remoteip,omitempty"`
}

// VerifyToken 向 Cap 实例发送 siteverify 请求。
// verifyURL 形如 https://cap.example.com/<siteKey>/siteverify。
func (v *turnstileVerifier) VerifyToken(ctx context.Context, verifyURL, secretKey, token, remoteIP string) (*service.TurnstileVerifyResponse, error) {
	payload, err := json.Marshal(capVerifyRequest{
		Secret:   secretKey,
		Response: token,
		RemoteIP: remoteIP,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result service.TurnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
