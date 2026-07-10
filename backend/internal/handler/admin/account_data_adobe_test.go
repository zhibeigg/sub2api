package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestExportAccountCredentialsAlwaysRedactsAdobeSecrets(t *testing.T) {
	t.Parallel()
	account := service.Account{
		Platform: service.PlatformAdobe,
		Credentials: map[string]any{
			"access_token": "access-secret",
			"cookie":       "cookie-secret",
			"password":     "password-secret",
			"device_token": "device-secret",
			"device_id":    "device-id-secret",
			"expires_at":   "2030-01-01T00:00:00Z",
		},
	}

	credentials, status := exportAccountCredentials(account)
	for _, key := range []string{"access_token", "cookie", "password", "device_token", "device_id"} {
		require.NotContains(t, credentials, key)
		require.True(t, status["has_"+key])
	}
	require.Equal(t, "2030-01-01T00:00:00Z", credentials["expires_at"])
}
