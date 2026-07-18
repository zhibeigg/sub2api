package kiro

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// oauthLoginTimeout bounds a single OIDC HTTP round-trip during interactive
// login. Each call does exactly one request; polling is driven by the caller.
const oauthLoginTimeout = 30 * time.Second

// defaultStartURL is the AWS SSO start URL used for Builder ID / SSO flows.
const defaultStartURL = "https://view.awsapps.com/start"

// ssoPortalBase is the AWS SSO portal host used by the SSO-token import flow.
const ssoPortalBase = "https://portal.sso.us-east-1.amazonaws.com"

// oidcScopes are the CodeWhisperer scopes requested during client registration.
var oidcScopes = []string{
	"codewhisperer:completions",
	"codewhisperer:analysis",
	"codewhisperer:conversations",
	"codewhisperer:transformations",
	"codewhisperer:taskassist",
}

func oidcBaseURL(region string) string {
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("https://oidc.%s.amazonaws.com", region)
}

func oauthClient(proxyURL string) *http.Client {
	return GetHTTPClientForProxy(proxyURL, oauthLoginTimeout)
}

// -----------------------------------------------------------------------------
// Builder ID — device code flow
// -----------------------------------------------------------------------------

// DeviceAuthResult is returned when a device authorization is started.
type DeviceAuthResult struct {
	ClientID        string
	ClientSecret    string
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int
	ExpiresIn       int
	Region          string
}

// DevicePollResult is returned by a single poll attempt.
type DevicePollResult struct {
	Status       string // "pending" | "slow_down" | "completed"
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// StartBuilderIDLogin registers an OIDC client and starts device authorization.
// It performs two upstream requests and returns immediately; the caller polls
// PollDeviceToken on the reported interval.
func StartBuilderIDLogin(ctx context.Context, region, proxyURL string) (*DeviceAuthResult, error) {
	if region == "" {
		region = "us-east-1"
	}
	base := oidcBaseURL(region)
	client := oauthClient(proxyURL)

	clientID, clientSecret, err := registerDeviceClient(ctx, client, base)
	if err != nil {
		return nil, fmt.Errorf("register client: %w", err)
	}

	authResp, err := startDeviceAuthorization(ctx, client, base, clientID, clientSecret)
	if err != nil {
		return nil, fmt.Errorf("device authorization: %w", err)
	}

	interval := authResp.Interval
	if interval <= 0 {
		interval = 5
	}
	expiresIn := authResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}
	verificationURI := authResp.VerificationURIComplete
	if verificationURI == "" {
		verificationURI = authResp.VerificationURI
	}

	return &DeviceAuthResult{
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		DeviceCode:      authResp.DeviceCode,
		UserCode:        authResp.UserCode,
		VerificationURI: verificationURI,
		Interval:        interval,
		ExpiresIn:       expiresIn,
		Region:          region,
	}, nil
}

// PollDeviceToken performs a single device-token poll. It never blocks/sleeps;
// the caller schedules the next poll based on the returned status and interval.
func PollDeviceToken(ctx context.Context, region, clientID, clientSecret, deviceCode, proxyURL string) (*DevicePollResult, error) {
	base := oidcBaseURL(region)
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"grantType":    "urn:ietf:params:oauth:grant-type:device_code",
		"deviceCode":   deviceCode,
	}
	status, tokenResp, err := doTokenPoll(ctx, oauthClient(proxyURL), base, payload)
	if err != nil {
		return nil, err
	}
	if status != "" {
		return &DevicePollResult{Status: status}, nil
	}
	return &DevicePollResult{
		Status:       "completed",
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
	}, nil
}

// -----------------------------------------------------------------------------
// IAM Identity Center — authorization code + PKCE
// -----------------------------------------------------------------------------

// AuthCodeStartResult is returned when an IAM SSO authorize URL is generated.
type AuthCodeStartResult struct {
	ClientID     string
	ClientSecret string
	CodeVerifier string
	State        string
	AuthorizeURL string
	RedirectURI  string
	Region       string
	ExpiresIn    int
}

// AuthCodeTokenResult is returned after exchanging an authorization code.
type AuthCodeTokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

const iamSSORedirectURI = "http://127.0.0.1/oauth/callback"

