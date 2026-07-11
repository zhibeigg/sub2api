package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultDashboardBaseURL = "https://api2.cursor.sh"
	dashboardUsagePath      = "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	dashboardRefreshPath    = "/oauth/token"
	dashboardOAuthClientID  = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB"
)

type DashboardClientConfig struct {
	BaseURL        string
	RequestTimeout time.Duration
	MaxErrorBody   int
}

type DashboardClient struct {
	httpClient     *http.Client
	baseURL        string
	accessToken    string
	requestTimeout time.Duration
	maxErrorBody   int
}

type DashboardUsage struct {
	Enabled           *bool                     `json:"enabled,omitempty"`
	BillingCycleStart int64                     `json:"billingCycleStart,omitempty"`
	BillingCycleEnd   int64                     `json:"billingCycleEnd,omitempty"`
	PlanUsage         *DashboardPlanUsage       `json:"planUsage,omitempty"`
	SpendLimitUsage   *DashboardSpendLimitUsage `json:"spendLimitUsage,omitempty"`
}

type DashboardPlanUsage struct {
	Limit            *float64 `json:"limit,omitempty"`
	TotalSpend       *float64 `json:"totalSpend,omitempty"`
	Remaining        *float64 `json:"remaining,omitempty"`
	TotalPercentUsed *float64 `json:"totalPercentUsed,omitempty"`
	AutoPercentUsed  *float64 `json:"autoPercentUsed,omitempty"`
	APIPercentUsed   *float64 `json:"apiPercentUsed,omitempty"`
}

type DashboardSpendLimitUsage struct {
	LimitType           string   `json:"limitType,omitempty"`
	IndividualLimit     *float64 `json:"individualLimit,omitempty"`
	IndividualRemaining *float64 `json:"individualRemaining,omitempty"`
	IndividualUsed      *float64 `json:"individualUsed,omitempty"`
	PooledLimit         *float64 `json:"pooledLimit,omitempty"`
	PooledRemaining     *float64 `json:"pooledRemaining,omitempty"`
	PooledUsed          *float64 `json:"pooledUsed,omitempty"`
	TotalSpend          *float64 `json:"totalSpend,omitempty"`
}

type DashboardTokenRefresh struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

func NewDashboardClient(httpClient *http.Client, accessToken string, config DashboardClientConfig) (*DashboardClient, error) {
	if httpClient == nil {
		return nil, badRequest("create dashboard client", fmt.Errorf("http client is required"))
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, badRequest("create dashboard client", fmt.Errorf("access token is required"))
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultDashboardBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, badRequest("create dashboard client", fmt.Errorf("invalid base URL"))
	}
	maxErrorBody := config.MaxErrorBody
	if maxErrorBody <= 0 {
		maxErrorBody = defaultCloudMaxErrorBody
	}
	return &DashboardClient{
		httpClient:     httpClient,
		baseURL:        baseURL,
		accessToken:    accessToken,
		requestTimeout: config.RequestTimeout,
		maxErrorBody:   maxErrorBody,
	}, nil
}

func (c *DashboardClient) FetchUsage(ctx context.Context) (*DashboardUsage, error) {
	var result DashboardUsage
	if err := c.doJSON(ctx, http.MethodPost, dashboardUsagePath, []byte("{}"), map[string]string{
		"Authorization":            "Bearer " + c.accessToken,
		"Content-Type":             "application/json",
		"Connect-Protocol-Version": "1",
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *DashboardClient) RefreshAccessToken(ctx context.Context, refreshToken string) (*DashboardTokenRefresh, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, badRequest("refresh dashboard token", fmt.Errorf("refresh token is required"))
	}
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     dashboardOAuthClientID,
		"refresh_token": refreshToken,
	})
	if err != nil {
		return nil, badRequest("refresh dashboard token", err)
	}
	var result DashboardTokenRefresh
	if err := c.doJSON(ctx, http.MethodPost, dashboardRefreshPath, body, map[string]string{
		"Content-Type": "application/json",
	}, &result); err != nil {
		return nil, err
	}
	result.AccessToken = strings.TrimSpace(result.AccessToken)
	result.RefreshToken = strings.TrimSpace(result.RefreshToken)
	if result.AccessToken == "" {
		return nil, protocolError("refresh dashboard token", fmt.Errorf("response did not include access_token"))
	}
	return &result, nil
}

func (c *DashboardClient) doJSON(ctx context.Context, method, path string, body []byte, headers map[string]string, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cancel := func() {}
	if c.requestTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.requestTimeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return badRequest("create dashboard request", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return &Error{Kind: ErrorTransport, Operation: strings.ToLower(method) + " " + path, Err: err}
		}
		return transportError(strings.ToLower(method)+" "+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.httpError(strings.ToLower(method)+" "+path, resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return protocolError(strings.ToLower(method)+" "+path, err)
	}
	return nil
}

func (c *DashboardClient) httpError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxErrorBody)+1))
	message := strings.TrimSpace(string(body))
	if len(body) > c.maxErrorBody {
		message = strings.TrimSpace(string(body[:c.maxErrorBody])) + "..."
	}
	return HTTPError(resp.StatusCode, operation, message)
}
