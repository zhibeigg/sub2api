package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	opencodepkg "github.com/Wei-Shaw/sub2api/internal/pkg/opencode"
	"golang.org/x/sync/singleflight"
)

const (
	openCodeQuotaSiteBaseURL = "https://opencode.ai"
	openCodeQuotaServerURL   = "https://opencode.ai/_server"
	openCodeQuotaBodyLimit   = int64(1 << 20)
)

var errOpenCodeQuotaUnavailable = errors.New("OpenCode Go did not return quota windows for this workspace; the Go entitlement may be inactive")

type OpenCodeQuotaInfo struct {
	Configured  bool                     `json:"configured"`
	State       string                   `json:"state"` // missing/unavailable/cached/verified/stale/error
	Message     string                   `json:"message,omitempty"`
	WorkspaceID string                   `json:"workspace_id,omitempty"`
	FetchedAt   *time.Time               `json:"fetched_at,omitempty"`
	Rolling     *opencodepkg.QuotaWindow `json:"rolling,omitempty"`
	Weekly      *opencodepkg.QuotaWindow `json:"weekly,omitempty"`
	Monthly     *opencodepkg.QuotaWindow `json:"monthly,omitempty"`
}

type openCodeQuotaCacheEntry struct {
	info      *OpenCodeQuotaInfo
	fetchedAt time.Time
}

type OpenCodeQuotaService struct {
	httpUpstream HTTPUpstream
	proxyRepo    ProxyRepository
	cfg          *config.Config

	siteBaseURL       string
	serverURL         string
	workspaceServerID string
	now               func() time.Time

	cache      sync.Map
	refreshing sync.Map
	flight     singleflight.Group
}

func NewOpenCodeQuotaService(httpUpstream HTTPUpstream, proxyRepo ProxyRepository, cfg *config.Config) *OpenCodeQuotaService {
	return &OpenCodeQuotaService{
		httpUpstream:      httpUpstream,
		proxyRepo:         proxyRepo,
		cfg:               cfg,
		siteBaseURL:       openCodeQuotaSiteBaseURL,
		serverURL:         openCodeQuotaServerURL,
		workspaceServerID: opencodepkg.WorkspaceServerFunctionID,
		now:               time.Now,
	}
}

func (s *OpenCodeQuotaService) GetQuota(ctx context.Context, account *Account, force bool) *OpenCodeQuotaInfo {
	cookie := ""
	if account != nil {
		cookie = opencodepkg.SanitizeQuotaCookie(account.GetOpenCodeQuotaCookie())
	}
	if cookie == "" {
		return &OpenCodeQuotaInfo{Configured: false, State: "missing", Message: "OpenCode Go quota cookie is not configured"}
	}
	if account == nil {
		return &OpenCodeQuotaInfo{Configured: true, State: "error", Message: "OpenCode Go account is required"}
	}

	now := s.currentTime()
	if !force {
		if cached := s.cachedQuota(account.ID); cached != nil {
			age := now.Sub(cached.fetchedAt)
			if age <= s.freshTTL() {
				return cloneOpenCodeQuotaInfo(cached.info, "cached", "")
			}
			if age <= s.staleTTL() {
				s.refreshAsync(account)
				return cloneOpenCodeQuotaInfo(cached.info, "stale", "Refreshing OpenCode Go quota in the background")
			}
		}
	}

	info, err := s.refresh(ctx, account)
	if err == nil {
		return info
	}
	if errors.Is(err, errOpenCodeQuotaUnavailable) {
		s.cache.Delete(account.ID)
		return &OpenCodeQuotaInfo{Configured: true, State: "unavailable", Message: err.Error()}
	}
	if cached := s.cachedQuota(account.ID); cached != nil && now.Sub(cached.fetchedAt) <= s.staleTTL() {
		return cloneOpenCodeQuotaInfo(cached.info, "stale", err.Error())
	}
	return &OpenCodeQuotaInfo{Configured: true, State: "error", Message: err.Error()}
}

func (s *OpenCodeQuotaService) refreshAsync(account *Account) {
	if account == nil {
		return
	}
	if _, loaded := s.refreshing.LoadOrStore(account.ID, struct{}{}); loaded {
		return
	}
	go func() {
		defer s.refreshing.Delete(account.ID)
		ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout())
		defer cancel()
		_, _ = s.refresh(ctx, account)
	}()
}

