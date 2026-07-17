package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- 企业微信 JS-SDK 协议固定要求 SHA-1。
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

const (
	weComAPIBaseURL           = "https://qyapi.weixin.qq.com"
	weComHTTPTimeout          = 8 * time.Second
	weComMaxResponseBodyBytes = 64 << 10
	weComCacheMaxTTL          = 2 * time.Hour
	weComCacheSafetyWindow    = 5 * time.Minute
)

var weComInstanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// WeComOAuthAppCredentials 是后续 OAuth 编排传入的企业微信自建应用凭据。
// Secret 仅作为输入使用，不会出现在任何返回 DTO 或错误中。
type WeComOAuthAppCredentials struct {
	InstanceID string
	CorpID     string
	Secret     string
}

// WeComOAuthIdentity 是 OAuth code 解析后的最小身份结果。
// 内部成员的 UserID 会先转换为 OpenID，不向上层暴露。
type WeComOAuthIdentity struct {
	OpenID string `json:"openid"`
}

// WeComJSConfig 是调用 wx.config 所需的动态签名字段。
// jsApiList 由上层按业务场景固定，不由本客户端返回。
type WeComJSConfig struct {
	AppID     string `json:"appId"`
	Timestamp int64  `json:"timestamp"`
	NonceStr  string `json:"nonceStr"`
	Signature string `json:"signature"`
}

// WeComOAuthClient 是企业微信支付 OAuth 编排所需的窄接口。
type WeComOAuthClient interface {
	ResolveOpenID(ctx context.Context, app WeComOAuthAppCredentials, code string) (WeComOAuthIdentity, error)
	// BuildJSConfig 只接受已经由调用方完成同源校验的页面 URL；本方法仅移除 fragment 并签名。
	BuildJSConfig(ctx context.Context, app WeComOAuthAppCredentials, pageURL string) (WeComJSConfig, error)
}

// WeComOAuthCache 隔离企业微信 access_token 与 jsapi_ticket 的短期缓存。
// scope 必须由客户端生成，只包含实例 ID 与不可逆配置摘要。
type WeComOAuthCache interface {
	GetAccessToken(ctx context.Context, scope string) (value string, found bool, err error)
	SetAccessToken(ctx context.Context, scope, value string, ttl time.Duration) error
	DeleteAccessToken(ctx context.Context, scope string) error

	GetJSAPITicket(ctx context.Context, scope string) (value string, found bool, err error)
	SetJSAPITicket(ctx context.Context, scope, value string, ttl time.Duration) error
	DeleteJSAPITicket(ctx context.Context, scope string) error
}

type weComOAuthClient struct {
	cache       WeComOAuthCache
	httpClient  *http.Client
	now         func() time.Time
	nonce       func() (string, error)
	tokenGroup  singleflight.Group
	ticketGroup singleflight.Group
}

var _ WeComOAuthClient = (*weComOAuthClient)(nil)

// NewWeComOAuthClient 创建企业微信 OAuth 客户端。
// 注入的 HTTP client 仅复用 Transport；超时与禁止重定向策略始终由本客户端固定。
func NewWeComOAuthClient(cache WeComOAuthCache, httpClient *http.Client) WeComOAuthClient {
	clientCopy := http.Client{}
	if httpClient != nil {
		clientCopy = *httpClient
	}
	clientCopy.Timeout = weComHTTPTimeout
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &weComOAuthClient{
		cache:      cache,
		httpClient: &clientCopy,
		now:        time.Now,
		nonce:      newWeComNonce,
	}
}

