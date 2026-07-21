//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingServiceGetChannelMonitorRuntimeStrict(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorEnabled:                "false",
		SettingKeyChannelMonitorDefaultIntervalSeconds: "120",
	}}, &config.Config{})

	runtime, err := svc.GetChannelMonitorRuntimeStrict(context.Background())
	require.NoError(t, err)
	require.False(t, runtime.Enabled)
	require.Equal(t, 120, runtime.DefaultIntervalSeconds)
}

func TestSettingServiceGetChannelMonitorRuntimeStrictFailsClosed(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{getMultipleErr: errors.New("storage unavailable")}, &config.Config{})

	runtime, err := svc.GetChannelMonitorRuntimeStrict(context.Background())
	require.Error(t, err)
	require.False(t, runtime.Enabled)
}
