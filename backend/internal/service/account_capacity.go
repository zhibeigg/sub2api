package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"golang.org/x/sync/singleflight"
)

const (
	AccountCapacityModeUpstreamBalance = "upstream_balance"
	AccountCapacityModeUsageWindow     = "usage_window"
	AccountCapacityModeLocalQuota      = "local_quota"

	AccountCapacityStateVerified    = "verified"
	AccountCapacityStateEstimated   = "estimated"
	AccountCapacityStateStale       = "stale"
	AccountCapacityStateUnsupported = "unsupported"
	AccountCapacityStateUnknown     = "unknown"
	AccountCapacityStateUnlimited   = "unlimited"

	AccountCapacityProviderSub2API = "sub2api"
	AccountCapacityProviderLocal   = "local"

	accountCapacityMaxBodyBytes = int64(64 * 1024)
	accountCapacityMaxNumber    = 1e15
)

// AccountCapacitySnapshot is a normalized account-capacity view shared by the
// admin usage window and the pool-capacity alert evaluator.
type AccountCapacitySnapshot struct {
	Mode                       string     `json:"mode"`
	State                      string     `json:"state"`
	Provider                   string     `json:"provider,omitempty"`
	Scope                      string     `json:"scope,omitempty"`
	Authoritative              bool       `json:"authoritative"`
	Remaining                  *float64   `json:"remaining,omitempty"`
	Total                      *float64   `json:"total,omitempty"`
	Used                       *float64   `json:"used,omitempty"`
	Unit                       string     `json:"unit,omitempty"`
	EstimatedRemainingRequests *int64     `json:"estimated_remaining_requests,omitempty"`
	AverageCostPerRequest      *float64   `json:"average_cost_per_request,omitempty"`
	SampleRequests             int64      `json:"sample_requests,omitempty"`
	FetchedAt                  *time.Time `json:"fetched_at,omitempty"`
	ResetAt                    *time.Time `json:"reset_at,omitempty"`
	MessageCode                string     `json:"message_code,omitempty"`
}

// PoolBalanceReader intentionally exposes only the authoritative pool balance
// operation required by AccountUsageService and PoolCapacityAlertService.
type PoolBalanceReader interface {
	GetPoolBalance(ctx context.Context, account *Account, force bool) (*AccountCapacitySnapshot, error)
}

type accountCapacityCacheEntry struct {
	lastResult    *AccountCapacitySnapshot
	lastAttemptAt time.Time
	lastSuccess   *AccountCapacitySnapshot
}

// AccountCapacityService retrieves Sub2API-compatible upstream balances using
// the account's configured proxy and TLS profile. It never sends native AWS
// credentials as Bearer tokens.
type AccountCapacityService struct {
	httpUpstream HTTPUpstream
	cfg          *config.Config
	tlsProfiles  *TLSFingerprintProfileService

	cache  sync.Map
	flight singleflight.Group
	now    func() time.Time
}

func NewAccountCapacityService(httpUpstream HTTPUpstream, cfg *config.Config, tlsProfiles *TLSFingerprintProfileService) *AccountCapacityService {
	return &AccountCapacityService{
		httpUpstream: httpUpstream,
		cfg:          cfg,
		tlsProfiles:  tlsProfiles,
		now:          time.Now,
	}
}

func (s *AccountCapacityService) GetPoolBalance(ctx context.Context, account *Account, force bool) (*AccountCapacitySnapshot, error) {
	if account == nil || !account.IsPoolMode() {
		return newCapacityState(AccountCapacityStateUnsupported, "pool_mode_required"), nil
	}
	baseURL, apiKey, state := poolBalanceCredentials(account)
	if state != nil {
		return state, nil
	}
	if s == nil || s.httpUpstream == nil {
		return newCapacityState(AccountCapacityStateUnknown, "transport_unavailable"), nil
	}

	identity := accountCapacityIdentity(account, baseURL, apiKey)
	now := s.currentTime()
	if !force {
		if cached := s.cachedResult(identity, now); cached != nil {
			return cached, nil
		}
	}

	resultCh := s.flight.DoChan(identity, func() (any, error) {
		queryNow := s.currentTime()
		if !force {
			if cached := s.cachedResult(identity, queryNow); cached != nil {
				return cached, nil
			}
		}
		snapshot, queryErr := s.fetchSub2APIUsage(context.Background(), account, baseURL, apiKey, queryNow)
		if queryErr != nil {
			return nil, queryErr
		}
		s.storeResult(identity, snapshot, queryNow)
		if snapshot.State != AccountCapacityStateVerified && snapshot.State != AccountCapacityStateUnlimited {
			if stale := s.staleResult(identity, queryNow, snapshot.MessageCode); stale != nil {
				return stale, nil
			}
		}
		return cloneAccountCapacitySnapshot(snapshot), nil
	})
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		snapshot, _ := result.Val.(*AccountCapacitySnapshot)
		return cloneAccountCapacitySnapshot(snapshot), nil
	}
}