func (s *OpenCodeQuotaService) refresh(ctx context.Context, account *Account) (*OpenCodeQuotaInfo, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, fmt.Errorf("OpenCode Go quota HTTP client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, s.requestTimeout())
	defer cancel()
	key := fmt.Sprintf("opencode-quota:%d", account.ID)
	value, err, _ := s.flight.Do(key, func() (any, error) {
		data, fetchErr := s.fetchQuota(ctx, account)
		if fetchErr != nil {
			return nil, fetchErr
		}
		fetchedAt := data.FetchedAt
		rolling := data.Rolling
		weekly := data.Weekly
		info := &OpenCodeQuotaInfo{
			Configured:  true,
			State:       "verified",
			WorkspaceID: data.WorkspaceID,
			FetchedAt:   &fetchedAt,
			Rolling:     &rolling,
			Weekly:      &weekly,
		}
		if data.Monthly != nil {
			monthly := *data.Monthly
			info.Monthly = &monthly
		}
		s.cache.Store(account.ID, &openCodeQuotaCacheEntry{info: info, fetchedAt: fetchedAt})
		return info, nil
	})
	if err != nil {
		return nil, err
	}
	return cloneOpenCodeQuotaInfo(value.(*OpenCodeQuotaInfo), "verified", ""), nil
}

func (s *OpenCodeQuotaService) fetchQuota(ctx context.Context, account *Account) (*opencodepkg.QuotaData, error) {
	cookie := opencodepkg.SanitizeQuotaCookie(account.GetOpenCodeQuotaCookie())
	if cookie == "" {
		return nil, fmt.Errorf("OpenCode Go quota cookie is not configured")
	}
	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, err
	}

	configuredWorkspace := strings.TrimSpace(account.GetOpenCodeQuotaWorkspaceID())
	var pageErr error
	if configuredWorkspace != "" {
		var data *opencodepkg.QuotaData
		data, pageErr = s.fetchQuotaPage(ctx, account, proxyURL, cookie, configuredWorkspace)
		if pageErr == nil {
			return data, nil
		}
	}
	resolvedWorkspace, resolveErr := s.resolveWorkspaceID(ctx, account, proxyURL, cookie)
	if resolveErr == nil && resolvedWorkspace != "" && resolvedWorkspace != configuredWorkspace {
		var data *opencodepkg.QuotaData
		data, pageErr = s.fetchQuotaPage(ctx, account, proxyURL, cookie, resolvedWorkspace)
		if pageErr == nil {
			return data, nil
		}
	}
	if errors.Is(pageErr, errOpenCodeQuotaUnavailable) {
		return nil, pageErr
	}
	if resolveErr != nil {
		return nil, fmt.Errorf("failed to resolve OpenCode Go workspace: %w", resolveErr)
	}
	return nil, fmt.Errorf("failed to fetch OpenCode Go quota; cookie may be expired or the page format changed")
}

func (s *OpenCodeQuotaService) resolveWorkspaceID(ctx context.Context, account *Account, proxyURL, cookie string) (string, error) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		body, status, err := s.callWorkspaceServer(ctx, account, proxyURL, cookie, method)
		if err != nil {
			return "", err
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden || opencodepkg.LooksSignedOut(string(body)) {
			return "", fmt.Errorf("OpenCode Go quota cookie is unauthorized or expired")
		}
		if ids := opencodepkg.ParseWorkspaceIDs(string(body)); len(ids) > 0 {
			return ids[0], nil
		}
	}
	return "", fmt.Errorf("no OpenCode Go workspace was found")
}

func (s *OpenCodeQuotaService) callWorkspaceServer(ctx context.Context, account *Account, proxyURL, cookie, method string) ([]byte, int, error) {
	endpoint := s.serverURL + "?id=" + url.QueryEscape(s.workspaceServerID)
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewReader([]byte("[]"))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, 0, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("X-Server-Id", s.workspaceServerID)
	req.Header.Set("X-Server-Instance", "server-fn:"+openCodeRandomHex(16))
	req.Header.Set("User-Agent", opencodepkg.QuotaUserAgent)
	req.Header.Set("Origin", s.siteBaseURL)
	req.Header.Set("Referer", s.siteBaseURL)
	req.Header.Set("Accept", "text/javascript, application/json;q=0.9, */*;q=0.8")
	response, err := s.httpUpstream.Do(req, proxyURL, account.ID, opencodeAccountConcurrency(account))
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, openCodeQuotaBodyLimit+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if int64(len(responseBody)) > openCodeQuotaBodyLimit {
		return nil, response.StatusCode, fmt.Errorf("OpenCode Go workspace response is too large")
	}
	return responseBody, response.StatusCode, nil
}

