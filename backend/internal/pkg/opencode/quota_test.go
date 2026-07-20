package opencode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSanitizeQuotaCookie(t *testing.T) {
	t.Parallel()
	require.Equal(t, "auth=abc", SanitizeQuotaCookie(" cookie: abc "))
	require.Equal(t, "auth=abc; workspace=one", SanitizeQuotaCookie("auth=abc; ; workspace=one"))
	require.Empty(t, SanitizeQuotaCookie("  "))
}

func TestParseWorkspaceIDs(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{"wrk_one", "wrk_two"}, ParseWorkspaceIDs(`{ id: "wrk_one" }, { id: "wrk_two" }, { id: "wrk_one" }`))
	require.Equal(t, []string{"wrk_json"}, ParseWorkspaceIDs(`["wrk_json", "other"]`))
}

func TestParseQuotaPageJSON(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	data, err := ParseQuotaPage(`{
		"rollingUsage":{"status":"active","usagePercent":50,"resetInSec":3600},
		"weeklyUsage":{"status":"active","usedPercent":30,"resetInSec":86400},
		"monthlyUsage":{"status":"unlimited","usagePercent":0,"resetInSec":0}
	}`, "wrk_one", now)
	require.NoError(t, err)
	require.Equal(t, 50, data.Rolling.UsagePercent)
	require.Equal(t, now.Add(time.Hour), *data.Rolling.ResetAt)
	require.Equal(t, 30, data.Weekly.UsagePercent)
	require.Nil(t, data.Monthly)
	require.Equal(t, "wrk_one", data.WorkspaceID)
}

func TestParseQuotaPageRegex(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	data, err := ParseQuotaPage(`<script>rollingUsage:{usagePercent:45,resetInSec:1800} weeklyUsage:{usagePercent:25,resetInSec:7200} monthlyUsage:{usagePercent:5,resetInSec:3600}</script>`, "wrk_two", now)
	require.NoError(t, err)
	require.Equal(t, 45, data.Rolling.UsagePercent)
	require.Equal(t, 25, data.Weekly.UsagePercent)
	require.NotNil(t, data.Monthly)
	require.Equal(t, 5, data.Monthly.UsagePercent)
}

func TestParseQuotaPageRejectsUnrelatedContent(t *testing.T) {
	t.Parallel()
	_, err := ParseQuotaPage("not quota data", "", time.Now())
	require.Error(t, err)
}

func TestLooksSignedOut(t *testing.T) {
	t.Parallel()
	require.True(t, LooksSignedOut("Please sign in to continue"))
	require.False(t, LooksSignedOut(`{"rollingUsage":{}}`))
}

func TestLooksQuotaUnavailable(t *testing.T) {
	t.Parallel()
	require.True(t, LooksQuotaUnavailable(`<script>{monthlyLimit:null, monthlyUsage:null, subscription:null, lite:null}</script>`))
	require.True(t, LooksQuotaUnavailable(`<script>{"monthlyLimit": null, "monthlyUsage": null, "subscription": null, "lite": null}</script>`))
	require.False(t, LooksQuotaUnavailable(`<script>{monthlyLimit:null,monthlyUsage:0,subscription:null,lite:{},rollingUsage:{usagePercent:4},weeklyUsage:{usagePercent:3}}</script>`))
	require.False(t, LooksQuotaUnavailable("unrelated page content"))
}