func (s *AccountCapacityService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *AccountCapacityService) successTTL() time.Duration {
	seconds := 60
	if s != nil && s.cfg != nil && s.cfg.AccountCapacity.SuccessCacheSeconds > 0 {
		seconds = s.cfg.AccountCapacity.SuccessCacheSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *AccountCapacityService) errorTTL() time.Duration {
	seconds := 30
	if s != nil && s.cfg != nil && s.cfg.AccountCapacity.ErrorCacheSeconds > 0 {
		seconds = s.cfg.AccountCapacity.ErrorCacheSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *AccountCapacityService) staleTTL() time.Duration {
	seconds := 300
	if s != nil && s.cfg != nil && s.cfg.AccountCapacity.StaleCacheSeconds > 0 {
		seconds = s.cfg.AccountCapacity.StaleCacheSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *AccountCapacityService) requestTimeout() time.Duration {
	seconds := 10
	if s != nil && s.cfg != nil && s.cfg.AccountCapacity.UpstreamTimeoutSeconds > 0 {
		seconds = s.cfg.AccountCapacity.UpstreamTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *AccountCapacityService) cachedResult(identity string, now time.Time) *AccountCapacitySnapshot {
	raw, ok := s.cache.Load(identity)
	if !ok {
		return nil
	}
	entry, ok := raw.(*accountCapacityCacheEntry)
	if !ok || entry == nil || entry.lastResult == nil {
		return nil
	}
	age := now.Sub(entry.lastAttemptAt)
	if entry.lastResult.State == AccountCapacityStateVerified || entry.lastResult.State == AccountCapacityStateUnlimited {
		if age < s.successTTL() {
			return cloneAccountCapacitySnapshot(entry.lastResult)
		}
		return nil
	}
	if age >= s.errorTTL() {
		return nil
	}
	if stale := s.staleFromEntry(entry, now, entry.lastResult.MessageCode); stale != nil {
		return stale
	}
	return cloneAccountCapacitySnapshot(entry.lastResult)
}

func (s *AccountCapacityService) staleResult(identity string, now time.Time, messageCode string) *AccountCapacitySnapshot {
	raw, ok := s.cache.Load(identity)
	if !ok {
		return nil
	}
	entry, _ := raw.(*accountCapacityCacheEntry)
	return s.staleFromEntry(entry, now, messageCode)
}

func (s *AccountCapacityService) staleFromEntry(entry *accountCapacityCacheEntry, now time.Time, messageCode string) *AccountCapacitySnapshot {
	if entry == nil || entry.lastSuccess == nil || entry.lastSuccess.FetchedAt == nil || now.Sub(*entry.lastSuccess.FetchedAt) >= s.staleTTL() {
		return nil
	}
	stale := cloneAccountCapacitySnapshot(entry.lastSuccess)
	stale.State = AccountCapacityStateStale
	stale.Authoritative = false
	stale.MessageCode = messageCode
	return stale
}

func (s *AccountCapacityService) storeResult(identity string, snapshot *AccountCapacitySnapshot, now time.Time) {
	entry := &accountCapacityCacheEntry{
		lastResult:    cloneAccountCapacitySnapshot(snapshot),
		lastAttemptAt: now,
	}
	if raw, ok := s.cache.Load(identity); ok {
		if previous, ok := raw.(*accountCapacityCacheEntry); ok && previous != nil {
			entry.lastSuccess = cloneAccountCapacitySnapshot(previous.lastSuccess)
		}
	}
	if snapshot != nil && (snapshot.State == AccountCapacityStateVerified || snapshot.State == AccountCapacityStateUnlimited) {
		entry.lastSuccess = cloneAccountCapacitySnapshot(snapshot)
	}
	s.cache.Store(identity, entry)
}

func poolBalanceCredentials(account *Account) (string, string, *AccountCapacitySnapshot) {
	if account == nil || !account.IsAPIKeyOrBedrock() {
		return "", "", newCapacityState(AccountCapacityStateUnsupported, "account_type_unsupported")
	}
	if account.IsBedrock() && !account.IsBedrockAPIKey() {
		return "", "", newCapacityState(AccountCapacityStateUnsupported, "native_bedrock_balance_unsupported")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" {
		return "", "", newCapacityState(AccountCapacityStateUnsupported, "custom_base_url_required")
	}
	if apiKey == "" {
		return "", "", newCapacityState(AccountCapacityStateUnknown, "missing_api_key")
	}
	return baseURL, apiKey, nil
}

func accountCapacityIdentity(account *Account, baseURL, apiKey string) string {
	proxyID := int64(0)
	if account != nil && account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	payload := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%s", account.ID, strings.TrimSpace(baseURL), apiKey, proxyID, account.Platform, account.Type)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func (s *AccountCapacityService) fetchSub2APIUsage(ctx context.Context, account *Account, baseURL, apiKey string, now time.Time) (*AccountCapacitySnapshot, error) {
	normalizedBaseURL, err := validateConfiguredUpstreamBaseURL(s.cfg, baseURL)
	if err != nil {
		return newCapacityState(AccountCapacityStateUnknown, "invalid_base_url"), nil
	}
	parsedBaseURL, err := url.Parse(normalizedBaseURL)
	if err != nil || parsedBaseURL.User != nil {
		return newCapacityState(AccountCapacityStateUnknown, "invalid_base_url"), nil
	}
	parsedBaseURL.RawQuery = ""
	parsedBaseURL.Fragment = ""
	endpoint := buildOpenAIEndpointURL(parsedBaseURL.String(), "/v1/usage")

	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return newCapacityState(AccountCapacityStateUnknown, "proxy_unavailable"), nil
		}
		if account.Proxy.ID != *account.ProxyID {
			return newCapacityState(AccountCapacityStateUnknown, "proxy_identity_changed"), nil
		}
		proxyURL = account.Proxy.URL()
	}

	requestCtx, cancel := context.WithTimeout(ctx, s.requestTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return newCapacityState(AccountCapacityStateUnknown, "request_build_failed"), nil
	}
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	var profile *tlsfingerprint.Profile
	if s.tlsProfiles != nil {
		profile = s.tlsProfiles.ResolveTLSProfile(account)
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, profile)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || requestCtx.Err() != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return newCapacityState(AccountCapacityStateUnknown, "upstream_timeout"), nil
		}
		return newCapacityState(AccountCapacityStateUnknown, "upstream_request_failed"), nil
	}
	if resp == nil || resp.Body == nil {
		return newCapacityState(AccountCapacityStateUnknown, "empty_response"), nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, accountCapacityMaxBodyBytes+1))
	if err != nil {
		return newCapacityState(AccountCapacityStateUnknown, "response_read_failed"), nil
	}
	if int64(len(body)) > accountCapacityMaxBodyBytes {
		return newCapacityState(AccountCapacityStateUnknown, "response_too_large"), nil
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return newCapacityState(AccountCapacityStateUnknown, "redirect_not_allowed"), nil
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return newCapacityState(AccountCapacityStateUnknown, "upstream_auth_failed"), nil
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return newCapacityState(AccountCapacityStateUnsupported, "upstream_usage_unsupported"), nil
	case http.StatusTooManyRequests:
		return newCapacityState(AccountCapacityStateUnknown, "upstream_rate_limited"), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newCapacityState(AccountCapacityStateUnknown, "upstream_http_error"), nil
	}

	snapshot, err := parseSub2APIUsageResponse(body, now)
	if err != nil {
		return newCapacityState(AccountCapacityStateUnknown, "upstream_response_invalid"), nil
	}
	return snapshot, nil
}

