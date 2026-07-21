package qqbot

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type eventSink interface{ enqueueEvent(InboundEvent) error }

var (
	registerHandlersOnce sync.Once
	activeEventSink      atomic.Value
)

type sinkHolder struct{ sink eventSink }

type webhookPayload struct {
	OPCode  int             `json:"op"`
	EventID string          `json:"id"`
	Type    string          `json:"t"`
	Data    json.RawMessage `json:"d"`
}

type eventData struct {
	ID           string    `json:"id"`
	Content      string    `json:"content"`
	GroupID      string    `json:"group_id"`
	GroupOpenID  string    `json:"group_openid"`
	GuildID      string    `json:"guild_id"`
	ChannelID    string    `json:"channel_id"`
	UserOpenID   string    `json:"user_openid"`
	MemberOpenID string    `json:"member_openid"`
	Nick         string    `json:"nick"`
	Author       eventUser `json:"author"`
	User         eventUser `json:"user"`
}
type eventUser struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	UserOpenID   string `json:"user_openid"`
	MemberOpenID string `json:"member_openid"`
	UnionOpenID  string `json:"union_openid"`
	Bot          bool   `json:"bot"`
}

func registerGlobalHandlers() {
	registerHandlersOnce.Do(func() { activeEventSink.Store(sinkHolder{}) })
}
func setActiveEventSink(sink eventSink) {
	registerGlobalHandlers()
	activeEventSink.Store(sinkHolder{sink: sink})
}
func clearActiveEventSink(sink eventSink) {
	current, _ := activeEventSink.Load().(sinkHolder)
	if current.sink == sink {
		activeEventSink.Store(sinkHolder{})
	}
}
func dispatchEvent(value InboundEvent) error {
	holder, _ := activeEventSink.Load().(sinkHolder)
	if holder.sink == nil {
		return errors.New("qqbot runtime is not accepting events")
	}
	return holder.sink.enqueueEvent(value)
}

func dispatchWebhookPayload(payload webhookPayload) error {
	var data eventData
	if len(payload.Data) > 0 && string(payload.Data) != "null" {
		if err := json.Unmarshal(payload.Data, &data); err != nil {
			return fmt.Errorf("decode qqbot event data: %w", err)
		}
	}
	eventID := strings.TrimSpace(payload.EventID)
	if eventID == "" {
		eventID = Fingerprint(payload.Type + ":" + data.ID + ":" + data.Author.ID + ":" + data.UserOpenID)
	}
	switch strings.ToUpper(strings.TrimSpace(payload.Type)) {
	case "GROUP_AT_MESSAGE_CREATE":
		return dispatchEvent(InboundEvent{EventID: eventID, MessageID: data.ID, Scene: SceneGroup, Content: data.Content, ProviderSubject: firstNonEmpty(data.Author.MemberOpenID, data.Author.UserOpenID, data.Author.ID, data.MemberOpenID, data.UserOpenID), SourceID: firstNonEmpty(data.GroupOpenID, data.GroupID), DisplayName: data.Author.Username})
	case "C2C_MESSAGE_CREATE":
		subject := firstNonEmpty(data.Author.UserOpenID, data.UserOpenID)
		if subject == "" {
			return nil
		}
		return dispatchEvent(InboundEvent{EventID: eventID, MessageID: data.ID, Scene: SceneC2C, Content: data.Content, ProviderSubject: subject, DisplayName: data.Author.Username})
	case "AT_MESSAGE_CREATE":
		return dispatchEvent(InboundEvent{EventID: eventID, MessageID: data.ID, Scene: SceneGuild, Content: data.Content, ProviderSubject: firstNonEmpty(data.Author.UnionOpenID, data.Author.ID), SourceID: data.GuildID, GuildID: data.GuildID, ChannelID: data.ChannelID, DisplayName: data.Author.Username})
	case "GUILD_MEMBER_ADD":
		if data.User.Bot {
			return nil
		}
		return dispatchEvent(InboundEvent{EventID: eventID, Scene: SceneGuild, ProviderSubject: data.User.ID, SourceID: data.GuildID, GuildID: data.GuildID, DisplayName: firstNonEmpty(data.Nick, data.User.Username), MemberJoined: true})
	case "ENTER_AIO":
		subject := firstNonEmpty(data.UserOpenID, data.Author.UserOpenID)
		if subject == "" {
			return nil
		}
		return dispatchEvent(InboundEvent{EventID: eventID, Scene: SceneC2C, ProviderSubject: subject, EnterAIO: true})
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
