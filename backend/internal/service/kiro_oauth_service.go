package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// KiroOAuthTokenService is the subset of Kiro OAuth behavior the token refresher
// depends on (mirrors GrokOAuthTokenService).
type KiroOAuthTokenService interface {
	RefreshAccountToken(ctx context.Context, account *Account) (*KiroTokenInfo, error)
	BuildAccountCredentials(tokenInfo *KiroTokenInfo) map[string]any
}

// KiroTokenInfo is the normalized result of a Kiro token refresh.
type KiroTokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64 // Unix seconds
	ProfileArn   string
	// Static fields carried over from the account (not returned by refresh).
	ClientID     string
	ClientSecret string
	AuthMethod   string
	Region       string
	MachineID    string
	Provider     string
	Email        string
}

// KiroOAuthService handles Kiro account credential validation and token refresh.
// Unlike Grok/Antigravity it does not run an interactive OAuth authorize flow in
// the browser sense: Kiro accounts are imported via credentials JSON (AWS Builder
// ID / IAM Identity Center / social), and this service refreshes their tokens.
type KiroOAuthService struct {
	proxyRepo ProxyRepository
	sessions  *kiroSessionStore
}

// NewKiroOAuthService constructs a KiroOAuthService.
func NewKiroOAuthService(proxyRepo ProxyRepository) *KiroOAuthService {
	return &KiroOAuthService{
		proxyRepo: proxyRepo,
		sessions:  newKiroSessionStore(),
	}
}

// credentialFromAccount assembles a kiro.Credential from stored account credentials.
func (s *KiroOAuthService) credentialFromAccount(ctx context.Context, account *Account) (*kiro.Credential, error) {
	proxyURL := ""
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}
	cred := &kiro.Credential{
		AccessToken:  account.GetCredential("access_token"),
		RefreshToken: account.GetCredential("refresh_token"),
		ClientID:     account.GetCredential("client_id"),
		ClientSecret: account.GetCredential("client_secret"),
		AuthMethod:   account.GetCredential("auth_method"),
		Region:       account.GetCredential("region"),
		ProfileArn:   account.GetCredential("profile_arn"),
		MachineID:    account.GetCredential("machine_id"),
		Provider:     account.GetCredential("provider"),
		Email:        account.GetCredential("email"),
		ProxyURL:     proxyURL,
	}
	return cred, nil
}

// RefreshAccountToken refreshes the account's Kiro token.
func (s *KiroOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KiroTokenInfo, error) {
	if account == nil || account.Platform != PlatformKiro {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_OAUTH_INVALID_ACCOUNT", "account is not a Kiro account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}
	cred, err := s.credentialFromAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cred.RefreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}

	result, err := kiro.RefreshToken(ctx, cred)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "KIRO_OAUTH_REFRESH_FAILED", err.Error())
	}

	info := &KiroTokenInfo{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
		ProfileArn:   result.ProfileArn,
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
		AuthMethod:   cred.AuthMethod,
		Region:       cred.Region,
		MachineID:    cred.MachineID,
		Provider:     cred.Provider,
		Email:        cred.Email,
	}
	// The refresh endpoint may not return a new refresh token or profileArn;
	// fall back to the existing values so they are preserved.
	if info.RefreshToken == "" {
		info.RefreshToken = cred.RefreshToken
	}
	if info.ProfileArn == "" {
		info.ProfileArn = cred.ProfileArn
	}
	return info, nil
}

