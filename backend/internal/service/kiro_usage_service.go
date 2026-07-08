package service

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// kiroUsageSnapshotExtraKey is the Account.Extra key holding the latest Kiro
// usage/subscription/overage snapshot. It is scheduler-neutral (observation
// only) so writes must not trigger scheduler bucket recomputation.
const kiroUsageSnapshotExtraKey = "kiro_usage_snapshot"

// kiroContextUsageExtraKey stores the latest observed context-usage percentage
// (0-100). Observation-only; must be scheduler-neutral.
const kiroContextUsageExtraKey = "kiro_context_usage_pct"

const (
	kiroUsageProbeTimeout = 25 * time.Second
)

// KiroUsageSnapshot is the observability snapshot persisted to Account.Extra.
type KiroUsageSnapshot struct {
	SubscriptionType string  `json:"subscription_type,omitempty"`
	SubscriptionRaw  string  `json:"subscription_raw,omitempty"`
	UsageCurrent     float64 `json:"usage_current,omitempty"`
	UsageLimit       float64 `json:"usage_limit,omitempty"`
	UsagePercent     float64 `json:"usage_percent,omitempty"`
	TrialCurrent     float64 `json:"trial_current,omitempty"`
	TrialLimit       float64 `json:"trial_limit,omitempty"`
	TrialStatus      string  `json:"trial_status,omitempty"`
	NextResetDate    string  `json:"next_reset_date,omitempty"`
	OverageStatus    string  `json:"overage_status,omitempty"`
	OverageCap       float64 `json:"overage_cap,omitempty"`
	OverageRate      float64 `json:"overage_rate,omitempty"`
	CurrentOverages  float64 `json:"current_overages,omitempty"`
	Email            string  `json:"email,omitempty"`
	UserID           string  `json:"user_id,omitempty"`
	CheckedAt        int64   `json:"checked_at,omitempty"`
}

// KiroUsageProbeResult is returned by ProbeUsage.
type KiroUsageProbeResult struct {
	Snapshot  *KiroUsageSnapshot `json:"snapshot"`
	FetchedAt int64              `json:"fetched_at"`
}

// kiroDiscoveredModels caches upstream-discovered Kiro model IDs (populated as a
// side effect of usage probes). It augments the hardcoded model whitelist for
// the /v1/models endpoint, which is account-agnostic.
var kiroDiscoveredModels sync.Map // map[string]struct{}

// KiroDiscoveredModelIDs returns the set of model IDs discovered from the
// upstream ListAvailableModels calls, sorted for stable output.
func KiroDiscoveredModelIDs() []string {
	ids := make([]string, 0)
	kiroDiscoveredModels.Range(func(key, _ any) bool {
		if id, ok := key.(string); ok && id != "" {
			ids = append(ids, id)
		}
		return true
	})
	sort.Strings(ids)
	return ids
}

func rememberKiroModels(models []kiro.ModelInfo) {
	for _, m := range models {
		id := strings.TrimSpace(m.ModelId)
		if id == "" {
			id = strings.TrimSpace(m.ModelName)
		}
		if id != "" {
			kiroDiscoveredModels.Store(id, struct{}{})
		}
	}
}

// KiroUsageService fetches account usage / subscription / overage metadata from
// the AWS CodeWhisperer / Q REST APIs and persists snapshots to Account.Extra.
// It mirrors GrokQuotaService's structure.
type KiroUsageService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *KiroTokenProvider
}

// NewKiroUsageService constructs a KiroUsageService.
func NewKiroUsageService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *KiroTokenProvider,
) *KiroUsageService {
	return &KiroUsageService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
	}
}

// ProbeUsage fetches usage limits (and best-effort overage) for an account and
// persists the snapshot. On suspension/auth errors it marks the account errored.
func (s *KiroUsageService) ProbeUsage(ctx context.Context, accountID int64) (*KiroUsageProbeResult, error) {
	account, cred, err := s.prepare(ctx, accountID)
	if err != nil {
		return nil, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, kiroUsageProbeTimeout)
	defer cancel()

	info, err := kiro.RefreshAccountInfo(probeCtx, cred)
	if err != nil {
		s.handleProbeError(ctx, account, err)
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_USAGE_PROBE_FAILED", "usage probe failed: %v", err)
	}

	snapshot := &KiroUsageSnapshot{
		SubscriptionType: info.SubscriptionType,
		SubscriptionRaw:  info.SubscriptionRaw,
		UsageCurrent:     info.UsageCurrent,
		UsageLimit:       info.UsageLimit,
		UsagePercent:     info.UsagePercent,
		TrialCurrent:     info.TrialUsageCurrent,
		TrialLimit:       info.TrialUsageLimit,
		TrialStatus:      info.TrialStatus,
		NextResetDate:    info.NextResetDate,
		Email:            info.Email,
		UserID:           info.UserId,
		CheckedAt:        time.Now().Unix(),
	}

	// Best-effort overage snapshot; failure here must not fail the probe.
	if overage, overErr := kiro.FetchOverageStatus(probeCtx, cred); overErr == nil && overage != nil {
		snapshot.OverageStatus = overage.Status
		snapshot.OverageCap = overage.OverageCap
		snapshot.OverageRate = overage.OverageRate
		snapshot.CurrentOverages = overage.CurrentOverages
	}

	// Best-effort model discovery to augment the account-agnostic /v1/models list.
	if models, modelErr := kiro.ListAvailableModels(probeCtx, cred); modelErr == nil && len(models) > 0 {
		rememberKiroModels(models)
	}

	s.persistSnapshot(ctx, account, snapshot)
	s.clearRecoverableError(ctx, account)

	return &KiroUsageProbeResult{Snapshot: snapshot, FetchedAt: snapshot.CheckedAt}, nil
}

