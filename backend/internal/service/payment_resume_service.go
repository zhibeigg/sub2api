package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const paymentResultReturnPath = "/payment/result"

const (
	PaymentSourceHostedRedirect    = "hosted_redirect"
	PaymentSourceWechatInAppResume = "wechat_in_app_resume"

	SettingPaymentVisibleMethodAlipaySource  = "payment_visible_method_alipay_source"
	SettingPaymentVisibleMethodWxpaySource   = "payment_visible_method_wxpay_source"
	SettingPaymentVisibleMethodQQPaySource   = "payment_visible_method_qqpay_source"
	SettingPaymentVisibleMethodAlipayEnabled = "payment_visible_method_alipay_enabled"
	SettingPaymentVisibleMethodWxpayEnabled  = "payment_visible_method_wxpay_enabled"
	SettingPaymentVisibleMethodQQPayEnabled  = "payment_visible_method_qqpay_enabled"

	VisibleMethodSourceOfficialAlipay = "official_alipay"
	VisibleMethodSourceEasyPayAlipay  = "easypay_alipay"
	VisibleMethodSourceOfficialWechat = "official_wxpay"
	VisibleMethodSourceEasyPayWechat  = "easypay_wxpay"
	VisibleMethodSourceEasyPayQQPay   = "easypay_qqpay"

	wechatPaymentResumeTokenType       = "wechat_payment_resume"
	wechatPaymentOAuthContextTokenType = "wechat_payment_oauth_context"
	wechatPaymentClaimsVersionV2       = 2

	paymentResumeNotConfiguredCode    = "PAYMENT_RESUME_NOT_CONFIGURED"
	paymentResumeNotConfiguredMessage = "payment resume tokens require a configured signing key"

	paymentResumeTokenTTL          = 24 * time.Hour
	wechatPaymentResumeTokenTTL    = 15 * time.Minute
	wechatPaymentOAuthContextTTL   = 10 * time.Minute
	paymentTokenClockSkewTolerance = time.Minute
)

type ResumeTokenClaims struct {
	OrderID            int64  `json:"oid"`
	UserID             int64  `json:"uid,omitempty"`
	ProviderInstanceID string `json:"pi,omitempty"`
	ProviderKey        string `json:"pk,omitempty"`
	PaymentType        string `json:"pt,omitempty"`
	CanonicalReturnURL string `json:"ru,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp,omitempty"`
}

type WeChatPaymentOAuthContextClaims struct {
	TokenType          string `json:"tk,omitempty"`
	Version            int    `json:"v,omitempty"`
	UserID             int64  `json:"uid"`
	ProviderInstanceID string `json:"pi"`
	ProviderKey        string `json:"pk"`
	AuthType           string `json:"at"`
	JSAPIAppID         string `json:"aid"`
	PaymentType        string `json:"pt"`
	Amount             string `json:"amt,omitempty"`
	OrderType          string `json:"ot,omitempty"`
	PlanID             int64  `json:"pid,omitempty"`
	RedirectTo         string `json:"rd,omitempty"`
	WeChatPageURL      string `json:"pu,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp"`
}

type WeChatPaymentResumeClaims struct {
	TokenType          string `json:"tk,omitempty"`
	Version            int    `json:"v,omitempty"`
	UserID             int64  `json:"uid,omitempty"`
	ProviderInstanceID string `json:"pi,omitempty"`
	ProviderKey        string `json:"pk,omitempty"`
	AuthType           string `json:"at,omitempty"`
	JSAPIAppID         string `json:"aid,omitempty"`
	OpenID             string `json:"openid"`
	PaymentType        string `json:"pt,omitempty"`
	Amount             string `json:"amt,omitempty"`
	OrderType          string `json:"ot,omitempty"`
	PlanID             int64  `json:"pid,omitempty"`
	RedirectTo         string `json:"rd,omitempty"`
	Scope              string `json:"scp,omitempty"`
	WeChatPageURL      string `json:"pu,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp,omitempty"`
	Legacy             bool   `json:"-"`
}

type PaymentResumeService struct {
	signingKey []byte
	verifyKeys [][]byte
}

