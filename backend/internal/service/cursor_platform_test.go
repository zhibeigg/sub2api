package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCursorPlatformCapabilities(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidatePlatformAccountType(PlatformCursor, AccountTypeCookie))
	require.Error(t, ValidatePlatformAccountType(PlatformCursor, AccountTypeOAuth))
	require.False(t, PlatformSupportsImageGeneration(PlatformCursor))
	require.False(t, PlatformSupportsVideoGeneration(PlatformCursor))
	require.False(t, PlatformSupportsBatchImageGeneration(PlatformCursor))
	require.False(t, PlatformSupportsUpstreamModelSync(PlatformCursor))
	require.Equal(t, 1, DefaultAccountConcurrency(PlatformCursor))
	require.False(t, IsMixedSchedulingCapablePlatform(PlatformCursor))
	require.Contains(t, AllowedQuotaPlatforms, PlatformCursor)
}

func TestCursorAccountDefaultMapping(t *testing.T) {
	t.Parallel()
	account := &Account{Platform: PlatformCursor, Type: AccountTypeCookie, Credentials: map[string]any{"cookie": "_vcrcs=secret"}}
	require.Equal(t, "google/gemini-3-flash", account.GetModelMapping()["cursor-chat"])
	require.Equal(t, "google/gemini-3-flash", account.GetModelMapping()["google/gemini-3-flash"])
}

func TestValidateCursorAccountCredentials(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateCursorAccountCredentials(AccountTypeCookie, map[string]any{"cookie": "foo=bar; _vcrcs=secret"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeOAuth, map[string]any{"cookie": "_vcrcs=secret"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeCookie, map[string]any{"cookie": "foo=bar"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeCookie, map[string]any{"cookie": "_vcrcs=secret", "access_token": "bad"}))
}

func TestNormalizeCursorCredentialExpiry(t *testing.T) {
	t.Parallel()
	credentials := map[string]any{"cookie": "_vcrcs=secret", "cookie_expires_at": time.Date(2030, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 8*3600)).Unix()}
	NormalizeCursorCredentialExpiry(credentials)
	require.Equal(t, "2030-01-01T19:04:05Z", credentials["cookie_expires_at"])
	require.NoError(t, ValidateCursorAccountCredentials(AccountTypeCookie, credentials))
}
