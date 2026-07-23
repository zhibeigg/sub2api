package qqbot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type oneBotCallerStub struct {
	action string
	params any
	data   any
	err    error
}

func (s *oneBotCallerStub) Call(_ context.Context, action string, params any, result any) error {
	s.action = action
	s.params = params
	if s.err != nil {
		return s.err
	}
	if result != nil && s.data != nil {
		raw, _ := json.Marshal(s.data)
		return json.Unmarshal(raw, result)
	}
	return nil
}

func TestOneBotMessengerProbeAndTextMessages(t *testing.T) {
	caller := &oneBotCallerStub{data: map[string]any{"user_id": 3944007489, "nickname": "PokeAPI助手"}}
	messenger, err := NewOneBotMessenger(caller)
	require.NoError(t, err)

	botID, err := messenger.Probe(t.Context())
	require.NoError(t, err)
	require.Equal(t, "3944007489", botID)
	require.Equal(t, "get_login_info", caller.action)

	require.NoError(t, messenger.SendGroup(t.Context(), "30001", "", "", "hello", 1))
	require.Equal(t, "send_group_msg", caller.action)
	groupParams, ok := caller.params.(oneBotSendGroupParams)
	require.True(t, ok)
	require.Equal(t, "30001", groupParams.GroupID)
	require.Equal(t, []OneBotMessageSegment{{Type: "text", Data: map[string]string{"text": "hello"}}}, groupParams.Message)

	require.NoError(t, messenger.SendC2C(t.Context(), "20001", "", "", "private", 1))
	require.Equal(t, "send_private_msg", caller.action)
}

func TestOneBotMessengerWelcomeUsesAtAndPlainTextSegments(t *testing.T) {
	caller := &oneBotCallerStub{}
	messenger, err := NewOneBotMessenger(caller)
	require.NoError(t, err)
	require.NoError(t, messenger.SendGroupWelcome(t.Context(), "30001", "20001", "welcome [CQ:at,qq=all]"))

	params, ok := caller.params.(oneBotSendGroupParams)
	require.True(t, ok)
	require.Equal(t, []OneBotMessageSegment{
		{Type: "at", Data: map[string]string{"qq": "20001"}},
		{Type: "text", Data: map[string]string{"text": "welcome [CQ:at,qq=all]"}},
	}, params.Message)
}

func TestOneBotMessengerImagesAndUnsupportedChannels(t *testing.T) {
	caller := &oneBotCallerStub{}
	messenger, err := NewOneBotMessenger(caller)
	require.NoError(t, err)
	require.NoError(t, messenger.SendGroupImage(t.Context(), "30001", "", "", "https://example.com/status.png", 1))
	params := caller.params.(oneBotSendGroupParams)
	require.Equal(t, "image", params.Message[0].Type)
	require.Equal(t, "https://example.com/status.png", params.Message[0].Data["file"])
	require.Error(t, messenger.SendGroupImage(t.Context(), "30001", "", "", "file:///tmp/status.png", 1))
	require.ErrorIs(t, messenger.SendChannel(t.Context(), "channel", "", "", "text", 1), ErrOneBotChannelUnsupported)
}
