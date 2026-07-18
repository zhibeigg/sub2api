package qqbot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
