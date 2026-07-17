package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	wechatPaymentMPAccessTokenURL = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatPaymentOAuthHTTPTimeout = 8 * time.Second
	wechatPaymentOAuthMaxBody     = 64 << 10

	PaymentClientEnvironmentOther  = "other"
	PaymentClientEnvironmentWeChat = "wechat"
	PaymentClientEnvironmentWeCom  = "wecom"
)

var wechatPaymentJSAPIList = []string{"chooseWXPay"}

type WeChatPaymentOAuthApp struct {
	InstanceID  string
	ProviderKey string
	AuthType    string
	AppID       string
	Secret      string
	AgentID     string
}

// WeChatPaymentOAuthService is the narrow payment-only OAuth boundary used by
// AuthHandler and PaymentService. It never exposes stored provider configuration
// maps, merchant payment keys, or upstream response bodies.
type WeChatPaymentOAuthService struct {
	entClient     *dbent.Client
	configService *PaymentConfigService
	settingSvc    *SettingService
	weComClient   WeComOAuthClient
	resumeService *PaymentResumeService
	httpClient    *http.Client
}

func NewWeChatPaymentOAuthService(
	entClient *dbent.Client,
	configService *PaymentConfigService,
	settingSvc *SettingService,
	weComClient WeComOAuthClient,
) *WeChatPaymentOAuthService {
	client := &http.Client{
		Timeout: wechatPaymentOAuthHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &WeChatPaymentOAuthService{
		entClient:     entClient,
		configService: configService,
		settingSvc:    settingSvc,
		weComClient:   weComClient,
		resumeService: psNewPaymentResumeService(configService),
		httpClient:    client,
	}
}

func (s *WeChatPaymentOAuthService) CreateContextToken(claims WeChatPaymentOAuthContextClaims) (string, error) {
	if s == nil || s.resumeService == nil {
		return "", infraerrors.ServiceUnavailable("WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED", "wechat payment oauth service is not configured")
	}
	return s.resumeService.CreateWeChatPaymentOAuthContextToken(claims)
}

func (s *WeChatPaymentOAuthService) ParseContextToken(token string) (*WeChatPaymentOAuthContextClaims, error) {
	if s == nil || s.resumeService == nil {
		return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED", "wechat payment oauth service is not configured")
	}
	return s.resumeService.ParseWeChatPaymentOAuthContextToken(strings.TrimSpace(token))
}

func (s *WeChatPaymentOAuthService) ParseResumeToken(token string) (*WeChatPaymentResumeClaims, error) {
	if s == nil || s.resumeService == nil {
		return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED", "wechat payment oauth service is not configured")
	}
	return s.resumeService.ParseWeChatPaymentResumeToken(strings.TrimSpace(token))
}

func (s *WeChatPaymentOAuthService) CreateResumeToken(claims *WeChatPaymentOAuthContextClaims, openID, scope string) (string, error) {
	if claims == nil {
		return "", infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context is missing")
	}
	if s == nil || s.resumeService == nil {
		return "", infraerrors.ServiceUnavailable("WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED", "wechat payment oauth service is not configured")
	}
	return s.resumeService.CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		UserID:             claims.UserID,
		ProviderInstanceID: claims.ProviderInstanceID,
		ProviderKey:        claims.ProviderKey,
		AuthType:           claims.AuthType,
		JSAPIAppID:         claims.JSAPIAppID,
		OpenID:             strings.TrimSpace(openID),
		PaymentType:        claims.PaymentType,
		Amount:             claims.Amount,
		OrderType:          claims.OrderType,
		PlanID:             claims.PlanID,
		RedirectTo:         claims.RedirectTo,
		Scope:              strings.TrimSpace(scope),
		WeChatPageURL:      claims.WeChatPageURL,
	})
}

func (s *WeChatPaymentOAuthService) ResolveContextApp(ctx context.Context, claims *WeChatPaymentOAuthContextClaims) (*WeChatPaymentOAuthApp, error) {
	if claims == nil {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth context is missing")
	}
	return s.resolveBoundApp(ctx, claims.ProviderInstanceID, claims.ProviderKey, claims.AuthType, claims.JSAPIAppID)
}

func (s *WeChatPaymentOAuthService) ValidateResumeBinding(ctx context.Context, claims *WeChatPaymentResumeClaims) (*WeChatPaymentOAuthApp, error) {
	if claims == nil {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume context is missing")
	}
	if claims.Legacy {
		return nil, nil
	}
	return s.resolveBoundApp(ctx, claims.ProviderInstanceID, claims.ProviderKey, claims.AuthType, claims.JSAPIAppID)
}

