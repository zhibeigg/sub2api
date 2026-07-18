package ims

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

var imsTokenURL = "https://adobeid-na1.services.adobe.com/ims/check/v6/token?jslVersion=v2-v0.48.0-1-g1e322cb"
var imsDeviceTokenURL = "https://ims-na1.adobelogin.com/ims/token/v4"
var imsProfileURL = "https://ims-na1.adobelogin.com/ims/profile/v1"
var creditsURL = "https://firefly.adobe.io/v1/credits/balance"

const defaultClientID = "clio-playground-web"
const defaultScope = "AdobeID,firefly_api,openid,pps.read,pps.write,additional_info.projectedProductContext,additional_info.ownerOrg,uds_read,uds_write,ab.manage,read_organizations,additional_info.roles,account_cluster.read,creative_production"
const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
const maxIMSBody = 64 << 10

type RefreshOptions struct {
	ProxyURL, ClientID, Scope, CreditsAPIKey, UserAgent, Origin string
	Timeout                                                     time.Duration
}
type RefreshTokenResult struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	ExpiresAt   int64  `json:"expires_at"`
}
type AccountInfo struct {
	DisplayName string
	Email       string
	UserID      string
}
type FullResult struct {
	AccessToken string
	ExpiresAt   int64
	ExpiresIn   int64
	UserID      string
	DisplayName string
	Email       string
	Credits     float64
}

var ErrNoCookie = errors.New("adobe IMS cookie is required")

func (o *RefreshOptions) defaults() {
	if o.ClientID == "" {
		o.ClientID = defaultClientID
	}
	if o.Scope == "" {
		o.Scope = defaultScope
	}
	if o.CreditsAPIKey == "" {
		o.CreditsAPIKey = defaultClientID
	}
	if o.UserAgent == "" {
		o.UserAgent = defaultUA
	}
	if o.Origin == "" {
		o.Origin = "https://firefly.adobe.com"
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
}
func client(opt RefreshOptions) *req.Client {
	c := req.C().SetTimeout(opt.Timeout).ImpersonateChrome()
	if strings.TrimSpace(opt.ProxyURL) != "" {
		c.SetProxyURL(opt.ProxyURL)
	}
	return c
}
func do(ctx context.Context, opt RefreshOptions, method, endpoint string, headers map[string]string, body string) (int, []byte, error) {
	r := client(opt).R().SetContext(ctx).SetHeaders(headers)
	if body != "" {
		r.SetBodyString(body)
	}
	resp, err := r.Send(method, endpoint)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxIMSBody+1))
	if err != nil {
		return 0, nil, err
	}
	if len(b) > maxIMSBody {
		return 0, nil, fmt.Errorf("adobe IMS response exceeds size limit")
	}
	return resp.StatusCode, b, nil
}

func RefreshAccessTokenViaCookie(ctx context.Context, cookie string, opt RefreshOptions) (*RefreshTokenResult, error) {
	if strings.TrimSpace(cookie) == "" {
		return nil, ErrNoCookie
	}
	opt.defaults()
	form := url.Values{"client_id": {opt.ClientID}, "guest_allowed": {"true"}, "scope": {opt.Scope}}
	status, b, err := do(ctx, opt, http.MethodPost, imsTokenURL, map[string]string{"cookie": cookie, "content-type": "application/x-www-form-urlencoded;charset=UTF-8", "accept": "*/*", "origin": opt.Origin, "referer": opt.Origin + "/", "user-agent": opt.UserAgent}, form.Encode())
	if err != nil {
		return nil, fmt.Errorf("adobe IMS cookie refresh failed")
	}
	if status != 200 {
		return nil, fmt.Errorf("adobe IMS cookie refresh HTTP %d", status)
	}
	return parseTokenResult(b)
}
func RefreshAccessTokenViaDeviceToken(ctx context.Context, deviceToken, deviceID string, opt RefreshOptions) (*RefreshTokenResult, error) {
	if strings.TrimSpace(deviceToken) == "" || strings.TrimSpace(deviceID) == "" {
		return nil, errors.New("adobe IMS device credentials are required")
	}
	opt.defaults()
	form := url.Values{"client_id": {"FF-iOS"}, "grant_type": {"device"}, "device_token": {deviceToken}, "device_id": {deviceID}}
	status, b, err := do(ctx, opt, http.MethodPost, imsDeviceTokenURL, map[string]string{"content-type": "application/x-www-form-urlencoded", "accept": "application/json", "user-agent": "Firefly/26.10.0 (AdobeCreativeSDK 11.0.2434;Apple;iPhone;iOS;26.6)", "x-ims-clientid": "FF-iOS"}, form.Encode())
	if err != nil {
		return nil, fmt.Errorf("adobe IMS device refresh failed")
	}
	if status != 200 {
		return nil, fmt.Errorf("adobe IMS device refresh HTTP %d", status)
	}
	return parseTokenResult(b)
}
func parseTokenResult(b []byte) (*RefreshTokenResult, error) {
	var v struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   any    `json:"expires_in"`
	}
	if json.Unmarshal(b, &v) != nil || v.AccessToken == "" {
		return nil, errors.New("adobe IMS returned an invalid token response")
	}
	ttl := toInt64(v.ExpiresIn)
	if ttl > 172800 {
		ttl /= 1000
	}
	exp := ExtractJWTExpiry(v.AccessToken)
	if exp == 0 && ttl > 0 {
		exp = time.Now().Unix() + ttl
	}
	return &RefreshTokenResult{AccessToken: v.AccessToken, ExpiresIn: ttl, ExpiresAt: exp}, nil
}