func (c *weComOAuthClient) ResolveOpenID(ctx context.Context, app WeComOAuthAppCredentials, code string) (WeComOAuthIdentity, error) {
	app, err := normalizeWeComOAuthApp(app)
	if err != nil {
		return WeComOAuthIdentity{}, err
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 1024 {
		return WeComOAuthIdentity{}, infraerrors.BadRequest("WECOM_OAUTH_CODE_INVALID", "enterprise WeChat OAuth code is invalid")
	}
	if err := c.validateConfigured(); err != nil {
		return WeComOAuthIdentity{}, err
	}

	token, err := c.getAccessToken(ctx, app)
	if err != nil {
		return WeComOAuthIdentity{}, mapWeComOAuthError(err)
	}

	identity, err := c.resolveOpenIDWithToken(ctx, token, code)
	if !isWeComTokenInvalidError(err) {
		if err != nil {
			return WeComOAuthIdentity{}, mapWeComOAuthError(err)
		}
		return identity, nil
	}

	token, err = c.refreshAccessToken(ctx, app, token)
	if err != nil {
		return WeComOAuthIdentity{}, mapWeComOAuthError(err)
	}
	identity, err = c.resolveOpenIDWithToken(ctx, token, code)
	if err != nil {
		return WeComOAuthIdentity{}, mapWeComOAuthError(err)
	}
	return identity, nil
}

func (c *weComOAuthClient) BuildJSConfig(ctx context.Context, app WeComOAuthAppCredentials, pageURL string) (WeComJSConfig, error) {
	app, err := normalizeWeComOAuthApp(app)
	if err != nil {
		return WeComJSConfig{}, err
	}
	pageURL, err = normalizeWeComPageURL(pageURL)
	if err != nil {
		return WeComJSConfig{}, err
	}
	if err := c.validateConfigured(); err != nil {
		return WeComJSConfig{}, err
	}

	ticket, err := c.getJSAPITicket(ctx, app)
	if err != nil {
		return WeComJSConfig{}, mapWeComOAuthError(err)
	}
	nonceStr, err := c.nonce()
	if err != nil || strings.TrimSpace(nonceStr) == "" {
		return WeComJSConfig{}, infraerrors.New(http.StatusInternalServerError, "WECOM_NONCE_GENERATION_FAILED", "failed to generate enterprise WeChat JS-SDK nonce")
	}
	timestamp := c.now().Unix()
	if timestamp <= 0 {
		return WeComJSConfig{}, infraerrors.New(http.StatusInternalServerError, "WECOM_CLOCK_INVALID", "enterprise WeChat JS-SDK clock is invalid")
	}

	return WeComJSConfig{
		AppID:     app.CorpID,
		Timestamp: timestamp,
		NonceStr:  nonceStr,
		Signature: signWeComJSConfig(ticket, pageURL, timestamp, nonceStr),
	}, nil
}

func (c *weComOAuthClient) validateConfigured() error {
	if c == nil || c.cache == nil || c.httpClient == nil || c.now == nil || c.nonce == nil {
		return infraerrors.New(http.StatusServiceUnavailable, "WECOM_OAUTH_NOT_CONFIGURED", "enterprise WeChat OAuth client is not configured")
	}
	return nil
}

func normalizeWeComOAuthApp(app WeComOAuthAppCredentials) (WeComOAuthAppCredentials, error) {
	app.InstanceID = strings.TrimSpace(app.InstanceID)
	app.CorpID = strings.TrimSpace(app.CorpID)
	app.Secret = strings.TrimSpace(app.Secret)
	if !weComInstanceIDPattern.MatchString(app.InstanceID) || app.CorpID == "" || len(app.CorpID) > 128 || app.Secret == "" || len(app.Secret) > 512 {
		return WeComOAuthAppCredentials{}, infraerrors.BadRequest("WECOM_OAUTH_CONFIG_INVALID", "enterprise WeChat OAuth configuration is invalid")
	}
	return app, nil
}

func normalizeWeComPageURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 4096 {
		return "", infraerrors.BadRequest("WECOM_JS_CONFIG_URL_INVALID", "enterprise WeChat JS-SDK page URL is invalid")
	}
	if fragment := strings.IndexByte(raw, '#'); fragment >= 0 {
		raw = raw[:fragment]
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", infraerrors.BadRequest("WECOM_JS_CONFIG_URL_INVALID", "enterprise WeChat JS-SDK page URL is invalid")
	}
	return raw, nil
}

func weComOAuthCacheScope(app WeComOAuthAppCredentials) string {
	digest := sha256.Sum256([]byte("wecom-oauth-v1\x00" + app.CorpID + "\x00" + app.Secret))
	return app.InstanceID + ":" + hex.EncodeToString(digest[:])
}

