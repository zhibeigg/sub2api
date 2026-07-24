package qqbot

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdaptOneBotGroupAndPrivateMessages(t *testing.T) {
	groupRaw := []byte(`{"time":1720000000,"self_id":3944007489,"post_type":"message","message_type":"group","message_id":101,"user_id":20001,"group_id":30001,"message":[{"type":"at","data":{"qq":"3944007489"}},{"type":"text","data":{"text":" /help "}}],"sender":{"nickname":"Nick","card":"Group Card"}}`)
	group, accepted, err := AdaptOneBotEvent(groupRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, SceneGroup, group.Scene)
	require.Equal(t, "30001", group.SourceID)
	require.Equal(t, "20001", group.ProviderSubject)
	require.Equal(t, "101", group.MessageID)
	require.Equal(t, " /help ", group.Content)
	require.Equal(t, "Group Card", group.DisplayName)
	require.NotEmpty(t, group.EventID)

	bindRaw := []byte(`{"time":1720000000,"self_id":3944007489,"post_type":"message","message_type":"group","message_id":102,"user_id":3252237236,"group_id":1054130674,"message":[{"type":"at","data":{"qq":"3944007489"}},{"type":"text","data":{"text":" /bind 3252237236@qq.com"}}],"sender":{"nickname":"mtk- rocky"}}`)
	bindEvent, accepted, err := AdaptOneBotEvent(bindRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, SceneGroup, bindEvent.Scene)
	require.Equal(t, "1054130674", bindEvent.SourceID)
	require.Equal(t, " /bind 3252237236@qq.com", bindEvent.Content)
	require.Equal(t, Command{Kind: CommandBind, Email: "3252237236@qq.com"}, ParseCommand(bindEvent.Content))

	privateRaw := []byte(`{"time":1720000001,"self_id":"3944007489","post_type":"message","message_type":"private","sub_type":"friend","message_id":"102","user_id":"20002","raw_message":"[CQ:reply,id=1]/bind user@example.com","sender":{"nickname":"Private User"}}`)
	private, accepted, err := AdaptOneBotEvent(privateRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, SceneC2C, private.Scene)
	require.Equal(t, "20002", private.ProviderSubject)
	require.True(t, private.FriendConversation)
	require.Equal(t, "/bind user@example.com", private.Content)
	require.Equal(t, "Private User", private.DisplayName)
}

func TestAdaptOneBotFriendAddAndRejectsTemporaryPrivateMessages(t *testing.T) {
	friendAddRaw := []byte(`{"time":1720000002,"self_id":3944007489,"post_type":"notice","notice_type":"friend_add","user_id":20003}`)
	friendAdded, accepted, err := AdaptOneBotEvent(friendAddRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, SceneC2C, friendAdded.Scene)
	require.Equal(t, "20003", friendAdded.ProviderSubject)
	require.True(t, friendAdded.FriendConversation)
	require.True(t, friendAdded.FriendAdded)

	for _, raw := range [][]byte{
		[]byte(`{"self_id":3944007489,"post_type":"message","message_type":"private","sub_type":"group","message_id":103,"user_id":20004,"raw_message":"/help"}`),
		[]byte(`{"self_id":3944007489,"post_type":"message","message_type":"private","sub_type":"other","message_id":104,"user_id":20005,"raw_message":"/help"}`),
		[]byte(`{"self_id":3944007489,"post_type":"message","message_type":"private","message_id":105,"user_id":20006,"raw_message":"/help"}`),
	} {
		_, accepted, err := AdaptOneBotEvent(raw, "3944007489")
		require.NoError(t, err)
		require.False(t, accepted)
	}
}

func TestAdaptOneBotGroupIncrease(t *testing.T) {
	raw := []byte(`{"time":1720000010,"self_id":3944007489,"post_type":"notice","notice_type":"group_increase","sub_type":"invite","group_id":30001,"user_id":20003,"operator_id":20004}`)
	event, accepted, err := AdaptOneBotEvent(raw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.True(t, event.MemberJoined)
	require.Equal(t, SceneGroup, event.Scene)
	require.Equal(t, "30001", event.SourceID)
	require.Equal(t, "20003", event.ProviderSubject)

	again, accepted, err := AdaptOneBotEvent(raw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, event.EventID, again.EventID)
}

func TestAdaptOneBotFriendAndGroupJoinRequests(t *testing.T) {
	friendRaw := []byte(`{"time":1720000020,"self_id":3944007489,"post_type":"request","request_type":"friend","user_id":20005,"flag":"friend-opaque-flag"}`)
	friend, accepted, err := AdaptOneBotEvent(friendRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, SceneC2C, friend.Scene)
	require.Equal(t, "20005", friend.ProviderSubject)
	require.Equal(t, &OneBotRequestApproval{Kind: "friend", Flag: "friend-opaque-flag"}, friend.OneBotRequest)

	groupRaw := []byte(`{"time":1720000021,"self_id":3944007489,"post_type":"request","request_type":"group","sub_type":"add","user_id":20006,"group_id":30001,"flag":123456}`)
	group, accepted, err := AdaptOneBotEvent(groupRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, SceneGroup, group.Scene)
	require.Equal(t, "30001", group.SourceID)
	require.Equal(t, &OneBotRequestApproval{Kind: "group", Flag: "123456", SubType: "add"}, group.OneBotRequest)

	again, accepted, err := AdaptOneBotEvent(groupRaw, "3944007489")
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, group.EventID, again.EventID)
}

func TestAdaptOneBotIgnoresSelfAndUnknownEvents(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"self_id":3944007489,"post_type":"message","message_type":"group","message_id":1,"user_id":3944007489,"group_id":30001,"raw_message":"self"}`),
		[]byte(`{"self_id":3944007489,"post_type":"notice","notice_type":"group_increase","sub_type":"approve","group_id":30001,"user_id":3944007489}`),
		[]byte(`{"self_id":3944007489,"post_type":"meta_event","meta_event_type":"heartbeat"}`),
		[]byte(`{"self_id":111111,"post_type":"message","message_type":"private","message_id":1,"user_id":20001,"raw_message":"hello"}`),
		[]byte(`{"self_id":3944007489,"post_type":"request","request_type":"group","sub_type":"invite","user_id":20001,"group_id":30001,"flag":"invite"}`),
		[]byte(`{"self_id":3944007489,"post_type":"request","request_type":"friend","user_id":20001}`),
	}
	for _, raw := range cases {
		_, accepted, err := AdaptOneBotEvent(raw, "3944007489")
		require.NoError(t, err)
		require.False(t, accepted)
	}
}

func TestAdaptOneBotRejectsMalformedPayload(t *testing.T) {
	_, accepted, err := AdaptOneBotEvent([]byte(`{"self_id":`), "3944007489")
	require.ErrorIs(t, err, ErrInvalidOneBotEvent)
	require.False(t, accepted)
}
