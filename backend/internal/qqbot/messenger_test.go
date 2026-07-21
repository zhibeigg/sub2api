package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type botGoMediaStoreStub struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *botGoMediaStoreStub) GetMediaFileInfo(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return value, ok, nil
}

func (s *botGoMediaStoreStub) SetMediaFileInfo(_ context.Context, key, fileInfo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = fileInfo
	return nil
}

func TestBotGoMessengerPreservesReplyIdentifiers(t *testing.T) {
	var sent botGoMessage
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "access_token": "token", "expires_in": "7200"})
		case "/users/@me":
			if request.Header.Get("Authorization") != "QQBot token" || request.Header.Get("X-Union-Appid") != "app" {
				t.Errorf("missing botgo headers")
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"id": "bot-id"})
		case "/v2/groups/group/messages":
			if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"id": "sent"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	messenger, err := NewBotGoMessenger("app", "secret", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	messenger.baseURL = server.URL
	messenger.tokenURL = server.URL + "/token"
	if botID, err := messenger.Probe(t.Context()); err != nil || botID != "app" {
		t.Fatalf("probe=%q err=%v", botID, err)
	}
	if err := messenger.SendGroup(t.Context(), "group", "message-id", "event-id", "hello", 3); err != nil {
		t.Fatal(err)
	}
	if sent.MsgID != "message-id" || sent.EventID != "event-id" || sent.MsgSeq != 3 || sent.Content != "hello" {
		t.Fatalf("sent=%#v", sent)
	}
}

func TestBotGoMessengerProactiveC2COmitsInboundReplyIdentifiers(t *testing.T) {
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "access_token": "token", "expires_in": "7200"})
		case "/v2/users/openid/messages":
			if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	messenger, err := NewBotGoMessenger("app", "secret", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	messenger.baseURL = server.URL
	messenger.tokenURL = server.URL + "/token"
	if err := messenger.SendProactiveC2C(t.Context(), "openid", "admin alert"); err != nil {
		t.Fatal(err)
	}
	if sent["content"] != "admin alert" {
		t.Fatalf("sent=%#v", sent)
	}
	for _, forbidden := range []string{"msg_id", "event_id", "msg_seq"} {
		if _, exists := sent[forbidden]; exists {
			t.Fatalf("proactive request contains %s: %#v", forbidden, sent)
		}
	}
}

func TestBotGoMessengerUploadsAndRepliesWithGroupImage(t *testing.T) {
	var upload botGoMediaUploadRequest
	var sent botGoMessage
	var order []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "access_token": "token", "expires_in": "7200"})
		case "/v2/groups/group/files":
			order = append(order, "files")
			if err := json.NewDecoder(request.Body).Decode(&upload); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"file_info": "opaque-file-info"})
		case "/v2/groups/group/messages":
			order = append(order, "messages")
			if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	messenger, err := NewBotGoMessenger("app", "secret", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	messenger.baseURL = server.URL
	messenger.tokenURL = server.URL + "/token"
	imageURL := "https://status.example.com/api/v1/public/qqbot/channel-status.png?sig=opaque"
	if err := messenger.SendGroupImage(t.Context(), "group", "message-id", "event-id", imageURL, 4); err != nil {
		t.Fatal(err)
	}
	if err := messenger.SendGroupImage(t.Context(), "group", "message-id", "event-id", imageURL+"&retry=1", 4); err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[0] != "files" || order[1] != "messages" || order[2] != "messages" {
		t.Fatalf("unexpected request order: %#v", order)
	}
	if upload.FileType != 1 || upload.URL != imageURL || upload.SrvSendMsg {
		t.Fatalf("upload=%#v", upload)
	}
	if sent.MsgType != 7 || sent.MsgID != "message-id" || sent.EventID != "event-id" || sent.MsgSeq != 4 || sent.Media == nil || sent.Media.FileInfo != "opaque-file-info" {
		t.Fatalf("sent=%#v", sent)
	}
}

func TestBotGoMessengerReusesPersistedFileInfoAcrossInstances(t *testing.T) {
	var filesCalls int
	var messageCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "access_token": "token", "expires_in": "7200"})
		case "/v2/groups/group/files":
			filesCalls++
			_ = json.NewEncoder(writer).Encode(map[string]string{"file_info": "persisted-file-info"})
		case "/v2/groups/group/messages":
			messageCalls++
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	store := &botGoMediaStoreStub{values: map[string]string{}}
	newMessenger := func() *BotGoMessenger {
		messenger, err := NewBotGoMessenger("app", "secret", false, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		messenger.baseURL = server.URL
		messenger.tokenURL = server.URL + "/token"
		messenger.setMediaStore(store)
		return messenger
	}
	first := newMessenger()
	second := newMessenger()
	if err := first.SendGroupImage(t.Context(), "group", "message-id", "event-id", "https://status.example.com/image.png?sig=first", 1); err != nil {
		t.Fatal(err)
	}
	if err := second.SendGroupImage(t.Context(), "group", "message-id", "event-id", "https://status.example.com/image.png?sig=second", 1); err != nil {
		t.Fatal(err)
	}
	if filesCalls != 1 || messageCalls != 2 {
		t.Fatalf("files_calls=%d message_calls=%d", filesCalls, messageCalls)
	}
}

func TestBotGoMessengerUsesOfficialChannelImageField(t *testing.T) {
	var sent botGoMessage
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "access_token": "token", "expires_in": "7200"})
		case "/channels/channel/messages":
			if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"id": "sent"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	messenger, err := NewBotGoMessenger("app", "secret", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	messenger.baseURL = server.URL
	messenger.tokenURL = server.URL + "/token"
	imageURL := "https://status.example.com/channel-status.png?sig=opaque"
	if err := messenger.SendChannelImage(t.Context(), "channel", "message-id", "event-id", imageURL, 2); err != nil {
		t.Fatal(err)
	}
	if sent.Image != imageURL || sent.MsgID != "message-id" || sent.EventID != "event-id" || sent.MsgSeq != 2 || sent.Media != nil {
		t.Fatalf("sent=%#v", sent)
	}
}

func TestBotGoMessengerClassifiesUploadClientErrorsAsDefinitive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "access_token": "token", "expires_in": "7200"})
		case "/v2/users/user/files":
			writer.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 40034025, "message": "invalid media"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	messenger, err := NewBotGoMessenger("app", "secret", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	messenger.baseURL = server.URL
	messenger.tokenURL = server.URL + "/token"
	err = messenger.SendC2CImage(t.Context(), "user", "message-id", "event-id", "https://status.example.com/image.png", 1)
	var apiErr *BotGoAPIError
	if !errors.As(err, &apiErr) || !apiErr.Definitive() || apiErr.Code != 40034025 {
		t.Fatalf("unexpected upload error: %v", err)
	}
	if (&BotGoAPIError{StatusCode: http.StatusTooManyRequests}).Definitive() {
		t.Fatal("rate limit response was classified as definitive")
	}
}