func (c *weComOAuthClient) getAccessToken(ctx context.Context, app WeComOAuthAppCredentials) (string, error) {
	scope := weComOAuthCacheScope(app)
	if token, found, err := c.readCachedAccessToken(ctx, scope); err != nil {
		return "", err
	} else if found {
		return token, nil
	}

	value, err, _ := c.tokenGroup.Do(scope, func() (any, error) {
		if token, found, cacheErr := c.readCachedAccessToken(ctx, scope); cacheErr != nil {
			return "", cacheErr
		} else if found {
			return token, nil
		}
		return c.fetchAndCacheAccessToken(ctx, app, scope)
	})
	return weComTokenFlightResult(value, err)
}

func (c *weComOAuthClient) refreshAccessToken(ctx context.Context, app WeComOAuthAppCredentials, rejectedToken string) (string, error) {
	scope := weComOAuthCacheScope(app)
	rejectedToken = strings.TrimSpace(rejectedToken)
	value, err, _ := c.tokenGroup.Do(scope, func() (any, error) {
		currentToken, found, cacheErr := c.readCachedAccessToken(ctx, scope)
		if cacheErr != nil {
			return "", cacheErr
		}
		if found && currentToken != rejectedToken {
			return currentToken, nil
		}
		if found {
			if cacheErr := c.cache.DeleteAccessToken(ctx, scope); cacheErr != nil {
				return "", weComCacheUnavailableError()
			}
		}
		return c.fetchAndCacheAccessToken(ctx, app, scope)
	})
	return weComTokenFlightResult(value, err)
}

func (c *weComOAuthClient) readCachedAccessToken(ctx context.Context, scope string) (string, bool, error) {
	token, found, err := c.cache.GetAccessToken(ctx, scope)
	if err != nil {
		return "", false, weComCacheUnavailableError()
	}
	token = strings.TrimSpace(token)
	return token, found && token != "", nil
}

func (c *weComOAuthClient) fetchAndCacheAccessToken(ctx context.Context, app WeComOAuthAppCredentials, scope string) (string, error) {
	token, expiresIn, err := c.fetchAccessToken(ctx, app)
	if err != nil {
		return "", err
	}
	ttl := weComCacheTTL(expiresIn)
	if ttl <= 0 {
		return "", weComInvalidResponseError("access token")
	}
	if err := c.cache.SetAccessToken(ctx, scope, token, ttl); err != nil {
		return "", weComCacheUnavailableError()
	}
	return token, nil
}

func weComTokenFlightResult(value any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	token, ok := value.(string)
	token = strings.TrimSpace(token)
	if !ok || token == "" {
		return "", weComInvalidResponseError("access token")
	}
	return token, nil
}

func (c *weComOAuthClient) getJSAPITicket(ctx context.Context, app WeComOAuthAppCredentials) (string, error) {
	scope := weComOAuthCacheScope(app)
	if ticket, found, err := c.cache.GetJSAPITicket(ctx, scope); err != nil {
		return "", weComCacheUnavailableError()
	} else if ticket = strings.TrimSpace(ticket); found && ticket != "" {
		return ticket, nil
	}

	value, err, _ := c.ticketGroup.Do(scope, func() (any, error) {
		if ticket, found, cacheErr := c.cache.GetJSAPITicket(ctx, scope); cacheErr != nil {
			return "", weComCacheUnavailableError()
		} else if ticket = strings.TrimSpace(ticket); found && ticket != "" {
			return ticket, nil
		}

		ticket, expiresIn, fetchErr := c.fetchJSAPITicketWithRetry(ctx, app)
		if fetchErr != nil {
			return "", fetchErr
		}
		ttl := weComCacheTTL(expiresIn)
		if ttl <= 0 {
			return "", weComInvalidResponseError("JS-SDK ticket")
		}
		if cacheErr := c.cache.SetJSAPITicket(ctx, scope, ticket, ttl); cacheErr != nil {
			return "", weComCacheUnavailableError()
		}
		return ticket, nil
	})
	if err != nil {
		return "", err
	}
	ticket, ok := value.(string)
	ticket = strings.TrimSpace(ticket)
	if !ok || ticket == "" {
		return "", weComInvalidResponseError("JS-SDK ticket")
	}
	return ticket, nil
}

func (c *weComOAuthClient) fetchJSAPITicketWithRetry(ctx context.Context, app WeComOAuthAppCredentials) (string, int64, error) {
	token, err := c.getAccessToken(ctx, app)
	if err != nil {
		return "", 0, err
	}
	ticket, expiresIn, err := c.fetchJSAPITicket(ctx, token)
	if !isWeComTokenInvalidError(err) {
		return ticket, expiresIn, err
	}

	token, err = c.refreshAccessToken(ctx, app, token)
	if err != nil {
		return "", 0, err
	}
	return c.fetchJSAPITicket(ctx, token)
}

