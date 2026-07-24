package qqbot

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newOneBotRequestApprovalRuntime(friendEnabled, groupEnabled bool, allowedGroups []string) *OneBotRuntime {
	oneBotManager := &OneBotConfigManager{}
	oneBotManager.snapshot.Store(&oneBotConfigSnapshot{storage: oneBotStorageConfig{
		AutoApproveFriendRequests: friendEnabled,
		AutoApproveGroupRequests:  groupEnabled,
	}})
	businessManager := &ConfigManager{}
	businessManager.snapshot.Store(&configSnapshot{settings: service.QQBotSettings{AllowedGroupIDs: allowedGroups}})
	return &OneBotRuntime{manager: oneBotManager, processor: &Runtime{manager: businessManager}}
}

func TestOneBotRequestApprovalRespectsPolicyAndGroupAllowlist(t *testing.T) {
	friendEvent := InboundEvent{
		EventID:         "onebot-friend-request",
		Scene:           SceneC2C,
		ProviderSubject: "20001",
		OneBotRequest:   &OneBotRequestApproval{Kind: "friend", Flag: "friend-flag"},
	}
	groupEvent := InboundEvent{
		EventID:         "onebot-group-request",
		Scene:           SceneGroup,
		ProviderSubject: "20002",
		SourceID:        "30001",
		OneBotRequest:   &OneBotRequestApproval{Kind: "group", Flag: "group-flag", SubType: "add"},
	}

	caller := &oneBotCallerStub{}
	messenger, err := NewOneBotMessenger(caller)
	require.NoError(t, err)

	attempted, err := newOneBotRequestApprovalRuntime(false, false, nil).processRequestApproval(t.Context(), messenger, friendEvent)
	require.NoError(t, err)
	require.False(t, attempted)
	require.Empty(t, caller.action)

	attempted, err = newOneBotRequestApprovalRuntime(true, false, nil).processRequestApproval(t.Context(), messenger, friendEvent)
	require.NoError(t, err)
	require.True(t, attempted)
	require.Equal(t, "set_friend_add_request", caller.action)

	caller.action = ""
	attempted, err = newOneBotRequestApprovalRuntime(false, true, []string{"30002"}).processRequestApproval(t.Context(), messenger, groupEvent)
	require.NoError(t, err)
	require.False(t, attempted)
	require.Empty(t, caller.action)

	attempted, err = newOneBotRequestApprovalRuntime(false, true, []string{"30001"}).processRequestApproval(t.Context(), messenger, groupEvent)
	require.NoError(t, err)
	require.True(t, attempted)
	require.Equal(t, "set_group_add_request", caller.action)
}