// BuildAccountCredentials assembles the credentials map to persist after refresh.
func (s *KiroOAuthService) BuildAccountCredentials(tokenInfo *KiroTokenInfo) map[string]any {
	if tokenInfo == nil {
		return nil
	}
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
	}
	if tokenInfo.ExpiresAt > 0 {
		creds["expires_at"] = time.Unix(tokenInfo.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.ProfileArn != "" {
		creds["profile_arn"] = tokenInfo.ProfileArn
	}
	if tokenInfo.ClientID != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	if tokenInfo.ClientSecret != "" {
		creds["client_secret"] = tokenInfo.ClientSecret
	}
	if tokenInfo.AuthMethod != "" {
		creds["auth_method"] = tokenInfo.AuthMethod
	}
	if tokenInfo.Region != "" {
		creds["region"] = tokenInfo.Region
	}
	if tokenInfo.MachineID != "" {
		creds["machine_id"] = tokenInfo.MachineID
	}
	if tokenInfo.Provider != "" {
		creds["provider"] = tokenInfo.Provider
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	return creds
}

// kiroImportPayload is the credentials JSON pasted by an admin when importing a
// Kiro account. Field names accept both the Kiro-Go export (camelCase) and
// snake_case variants.
type kiroImportPayload struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	AuthMethod   string `json:"authMethod"`
	Provider     string `json:"provider"`
	Region       string `json:"region"`
	ProfileArn   string `json:"profileArn"`
	MachineID    string `json:"machineId"`
	Email        string `json:"email"`
	ExpiresAt    int64  `json:"expiresAt"`

	// snake_case fallbacks
	AccessTokenSnake  string `json:"access_token"`
	RefreshTokenSnake string `json:"refresh_token"`
	ClientIDSnake     string `json:"client_id"`
	ClientSecretSnake string `json:"client_secret"`
	AuthMethodSnake   string `json:"auth_method"`
	RegionSnake       string `json:"region_name"`
	ProfileArnSnake   string `json:"profile_arn"`
	MachineIDSnake    string `json:"machine_id"`
	ExpiresAtSnake    int64  `json:"expires_at"`
}

// ParseKiroImportCredentials parses a pasted Kiro credentials JSON string into a
// credentials map suitable for storing on an account. It validates the minimum
// required fields per auth method.
func ParseKiroImportCredentials(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_IMPORT_EMPTY", "credentials JSON is empty")
	}
	var p kiroImportPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_IMPORT_INVALID_JSON", "invalid credentials JSON: "+err.Error())
	}

	pick := func(a, b string) string {
		if strings.TrimSpace(a) != "" {
			return a
		}
		return b
	}
	accessToken := pick(p.AccessToken, p.AccessTokenSnake)
	refreshToken := pick(p.RefreshToken, p.RefreshTokenSnake)
	clientID := pick(p.ClientID, p.ClientIDSnake)
	clientSecret := pick(p.ClientSecret, p.ClientSecretSnake)
	authMethod := pick(p.AuthMethod, p.AuthMethodSnake)
	region := pick(p.Region, p.RegionSnake)
	profileArn := pick(p.ProfileArn, p.ProfileArnSnake)
	machineID := pick(p.MachineID, p.MachineIDSnake)
	expiresAt := p.ExpiresAt
	if expiresAt == 0 {
		expiresAt = p.ExpiresAtSnake
	}

	if strings.TrimSpace(refreshToken) == "" && strings.TrimSpace(accessToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_IMPORT_NO_TOKEN", "credentials must contain accessToken or refreshToken")
	}
	if authMethod == "" {
		// Default to idc (AWS Builder ID / IAM Identity Center) when unspecified.
		authMethod = "idc"
	}
	if authMethod != "social" && strings.TrimSpace(refreshToken) != "" && (clientID == "" || clientSecret == "") {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_IMPORT_MISSING_CLIENT",
			"idc credentials require clientId and clientSecret for token refresh")
	}

	creds := map[string]any{}
	setIf := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			creds[k] = v
		}
	}
	setIf("access_token", accessToken)
	setIf("refresh_token", refreshToken)
	setIf("client_id", clientID)
	setIf("client_secret", clientSecret)
	setIf("auth_method", authMethod)
	setIf("region", region)
	setIf("profile_arn", profileArn)
	setIf("machine_id", machineID)
	setIf("provider", pick(p.Provider, ""))
	setIf("email", pick(p.Email, ""))
	if expiresAt > 0 {
		creds["expires_at"] = time.Unix(expiresAt, 0).UTC().Format(time.RFC3339)
	}
	return creds, nil
}

// Ensure interface compliance at compile time.
var _ KiroOAuthTokenService = (*KiroOAuthService)(nil)
