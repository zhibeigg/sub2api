package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// kiroRestAPIBase is the CodeWhisperer runtime REST host used for account
// metadata calls (usage limits, models, profiles). Regionalized per-profile.
const kiroRestAPIBase = "https://codewhisperer.us-east-1.amazonaws.com"

// profileArnUnsupportedCooldown suppresses repeated ListAvailableProfiles calls
// for Builder ID accounts that AWS reports as unsupported for that operation.
const profileArnUnsupportedCooldown = 24 * time.Hour

// profileArnResolutionCooldowns tracks accounts whose profile lookup is
// temporarily suppressed (keyed by provider + identity).
var profileArnResolutionCooldowns sync.Map

// restHTTPTimeout is the timeout for REST metadata requests.
const restHTTPTimeout = 30 * time.Second

// UsageLimitsResponse mirrors AWS getUsageLimits.
type UsageLimitsResponse struct {
	UsageBreakdownList []UsageBreakdown  `json:"usageBreakdownList"`
	NextDateReset      json.Number       `json:"nextDateReset"`
	SubscriptionInfo   *SubscriptionInfo `json:"subscriptionInfo"`
	UserInfo           *UsageUserInfo    `json:"userInfo"`
}

// UsageBreakdown is one resource-type usage row.
type UsageBreakdown struct {
	ResourceType  string         `json:"resourceType"`
	CurrentUsage  float64        `json:"currentUsage"`
	UsageLimit    float64        `json:"usageLimit"`
	Currency      string         `json:"currency"`
	Unit          string         `json:"unit"`
	OverageRate   float64        `json:"overageRate"`
	FreeTrialInfo *FreeTrialInfo `json:"freeTrialInfo"`
	Bonuses       []BonusInfo    `json:"bonuses"`
}

// FreeTrialInfo describes a free-trial allotment.
type FreeTrialInfo struct {
	CurrentUsage    float64     `json:"currentUsage"`
	UsageLimit      float64     `json:"usageLimit"`
	FreeTrialStatus string      `json:"freeTrialStatus"`
	FreeTrialExpiry json.Number `json:"freeTrialExpiry"`
}

// BonusInfo describes a promotional credit bucket.
type BonusInfo struct {
	BonusCode    string      `json:"bonusCode"`
	DisplayName  string      `json:"displayName"`
	CurrentUsage float64     `json:"currentUsage"`
	UsageLimit   float64     `json:"usageLimit"`
	ExpiresAt    json.Number `json:"expiresAt"`
	Status       string      `json:"status"`
}

// SubscriptionInfo describes the account plan.
type SubscriptionInfo struct {
	SubscriptionName  string `json:"subscriptionName"`
	SubscriptionTitle string `json:"subscriptionTitle"`
	SubscriptionType  string `json:"subscriptionType"`
	Status            string `json:"status"`
	UpgradeCapability string `json:"upgradeCapability"`
}

// UsageUserInfo is the userInfo block returned by getUsageLimits.
type UsageUserInfo struct {
	Email  string `json:"email"`
	UserId string `json:"userId"`
}

// UserInfoResponse mirrors GetUserInfo.
type UserInfoResponse struct {
	Email  string `json:"email"`
	UserId string `json:"userId"`
	Idp    string `json:"idp"`
	Status string `json:"status"`
}

// ModelInfo describes an upstream-available model.
type ModelInfo struct {
	ModelId        string   `json:"modelId"`
	ModelName      string   `json:"modelName"`
	Description    string   `json:"description"`
	InputTypes     []string `json:"supportedInputTypes"`
	RateMultiplier float64  `json:"rateMultiplier"`
	TokenLimits    *struct {
		MaxInputTokens  int `json:"maxInputTokens"`
		MaxOutputTokens int `json:"maxOutputTokens"`
	} `json:"tokenLimits"`
}

// AccountInfo is the normalized account metadata parsed from getUsageLimits.
type AccountInfo struct {
	Email             string
	UserId            string
	SubscriptionType  string // FREE / PRO / PRO_PLUS / POWER
	SubscriptionTitle string
	SubscriptionName  string
	SubscriptionRaw   string
	UsageCurrent      float64
	UsageLimit        float64
	UsagePercent      float64
	NextResetDate     string
	TrialUsageCurrent float64
	TrialUsageLimit   float64
	TrialUsagePercent float64
	TrialStatus       string
	TrialExpiresAt    int64
	LastRefresh       int64
}