type visibleMethodLoadBalancer struct {
	inner         payment.LoadBalancer
	configService *PaymentConfigService
}

func NewPaymentResumeService(signingKey []byte, verifyFallbacks ...[]byte) *PaymentResumeService {
	svc := &PaymentResumeService{}
	if len(signingKey) > 0 {
		svc.signingKey = append([]byte(nil), signingKey...)
		svc.verifyKeys = append(svc.verifyKeys, svc.signingKey)
	}
	for _, fallback := range verifyFallbacks {
		if len(fallback) == 0 {
			continue
		}
		cloned := append([]byte(nil), fallback...)
		duplicate := false
		for _, existing := range svc.verifyKeys {
			if bytes.Equal(existing, cloned) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			svc.verifyKeys = append(svc.verifyKeys, cloned)
		}
	}
	return svc
}

func (s *PaymentResumeService) isSigningConfigured() bool {
	return s != nil && len(s.signingKey) > 0
}

func (s *PaymentResumeService) ensureSigningKey() error {
	if s.isSigningConfigured() {
		return nil
	}
	return infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
}

func NormalizeVisibleMethod(method string) string {
	return payment.GetBasePaymentType(strings.TrimSpace(method))
}

func NormalizeVisibleMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(methods))
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		normalized := NormalizeVisibleMethod(method)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func NormalizePaymentSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "", PaymentSourceHostedRedirect:
		return PaymentSourceHostedRedirect
	case "wechat_in_app", "wxpay_resume", PaymentSourceWechatInAppResume:
		return PaymentSourceWechatInAppResume
	default:
		return strings.TrimSpace(strings.ToLower(source))
	}
}

func NormalizeVisibleMethodSource(method, source string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialAlipay, payment.TypeAlipay, payment.TypeAlipayDirect, "official":
			return VisibleMethodSourceOfficialAlipay
		case VisibleMethodSourceEasyPayAlipay, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayAlipay
		}
	case payment.TypeWxpay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialWechat, payment.TypeWxpay, payment.TypeWxpayDirect, "wechat", "official":
			return VisibleMethodSourceOfficialWechat
		case VisibleMethodSourceEasyPayWechat, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayWechat
		}
	case payment.TypeQQPay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceEasyPayQQPay, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayQQPay
		}
	}
	return ""
}

func VisibleMethodProviderKeyForSource(method, source string) (string, bool) {
	switch NormalizeVisibleMethodSource(method, source) {
	case VisibleMethodSourceOfficialAlipay:
		return payment.TypeAlipay, NormalizeVisibleMethod(method) == payment.TypeAlipay
	case VisibleMethodSourceEasyPayAlipay:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeAlipay
	case VisibleMethodSourceOfficialWechat:
		return payment.TypeWxpay, NormalizeVisibleMethod(method) == payment.TypeWxpay
	case VisibleMethodSourceEasyPayWechat:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeWxpay
	case VisibleMethodSourceEasyPayQQPay:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeQQPay
	default:
		return "", false
	}
}

func newVisibleMethodLoadBalancer(inner payment.LoadBalancer, configService *PaymentConfigService) payment.LoadBalancer {
	if inner == nil || configService == nil || configService.entClient == nil {
		return inner
	}
	return &visibleMethodLoadBalancer{inner: inner, configService: configService}
}

func (lb *visibleMethodLoadBalancer) GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error) {
	return lb.inner.GetInstanceConfig(ctx, instanceID)
}

func (lb *visibleMethodLoadBalancer) GetInstanceSelection(ctx context.Context, instanceID string) (*payment.InstanceSelection, error) {
	return lb.inner.GetInstanceSelection(ctx, instanceID)
}

func (lb *visibleMethodLoadBalancer) SelectInstance(ctx context.Context, providerKey string, paymentType payment.PaymentType, strategy payment.Strategy, orderAmount float64) (*payment.InstanceSelection, error) {
	visibleMethod := NormalizeVisibleMethod(paymentType)
	if providerKey != "" || (visibleMethod != payment.TypeAlipay && visibleMethod != payment.TypeWxpay && visibleMethod != payment.TypeQQPay) {
		return lb.inner.SelectInstance(ctx, providerKey, paymentType, strategy, orderAmount)
	}

	inst, err := lb.configService.resolveEnabledVisibleMethodInstance(ctx, visibleMethod)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("visible payment method %s has no enabled provider instance", visibleMethod)
	}
	return lb.inner.SelectInstance(ctx, inst.ProviderKey, paymentType, strategy, orderAmount)
}

