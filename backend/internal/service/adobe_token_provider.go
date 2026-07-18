package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/ims"
	"golang.org/x/sync/singleflight"
)

const defaultAdobeTokenRefreshSkew = 5 * time.Minute

type adobeTokenAccountRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	Update(ctx context.Context, account *Account) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

type adobeTokenProxyRepository interface {
	GetByID(ctx context.Context, id int64) (*Proxy, error)
}

// AdobeTokenProvider owns Adobe access-token refresh and profile/credits refresh.
type AdobeTokenProvider struct {
	accountRepo adobeTokenAccountRepository
	proxyRepo   adobeTokenProxyRepository
	cfg         *config.Config
	flight      singleflight.Group
	invalidated sync.Map

	refreshViaDevice func(context.Context, string, string, ims.RefreshOptions) (*ims.FullResult, error)
	refreshViaCookie func(context.Context, string, ims.RefreshOptions) (*ims.FullResult, error)
	fetchOnly        func(context.Context, string, ims.RefreshOptions) *ims.FullResult
}

func NewAdobeTokenProvider(accountRepo adobeTokenAccountRepository, proxyRepo adobeTokenProxyRepository, cfg *config.Config) *AdobeTokenProvider {
	return &AdobeTokenProvider{
		accountRepo:      accountRepo,
		proxyRepo:        proxyRepo,
		cfg:              cfg,
		refreshViaDevice: ims.RefreshOneViaDeviceToken,
		refreshViaCookie: ims.RefreshOne,
		fetchOnly:        ims.FetchOnly,
	}
}