// SetOverage flips the account's Overages switch upstream and refreshes the
// snapshot.
func (s *KiroUsageService) SetOverage(ctx context.Context, accountID int64, enabled bool) (*KiroUsageProbeResult, error) {
	account, cred, err := s.prepare(ctx, accountID)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, kiroUsageProbeTimeout)
	defer cancel()

	overage, err := kiro.SetOverageStatus(callCtx, cred, enabled)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_OVERAGE_SET_FAILED", "set overage failed: %v", err)
	}

	snapshot := s.loadSnapshot(account)
	if snapshot == nil {
		snapshot = &KiroUsageSnapshot{}
	}
	snapshot.OverageStatus = overage.Status
	snapshot.OverageCap = overage.OverageCap
	snapshot.OverageRate = overage.OverageRate
	snapshot.CurrentOverages = overage.CurrentOverages
	snapshot.CheckedAt = time.Now().Unix()

	s.persistSnapshot(ctx, account, snapshot)

	return &KiroUsageProbeResult{Snapshot: snapshot, FetchedAt: snapshot.CheckedAt}, nil
}

func (s *KiroUsageService) prepare(ctx context.Context, accountID int64) (*Account, *kiro.Credential, error) {
	if s == nil || s.accountRepo == nil || s.tokenProvider == nil {
		return nil, nil, infraerrors.New(http.StatusInternalServerError, "KIRO_USAGE_NOT_CONFIGURED", "kiro usage service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, nil, infraerrors.New(http.StatusNotFound, "KIRO_USAGE_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformKiro {
		return nil, nil, infraerrors.New(http.StatusBadRequest, "KIRO_USAGE_INVALID_PLATFORM", "account is not a Kiro account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, nil, infraerrors.New(http.StatusBadRequest, "KIRO_USAGE_INVALID_TYPE", "account is not an OAuth account")
	}

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_USAGE_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
	}

	cred := &kiro.Credential{
		AccessToken:  token,
		RefreshToken: account.GetCredential("refresh_token"),
		ClientID:     account.GetCredential("client_id"),
		ClientSecret: account.GetCredential("client_secret"),
		AuthMethod:   account.GetCredential("auth_method"),
		Region:       account.GetCredential("region"),
		ProfileArn:   account.GetCredential("profile_arn"),
		MachineID:    account.GetCredential("machine_id"),
		Provider:     account.GetCredential("provider"),
		Email:        account.GetCredential("email"),
		ProxyURL:     s.resolveProxyURL(ctx, account),
	}
	return account, cred, nil
}

func (s *KiroUsageService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s.proxyRepo != nil {
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			return proxy.URL()
		}
	}
	return ""
}

// persistSnapshot writes the snapshot to Account.Extra. The resolved profile
// ARN (if newly discovered) is cached in-process by pkg/kiro, so it does not
// need to be persisted here.
func (s *KiroUsageService) persistSnapshot(ctx context.Context, account *Account, snapshot *KiroUsageSnapshot) {
	if s.accountRepo == nil || account == nil || snapshot == nil {
		return
	}
	_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		kiroUsageSnapshotExtraKey: snapshot,
	})
}

func (s *KiroUsageService) loadSnapshot(account *Account) *KiroUsageSnapshot {
	if account == nil || account.Extra == nil {
		return nil
	}
	raw, ok := account.Extra[kiroUsageSnapshotExtraKey]
	if !ok {
		return nil
	}
	return decodeKiroSnapshot(raw)
}

func (s *KiroUsageService) handleProbeError(ctx context.Context, account *Account, err error) {
	var infoErr *kiro.AccountInfoError
	if !errors.As(err, &infoErr) {
		return
	}
	if s.accountRepo == nil || account == nil {
		return
	}
	switch {
	case infoErr.Suspended:
		_ = s.accountRepo.SetError(ctx, account.ID, "AWS temporarily suspended - unusual activity detected")
	case infoErr.AuthError:
		_ = s.accountRepo.SetError(ctx, account.ID, "Kiro authentication failed - token invalid or expired")
	}
}

func (s *KiroUsageService) clearRecoverableError(ctx context.Context, account *Account) {
	if s.accountRepo == nil || account == nil {
		return
	}
	if account.Status == StatusError {
		_ = s.accountRepo.ClearError(ctx, account.ID)
	}
}