func FetchAccountInfo(ctx context.Context, token string, opt RefreshOptions) AccountInfo {
	if strings.TrimSpace(token) == "" {
		return AccountInfo{}
	}
	opt.defaults()
	status, b, err := do(ctx, opt, http.MethodGet, imsProfileURL, map[string]string{"authorization": "Bearer " + token, "accept": "application/json", "user-agent": opt.UserAgent}, "")
	if err != nil || status != 200 {
		return AccountInfo{}
	}
	var v struct {
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
		UserID      string `json:"userId"`
	}
	if json.Unmarshal(b, &v) != nil {
		return AccountInfo{}
	}
	return AccountInfo{v.DisplayName, v.Email, v.UserID}
}
func FetchCredits(ctx context.Context, token, accountID string, opt RefreshOptions) float64 {
	if strings.TrimSpace(token) == "" {
		return -1
	}
	opt.defaults()
	if accountID == "" {
		accountID = ExtractAccountIDFromJWT(token)
	}
	if accountID == "" {
		return -1
	}
	status, b, err := do(ctx, opt, http.MethodGet, creditsURL, map[string]string{"authorization": "Bearer " + token, "x-api-key": opt.CreditsAPIKey, "x-account-id": accountID, "accept": "application/json", "user-agent": opt.UserAgent}, "")
	if err != nil || status != 200 {
		return -1
	}
	var v struct {
		Total struct {
			Quota struct {
				Available *float64 `json:"available"`
			} `json:"quota"`
		} `json:"total"`
		Balance *float64 `json:"balance"`
	}
	if json.Unmarshal(b, &v) != nil {
		return -1
	}
	if v.Total.Quota.Available != nil {
		return *v.Total.Quota.Available
	}
	if v.Balance != nil {
		return *v.Balance
	}
	return -1
}
func RefreshOne(ctx context.Context, cookie string, opt RefreshOptions) (*FullResult, error) {
	tok, err := RefreshAccessTokenViaCookie(ctx, cookie, opt)
	if err != nil {
		return nil, err
	}
	return enrich(ctx, tok, opt), nil
}
func RefreshOneViaDeviceToken(ctx context.Context, deviceToken, deviceID string, opt RefreshOptions) (*FullResult, error) {
	tok, err := RefreshAccessTokenViaDeviceToken(ctx, deviceToken, deviceID, opt)
	if err != nil {
		return nil, err
	}
	return enrich(ctx, tok, opt), nil
}
func enrich(ctx context.Context, t *RefreshTokenResult, opt RefreshOptions) *FullResult {
	o := &FullResult{AccessToken: t.AccessToken, ExpiresAt: t.ExpiresAt, ExpiresIn: t.ExpiresIn, Credits: -1}
	info := FetchAccountInfo(ctx, t.AccessToken, opt)
	o.DisplayName, o.Email = info.DisplayName, info.Email
	o.UserID = info.UserID
	if o.UserID == "" {
		o.UserID = ExtractAccountIDFromJWT(t.AccessToken)
	}
	o.Credits = FetchCredits(ctx, t.AccessToken, o.UserID, opt)
	return o
}
func FetchOnly(ctx context.Context, token string, opt RefreshOptions) *FullResult {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return enrich(ctx, &RefreshTokenResult{AccessToken: token, ExpiresAt: ExtractJWTExpiry(token)}, opt)
}