// StartIAMSSOLogin registers an authorization-code OIDC client (PKCE S256) and
// builds the authorize URL. The caller stores the returned session fields and
// completes the flow with ExchangeAuthCode.
func StartIAMSSOLogin(ctx context.Context, startURL, region, proxyURL string) (*AuthCodeStartResult, error) {
	if region == "" {
		region = "us-east-1"
	}
	if startURL == "" {
		startURL = defaultStartURL
	}
	base := oidcBaseURL(region)
	client := oauthClient(proxyURL)

	clientID, clientSecret, err := registerAuthCodeClient(ctx, client, base, startURL, iamSSORedirectURI)
	if err != nil {
		return nil, fmt.Errorf("register client: %w", err)
	}

	codeVerifier := generateCodeVerifier()
	codeChallenge := generateCodeChallenge(codeVerifier)
	state := generateState()

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", iamSSORedirectURI)
	params.Set("scopes", strings.Join(oidcScopes, ","))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	return &AuthCodeStartResult{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CodeVerifier: codeVerifier,
		State:        state,
		AuthorizeURL: fmt.Sprintf("%s/authorize?%s", base, params.Encode()),
		RedirectURI:  iamSSORedirectURI,
		Region:       region,
		ExpiresIn:    600,
	}, nil
}

// ExchangeAuthCode exchanges an authorization code for tokens using the stored
// PKCE code verifier.
func ExchangeAuthCode(ctx context.Context, region, clientID, clientSecret, code, codeVerifier, redirectURI, proxyURL string) (authResult *AuthCodeTokenResult, err error) {
	base := oidcBaseURL(region)
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"grantType":    "authorization_code",
		"redirectUri":  redirectURI,
		"code":         code,
		"codeVerifier": codeVerifier,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := oauthClient(proxyURL).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result oidcTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &AuthCodeTokenResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
	}, nil
}

// ParseAuthCodeCallback extracts and validates the code from a redirect URL,
// checking the returned state against the expected one.
func ParseAuthCodeCallback(callbackURL, expectedState string) (code string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil {
		return "", fmt.Errorf("invalid callback URL")
	}
	q := parsed.Query()
	if errParam := q.Get("error"); errParam != "" {
		return "", fmt.Errorf("authorization failed: %s", errParam)
	}
	state := q.Get("state")
	if state == "" || expectedState == "" || !constantTimeEqual(state, expectedState) {
		return "", fmt.Errorf("state mismatch: possible security risk")
	}
	code = q.Get("code")
	if code == "" {
		return "", fmt.Errorf("no authorization code received")
	}
	return code, nil
}

// -----------------------------------------------------------------------------
// SSO Token import (x-amz-sso_authn bearer)
// -----------------------------------------------------------------------------

// SSOTokenImportResult carries the credentials produced by importing an SSO
// bearer token.
type SSOTokenImportResult struct {
	AccessToken  string
	RefreshToken string
	ClientID     string
	ClientSecret string
	ExpiresIn    int
	Region       string
}

// ImportFromSSOToken performs the multi-step AWS SSO device-approval flow using
// a pre-obtained x-amz-sso_authn bearer token, returning device-code tokens.
// Polling is bounded by ctx (caller should pass a timeout, e.g. 2 minutes).
func ImportFromSSOToken(ctx context.Context, bearerToken, region, proxyURL string) (*SSOTokenImportResult, error) {
	if region == "" {
		region = "us-east-1"
	}
	base := oidcBaseURL(region)
	client := oauthClient(proxyURL)

	clientID, clientSecret, err := registerDeviceClient(ctx, client, base)
	if err != nil {
		return nil, fmt.Errorf("register client: %w", err)
	}

	authResp, err := startDeviceAuthorization(ctx, client, base, clientID, clientSecret)
	if err != nil {
		return nil, fmt.Errorf("device authorization: %w", err)
	}

	if err := verifyBearerToken(ctx, client, bearerToken); err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}

	deviceSessionToken, err := getDeviceSessionToken(ctx, client, bearerToken)
	if err != nil {
		return nil, fmt.Errorf("device session: %w", err)
	}

	deviceContext, err := acceptUserCode(ctx, client, base, authResp.UserCode, deviceSessionToken)
	if err != nil {
		return nil, fmt.Errorf("accept user code: %w", err)
	}
	if deviceContext != nil {
		if err := approveAuth(ctx, client, base, deviceContext, deviceSessionToken); err != nil {
			return nil, fmt.Errorf("approve auth: %w", err)
		}
	}

	interval := authResp.Interval
	if interval <= 0 {
		interval = 1
	}
	access, refresh, expiresIn, err := pollForTokenBounded(ctx, client, base, clientID, clientSecret, authResp.DeviceCode, interval)
	if err != nil {
		return nil, fmt.Errorf("poll token: %w", err)
	}

	return &SSOTokenImportResult{
		AccessToken:  access,
		RefreshToken: refresh,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		ExpiresIn:    expiresIn,
		Region:       region,
	}, nil
}

// -----------------------------------------------------------------------------
// Shared OIDC primitives
// -----------------------------------------------------------------------------

type oidcTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expiresIn"`
}