func visibleMethodEnabledSettingKey(method string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		return SettingPaymentVisibleMethodAlipayEnabled
	case payment.TypeWxpay:
		return SettingPaymentVisibleMethodWxpayEnabled
	case payment.TypeQQPay:
		return SettingPaymentVisibleMethodQQPayEnabled
	default:
		return ""
	}
}

func visibleMethodSourceSettingKey(method string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		return SettingPaymentVisibleMethodAlipaySource
	case payment.TypeWxpay:
		return SettingPaymentVisibleMethodWxpaySource
	case payment.TypeQQPay:
		return SettingPaymentVisibleMethodQQPaySource
	default:
		return ""
	}
}

func CanonicalizeReturnURL(raw string, srcHost string, srcURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be an absolute http/https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use http or https")
	}
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if parsed.Path != paymentResultReturnPath {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must target the canonical internal payment result page")
	}
	if !allowedReturnURLHost(parsed.Host, srcHost, srcURL) {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use the same host as the current site or browser origin")
	}
	return parsed.String(), nil
}

func allowedReturnURLHost(returnURLHost string, requestHost string, refererURL string) bool {
	if sameOriginHost(returnURLHost, requestHost) {
		return true
	}

	refererURL = strings.TrimSpace(refererURL)
	if refererURL == "" {
		return false
	}
	parsedReferer, err := url.Parse(refererURL)
	if err != nil || parsedReferer.Host == "" {
		return false
	}
	return sameOriginHost(returnURLHost, parsedReferer.Host)
}

func buildPaymentReturnURL(base string, orderID int64, outTradeNo string, resumeToken string) (string, error) {
	canonical := strings.TrimSpace(base)
	if canonical == "" {
		return "", nil
	}

	parsed, err := url.Parse(canonical)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid URL")
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid absolute URL")
	}
	parsed.Fragment = ""

	query := parsed.Query()
	if orderID > 0 {
		query.Set("order_id", strconv.FormatInt(orderID, 10))
	}
	if strings.TrimSpace(outTradeNo) != "" {
		query.Set("out_trade_no", strings.TrimSpace(outTradeNo))
	}
	if strings.TrimSpace(resumeToken) != "" {
		query.Set("resume_token", strings.TrimSpace(resumeToken))
	}
	query.Set("status", "success")
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func sameOriginHost(returnURLHost string, requestHost string) bool {
	returnHost := strings.TrimSpace(returnURLHost)
	reqHost := strings.TrimSpace(requestHost)
	if returnHost == "" || reqHost == "" {
		return false
	}
	if strings.EqualFold(returnHost, reqHost) {
		return true
	}

	returnName, returnPort := splitHostPortDefault(returnHost)
	reqName, reqPort := splitHostPortDefault(reqHost)
	if returnName == "" || reqName == "" {
		return false
	}
	return strings.EqualFold(returnName, reqName) && returnPort == reqPort
}

func splitHostPortDefault(raw string) (string, string) {
	if host, port, err := net.SplitHostPort(raw); err == nil {
		return host, port
	}
	return raw, ""
}

func (s *PaymentResumeService) CreateToken(claims ResumeTokenClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
		return "", err
	}
	if claims.OrderID <= 0 {
		return "", fmt.Errorf("resume token requires order id")
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(paymentResumeTokenTTL).Unix()
	}
	return s.createSignedToken(claims)
}

func (s *PaymentResumeService) ParseToken(token string) (*ResumeTokenClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
	}
	var claims ResumeTokenClaims
	if err := s.parseSignedToken(token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is invalid")
	}
	if claims.OrderID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token missing order id")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_RESUME_TOKEN", "resume token has expired"); err != nil {
		return nil, err
	}
	return &claims, nil
}

