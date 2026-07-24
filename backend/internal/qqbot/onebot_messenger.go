package qqbot

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

type OneBotActionCaller interface {
	Call(ctx context.Context, action string, params any, result any) error
}

// GroupWelcomeMessenger is an optional capability. A handler can type-assert it
// without expanding the core Messenger interface used by existing transports.
type GroupWelcomeMessenger interface {
	SendGroupWelcome(ctx context.Context, groupID, userID, content string) error
}

// OneBotRequestApprover is an optional OneBot-only capability used for incoming
// friend and group join requests. It is intentionally separate from Messenger,
// because Tencent BotGo does not expose equivalent request actions.
type OneBotRequestApprover interface {
	ApproveFriendRequest(ctx context.Context, flag string) error
	ApproveGroupRequest(ctx context.Context, flag, subType string) error
}

type oneBotUnsupportedError struct{}

func (oneBotUnsupportedError) Error() string    { return "onebot v11 does not support channel messages" }
func (oneBotUnsupportedError) Definitive() bool { return true }

var ErrOneBotChannelUnsupported error = oneBotUnsupportedError{}

type OneBotMessenger struct {
	caller OneBotActionCaller
}

type OneBotMessageSegment struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

type oneBotLoginInfo struct {
	UserID   oneBotID `json:"user_id"`
	Nickname string   `json:"nickname"`
}

type oneBotSendGroupParams struct {
	GroupID string                 `json:"group_id"`
	Message []OneBotMessageSegment `json:"message"`
}

type oneBotSendPrivateParams struct {
	UserID  string                 `json:"user_id"`
	Message []OneBotMessageSegment `json:"message"`
}

type oneBotFriendRequestParams struct {
	Flag    string `json:"flag"`
	Approve bool   `json:"approve"`
	Remark  string `json:"remark"`
}

type oneBotGroupRequestParams struct {
	Flag    string `json:"flag"`
	SubType string `json:"sub_type"`
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

func NewOneBotMessenger(caller OneBotActionCaller) (*OneBotMessenger, error) {
	if caller == nil {
		return nil, errors.New("onebot action caller is required")
	}
	return &OneBotMessenger{caller: caller}, nil
}

func (m *OneBotMessenger) Probe(ctx context.Context) (string, error) {
	var result oneBotLoginInfo
	if err := m.call(ctx, "get_login_info", struct{}{}, &result); err != nil {
		return "", err
	}
	userID := result.UserID.String()
	if !validOneBotID(userID) {
		return "", errors.New("onebot login info returned invalid user ID")
	}
	return userID, nil
}

func (m *OneBotMessenger) SendGroup(ctx context.Context, groupID, _, _, content string, _ uint32) error {
	return m.sendGroupSegments(ctx, groupID, []OneBotMessageSegment{oneBotTextSegment(content)})
}

func (m *OneBotMessenger) SendC2C(ctx context.Context, userID, _, _, content string, _ uint32) error {
	return m.sendPrivateSegments(ctx, userID, []OneBotMessageSegment{oneBotTextSegment(content)})
}

func (m *OneBotMessenger) SendChannel(context.Context, string, string, string, string, uint32) error {
	return ErrOneBotChannelUnsupported
}

func (m *OneBotMessenger) ApproveFriendRequest(ctx context.Context, flag string) error {
	if !validOneBotRequestFlag(flag) {
		return errors.New("onebot friend request flag is invalid")
	}
	return m.call(ctx, "set_friend_add_request", oneBotFriendRequestParams{Flag: strings.TrimSpace(flag), Approve: true}, nil)
}

func (m *OneBotMessenger) ApproveGroupRequest(ctx context.Context, flag, subType string) error {
	if !validOneBotRequestFlag(flag) || strings.ToLower(strings.TrimSpace(subType)) != "add" {
		return errors.New("onebot group request is invalid")
	}
	return m.call(ctx, "set_group_add_request", oneBotGroupRequestParams{Flag: strings.TrimSpace(flag), SubType: "add", Approve: true}, nil)
}

func (m *OneBotMessenger) SendGroupImage(ctx context.Context, groupID, _, _, imageURL string, _ uint32) error {
	if err := validateOneBotImageURL(imageURL); err != nil {
		return err
	}
	return m.sendGroupSegments(ctx, groupID, []OneBotMessageSegment{oneBotImageSegment(imageURL)})
}

func (m *OneBotMessenger) SendC2CImage(ctx context.Context, userID, _, _, imageURL string, _ uint32) error {
	if err := validateOneBotImageURL(imageURL); err != nil {
		return err
	}
	return m.sendPrivateSegments(ctx, userID, []OneBotMessageSegment{oneBotImageSegment(imageURL)})
}

func (m *OneBotMessenger) SendChannelImage(context.Context, string, string, string, string, uint32) error {
	return ErrOneBotChannelUnsupported
}

func (m *OneBotMessenger) SendGroupWelcome(ctx context.Context, groupID, userID, content string) error {
	if !validOneBotID(userID) {
		return errors.New("onebot user ID is invalid")
	}
	segments := []OneBotMessageSegment{
		{Type: "at", Data: map[string]string{"qq": userID}},
		oneBotTextSegment(content),
	}
	return m.sendGroupSegments(ctx, groupID, segments)
}

func (m *OneBotMessenger) sendGroupSegments(ctx context.Context, groupID string, segments []OneBotMessageSegment) error {
	groupID = strings.TrimSpace(groupID)
	if !validOneBotID(groupID) {
		return errors.New("onebot group ID is invalid")
	}
	return m.call(ctx, "send_group_msg", oneBotSendGroupParams{GroupID: groupID, Message: segments}, nil)
}

func (m *OneBotMessenger) sendPrivateSegments(ctx context.Context, userID string, segments []OneBotMessageSegment) error {
	userID = strings.TrimSpace(userID)
	if !validOneBotID(userID) {
		return errors.New("onebot user ID is invalid")
	}
	return m.call(ctx, "send_private_msg", oneBotSendPrivateParams{UserID: userID, Message: segments}, nil)
}

func (m *OneBotMessenger) call(ctx context.Context, action string, params any, result any) error {
	if m == nil || m.caller == nil {
		return ErrOneBotDisconnected
	}
	return m.caller.Call(ctx, action, params, result)
}

func oneBotTextSegment(content string) OneBotMessageSegment {
	return OneBotMessageSegment{Type: "text", Data: map[string]string{"text": content}}
}

func oneBotImageSegment(imageURL string) OneBotMessageSegment {
	return OneBotMessageSegment{Type: "image", Data: map[string]string{"file": strings.TrimSpace(imageURL)}}
}

func validateOneBotImageURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return errors.New("onebot image URL is invalid")
	}
	return nil
}

var _ Messenger = (*OneBotMessenger)(nil)
var _ GroupWelcomeMessenger = (*OneBotMessenger)(nil)
var _ OneBotRequestApprover = (*OneBotMessenger)(nil)