// kiroRegionForProfile returns the AWS data-plane region for a credential,
// preferring the profile ARN's region over the OIDC region.
func kiroRegionForProfile(cred *Credential, profileArn string) string {
	if r := regionFromProfileArn(profileArn); r != "" {
		return r
	}
	if cred != nil {
		if r := regionFromProfileArn(cred.ProfileArn); r != "" {
			return r
		}
		if r := strings.TrimSpace(cred.Region); r != "" {
			return r
		}
	}
	return "us-east-1"
}

// regionalizeRESTURL points a hardcoded us-east-1 REST endpoint at the
// credential's data-plane region. No-op for us-east-1.
func regionalizeRESTURL(rawURL string, cred *Credential) string {
	region := kiroRegionForProfile(cred, "")
	if region == "us-east-1" {
		return rawURL
	}
	regionalHost := "q." + region + ".amazonaws.com"
	return strings.NewReplacer(
		"q.us-east-1.amazonaws.com", regionalHost,
		"codewhisperer.us-east-1.amazonaws.com", regionalHost,
	).Replace(rawURL)
}

func withProfileArnQuery(rawURL string, cred *Credential) string {
	if cred == nil {
		return rawURL
	}
	profileArn := strings.TrimSpace(cred.ProfileArn)
	if profileArn == "" {
		return rawURL
	}
	return rawURL + "&profileArn=" + url.QueryEscape(profileArn)
}