func registerDeviceClient(ctx context.Context, client *http.Client, base string) (clientID, clientSecret string, err error) {
	payload := map[string]any{
		"clientName": "Kiro",
		"clientType": "public",
		"scopes":     oidcScopes,
		"grantTypes": []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"},
		"issuerUrl":  defaultStartURL,
	}
	return doRegisterClient(ctx, client, base, payload)
}

func registerAuthCodeClient(ctx context.Context, client *http.Client, base, startURL, redirectURI string) (clientID, clientSecret string, err error) {
	payload := map[string]any{
		"clientName":   "Kiro",
		"clientType":   "public",
		"scopes":       oidcScopes,
		"grantTypes":   []string{"authorization_code", "refresh_token"},
		"redirectUris": []string{redirectURI},
		"issuerUrl":    startURL,
	}
	return doRegisterClient(ctx, client, base, payload)
}

func doRegisterClient(ctx context.Context, client *http.Client, base string, payload map[string]any) (clientID, clientSecret string, err error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/client/register", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	return result.ClientID, result.ClientSecret, nil
}

func startDeviceAuthorization(ctx context.Context, client *http.Client, base, clientID, clientSecret string) (authResult *deviceAuthResponse, err error) {
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"startUrl":     defaultStartURL,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/device_authorization", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result deviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// doTokenPoll performs a single device-token request. It returns a non-empty
// status ("pending"/"slow_down") when the grant is not yet ready, or the token
// response when completed.
func doTokenPoll(ctx context.Context, client *http.Client, base string, payload map[string]string) (status string, token *oidcTokenResponse, err error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/token", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode == http.StatusOK {
		var result oidcTokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", nil, err
		}
		return "", &result, nil
	}

	if resp.StatusCode == http.StatusBadRequest {
		var errResult struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResult)
		switch errResult.Error {
		case "authorization_pending":
			return "pending", nil, nil
		case "slow_down":
			return "slow_down", nil, nil
		case "expired_token":
			return "", nil, fmt.Errorf("device code expired")
		case "access_denied":
			return "", nil, fmt.Errorf("user denied authorization")
		default:
			return "", nil, fmt.Errorf("authorization error: %s", errResult.Error)
		}
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	return "", nil, fmt.Errorf("unexpected response: %d %s", resp.StatusCode, string(respBody))
}

// pollForTokenBounded polls until completion, ctx cancellation, or timeout.
// It uses a ticker rather than a fixed sleep and respects ctx.
func pollForTokenBounded(ctx context.Context, client *http.Client, base, clientID, clientSecret, deviceCode string, interval int) (accessToken, refreshToken string, expiresIn int, err error) {
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"grantType":    "urn:ietf:params:oauth:grant-type:device_code",
		"deviceCode":   deviceCode,
	}
	if interval <= 0 {
		interval = 1
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", "", 0, ctx.Err()
		case <-ticker.C:
			status, token, pollErr := doTokenPoll(ctx, client, base, payload)
			if pollErr != nil {
				return "", "", 0, pollErr
			}
			switch status {
			case "pending":
				continue
			case "slow_down":
				interval += 5
				ticker.Reset(time.Duration(interval) * time.Second)
				continue
			case "":
				return token.AccessToken, token.RefreshToken, token.ExpiresIn, nil
			}
		}
	}
}

func verifyBearerToken(ctx context.Context, client *http.Client, bearerToken string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ssoPortalBase+"/token/whoAmI", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func getDeviceSessionToken(ctx context.Context, client *http.Client, bearerToken string) (token string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ssoPortalBase+"/session/device", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

type deviceContextInfo struct {
	DeviceContextID string `json:"deviceContextId"`
	ClientID        string `json:"clientId"`
	ClientType      string `json:"clientType"`
}

func acceptUserCode(ctx context.Context, client *http.Client, base, userCode, deviceSessionToken string) (deviceContext *deviceContextInfo, err error) {
	payload := map[string]string{
		"userCode":      userCode,
		"userSessionId": deviceSessionToken,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/device_authorization/accept_user_code", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://view.awsapps.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		DeviceContext *deviceContextInfo `json:"deviceContext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.DeviceContext, nil
}

func approveAuth(ctx context.Context, client *http.Client, base string, deviceContext *deviceContextInfo, deviceSessionToken string) (err error) {
	payload := map[string]any{
		"deviceContext": map[string]string{
			"deviceContextId": deviceContext.DeviceContextID,
			"clientId":        deviceContext.ClientID,
			"clientType":      deviceContext.ClientType,
		},
		"userSessionId": deviceSessionToken,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/device_authorization/associate_token", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://view.awsapps.com/")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// -----------------------------------------------------------------------------
// PKCE / random helpers
// -----------------------------------------------------------------------------

func generateCodeVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func generateState() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// constantTimeEqual compares two strings without early exit (mitigate timing).
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