func (s *PaymentResumeService) CreateWeChatPaymentOAuthContextToken(claims WeChatPaymentOAuthContextClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
		return "", err
	}
	claims.ProviderInstanceID = strings.TrimSpace(claims.ProviderInstanceID)
	claims.ProviderKey = strings.TrimSpace(claims.ProviderKey)
	claims.AuthType = strings.ToLower(strings.TrimSpace(claims.AuthType))
	claims.JSAPIAppID = strings.TrimSpace(claims.JSAPIAppID)
	claims.PaymentType = NormalizeVisibleMethod(claims.PaymentType)
	claims.RedirectTo = strings.TrimSpace(claims.RedirectTo)
	claims.WeChatPageURL = strings.TrimSpace(claims.WeChatPageURL)
	if claims.UserID <= 0 || claims.ProviderInstanceID == "" || claims.ProviderKey == "" || claims.JSAPIAppID == "" || claims.PaymentType != payment.TypeWxpay {
		return "", fmt.Errorf("wechat payment oauth context is incomplete")
	}
	if claims.AuthType != "mp" && claims.AuthType != "wecom" {
		return "", fmt.Errorf("wechat payment oauth context auth type is invalid")
	}
	if claims.AuthType == "wecom" && claims.WeChatPageURL == "" {
		return "", fmt.Errorf("wechat payment oauth context requires page url")
	}
	now := time.Now()
	if claims.IssuedAt == 0 {
		claims.IssuedAt = now.Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = now.Add(wechatPaymentOAuthContextTTL).Unix()
	}
	claims.TokenType = wechatPaymentOAuthContextTokenType
	claims.Version = wechatPaymentClaimsVersionV2
	return s.createSignedToken(claims)
}

func (s *PaymentResumeService) ParseWeChatPaymentOAuthContextToken(token string) (*WeChatPaymentOAuthContextClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
	}
	var claims WeChatPaymentOAuthContextClaims
	if err := s.parseSignedToken(strings.TrimSpace(token), &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context is invalid")
	}
	claims.ProviderInstanceID = strings.TrimSpace(claims.ProviderInstanceID)
	claims.ProviderKey = strings.TrimSpace(claims.ProviderKey)
	claims.AuthType = strings.ToLower(strings.TrimSpace(claims.AuthType))
	claims.JSAPIAppID = strings.TrimSpace(claims.JSAPIAppID)
	claims.PaymentType = NormalizeVisibleMethod(claims.PaymentType)
	claims.RedirectTo = strings.TrimSpace(claims.RedirectTo)
	claims.WeChatPageURL = strings.TrimSpace(claims.WeChatPageURL)
	if claims.TokenType != wechatPaymentOAuthContextTokenType || claims.Version != wechatPaymentClaimsVersionV2 {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context type mismatch")
	}
	if claims.UserID <= 0 || claims.ProviderInstanceID == "" || claims.ProviderKey == "" || claims.JSAPIAppID == "" || claims.PaymentType != payment.TypeWxpay {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context is incomplete")
	}
	if claims.AuthType != "mp" && claims.AuthType != "wecom" {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context auth type is invalid")
	}
	if claims.AuthType == "wecom" && claims.WeChatPageURL == "" {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context page url is missing")
	}
	if claims.IssuedAt <= 0 || time.Now().Add(paymentTokenClockSkewTolerance).Unix() < claims.IssuedAt {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context issued time is invalid")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "WECHAT_PAYMENT_OAUTH_CONTEXT_EXPIRED", "wechat payment oauth context has expired"); err != nil {
		return nil, err
	}
	return &claims, nil
}