func (s *OpenCodeQuotaService) fetchQuotaPage(ctx context.Context, account *Account, proxyURL, cookie, workspaceID string) (*opencodepkg.QuotaData, error) {
	pageURL := strings.TrimRight(s.siteBaseURL, "/") + "/workspace/" + url.PathEscape(workspaceID) + "/go"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", opencodepkg.QuotaUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	response, err := s.httpUpstream.Do(req, proxyURL, account.ID, opencodeAccountConcurrency(account))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, openCodeQuotaBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(responseBody)) > openCodeQuotaBodyLimit {
		return nil, fmt.Errorf("OpenCode Go quota page is too large")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("OpenCode Go quota cookie is unauthorized or expired")
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("OpenCode Go quota page is rate limited")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenCode Go quota page returned HTTP %d", response.StatusCode)
	}
	pageText := string(responseBody)
	if opencodepkg.LooksSignedOut(pageText) {
		return nil, fmt.Errorf("OpenCode Go quota cookie is unauthorized or expired")
	}
	data, parseErr := opencodepkg.ParseQuotaPage(pageText, workspaceID, s.currentTime())
	if parseErr != nil && opencodepkg.LooksQuotaUnavailable(pageText) {
		return nil, errOpenCodeQuotaUnavailable
	}
	return data, parseErr
}

func (s *OpenCodeQuotaService) resolveProxyURL(ctx context.Context, account *Account) (string, error) {
	if account == nil || account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy != nil && account.Proxy.ID == *account.ProxyID {
		return account.Proxy.URL(), nil
	}
	if s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil {
		return "", fmt.Errorf("resolve OpenCode Go quota proxy: %w", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func (s *OpenCodeQuotaService) cachedQuota(accountID int64) *openCodeQuotaCacheEntry {
	value, ok := s.cache.Load(accountID)
	if !ok {
		return nil
	}
	entry, _ := value.(*openCodeQuotaCacheEntry)
	return entry
}

func (s *OpenCodeQuotaService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *OpenCodeQuotaService) freshTTL() time.Duration {
	seconds := 300
	if s != nil && s.cfg != nil && s.cfg.OpenCode.QuotaCacheTTLSeconds > 0 {
		seconds = s.cfg.OpenCode.QuotaCacheTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *OpenCodeQuotaService) staleTTL() time.Duration {
	seconds := 1800
	if s != nil && s.cfg != nil && s.cfg.OpenCode.QuotaStaleTTLSeconds > 0 {
		seconds = s.cfg.OpenCode.QuotaStaleTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *OpenCodeQuotaService) requestTimeout() time.Duration {
	seconds := 15
	if s != nil && s.cfg != nil && s.cfg.OpenCode.QuotaRequestTimeoutSeconds > 0 {
		seconds = s.cfg.OpenCode.QuotaRequestTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func cloneOpenCodeQuotaInfo(source *OpenCodeQuotaInfo, state, message string) *OpenCodeQuotaInfo {
	if source == nil {
		return nil
	}
	clone := *source
	clone.State = state
	clone.Message = message
	if source.FetchedAt != nil {
		value := *source.FetchedAt
		clone.FetchedAt = &value
	}
	if source.Rolling != nil {
		value := *source.Rolling
		clone.Rolling = &value
	}
	if source.Weekly != nil {
		value := *source.Weekly
		clone.Weekly = &value
	}
	if source.Monthly != nil {
		value := *source.Monthly
		clone.Monthly = &value
	}
	return &clone
}

func openCodeRandomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		fallback, _ := json.Marshal(time.Now().UnixNano())
		return hex.EncodeToString(fallback)
	}
	return hex.EncodeToString(buffer)
}
