package qqbot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
)

var ErrInvalidOneBotEvent = errors.New("invalid onebot event")

type oneBotID string

func (id *oneBotID) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*id = ""
		return nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		*id = oneBotID(strings.TrimSpace(value))
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value json.Number
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*id = oneBotID(value.String())
	return nil
}

func (id oneBotID) String() string {
	return strings.TrimSpace(string(id))
}

type oneBotEventPayload struct {
	Time        int64             `json:"time"`
	SelfID      oneBotID          `json:"self_id"`
	PostType    string            `json:"post_type"`
	MessageType string            `json:"message_type"`
	NoticeType  string            `json:"notice_type"`
	RequestType string            `json:"request_type"`
	SubType     string            `json:"sub_type"`
	Flag        json.RawMessage   `json:"flag"`
	MessageID   oneBotID          `json:"message_id"`
	UserID      oneBotID          `json:"user_id"`
	GroupID     oneBotID          `json:"group_id"`
	OperatorID  oneBotID          `json:"operator_id"`
	Message     json.RawMessage   `json:"message"`
	RawMessage  string            `json:"raw_message"`
	Sender      oneBotEventSender `json:"sender"`
}

type oneBotEventSender struct {
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
}

type oneBotMessageSegment struct {
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
}

// AdaptOneBotEvent converts a OneBot v11 event payload into the transport-neutral
// InboundEvent used by the QQ bot runtime. The boolean is false for valid payloads
// that must be ignored, including unsupported event types and events emitted by the bot itself.
func AdaptOneBotEvent(raw []byte, expectedSelfID string) (InboundEvent, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return InboundEvent{}, false, ErrInvalidOneBotEvent
	}
	var payload oneBotEventPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&payload); err != nil {
		return InboundEvent{}, false, fmt.Errorf("%w: decode payload", ErrInvalidOneBotEvent)
	}

	selfID := payload.SelfID.String()
	expectedSelfID = strings.TrimSpace(expectedSelfID)
	if !validOneBotID(selfID) || (expectedSelfID != "" && selfID != expectedSelfID) {
		return InboundEvent{}, false, nil
	}
	userID := payload.UserID.String()
	if userID == selfID {
		return InboundEvent{}, false, nil
	}

	switch strings.ToLower(strings.TrimSpace(payload.PostType)) {
	case "message", "message_sent":
		return adaptOneBotMessage(payload, selfID)
	case "notice":
		return adaptOneBotNotice(payload, selfID)
	case "request":
		return adaptOneBotRequest(payload, selfID)
	default:
		return InboundEvent{}, false, nil
	}
}

func adaptOneBotMessage(payload oneBotEventPayload, selfID string) (InboundEvent, bool, error) {
	userID := payload.UserID.String()
	messageID := payload.MessageID.String()
	if !validOneBotID(userID) || !validOneBotID(messageID) || userID == selfID {
		return InboundEvent{}, false, nil
	}
	content, err := oneBotMessageText(payload.Message, payload.RawMessage)
	if err != nil {
		return InboundEvent{}, false, fmt.Errorf("%w: decode message", ErrInvalidOneBotEvent)
	}
	event := InboundEvent{
		MessageID:       messageID,
		Content:         content,
		ProviderSubject: userID,
		DisplayName:     firstNonEmpty(payload.Sender.Card, payload.Sender.Nickname),
	}

	switch strings.ToLower(strings.TrimSpace(payload.MessageType)) {
	case "group":
		groupID := payload.GroupID.String()
		if !validOneBotID(groupID) {
			return InboundEvent{}, false, nil
		}
		event.Scene = SceneGroup
		event.SourceID = groupID
		event.EventID = stableOneBotEventID("message", "group", selfID, groupID, userID, messageID)
	case "private":
		event.Scene = SceneC2C
		event.EventID = stableOneBotEventID("message", "private", selfID, userID, messageID)
	default:
		return InboundEvent{}, false, nil
	}
	return event, true, nil
}