// GetUsageLimits fetches account usage and subscription info.
func GetUsageLimits(ctx context.Context, cred *Credential) (*UsageLimitsResponse, error) {
	if err := ensureRESTProfileArn(ctx, cred); err != nil {
		return nil, fmt.Errorf("resolve profileArn: %w", err)
	}

	rawURL := fmt.Sprintf("%s/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true", kiroRestAPIBase)
	rawURL = regionalizeRESTURL(rawURL, cred)
	rawURL = withProfileArnQuery(rawURL, cred)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	setKiroRuntimeHeaders(req, cred)

	resp, err := restClient(cred).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result UsageLimitsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUserInfo fetches user identity info.
func GetUserInfo(ctx context.Context, cred *Credential) (*UserInfoResponse, error) {
	rawURL := regionalizeRESTURL(fmt.Sprintf("%s/GetUserInfo", kiroRestAPIBase), cred)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(`{"origin":"KIRO_IDE"}`))
	if err != nil {
		return nil, err
	}
	setKiroRuntimeHeaders(req, cred)
	req.Header.Set("Content-Type", "application/json")

	resp, err := restClient(cred).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result UserInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAvailableModels fetches the upstream-available model list.
func ListAvailableModels(ctx context.Context, cred *Credential) ([]ModelInfo, error) {
	if err := ensureRESTProfileArn(ctx, cred); err != nil {
		return nil, fmt.Errorf("resolve profileArn: %w", err)
	}

	rawURL := fmt.Sprintf("%s/ListAvailableModels?origin=AI_EDITOR&maxResults=50", kiroRestAPIBase)
	rawURL = regionalizeRESTURL(rawURL, cred)
	rawURL = withProfileArnQuery(rawURL, cred)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	setKiroRuntimeHeaders(req, cred)

	resp, err := restClient(cred).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []ModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Models, nil
}

// ResolveProfileArn returns the credential's profile ARN, fetching and caching
// it when missing. First tries ListAvailableProfiles; on empty falls back to a
// token refresh (which returns profileArn). Returns the (possibly refreshed)
// result plus a soft flag when resolution was skipped/unsupported so callers can
// continue without failing hard.
//
// The caller is responsible for persisting cred.ProfileArn if it changes.
func ResolveProfileArn(ctx context.Context, cred *Credential) (string, error) {
	if cred == nil {
		return "", fmt.Errorf("credential is nil")
	}
	if profileArn := strings.TrimSpace(cred.ProfileArn); profileArn != "" {
		return profileArn, nil
	}

	suppressed := isProfileArnResolutionSuppressed(cred)
	var unsupportedErr error
	var unsupported bool

	if !suppressed {
		profileArn, err := listAvailableProfilesWithRetry(ctx, cred)
		if err == nil && profileArn != "" {
			cred.ProfileArn = profileArn
			return profileArn, nil
		}
		unsupportedErr = err
		unsupported = isBuilderIDProfileUnsupportedError(cred, err)
	}

	// Fallback: refresh token to obtain profileArn from the auth response.
	if strings.TrimSpace(cred.RefreshToken) != "" {
		result, refreshErr := RefreshToken(ctx, cred)
		if refreshErr == nil && result != nil && result.ProfileArn != "" {
			cred.ProfileArn = result.ProfileArn
			// Carry any rotated tokens back to the caller.
			if result.AccessToken != "" {
				cred.AccessToken = result.AccessToken
			}
			if result.RefreshToken != "" {
				cred.RefreshToken = result.RefreshToken
			}
			return result.ProfileArn, nil
		}
	}

	if suppressed {
		return "", fmt.Errorf("profile ARN resolution skipped: previous Builder ID profile lookup was unsupported")
	}
	if unsupported {
		suppressProfileArnResolution(cred)
		return "", fmt.Errorf("profile ARN unsupported for Builder ID account: %v", unsupportedErr)
	}
	return "", fmt.Errorf("no available Kiro profile")
}

func ensureRESTProfileArn(ctx context.Context, cred *Credential) error {
	if cred == nil || strings.TrimSpace(cred.ProfileArn) != "" {
		return nil
	}
	if _, err := ResolveProfileArn(ctx, cred); err != nil {
		if isProfileArnResolutionSoftError(err) {
			// Continue without profile ARN — the request may still succeed.
			return nil
		}
		return err
	}
	return nil
}

func listAvailableProfilesWithRetry(ctx context.Context, cred *Credential) (string, error) {
	const maxAttempts = 3
	backoff := 200 * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		profileArn, err := listAvailableProfiles(ctx, cred)
		if err == nil {
			return profileArn, nil
		}
		lastErr = err
		if !isTransientProfileFetchError(err) || attempt == maxAttempts {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return "", lastErr
}

func listAvailableProfiles(ctx context.Context, cred *Credential) (string, error) {
	rawURL := regionalizeRESTURL(fmt.Sprintf("%s/ListAvailableProfiles", kiroRestAPIBase), cred)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(`{"maxResults":10}`))
	if err != nil {
		return "", err
	}
	setKiroRuntimeHeaders(req, cred)
	req.Header.Set("Content-Type", "application/json")

	resp, err := restClient(cred).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Profiles []struct {
			Arn string `json:"arn"`
		} `json:"profiles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, profile := range result.Profiles {
		if profileArn := strings.TrimSpace(profile.Arn); profileArn != "" {
			return profileArn, nil
		}
	}
	return "", fmt.Errorf("empty profile list")
}

// isTransientProfileFetchError reports whether a ListAvailableProfiles error is
// worth retrying (network errors, 5xx, 429). Empty list / other 4xx are not.
func isTransientProfileFetchError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "empty profile list") {
		return false
	}
	if strings.HasPrefix(msg, "HTTP ") {
		return strings.HasPrefix(msg, "HTTP 5") || strings.HasPrefix(msg, "HTTP 429")
	}
	return true
}

func isBuilderIDProfileUnsupportedError(cred *Credential, err error) bool {
	if cred == nil || err == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(cred.Provider), "BuilderId") {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "HTTP 403") && strings.Contains(msg, "AWS Builder ID is not supported for this operation")
}

func profileArnCooldownKey(cred *Credential) string {
	if cred == nil {
		return ""
	}
	provider := strings.TrimSpace(cred.Provider)
	if id := strings.TrimSpace(cred.MachineID); id != "" {
		return provider + "\x00" + id
	}
	return provider + "\x00" + strings.TrimSpace(cred.Email)
}

func suppressProfileArnResolution(cred *Credential) {
	key := profileArnCooldownKey(cred)
	if key == "" {
		return
	}
	profileArnResolutionCooldowns.Store(key, time.Now().Add(profileArnUnsupportedCooldown))
}

func isProfileArnResolutionSuppressed(cred *Credential) bool {
	key := profileArnCooldownKey(cred)
	if key == "" {
		return false
	}
	value, ok := profileArnResolutionCooldowns.Load(key)
	if !ok {
		return false
	}
	until, ok := value.(time.Time)
	if !ok || time.Now().After(until) {
		profileArnResolutionCooldowns.Delete(key)
		return false
	}
	return true
}

func isProfileArnResolutionSoftError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "profile ARN resolution skipped") ||
		strings.Contains(msg, "profile ARN unsupported for Builder ID account")
}

// IsProfileArnResolutionSoftError reports whether streaming requests may safely
// continue without a profile ARN and fall back to the credential region.
func IsProfileArnResolutionSoftError(err error) bool {
	return isProfileArnResolutionSoftError(err)
}

// RefreshAccountInfo fetches usage limits and normalizes them into AccountInfo.
// It returns a sentinel-wrapped error when the account is suspended or the token
// is invalid so callers can react (disable / re-auth). Ban detection mirrors
// Kiro-Go's RefreshAccountInfo.
func RefreshAccountInfo(ctx context.Context, cred *Credential) (*AccountInfo, error) {
	usage, err := GetUsageLimits(ctx, cred)
	if err != nil {
		return nil, classifyAccountInfoError(err)
	}

	info := &AccountInfo{LastRefresh: time.Now().Unix()}

	if usage.UserInfo != nil {
		info.Email = usage.UserInfo.Email
		info.UserId = usage.UserInfo.UserId
	}

	if usage.SubscriptionInfo != nil {
		titleOrName := usage.SubscriptionInfo.SubscriptionTitle
		if titleOrName == "" {
			titleOrName = usage.SubscriptionInfo.SubscriptionName
		}
		if titleOrName == "" {
			titleOrName = usage.SubscriptionInfo.SubscriptionType
		}
		info.SubscriptionRaw = titleOrName
		info.SubscriptionType = parseSubscriptionType(titleOrName)
		info.SubscriptionTitle = usage.SubscriptionInfo.SubscriptionTitle
		if info.SubscriptionTitle == "" {
			info.SubscriptionTitle = usage.SubscriptionInfo.SubscriptionName
		}
		info.SubscriptionName = usage.SubscriptionInfo.SubscriptionName
	}

	if len(usage.UsageBreakdownList) > 0 {
		breakdown := usage.UsageBreakdownList[0]
		info.UsageCurrent = breakdown.CurrentUsage
		info.UsageLimit = breakdown.UsageLimit
		if info.UsageLimit > 0 {
			info.UsagePercent = info.UsageCurrent / info.UsageLimit
		}
		if breakdown.FreeTrialInfo != nil {
			info.TrialUsageCurrent = breakdown.FreeTrialInfo.CurrentUsage
			info.TrialUsageLimit = breakdown.FreeTrialInfo.UsageLimit
			if info.TrialUsageLimit > 0 {
				info.TrialUsagePercent = info.TrialUsageCurrent / info.TrialUsageLimit
			}
			info.TrialStatus = breakdown.FreeTrialInfo.FreeTrialStatus
			if breakdown.FreeTrialInfo.FreeTrialExpiry != "" {
				if ts := parseJSONNumberUnix(breakdown.FreeTrialInfo.FreeTrialExpiry); ts > 0 {
					info.TrialExpiresAt = ts
				}
			}
		}
	}

	if usage.NextDateReset != "" {
		if ts := parseJSONNumberUnix(usage.NextDateReset); ts > 0 {
			info.NextResetDate = time.Unix(ts, 0).UTC().Format("2006-01-02")
		}
	}

	return info, nil
}

// AccountInfoError classifies a usage-fetch failure.
type AccountInfoError struct {
	Suspended bool
	AuthError bool
	Err       error
}

func (e *AccountInfoError) Error() string {
	if e.Err == nil {
		return "kiro: account info error"
	}
	return e.Err.Error()
}

func (e *AccountInfoError) Unwrap() error { return e.Err }

func classifyAccountInfoError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "TEMPORARILY_SUSPENDED") {
		return &AccountInfoError{Suspended: true, Err: err}
	}
	if strings.Contains(msg, "403") || strings.Contains(msg, "401") ||
		strings.Contains(msg, "invalid") || strings.Contains(msg, "expired") {
		return &AccountInfoError{AuthError: true, Err: err}
	}
	return &AccountInfoError{Err: err}
}

func parseSubscriptionType(raw string) string {
	upper := strings.ToUpper(raw)
	if strings.Contains(upper, "PRO_PLUS") || strings.Contains(upper, "PROPLUS") {
		return "PRO_PLUS"
	}
	if strings.Contains(upper, "POWER") {
		return "POWER"
	}
	if strings.Contains(upper, "PRO") {
		return "PRO"
	}
	return "FREE"
}

func parseJSONNumberUnix(n json.Number) int64 {
	if ts, err := n.Int64(); err == nil && ts > 0 {
		return ts
	}
	if f, err := n.Float64(); err == nil && f > 0 {
		return int64(f)
	}
	return 0
}

func restClient(cred *Credential) *http.Client {
	return GetHTTPClientForProxy(proxyOf(cred), restHTTPTimeout)
}
