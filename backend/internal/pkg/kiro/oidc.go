package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TokenRefreshResult holds the outcome of a token refresh.
type TokenRefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64 // Unix seconds
	ProfileArn   string
}

// oidcTokenURL builds the idc/BuilderID refresh endpoint.
func oidcTokenURL(region string) string {
	return fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)
}

// socialTokenURL is the Kiro desktop (GitHub/Google social login) refresh endpoint.
const socialTokenURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

// RefreshToken refreshes an access token for the given credential, dispatching
// by AuthMethod: "social" uses the Kiro desktop endpoint; everything else
// (idc / AWS Builder ID / IAM Identity Center) uses the AWS OIDC endpoint.
func RefreshToken(ctx context.Context, cred *Credential) (*TokenRefreshResult, error) {
	client := GetHTTPClientForProxy(cred.ProxyURL, 30*time.Second)
	if cred.AuthMethod == "social" {
		return refreshSocialToken(ctx, cred.RefreshToken, client)
	}
	return refreshOIDCToken(ctx, cred.RefreshToken, cred.ClientID, cred.ClientSecret, cred.Region, client)
}

type refreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	ProfileArn   string `json:"profileArn"`
}

func refreshOIDCToken(ctx context.Context, refreshToken, clientID, clientSecret, region string, client *http.Client) (*TokenRefreshResult, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("kiro: OIDC refresh requires clientId and clientSecret")
	}
	if region == "" {
		region = "us-east-1"
	}
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	}
	return doRefresh(ctx, oidcTokenURL(region), payload, client)
}

func refreshSocialToken(ctx context.Context, refreshToken string, client *http.Client) (*TokenRefreshResult, error) {
	payload := map[string]string{"refreshToken": refreshToken}
	return doRefresh(ctx, socialTokenURL, payload, client)
}

func doRefresh(ctx context.Context, url string, payload map[string]string, client *http.Client) (*TokenRefreshResult, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("kiro: token refresh failed: %d %s", resp.StatusCode, string(respBody))
	}

	var result refreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &TokenRefreshResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(result.ExpiresIn),
		ProfileArn:   result.ProfileArn,
	}, nil
}