func (s *WeChatPaymentOAuthService) resolveBoundApp(ctx context.Context, instanceID, providerKey, authType, appID string) (*WeChatPaymentOAuthApp, error) {
	instanceID = strings.TrimSpace(instanceID)
	providerKey = strings.TrimSpace(providerKey)
	authType = strings.ToLower(strings.TrimSpace(authType))
	appID = strings.TrimSpace(appID)
	metadata := map[string]string{"instance_id": instanceID, "auth_type": authType}
	if s == nil || s.entClient == nil || s.configService == nil {
		return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED", "wechat payment oauth service is not configured").WithMetadata(metadata)
	}
	id, err := strconv.ParseInt(instanceID, 10, 64)
	if err != nil || id <= 0 || providerKey != payment.TypeWxpay || (authType != provider.WxpayJSAPIAuthTypeMP && authType != provider.WxpayJSAPIAuthTypeWeCom) || appID == "" {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth binding is invalid").WithMetadata(metadata)
	}

	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_INSTANCE_UNAVAILABLE", "the selected WeChat payment instance is no longer available").WithMetadata(metadata)
		}
		return nil, infraerrors.InternalServer("WECHAT_PAYMENT_INSTANCE_LOOKUP_FAILED", "failed to load the selected WeChat payment instance").WithMetadata(metadata).WithCause(err)
	}
	if !inst.Enabled || strings.TrimSpace(inst.ProviderKey) != providerKey || !payment.InstanceSupportsType(inst.SupportedTypes, payment.TypeWxpay) {
		return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_INSTANCE_UNAVAILABLE", "the selected WeChat payment instance is no longer available").WithMetadata(metadata)
	}
	config, err := s.configService.decryptConfig(inst.Config)
	if err != nil || len(config) == 0 {
		return nil, infraerrors.Conflict("WECHAT_PAYMENT_INSTANCE_CHANGED", "the selected WeChat payment configuration has changed").WithMetadata(metadata)
	}
	capabilities, err := provider.InspectWxpayCapabilities(config)
	if err != nil || !capabilities.JSAPIEnabled {
		return nil, infraerrors.Conflict("WECHAT_PAYMENT_INSTANCE_CHANGED", "the selected WeChat payment configuration has changed").WithMetadata(metadata)
	}
	if provider.ResolveWxpayJSAPIAuthType(config) != authType || provider.ResolveWxpayJSAPIAppID(config) != appID {
		return nil, infraerrors.Conflict("WECHAT_PAYMENT_INSTANCE_CHANGED", "the selected WeChat payment OAuth application has changed").WithMetadata(metadata)
	}

	app := &WeChatPaymentOAuthApp{
		InstanceID:  instanceID,
		ProviderKey: providerKey,
		AuthType:    authType,
		AppID:       appID,
	}
	switch authType {
	case provider.WxpayJSAPIAuthTypeMP:
		if s.settingSvc == nil {
			return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_MP_NOT_CONFIGURED", "wechat official account payment oauth is not configured").WithMetadata(metadata)
		}
		mp, cfgErr := s.settingSvc.GetWeChatConnectOAuthConfig(ctx)
		secret := strings.TrimSpace(mp.AppSecretForMode("mp"))
		if cfgErr != nil || !mp.SupportsMode("mp") || strings.TrimSpace(mp.AppIDForMode("mp")) != appID || secret == "" {
			return nil, infraerrors.ServiceUnavailable("WECHAT_PAYMENT_MP_APP_MISMATCH", "the selected payment instance does not match the configured WeChat official account").WithMetadata(metadata)
		}
		app.Secret = secret
	case provider.WxpayJSAPIAuthTypeWeCom:
		secret := strings.TrimSpace(config["wecomAppSecret"])
		if secret == "" {
			return nil, infraerrors.Conflict("WECHAT_PAYMENT_INSTANCE_CHANGED", "the selected enterprise WeChat payment configuration has changed").WithMetadata(metadata)
		}
		app.Secret = secret
		app.AgentID = strings.TrimSpace(config["wecomAgentId"])
	}
	return app, nil
}

func (s *WeChatPaymentOAuthService) BuildAuthorizeURL(app *WeChatPaymentOAuthApp, redirectURI, state string) (string, error) {
	if app == nil || strings.TrimSpace(app.AppID) == "" || strings.TrimSpace(redirectURI) == "" || strings.TrimSpace(state) == "" {
		return "", infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth authorization context is invalid")
	}
	endpoint, err := url.Parse("https://open.weixin.qq.com/connect/oauth2/authorize")
	if err != nil {
		return "", infraerrors.InternalServer("WECHAT_PAYMENT_OAUTH_URL_FAILED", "failed to build WeChat payment authorization URL")
	}
	query := endpoint.Query()
	query.Set("appid", strings.TrimSpace(app.AppID))
	query.Set("redirect_uri", strings.TrimSpace(redirectURI))
	query.Set("response_type", "code")
	query.Set("scope", "snsapi_base")
	query.Set("state", strings.TrimSpace(state))
	if app.AuthType == provider.WxpayJSAPIAuthTypeWeCom && strings.TrimSpace(app.AgentID) != "" {
		query.Set("agentid", strings.TrimSpace(app.AgentID))
	}
	endpoint.RawQuery = query.Encode()
	endpoint.Fragment = "wechat_redirect"
	return endpoint.String(), nil
}