type AdobeCredentialVerificationSummary struct {
	DisplayName  string     `json:"display_name,omitempty"`
	Email        string     `json:"email,omitempty"`
	Credits      *float64   `json:"credits,omitempty"`
	CreditsKnown bool       `json:"credits_known"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// VerifyCredentials validates transient Adobe credentials without loading or
// mutating a persisted account. It intentionally never calls persistResult.
func (p *AdobeTokenProvider) VerifyCredentials(ctx context.Context, account *Account) (*AdobeCredentialVerificationSummary, error) {
	if p == nil || p.fetchOnly == nil || p.refreshViaDevice == nil || p.refreshViaCookie == nil {
		return nil, errors.New("adobe token provider is not configured")
	}
	if account == nil || !account.IsAdobeOAuth() {
		return nil, errors.New("adobe OAuth account is required")
	}
	if err := ValidateAdobeAccountCredentials(account.Type, account.Credentials); err != nil {
		return nil, err
	}

	var result *ims.FullResult
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken != "" && !p.tokenNeedsRefresh(account) {
		result = p.fetchOnly(ctx, accessToken, p.refreshOptions(ctx, account))
	} else {
		var err error
		result, err = p.refresh(ctx, account)
		if err != nil {
			return nil, err
		}
	}
	if result == nil || strings.TrimSpace(result.AccessToken) == "" {
		return nil, errors.New("adobe credential verification failed")
	}
	if strings.TrimSpace(result.DisplayName) == "" &&
		strings.TrimSpace(result.Email) == "" &&
		strings.TrimSpace(result.UserID) == "" &&
		result.Credits < 0 {
		return nil, errors.New("adobe credential verification failed")
	}

	summary := &AdobeCredentialVerificationSummary{
		DisplayName:  result.DisplayName,
		Email:        result.Email,
		CreditsKnown: result.Credits >= 0,
	}
	if result.Credits >= 0 {
		credits := result.Credits
		summary.Credits = &credits
	}
	if result.ExpiresAt > 0 {
		expiresAt := time.Unix(result.ExpiresAt, 0).UTC()
		summary.ExpiresAt = &expiresAt
	}
	return summary, nil
}

func (p *AdobeTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	return p.getAccessToken(ctx, account, false)
}

func (p *AdobeTokenProvider) ForceRefresh(ctx context.Context, account *Account) (string, error) {
	return p.getAccessToken(ctx, account, true)
}

func (p *AdobeTokenProvider) InvalidateToken(_ context.Context, account *Account) error {
	if account != nil && account.ID > 0 {
		p.invalidated.Store(account.ID, struct{}{})
	}
	return nil
}

func (p *AdobeTokenProvider) RefreshProfileAndCredits(ctx context.Context, account *Account, force bool) (*AdobeCreditsInfo, error) {
	if account == nil || !account.IsAdobeOAuth() {
		return nil, errors.New("adobe OAuth account is required")
	}
	if !force {
		if cached := AdobeCreditsInfoFromExtra(account.Extra); cached != nil && !p.creditsCacheExpired(cached.UpdatedAt) {
			return cached, nil
		}
	}
	token, err := p.getAccessToken(ctx, account, false)
	if err != nil {
		return nil, err
	}
	result := p.fetchOnly(ctx, token, p.refreshOptions(ctx, account))
	if result == nil {
		return nil, errors.New("adobe IMS profile request failed")
	}
	if err := p.persistResult(ctx, account, result, false); err != nil {
		return nil, err
	}
	return AdobeCreditsInfoFromExtra(account.Extra), nil
}

func (p *AdobeTokenProvider) getAccessToken(ctx context.Context, account *Account, force bool) (string, error) {
	if p == nil || p.accountRepo == nil {
		return "", errors.New("adobe token provider is not configured")
	}
	if account == nil || !account.IsAdobeOAuth() {
		return "", errors.New("adobe OAuth account is required")
	}
	_, invalidated := p.invalidated.Load(account.ID)
	if !force && !invalidated {
		if token := strings.TrimSpace(account.GetCredential("access_token")); token != "" && !p.tokenNeedsRefresh(account) {
			return token, nil
		}
	}

	key := fmt.Sprintf("adobe:%d", account.ID)
	value, err, _ := p.flight.Do(key, func() (any, error) {
		current, loadErr := p.accountRepo.GetByID(ctx, account.ID)
		if loadErr != nil {
			return "", loadErr
		}
		if current == nil || !current.IsAdobeOAuth() {
			return "", errors.New("adobe OAuth account is required")
		}
		if !force {
			if token := strings.TrimSpace(current.GetCredential("access_token")); token != "" && !p.tokenNeedsRefresh(current) {
				p.invalidated.Delete(current.ID)
				account.Credentials = shallowCopyMap(current.Credentials)
				account.Extra = shallowCopyMap(current.Extra)
				return token, nil
			}
		}
		result, refreshErr := p.refresh(ctx, current)
		if refreshErr != nil {
			return "", refreshErr
		}
		if err := p.persistResult(ctx, current, result, true); err != nil {
			return "", err
		}
		p.invalidated.Delete(current.ID)
		account.Credentials = shallowCopyMap(current.Credentials)
		account.Extra = shallowCopyMap(current.Extra)
		return result.AccessToken, nil
	})
	if err != nil {
		return "", err
	}
	token, _ := value.(string)
	if strings.TrimSpace(token) == "" {
		return "", errors.New("adobe IMS returned an empty access token")
	}
	return token, nil
}

func (p *AdobeTokenProvider) refresh(ctx context.Context, account *Account) (*ims.FullResult, error) {
	options := p.refreshOptions(ctx, account)
	deviceToken := strings.TrimSpace(account.GetCredential("device_token"))
	deviceID := strings.TrimSpace(account.GetCredential("device_id"))
	cookie := strings.TrimSpace(account.GetCredential("cookie"))

	var deviceErr error
	if deviceToken != "" && deviceID != "" {
		result, err := p.refreshViaDevice(ctx, deviceToken, deviceID, options)
		if err == nil {
			return result, nil
		}
		deviceErr = err
	}
	if cookie != "" {
		result, err := p.refreshViaCookie(ctx, cookie, options)
		if err == nil {
			return result, nil
		}
		if deviceErr != nil {
			return nil, fmt.Errorf("adobe credential refresh failed for device and cookie sources")
		}
		return nil, fmt.Errorf("adobe cookie credential refresh failed")
	}
	if deviceErr != nil {
		return nil, fmt.Errorf("adobe device credential refresh failed")
	}
	return nil, errors.New("adobe account has no refresh source")
}

func (p *AdobeTokenProvider) persistResult(ctx context.Context, account *Account, result *ims.FullResult, persistToken bool) error {
	if result == nil {
		return errors.New("adobe IMS returned no result")
	}
	if persistToken {
		credentials := shallowCopyMap(account.Credentials)
		credentials["access_token"] = result.AccessToken
		if result.ExpiresAt > 0 {
			credentials["expires_at"] = time.Unix(result.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}
		if err := persistAccountCredentials(ctx, p.accountRepo, account, credentials); err != nil {
			return fmt.Errorf("persist Adobe access token: %w", err)
		}
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"adobe_profile_updated_at": now.Format(time.RFC3339),
		"adobe_credits_updated_at": now.Format(time.RFC3339),
		"adobe_credits_known":      result.Credits >= 0,
	}
	if result.DisplayName != "" {
		updates["adobe_display_name"] = result.DisplayName
	}
	if result.Email != "" {
		updates["adobe_email"] = result.Email
	}
	if result.UserID != "" {
		updates["adobe_user_id"] = result.UserID
	}
	if result.Credits >= 0 {
		updates["adobe_credits"] = result.Credits
	} else {
		delete(updates, "adobe_credits")
	}
	if err := p.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		return fmt.Errorf("persist Adobe profile and credits: %w", err)
	}
	mergeAccountExtra(account, updates)
	if result.Credits < 0 && account.Extra != nil {
		delete(account.Extra, "adobe_credits")
	}
	return nil
}

func (p *AdobeTokenProvider) tokenNeedsRefresh(account *Account) bool {
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return strings.TrimSpace(account.GetCredential("device_token")) != "" || strings.TrimSpace(account.GetCredential("cookie")) != ""
	}
	return !time.Now().Add(p.refreshSkew()).Before(*expiresAt)
}

func (p *AdobeTokenProvider) refreshSkew() time.Duration {
	if p != nil && p.cfg != nil && p.cfg.Adobe.TokenRefreshSkewSeconds > 0 {
		return time.Duration(p.cfg.Adobe.TokenRefreshSkewSeconds) * time.Second
	}
	return defaultAdobeTokenRefreshSkew
}

func (p *AdobeTokenProvider) creditsCacheExpired(updatedAt *time.Time) bool {
	if updatedAt == nil {
		return true
	}
	ttl := 5 * time.Minute
	if p != nil && p.cfg != nil && p.cfg.Adobe.CreditsCacheTTLSeconds > 0 {
		ttl = time.Duration(p.cfg.Adobe.CreditsCacheTTLSeconds) * time.Second
	}
	return time.Since(*updatedAt) >= ttl
}

func (p *AdobeTokenProvider) refreshOptions(ctx context.Context, account *Account) ims.RefreshOptions {
	options := ims.RefreshOptions{Timeout: 30 * time.Second}
	if p != nil && p.cfg != nil && p.cfg.Adobe.RequestTimeoutSeconds > 0 {
		options.Timeout = time.Duration(p.cfg.Adobe.RequestTimeoutSeconds) * time.Second
	}
	if account != nil && account.Proxy != nil {
		options.ProxyURL = account.Proxy.URL()
	} else if account != nil && account.ProxyID != nil && p != nil && p.proxyRepo != nil {
		if proxy, err := p.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			options.ProxyURL = proxy.URL()
		}
	}
	return options
}