type sub2APIUsageResponse struct {
	Object        string                    `json:"object"`
	SchemaVersion int                       `json:"schema_version"`
	Mode          string                    `json:"mode"`
	IsValid       *bool                     `json:"isValid"`
	PlanName      string                    `json:"planName"`
	Remaining     *json.Number              `json:"remaining"`
	Unit          string                    `json:"unit"`
	Balance       *json.Number              `json:"balance"`
	Quota         *sub2APIUsageQuota        `json:"quota"`
	Subscription  *sub2APIUsageSubscription `json:"subscription"`
}

type sub2APIUsageQuota struct {
	Limit     *json.Number `json:"limit"`
	Used      *json.Number `json:"used"`
	Remaining *json.Number `json:"remaining"`
	Unit      string       `json:"unit"`
}

type sub2APIUsageSubscription struct {
	DailyUsageUSD     *json.Number `json:"daily_usage_usd"`
	WeeklyUsageUSD    *json.Number `json:"weekly_usage_usd"`
	MonthlyUsageUSD   *json.Number `json:"monthly_usage_usd"`
	DailyLimitUSD     *json.Number `json:"daily_limit_usd"`
	WeeklyLimitUSD    *json.Number `json:"weekly_limit_usd"`
	MonthlyLimitUSD   *json.Number `json:"monthly_limit_usd"`
	WeeklyWindowStart *time.Time   `json:"weekly_window_start"`
	ExpiresAt         *time.Time   `json:"expires_at"`
}

