package service

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	opencodepkg "github.com/Wei-Shaw/sub2api/internal/pkg/opencode"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeQuotaServiceMissingCookieDoesNotCallUpstream(t *testing.T) {
	var calls atomic.Int32
	service := NewOpenCodeQuotaService(&openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		calls.Add(1)
		return nil, nil
	}}, nil, &config.Config{})

	info := service.GetQuota(t.Context(), &Account{ID: 1, Platform: PlatformOpenCode, Type: AccountTypeAPIKey}, false)
	require.False(t, info.Configured)
	require.Equal(t, "missing", info.State)
	require.Zero(t, calls.Load())
}

func TestOpenCodeQuotaServiceFetchesAndCachesConfiguredWorkspace(t *testing.T) {
	var calls atomic.Int32
	upstream := &openCodeHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, "/workspace/wrk_one/go", req.URL.Path)
		require.Equal(t, "auth=secret", req.Header.Get("Cookie"))
		return openCodeResponse(http.StatusOK, `{"rollingUsage":{"usagePercent":60,"resetInSec":3600},"weeklyUsage":{"usagePercent":40,"resetInSec":86400}}`, nil), nil
	}}
	cfg := &config.Config{OpenCode: config.OpenCodeConfig{QuotaCacheTTLSeconds: 300, QuotaStaleTTLSeconds: 1800, QuotaRequestTimeoutSeconds: 15}}
	service := NewOpenCodeQuotaService(upstream, nil, cfg)
	account := openCodeAccount(map[string]any{"quota_cookie": "secret", "quota_workspace_id": "wrk_one"})

	first := service.GetQuota(t.Context(), account, false)
	require.Equal(t, "verified", first.State)
	require.Equal(t, 60, first.Rolling.UsagePercent)
	require.Equal(t, "wrk_one", first.WorkspaceID)

	second := service.GetQuota(t.Context(), account, false)
	require.Equal(t, "cached", second.State)
	require.Equal(t, int32(1), calls.Load())
}

func TestOpenCodeQuotaServiceFallsBackToResolvedWorkspace(t *testing.T) {
	var calls atomic.Int32
	upstream := &openCodeHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		switch req.URL.Path {
		case "/workspace/wrk_stale/go":
			return openCodeResponse(http.StatusNotFound, `{}`, nil), nil
		case "/_server":
			require.Equal(t, opencodepkg.WorkspaceServerFunctionID, req.Header.Get("X-Server-Id"))
			return openCodeResponse(http.StatusOK, `{ id: "wrk_resolved" }`, nil), nil
		case "/workspace/wrk_resolved/go":
			return openCodeResponse(http.StatusOK, `{"rollingUsage":{"usagePercent":20,"resetInSec":60},"weeklyUsage":{"usagePercent":10,"resetInSec":120}}`, nil), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	}}
	service := NewOpenCodeQuotaService(upstream, nil, &config.Config{})
	account := openCodeAccount(map[string]any{"quota_cookie": "auth=secret", "quota_workspace_id": "wrk_stale"})

	info := service.GetQuota(t.Context(), account, true)
	require.Equal(t, "verified", info.State)
	require.Equal(t, "wrk_resolved", info.WorkspaceID)
	require.Equal(t, int32(3), calls.Load())
}

func TestOpenCodeQuotaServiceReportsUnavailableWhenWorkspaceHasNoGoEntitlement(t *testing.T) {
	var calls atomic.Int32
	upstream := &openCodeHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		switch req.URL.Path {
		case "/_server":
			return openCodeResponse(http.StatusOK, `{ id: "wrk_empty" }`, nil), nil
		case "/workspace/wrk_empty/go":
			return openCodeResponse(http.StatusOK, `<script>{monthlyLimit:null,monthlyUsage:null,subscription:null,lite:null}</script>`, nil), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	}}
	service := NewOpenCodeQuotaService(upstream, nil, &config.Config{})
	account := openCodeAccount(map[string]any{"quota_cookie": "auth=secret"})

	info := service.GetQuota(t.Context(), account, true)

	require.True(t, info.Configured)
	require.Equal(t, "unavailable", info.State)
	require.Contains(t, info.Message, "entitlement may be inactive")
	require.Nil(t, info.Rolling)
	require.Nil(t, info.Weekly)
	require.Equal(t, int32(2), calls.Load())
}

func TestOpenCodeQuotaCooldownResetUsesLatestExhaustedWindow(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	rollingReset := now.Add(time.Hour)
	weeklyReset := now.Add(24 * time.Hour)
	monthlyReset := now.Add(7 * 24 * time.Hour)

	reset := openCodeQuotaCooldownReset(&OpenCodeQuotaInfo{
		Rolling: &opencodepkg.QuotaWindow{UsagePercent: 100, ResetAt: &rollingReset},
		Weekly:  &opencodepkg.QuotaWindow{UsagePercent: 100, ResetAt: &weeklyReset},
		Monthly: &opencodepkg.QuotaWindow{UsagePercent: 99, ResetAt: &monthlyReset},
	})
	require.NotNil(t, reset)
	require.Equal(t, weeklyReset, *reset)
}
