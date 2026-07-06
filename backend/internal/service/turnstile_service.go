package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrTurnstileVerificationFailed = infraerrors.BadRequest("TURNSTILE_VERIFICATION_FAILED", "captcha verification failed")
	ErrTurnstileNotConfigured      = infraerrors.ServiceUnavailable("TURNSTILE_NOT_CONFIGURED", "captcha not configured")
	ErrTurnstileInvalidSecretKey   = infraerrors.BadRequest("TURNSTILE_INVALID_SECRET_KEY", "invalid captcha secret key")
)

// TurnstileVerifier 验证 Cap token 的接口。
// verifyURL 为完整的 Cap siteverify 地址（如 https://cap.example.com/<siteKey>/siteverify）。
type TurnstileVerifier interface {
	VerifyToken(ctx context.Context, verifyURL, secretKey, token, remoteIP string) (*TurnstileVerifyResponse, error)
}

// TurnstileService Cap 人机验证服务
type TurnstileService struct {
	settingService *SettingService
	verifier       TurnstileVerifier
}

// TurnstileVerifyResponse Cap siteverify 验证响应（兼容 reCAPTCHA 风格字段）
type TurnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
}

// NewTurnstileService 创建 Turnstile 服务实例
func NewTurnstileService(settingService *SettingService, verifier TurnstileVerifier) *TurnstileService {
	return &TurnstileService{
		settingService: settingService,
		verifier:       verifier,
	}
}

// buildVerifyURL 拼接 Cap siteverify 地址：{endpoint}/{siteKey}/siteverify
func buildVerifyURL(endpoint, siteKey string) (string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	siteKey = strings.Trim(strings.TrimSpace(siteKey), "/")
	if endpoint == "" || siteKey == "" {
		return "", ErrTurnstileNotConfigured
	}
	return fmt.Sprintf("%s/%s/siteverify", endpoint, siteKey), nil
}

// VerifyToken 验证 Cap token
func (s *TurnstileService) VerifyToken(ctx context.Context, token string, remoteIP string) error {
	// 检查是否启用人机验证
	if !s.settingService.IsTurnstileEnabled(ctx) {
		logger.LegacyPrintf("service.turnstile", "%s", "[Cap] Disabled, skipping verification")
		return nil
	}

	// 获取 Secret Key
	secretKey := s.settingService.GetTurnstileSecretKey(ctx)
	if secretKey == "" {
		logger.LegacyPrintf("service.turnstile", "%s", "[Cap] Secret key not configured")
		return ErrTurnstileNotConfigured
	}

	// 拼接 Cap 校验地址
	verifyURL, err := buildVerifyURL(s.settingService.GetTurnstileEndpoint(ctx), s.settingService.GetTurnstileSiteKey(ctx))
	if err != nil {
		logger.LegacyPrintf("service.turnstile", "%s", "[Cap] Endpoint or site key not configured")
		return err
	}

	// 如果 token 为空，返回错误
	if token == "" {
		logger.LegacyPrintf("service.turnstile", "%s", "[Cap] Token is empty")
		return ErrTurnstileVerificationFailed
	}

	logger.LegacyPrintf("service.turnstile", "[Cap] Verifying token for IP: %s", remoteIP)
	result, err := s.verifier.VerifyToken(ctx, verifyURL, secretKey, token, remoteIP)
	if err != nil {
		logger.LegacyPrintf("service.turnstile", "[Cap] Request failed: %v", err)
		return fmt.Errorf("send request: %w", err)
	}

	if !result.Success {
		logger.LegacyPrintf("service.turnstile", "[Cap] Verification failed, error codes: %v", result.ErrorCodes)
		return ErrTurnstileVerificationFailed
	}

	logger.LegacyPrintf("service.turnstile", "%s", "[Cap] Verification successful")
	return nil
}

// IsEnabled 检查人机验证是否启用
func (s *TurnstileService) IsEnabled(ctx context.Context) bool {
	return s.settingService.IsTurnstileEnabled(ctx)
}

// ValidateConfig 校验 Cap 配置是否可用（实例可达即视为有效）。
// Cap 没有 invalid-input-secret 语义，因此这里只确认能成功请求到 siteverify 端点。
// endpoint/siteKey 由调用方显式传入（保存前校验，避免读到旧值）。
func (s *TurnstileService) ValidateConfig(ctx context.Context, endpoint, siteKey, secretKey string) error {
	verifyURL, err := buildVerifyURL(endpoint, siteKey)
	if err != nil {
		return err
	}

	// 发送一个测试 token 的验证请求，确认端点可达且 secret 被接受（返回 success=false 属正常）
	if _, err := s.verifier.VerifyToken(ctx, verifyURL, secretKey, "test-validation", ""); err != nil {
		return fmt.Errorf("validate cap config: %w", err)
	}

	return nil
}