func (s *WeChatPaymentOAuthService) ResolveOpenID(ctx context.Context, app *WeChatPaymentOAuthApp, code string) (string, error) {
	if app == nil {
		return "", infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT", "wechat payment oauth application is missing")
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 1024 {
		return "", infraerrors.BadRequest("WECHAT_PAYMENT_OAUTH_CODE_INVALID", "wechat payment oauth code is invalid")
	}
	if app.AuthType == provider.WxpayJSAPIAuthTypeWeCom {
		if s == nil || s.weComClient == nil {
			return "", infraerrors.ServiceUnavailable("WECOM_OAUTH_NOT_CONFIGURED", "enterprise WeChat payment oauth is not configured")
		}
		identity, err := s.weComClient.ResolveOpenID(ctx, WeComOAuthAppCredentials{
			InstanceID: app.InstanceID,
			CorpID:     app.AppID,
			Secret:     app.Secret,
		}, code)
		if err != nil {
			return "", sanitizeWeChatPaymentOAuthUpstreamError(err, app)
		}
		openID := strings.TrimSpace(identity.OpenID)
		if openID == "" {
			return "", infraerrors.New(http.StatusBadGateway, "WECOM_OAUTH_IDENTITY_INVALID", "enterprise WeChat returned an invalid payment identity")
		}
		return openID, nil
	}
	return s.resolveMPOpenID(ctx, app, code)
}

func (s *WeChatPaymentOAuthService) resolveMPOpenID(ctx context.Context, app *WeChatPaymentOAuthApp, code string) (string, error) {
	if s == nil || s.httpClient == nil {
		return "", infraerrors.ServiceUnavailable("WECHAT_PAYMENT_MP_NOT_CONFIGURED", "wechat official account payment oauth is not configured")
	}
	endpoint, err := url.Parse(wechatPaymentMPAccessTokenURL)
	if err != nil {
		return "", infraerrors.InternalServer("WECHAT_PAYMENT_MP_REQUEST_FAILED", "failed to build WeChat official account oauth request")
	}
	query := endpoint.Query()
	query.Set("appid", app.AppID)
	query.Set("secret", app.Secret)
	query.Set("code", code)
	query.Set("grant_type", "authorization_code")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", infraerrors.InternalServer("WECHAT_PAYMENT_MP_REQUEST_FAILED", "failed to build WeChat official account oauth request")
	}
	request.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return "", infraerrors.New(http.StatusGatewayTimeout, "WECHAT_PAYMENT_MP_TIMEOUT", "WeChat official account oauth request timed out")
		}
		return "", infraerrors.New(http.StatusBadGateway, "WECHAT_PAYMENT_MP_UPSTREAM_UNAVAILABLE", "WeChat official account oauth request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, wechatPaymentOAuthMaxBody+1))
	if err != nil || len(body) > wechatPaymentOAuthMaxBody {
		return "", infraerrors.New(http.StatusBadGateway, "WECHAT_PAYMENT_MP_INVALID_RESPONSE", "WeChat official account returned an invalid oauth response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", infraerrors.New(http.StatusBadGateway, "WECHAT_PAYMENT_MP_UPSTREAM_ERROR", "WeChat official account oauth request failed")
	}
	var payload struct {
		OpenID  string `json:"openid"`
		ErrCode int64  `json:"errcode"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.ErrCode != 0 || strings.TrimSpace(payload.OpenID) == "" {
		return "", infraerrors.New(http.StatusBadGateway, "WECHAT_PAYMENT_MP_INVALID_RESPONSE", "WeChat official account returned an invalid oauth identity")
	}
	return strings.TrimSpace(payload.OpenID), nil
}

func sanitizeWeChatPaymentOAuthUpstreamError(err error, app *WeChatPaymentOAuthApp) error {
	if err == nil {
		return nil
	}
	status := http.StatusBadGateway
	reason := "WECHAT_PAYMENT_OAUTH_UPSTREAM_ERROR"
	message := "wechat payment oauth request failed"
	if applicationError := infraerrors.FromError(err); applicationError != nil && applicationError.Reason != "" {
		status = int(applicationError.Code)
		reason = applicationError.Reason
		message = applicationError.Message
	}
	metadata := map[string]string{}
	if app != nil {
		metadata["instance_id"] = strings.TrimSpace(app.InstanceID)
		metadata["auth_type"] = strings.TrimSpace(app.AuthType)
	}
	return infraerrors.New(status, reason, message).WithMetadata(metadata)
}

func (s *WeChatPaymentOAuthService) BuildJSConfig(ctx context.Context, claims *WeChatPaymentResumeClaims) (*payment.WechatJSConfig, error) {
	if claims == nil || claims.Legacy || claims.AuthType != provider.WxpayJSAPIAuthTypeWeCom {
		return nil, nil
	}
	app, err := s.ValidateResumeBinding(ctx, claims)
	if err != nil {
		return nil, err
	}
	if s == nil || s.weComClient == nil {
		return nil, infraerrors.ServiceUnavailable("WECOM_OAUTH_NOT_CONFIGURED", "enterprise WeChat payment oauth is not configured")
	}
	config, err := s.weComClient.BuildJSConfig(ctx, WeComOAuthAppCredentials{
		InstanceID: app.InstanceID,
		CorpID:     app.AppID,
		Secret:     app.Secret,
	}, claims.WeChatPageURL)
	if err != nil {
		return nil, sanitizeWeChatPaymentOAuthUpstreamError(err, app)
	}
	return &payment.WechatJSConfig{
		AppID:     config.AppID,
		Timestamp: config.Timestamp,
		NonceStr:  config.NonceStr,
		Signature: config.Signature,
		JSAPIList: append([]string(nil), wechatPaymentJSAPIList...),
	}, nil
}

func ValidatePaymentClientEnvironment(authType, environment string) error {
	authType = strings.ToLower(strings.TrimSpace(authType))
	environment = strings.ToLower(strings.TrimSpace(environment))
	valid := (authType == provider.WxpayJSAPIAuthTypeWeCom && environment == PaymentClientEnvironmentWeCom) ||
		(authType == provider.WxpayJSAPIAuthTypeMP && environment == PaymentClientEnvironmentWeChat)
	if valid {
		return nil
	}
	return infraerrors.BadRequest("WECHAT_PAYMENT_CLIENT_ENVIRONMENT_INVALID", "wechat payment oauth must run in the matching built-in browser").
		WithMetadata(map[string]string{"auth_type": authType, "client_environment": environment})
}

func CanonicalizeWeComPageURL(raw, requestScheme, requestHost, requestOrigin, refererURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 4096 {
		return "", invalidWeComPageURLError()
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", invalidWeComPageURLError()
	}
	if !samePaymentPageOrigin(parsed, requestScheme, requestHost, requestOrigin, refererURL) {
		return "", infraerrors.BadRequest("WECOM_PAYMENT_PAGE_URL_ORIGIN_MISMATCH", "enterprise WeChat page URL must use the current site origin")
	}
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func samePaymentPageOrigin(page *url.URL, requestScheme, requestHost, requestOrigin, refererURL string) bool {
	if page == nil {
		return false
	}
	requestURL := &url.URL{Scheme: strings.ToLower(strings.TrimSpace(requestScheme)), Host: strings.TrimSpace(requestHost)}
	if requestURL.Scheme == "https" && requestURL.Host != "" && sameURLOrigin(page, requestURL) {
		return true
	}

	requestOrigin = strings.TrimSpace(requestOrigin)
	if requestOrigin != "" {
		origin, err := url.Parse(requestOrigin)
		if err != nil || !origin.IsAbs() || origin.User != nil || origin.Opaque != "" || origin.Host == "" || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
			return false
		}
		return sameURLOrigin(page, origin)
	}

	referer, err := url.Parse(strings.TrimSpace(refererURL))
	if err != nil || !referer.IsAbs() || referer.User != nil || referer.Opaque != "" || referer.Host == "" {
		return false
	}
	return sameURLOrigin(page, referer)
}

func sameURLOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(strings.TrimSpace(left.Scheme), strings.TrimSpace(right.Scheme)) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectiveOriginPort(left) == effectiveOriginPort(right)
}

func effectiveOriginPort(value *url.URL) string {
	if value == nil {
		return ""
	}
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(strings.TrimSpace(value.Scheme)) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

func invalidWeComPageURLError() error {
	return infraerrors.BadRequest("WECOM_PAYMENT_PAGE_URL_INVALID", "enterprise WeChat page URL must be an absolute HTTPS URL without userinfo")
}

func (a WeChatPaymentOAuthApp) String() string {
	return fmt.Sprintf("wechat payment oauth app(instance=%s, auth_type=%s)", a.InstanceID, a.AuthType)
}
