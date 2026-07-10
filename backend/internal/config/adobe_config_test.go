package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultAdobeConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 120, cfg.Adobe.RequestTimeoutSeconds)
	require.Equal(t, 2, cfg.Adobe.ImagePollIntervalSeconds)
	require.Equal(t, 150, cfg.Adobe.ImageMaxPollAttempts)
	require.Equal(t, 259200, cfg.Adobe.VideoTaskTTLSeconds)
	require.Equal(t, 86400, cfg.Adobe.VideoTerminalTTLSeconds)
	require.Equal(t, 300, cfg.Adobe.TokenRefreshSkewSeconds)
	require.Equal(t, 300, cfg.Adobe.CreditsCacheTTLSeconds)

	for _, host := range []string{
		"adobeid-na1.services.adobe.com",
		"ims-na1.adobelogin.com",
		"firefly.adobe.io",
		"firefly-3p.ff.adobe.io",
		"*.ff.adobe.io",
	} {
		require.Contains(t, cfg.Security.URLAllowlist.UpstreamHosts, host)
	}
}

func TestLoadAdobeConfigFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("ADOBE_REQUEST_TIMEOUT_SECONDS", "45")
	t.Setenv("ADOBE_TOKEN_REFRESH_SKEW_SECONDS", "90")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 45, cfg.Adobe.RequestTimeoutSeconds)
	require.Equal(t, 90, cfg.Adobe.TokenRefreshSkewSeconds)
}
