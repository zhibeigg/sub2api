package cursor

import (
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

	"github.com/google/uuid"
)

const (
	DefaultDashboardWebsiteURL = "https://cursor.com"
	dashboardLoginPath         = "/loginDeepControl"
	dashboardPollPath          = "/auth/poll"
)

type DashboardAuthClientConfig struct {
	BaseURL        string
	WebsiteURL     string
	RequestTimeout time.Duration
	MaxErrorBody   int
}

type DashboardAuthClient struct {
	httpClient     *http.Client
	baseURL        string
	websiteURL     string
	requestTimeout time.Duration
	maxErrorBody   int
}

type DashboardLoginPKCE struct {
	UUID      string
	Verifier  string
	Challenge string
}

type DashboardLoginPollResult struct {
	Pending        bool
	AccessToken    string
	RefreshToken   string
	AuthID         string
	SelectedTeamID *int64
}

type DashboardTokenMetadata struct {
	Subject   string
	Issuer    string
	ExpiresAt time.Time
}

func NewDashboardAuthClient(httpClient *http.Client, config DashboardAuthClientConfig) (*DashboardAuthClient, error) {
	if httpClient == nil {
		return nil, badRequest("create dashboard auth client", fmt.Errorf("http client is required"))
	}
	baseURL, err := validateDashboardAuthURL(config.BaseURL, DefaultDashboardBaseURL)
	if err != nil {
		return nil, badRequest("create dashboard auth client", err)
	}
	websiteURL, err := validateDashboardAuthURL(config.WebsiteURL, DefaultDashboardWebsiteURL)
	if err != nil {
		return nil, badRequest("create dashboard auth client", err)
	}
	maxErrorBody := config.MaxErrorBody
	if maxErrorBody <= 0 {
		maxErrorBody = defaultCloudMaxErrorBody
	}
	return &DashboardAuthClient{
		httpClient:     httpClient,
		baseURL:        baseURL,
		websiteURL:     websiteURL,
		requestTimeout: config.RequestTimeout,
		maxErrorBody:   maxErrorBody,
	}, nil
}

func validateDashboardAuthURL(raw, fallback string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		raw = fallback
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("invalid HTTP URL")
	}
	return raw, nil
}

func GenerateDashboardLoginPKCE() (*DashboardLoginPKCE, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(verifier))
	return &DashboardLoginPKCE{
		UUID:      uuid.NewString(),
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(digest[:]),
	}, nil
}

func (c *DashboardAuthClient) BuildLoginURL(flow *DashboardLoginPKCE, mode string) (string, error) {
	if flow == nil || strings.TrimSpace(flow.UUID) == "" || strings.TrimSpace(flow.Challenge) == "" {
		return "", badRequest("build dashboard login URL", fmt.Errorf("PKCE flow is required"))
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "login"
	}
	parsed, err := url.Parse(c.websiteURL + dashboardLoginPath)
	if err != nil {
		return "", badRequest("build dashboard login URL", err)
	}
	query := parsed.Query()
	query.Set("challenge", flow.Challenge)
	query.Set("uuid", flow.UUID)
	query.Set("mode", mode)
	query.Set("supportsSelectedTeamLogin", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c *DashboardAuthClient) PollLogin(ctx context.Context, flow *DashboardLoginPKCE) (*DashboardLoginPollResult, error) {
	if flow == nil || strings.TrimSpace(flow.UUID) == "" || strings.TrimSpace(flow.Verifier) == "" {
		return nil, badRequest("poll dashboard login", fmt.Errorf("PKCE flow is required"))
	}
	parsed, _ := url.Parse(c.baseURL + dashboardPollPath)
	query := parsed.Query()
	query.Set("uuid", flow.UUID)
	query.Set("verifier", flow.Verifier)
	parsed.RawQuery = query.Encode()

	reqCtx, cancel := c.withTimeout(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, badRequest("create dashboard login poll request", err)
	}
	req.Header.Set("x-ghost-mode", "true")
	req.Header.Set("x-new-onboarding-completed", "false")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, &Error{Kind: ErrorTransport, Operation: "get " + dashboardPollPath, Err: err}
		}
		return nil, transportError("get "+dashboardPollPath, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return &DashboardLoginPollResult{Pending: true}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, dashboardAuthHTTPError("get "+dashboardPollPath, resp, c.maxErrorBody)
	}
	var wire struct {
		AccessToken    string `json:"accessToken"`
		RefreshToken   string `json:"refreshToken"`
		AuthID         string `json:"authId"`
		SelectedTeamID *int64 `json:"selectedTeamId"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&wire); err != nil {
		return nil, protocolError("get "+dashboardPollPath, err)
	}
	wire.AccessToken = strings.TrimSpace(wire.AccessToken)
	wire.RefreshToken = strings.TrimSpace(wire.RefreshToken)
	if wire.AccessToken == "" || wire.RefreshToken == "" {
		return &DashboardLoginPollResult{Pending: true}, nil
	}
	return &DashboardLoginPollResult{
		AccessToken:    wire.AccessToken,
		RefreshToken:   wire.RefreshToken,
		AuthID:         strings.TrimSpace(wire.AuthID),
		SelectedTeamID: wire.SelectedTeamID,
	}, nil
}

func (c *DashboardAuthClient) RefreshAccessToken(ctx context.Context, refreshToken string) (*DashboardTokenRefresh, error) {
	client, err := NewDashboardClient(c.httpClient, "placeholder", DashboardClientConfig{
		BaseURL:        c.baseURL,
		RequestTimeout: c.requestTimeout,
		MaxErrorBody:   c.maxErrorBody,
	})
	if err != nil {
		return nil, err
	}
	return client.RefreshAccessToken(ctx, refreshToken)
}

func ParseDashboardTokenMetadata(token string) (*DashboardTokenMetadata, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims struct {
		Subject string `json:"sub"`
		Issuer  string `json:"iss"`
		Expiry  int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT payload: %w", err)
	}
	if claims.Expiry <= 0 {
		return nil, fmt.Errorf("JWT exp is missing")
	}
	return &DashboardTokenMetadata{
		Subject:   strings.TrimSpace(claims.Subject),
		Issuer:    strings.TrimSpace(claims.Issuer),
		ExpiresAt: time.Unix(claims.Expiry, 0).UTC(),
	}, nil
}

func (c *DashboardAuthClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

func dashboardAuthHTTPError(operation string, resp *http.Response, maxErrorBody int) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxErrorBody)+1))
	message := strings.TrimSpace(string(body))
	if len(body) > maxErrorBody {
		message = strings.TrimSpace(string(body[:maxErrorBody])) + "..."
	}
	return HTTPError(resp.StatusCode, operation, message)
}