func (c *weComOAuthClient) fetchAccessToken(ctx context.Context, app WeComOAuthAppCredentials) (string, int64, error) {
	query := url.Values{
		"corpid":     {app.CorpID},
		"corpsecret": {app.Secret},
	}
	var response struct {
		weComAPIResponse
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.doJSON(ctx, "get access token", http.MethodGet, "/cgi-bin/gettoken", query, nil, &response); err != nil {
		return "", 0, err
	}
	if err := response.apiError("get access token"); err != nil {
		return "", 0, err
	}
	response.AccessToken = strings.TrimSpace(response.AccessToken)
	if response.AccessToken == "" || response.ExpiresIn <= 0 {
		return "", 0, weComInvalidResponseError("access token")
	}
	return response.AccessToken, response.ExpiresIn, nil
}

func (c *weComOAuthClient) resolveOpenIDWithToken(ctx context.Context, token, code string) (WeComOAuthIdentity, error) {
	query := url.Values{
		"access_token": {token},
		"code":         {code},
	}
	var response struct {
		weComAPIResponse
		UserID string `json:"UserId"`
		OpenID string `json:"OpenId"`
	}
	if err := c.doJSON(ctx, "get OAuth user info", http.MethodGet, "/cgi-bin/auth/getuserinfo", query, nil, &response); err != nil {
		return WeComOAuthIdentity{}, err
	}
	if err := response.apiError("get OAuth user info"); err != nil {
		return WeComOAuthIdentity{}, err
	}

	userID := strings.TrimSpace(response.UserID)
	openID := strings.TrimSpace(response.OpenID)
	if (userID == "") == (openID == "") {
		return WeComOAuthIdentity{}, infraerrors.New(http.StatusBadGateway, "WECOM_OAUTH_IDENTITY_INVALID", "enterprise WeChat returned an invalid OAuth identity")
	}
	if openID != "" {
		return WeComOAuthIdentity{OpenID: openID}, nil
	}
	return c.convertUserIDToOpenID(ctx, token, userID)
}

func (c *weComOAuthClient) convertUserIDToOpenID(ctx context.Context, token, userID string) (WeComOAuthIdentity, error) {
	query := url.Values{"access_token": {token}}
	request := struct {
		UserID string `json:"userid"`
	}{UserID: userID}
	var response struct {
		weComAPIResponse
		OpenID string `json:"openid"`
	}
	if err := c.doJSON(ctx, "convert member identity", http.MethodPost, "/cgi-bin/user/convert_to_openid", query, request, &response); err != nil {
		return WeComOAuthIdentity{}, err
	}
	if err := response.apiError("convert member identity"); err != nil {
		return WeComOAuthIdentity{}, err
	}
	openID := strings.TrimSpace(response.OpenID)
	if openID == "" {
		return WeComOAuthIdentity{}, weComInvalidResponseError("converted OAuth identity")
	}
	return WeComOAuthIdentity{OpenID: openID}, nil
}

func (c *weComOAuthClient) fetchJSAPITicket(ctx context.Context, token string) (string, int64, error) {
	query := url.Values{"access_token": {token}}
	var response struct {
		weComAPIResponse
		Ticket    string `json:"ticket"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := c.doJSON(ctx, "get JS-SDK ticket", http.MethodGet, "/cgi-bin/get_jsapi_ticket", query, nil, &response); err != nil {
		return "", 0, err
	}
	if err := response.apiError("get JS-SDK ticket"); err != nil {
		return "", 0, err
	}
	response.Ticket = strings.TrimSpace(response.Ticket)
	if response.Ticket == "" || response.ExpiresIn <= 0 {
		return "", 0, weComInvalidResponseError("JS-SDK ticket")
	}
	return response.Ticket, response.ExpiresIn, nil
}

func (c *weComOAuthClient) doJSON(ctx context.Context, operation, method, path string, query url.Values, body any, destination any) error {
	endpoint := weComAPIBaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return infraerrors.New(http.StatusInternalServerError, "WECOM_REQUEST_ENCODING_FAILED", "failed to encode enterprise WeChat request")
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return infraerrors.New(http.StatusInternalServerError, "WECOM_REQUEST_BUILD_FAILED", "failed to build enterprise WeChat request")
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return infraerrors.New(http.StatusGatewayTimeout, "WECOM_UPSTREAM_TIMEOUT", "enterprise WeChat upstream request timed out")
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return infraerrors.New(http.StatusGatewayTimeout, "WECOM_UPSTREAM_CANCELED", "enterprise WeChat upstream request was canceled")
		}
		return infraerrors.New(http.StatusBadGateway, "WECOM_UPSTREAM_UNAVAILABLE", "enterprise WeChat upstream request failed")
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(response.Body, weComMaxResponseBodyBytes+1))
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "WECOM_UPSTREAM_READ_FAILED", "failed to read enterprise WeChat upstream response")
	}
	if len(payload) > weComMaxResponseBodyBytes {
		return infraerrors.New(http.StatusBadGateway, "WECOM_UPSTREAM_RESPONSE_TOO_LARGE", "enterprise WeChat upstream response exceeded the size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return infraerrors.Newf(http.StatusBadGateway, "WECOM_UPSTREAM_HTTP_ERROR", "enterprise WeChat %s failed with HTTP status %d", operation, response.StatusCode)
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return infraerrors.New(http.StatusBadGateway, "WECOM_UPSTREAM_INVALID_RESPONSE", "enterprise WeChat upstream returned an invalid response")
	}
	return nil
}

type weComAPIResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (r weComAPIResponse) apiError(operation string) error {
	if r.ErrCode == 0 {
		return nil
	}
	return &weComUpstreamError{operation: operation, code: r.ErrCode}
}

type weComUpstreamError struct {
	operation string
	code      int
}

func (e *weComUpstreamError) Error() string {
	if e == nil {
		return "enterprise WeChat upstream error"
	}
	return fmt.Sprintf("enterprise WeChat %s failed with code %d", e.operation, e.code)
}

func isWeComTokenInvalidError(err error) bool {
	var upstreamError *weComUpstreamError
	if !errors.As(err, &upstreamError) || upstreamError == nil {
		return false
	}
	return upstreamError.code == 40014 || upstreamError.code == 42001
}

func mapWeComOAuthError(err error) error {
	if err == nil {
		return nil
	}
	var applicationError *infraerrors.ApplicationError
	if errors.As(err, &applicationError) {
		return err
	}
	var upstreamError *weComUpstreamError
	if errors.As(err, &upstreamError) && upstreamError != nil {
		return infraerrors.Newf(http.StatusBadGateway, "WECOM_UPSTREAM_ERROR", "enterprise WeChat %s failed with code %d", upstreamError.operation, upstreamError.code)
	}
	return infraerrors.New(http.StatusInternalServerError, "WECOM_OAUTH_INTERNAL_ERROR", "enterprise WeChat OAuth operation failed")
}

func weComCacheUnavailableError() error {
	return infraerrors.New(http.StatusServiceUnavailable, "WECOM_CACHE_UNAVAILABLE", "enterprise WeChat credential cache is unavailable")
}

func weComInvalidResponseError(subject string) error {
	return infraerrors.Newf(http.StatusBadGateway, "WECOM_UPSTREAM_INVALID_RESPONSE", "enterprise WeChat returned an invalid %s response", subject)
}

func weComCacheTTL(expiresIn int64) time.Duration {
	if expiresIn <= 0 {
		return 0
	}
	maxSeconds := int64(weComCacheMaxTTL / time.Second)
	if expiresIn > maxSeconds {
		expiresIn = maxSeconds
	}
	ttl := time.Duration(expiresIn) * time.Second
	if ttl > weComCacheSafetyWindow {
		return ttl - weComCacheSafetyWindow
	}
	return ttl / 2
}

func newWeComNonce() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func signWeComJSConfig(ticket, pageURL string, timestamp int64, nonceStr string) string {
	canonical := fmt.Sprintf("jsapi_ticket=%s&noncestr=%s&timestamp=%d&url=%s", ticket, nonceStr, timestamp, pageURL)
	digest := sha1.Sum([]byte(canonical)) // #nosec G401 -- 企业微信 JS-SDK 协议固定要求 SHA-1。
	return hex.EncodeToString(digest[:])
}