func adaptOneBotNotice(payload oneBotEventPayload, selfID string) (InboundEvent, bool, error) {
	if strings.ToLower(strings.TrimSpace(payload.NoticeType)) != "group_increase" {
		return InboundEvent{}, false, nil
	}
	subType := strings.ToLower(strings.TrimSpace(payload.SubType))
	if subType != "approve" && subType != "invite" {
		return InboundEvent{}, false, nil
	}
	groupID := payload.GroupID.String()
	userID := payload.UserID.String()
	operatorID := payload.OperatorID.String()
	if !validOneBotID(groupID) || !validOneBotID(userID) || userID == selfID || (operatorID != "" && !validOneBotID(operatorID)) {
		return InboundEvent{}, false, nil
	}
	return InboundEvent{
		EventID:         stableOneBotEventID("notice", "group_increase", subType, selfID, groupID, userID, operatorID, fmt.Sprintf("%d", payload.Time)),
		Scene:           SceneGroup,
		ProviderSubject: userID,
		SourceID:        groupID,
		MemberJoined:    true,
	}, true, nil
}

func adaptOneBotRequest(payload oneBotEventPayload, selfID string) (InboundEvent, bool, error) {
	userID := payload.UserID.String()
	if !validOneBotID(userID) || userID == selfID {
		return InboundEvent{}, false, nil
	}
	flag, ok := oneBotRequestFlag(payload.Flag)
	if !ok {
		return InboundEvent{}, false, nil
	}
	switch strings.ToLower(strings.TrimSpace(payload.RequestType)) {
	case "friend":
		return InboundEvent{
			EventID:         stableOneBotEventID("request", "friend", selfID, userID, flag, fmt.Sprintf("%d", payload.Time)),
			Scene:           SceneC2C,
			ProviderSubject: userID,
			OneBotRequest:   &OneBotRequestApproval{Kind: "friend", Flag: flag},
		}, true, nil
	case "group":
		if strings.ToLower(strings.TrimSpace(payload.SubType)) != "add" {
			return InboundEvent{}, false, nil
		}
		groupID := payload.GroupID.String()
		if !validOneBotID(groupID) {
			return InboundEvent{}, false, nil
		}
		return InboundEvent{
			EventID:         stableOneBotEventID("request", "group", "add", selfID, groupID, userID, flag, fmt.Sprintf("%d", payload.Time)),
			Scene:           SceneGroup,
			ProviderSubject: userID,
			SourceID:        groupID,
			OneBotRequest:   &OneBotRequestApproval{Kind: "group", Flag: flag, SubType: "add"},
		}, true, nil
	default:
		return InboundEvent{}, false, nil
	}
}

func oneBotRequestFlag(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	var value string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var number json.Number
		if err := decoder.Decode(&number); err != nil {
			return "", false
		}
		value = number.String()
	}
	value = strings.TrimSpace(value)
	return value, validOneBotRequestFlag(value)
}

func validOneBotRequestFlag(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 512
}

func oneBotMessageText(message json.RawMessage, rawMessage string) (string, error) {
	message = bytes.TrimSpace(message)
	if len(message) == 0 || bytes.Equal(message, []byte("null")) {
		return stripOneBotCQCodes(rawMessage), nil
	}
	if message[0] == '"' {
		var value string
		if err := json.Unmarshal(message, &value); err != nil {
			return "", err
		}
		return stripOneBotCQCodes(value), nil
	}
	var segments []oneBotMessageSegment
	if err := json.Unmarshal(message, &segments); err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, segment := range segments {
		if strings.ToLower(strings.TrimSpace(segment.Type)) != "text" {
			continue
		}
		rawText, ok := segment.Data["text"]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(rawText, &text); err != nil {
			return "", err
		}
		builder.WriteString(text)
	}
	return builder.String(), nil
}

func stripOneBotCQCodes(value string) string {
	value = html.UnescapeString(value)
	var builder strings.Builder
	for index := 0; index < len(value); {
		if strings.HasPrefix(value[index:], "[CQ:") {
			if end := strings.IndexByte(value[index:], ']'); end >= 0 {
				index += end + 1
				continue
			}
		}
		builder.WriteByte(value[index])
		index++
	}
	return builder.String()
}

func stableOneBotEventID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "onebot-" + hex.EncodeToString(digest[:16])
}

func validOneBotID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 32 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