func parseSub2APIUsageResponse(body []byte, now time.Time) (*AccountCapacitySnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var response sub2APIUsageResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("unexpected trailing json value")
		}
		return nil, err
	}
	if response.Mode != "quota_limited" && response.Mode != "unrestricted" {
		return nil, errors.New("unexpected usage mode")
	}
	if response.IsValid == nil {
		return nil, errors.New("missing isValid")
	}
	if !*response.IsValid {
		return newCapacityState(AccountCapacityStateUnknown, "upstream_account_invalid"), nil
	}
	if response.Object != "" {
		if response.Object != "sub2api.key_usage" || response.SchemaVersion != 1 {
			return nil, errors.New("unexpected usage schema")
		}
	} else if response.SchemaVersion != 0 {
		return nil, errors.New("schema version without object")
	}

	unit := strings.TrimSpace(response.Unit)
	if len(unit) > 16 {
		return nil, errors.New("invalid unit")
	}
	fetchedAt := now.UTC()
	base := &AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateVerified,
		Provider:      AccountCapacityProviderSub2API,
		Authoritative: true,
		Unit:          unit,
		FetchedAt:     &fetchedAt,
	}

	switch response.Mode {
	case "quota_limited":
		if response.Quota == nil || response.Subscription != nil || response.Balance != nil || response.Remaining == nil {
			return nil, errors.New("invalid quota payload shape")
		}
		limit, err := finiteNonNegativeNumber(response.Quota.Limit)
		if err != nil {
			return nil, err
		}
		used, err := finiteNonNegativeNumber(response.Quota.Used)
		if err != nil {
			return nil, err
		}
		remaining, err := finiteNonNegativeNumber(response.Quota.Remaining)
		if err != nil {
			return nil, err
		}
		if *used <= *limit {
			if !approximatelyEqual(*used+*remaining, *limit) {
				return nil, errors.New("invalid quota totals")
			}
		} else if !approximatelyEqual(*remaining, 0) {
			return nil, errors.New("invalid exhausted quota remaining")
		}
		top, err := finiteNonNegativeNumber(response.Remaining)
		if err != nil || !approximatelyEqual(*top, *remaining) {
			return nil, errors.New("quota remaining mismatch")
		}
		quotaUnit := strings.TrimSpace(response.Quota.Unit)
		if quotaUnit == "" || len(quotaUnit) > 16 || (unit != "" && !strings.EqualFold(unit, quotaUnit)) {
			return nil, errors.New("invalid quota unit")
		}
		base.Scope, base.Total, base.Used, base.Remaining, base.Unit = "quota", limit, used, remaining, quotaUnit
		return base, nil
	case "unrestricted":
		if response.Quota != nil || (response.Subscription != nil && response.Balance != nil) {
			return nil, errors.New("invalid unrestricted payload shape")
		}
		if response.Subscription != nil {
			return normalizeSub2APISubscription(base, response.Subscription, response.Remaining)
		}
		if response.Balance == nil || response.Remaining == nil {
			return nil, errors.New("missing recognized usage payload")
		}
		balance, err := finiteNonNegativeNumber(response.Balance)
		if err != nil {
			return nil, errors.New("invalid balance")
		}
		remaining, err := finiteNonNegativeNumber(response.Remaining)
		if err != nil || !approximatelyEqual(*balance, *remaining) {
			return nil, errors.New("balance remaining mismatch")
		}
		base.Scope, base.Remaining = "balance", balance
		return base, nil
	default:
		return nil, errors.New("unexpected usage mode")
	}
}