func (s *PaymentResumeService) CreateWeChatPaymentResumeToken(claims WeChatPaymentResumeClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
		return "", err
	}
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	claims.ProviderInstanceID = strings.TrimSpace(claims.ProviderInstanceID)
	claims.ProviderKey = strings.TrimSpace(claims.ProviderKey)
	claims.AuthType = strings.ToLower(strings.TrimSpace(claims.AuthType))
	claims.JSAPIAppID = strings.TrimSpace(claims.JSAPIAppID)
	claims.WeChatPageURL = strings.TrimSpace(claims.WeChatPageURL)
	if claims.OpenID == "" {
		return "", fmt.Errorf("wechat payment resume token requires openid")
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(wechatPaymentResumeTokenTTL).Unix()
	}
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
	}
	if claims.PaymentType == "" {
		claims.PaymentType = payment.TypeWxpay
	}
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
	}
	claims.TokenType = wechatPaymentResumeTokenType
	if claims.UserID > 0 || claims.ProviderInstanceID != "" || claims.ProviderKey != "" || claims.AuthType != "" || claims.JSAPIAppID != "" || claims.WeChatPageURL != "" {
		if claims.UserID <= 0 || claims.ProviderInstanceID == "" || claims.ProviderKey == "" || claims.JSAPIAppID == "" {
			return "", fmt.Errorf("wechat payment resume v2 token is incomplete")
		}
		if claims.AuthType != "mp" && claims.AuthType != "wecom" {
			return "", fmt.Errorf("wechat payment resume v2 auth type is invalid")
		}
		if claims.AuthType == "wecom" && claims.WeChatPageURL == "" {
			return "", fmt.Errorf("wechat payment resume v2 requires page url")
		}
		claims.Version = wechatPaymentClaimsVersionV2
	}
	return s.createSignedToken(claims)
}

func (s *PaymentResumeService) ParseWeChatPaymentResumeToken(token string) (*WeChatPaymentResumeClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
	}
	var claims WeChatPaymentResumeClaims
	if err := s.parseSignedToken(token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token payload is invalid")
	}
	if claims.TokenType != wechatPaymentResumeTokenType {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token type mismatch")
	}
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	claims.ProviderInstanceID = strings.TrimSpace(claims.ProviderInstanceID)
	claims.ProviderKey = strings.TrimSpace(claims.ProviderKey)
	claims.AuthType = strings.ToLower(strings.TrimSpace(claims.AuthType))
	claims.JSAPIAppID = strings.TrimSpace(claims.JSAPIAppID)
	claims.WeChatPageURL = strings.TrimSpace(claims.WeChatPageURL)
	if claims.OpenID == "" {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token missing openid")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token has expired"); err != nil {
		return nil, err
	}
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
	}
	if claims.PaymentType == "" {
		claims.PaymentType = payment.TypeWxpay
	}
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
	}
	switch claims.Version {
	case 0:
		// Transitional compatibility for legacy Official Account tokens. They do
		// not carry a user or provider-instance binding and must never be accepted
		// as enterprise WeChat credentials.
		claims.AuthType = "mp"
		claims.Legacy = true
	case wechatPaymentClaimsVersionV2:
		if claims.UserID <= 0 || claims.ProviderInstanceID == "" || claims.ProviderKey == "" || claims.JSAPIAppID == "" {
			return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token binding is incomplete")
		}
		if claims.AuthType != "mp" && claims.AuthType != "wecom" {
			return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token auth type is invalid")
		}
		if claims.AuthType == "wecom" && claims.WeChatPageURL == "" {
			return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token page url is missing")
		}
	default:
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token version is unsupported")
	}
	return &claims, nil
}

func (s *PaymentResumeService) createSignedToken(claims any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resume claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + s.sign(encodedPayload), nil
}

func (s *PaymentResumeService) parseSignedToken(token string, dest any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
	}
	if !s.verifySignature(parts[0], parts[1]) {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is malformed")
	}
	return json.Unmarshal(payload, dest)
}

func (s *PaymentResumeService) verifySignature(payload string, signature string) bool {
	if s == nil {
		return false
	}
	for _, key := range s.verifyKeys {
		if hmac.Equal([]byte(signature), []byte(signPaymentResumePayload(payload, key))) {
			return true
		}
	}
	return false
}

func validatePaymentResumeExpiry(expiresAt int64, code, message string) error {
	if expiresAt <= 0 {
		return nil
	}
	if time.Now().Unix() > expiresAt {
		return infraerrors.BadRequest(code, message)
	}
	return nil
}

func (s *PaymentResumeService) sign(payload string) string {
	return signPaymentResumePayload(payload, s.signingKey)
}

func signPaymentResumePayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