func normalizeSub2APISubscription(base *AccountCapacitySnapshot, subscription *sub2APIUsageSubscription, topRemaining *json.Number) (*AccountCapacitySnapshot, error) {
	type dimension struct {
		scope string
		used  *json.Number
		limit *json.Number
		reset *time.Time
	}
	weeklyReset := (*time.Time)(nil)
	if subscription.WeeklyWindowStart != nil {
		value := subscription.WeeklyWindowStart.Add(7 * 24 * time.Hour).UTC()
		weeklyReset = &value
	}
	dimensions := []dimension{
		{scope: "subscription_daily", used: subscription.DailyUsageUSD, limit: subscription.DailyLimitUSD},
		{scope: "subscription_weekly", used: subscription.WeeklyUsageUSD, limit: subscription.WeeklyLimitUSD, reset: weeklyReset},
		{scope: "subscription_monthly", used: subscription.MonthlyUsageUSD, limit: subscription.MonthlyLimitUSD, reset: subscription.ExpiresAt},
	}
	var selected *AccountCapacitySnapshot
	for _, dimension := range dimensions {
		limit, err := optionalFiniteNonNegativeNumber(dimension.limit)
		if err != nil {
			return nil, err
		}
		if limit == nil {
			continue
		}
		if dimension.used == nil {
			return nil, errors.New("subscription limit missing usage")
		}
		used, err := finiteNonNegativeNumber(dimension.used)
		if err != nil {
			return nil, err
		}
		usedValue := *used
		remainingValue := math.Max(*limit-usedValue, 0)
		candidate := cloneAccountCapacitySnapshot(base)
		candidate.Scope = dimension.scope
		candidate.Total = capacityFloat64Ptr(*limit)
		candidate.Used = capacityFloat64Ptr(usedValue)
		candidate.Remaining = capacityFloat64Ptr(remainingValue)
		candidate.ResetAt = cloneCapacityTimePtr(dimension.reset)
		if selected == nil || remainingValue < *selected.Remaining {
			selected = candidate
		}
	}
	if selected == nil {
		if topRemaining != nil && strings.TrimSpace(topRemaining.String()) == "-1" {
			unlimited := cloneAccountCapacitySnapshot(base)
			unlimited.State = AccountCapacityStateUnlimited
			unlimited.Scope = "subscription"
			return unlimited, nil
		}
		return nil, errors.New("subscription has no finite limit")
	}
	if topRemaining == nil {
		return nil, errors.New("subscription remaining is required")
	}
	top, err := finiteNonNegativeNumber(topRemaining)
	if err != nil || !approximatelyEqual(*top, *selected.Remaining) {
		return nil, errors.New("subscription remaining mismatch")
	}
	return selected, nil
}

func optionalFiniteNonNegativeNumber(value *json.Number) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	return finiteNonNegativeNumber(value)
}

func finiteNonNegativeNumber(value *json.Number) (*float64, error) {
	if value == nil {
		return nil, errors.New("missing number")
	}
	parsed, err := value.Float64()
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > accountCapacityMaxNumber {
		return nil, errors.New("invalid number")
	}
	return &parsed, nil
}

func approximatelyEqual(left, right float64) bool {
	return math.Abs(left-right) <= 1e-6
}

func newCapacityState(state, messageCode string) *AccountCapacitySnapshot {
	return &AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         state,
		Authoritative: false,
		MessageCode:   messageCode,
	}
}

func cloneAccountCapacitySnapshot(snapshot *AccountCapacitySnapshot) *AccountCapacitySnapshot {
	if snapshot == nil {
		return nil
	}
	copyValue := *snapshot
	copyValue.Remaining = cloneCapacityFloat64Ptr(snapshot.Remaining)
	copyValue.Total = cloneCapacityFloat64Ptr(snapshot.Total)
	copyValue.Used = cloneCapacityFloat64Ptr(snapshot.Used)
	copyValue.EstimatedRemainingRequests = cloneCapacityInt64Ptr(snapshot.EstimatedRemainingRequests)
	copyValue.AverageCostPerRequest = cloneCapacityFloat64Ptr(snapshot.AverageCostPerRequest)
	copyValue.FetchedAt = cloneCapacityTimePtr(snapshot.FetchedAt)
	copyValue.ResetAt = cloneCapacityTimePtr(snapshot.ResetAt)
	return &copyValue
}

func cloneCapacityFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneCapacityInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneCapacityTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func capacityFloat64Ptr(value float64) *float64 {
	return &value
}
