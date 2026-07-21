package qqbot

import (
	"encoding/json"
	"testing"
)

type captureEventSink struct {
	events []InboundEvent
}

func (s *captureEventSink) enqueueEvent(event InboundEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestC2CEventUsesOnlyOfficialUserOpenIDFields(t *testing.T) {
	sink := &captureEventSink{}
	setActiveEventSink(sink)
	defer clearActiveEventSink(sink)

	data, err := json.Marshal(eventData{
		ID:         "message-1",
		Content:    "/check",
		UserOpenID: "top-level-openid",
		Author:     eventUser{ID: "generic-author-id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatchWebhookPayload(webhookPayload{EventID: "event-1", Type: "C2C_MESSAGE_CREATE", Data: data}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].ProviderSubject != "top-level-openid" {
		t.Fatalf("events=%#v", sink.events)
	}
}

func TestC2CEventRejectsGenericAuthorIDFallback(t *testing.T) {
	sink := &captureEventSink{}
	setActiveEventSink(sink)
	defer clearActiveEventSink(sink)

	data, err := json.Marshal(eventData{ID: "message-1", Content: "/check", Author: eventUser{ID: "generic-author-id"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatchWebhookPayload(webhookPayload{EventID: "event-1", Type: "C2C_MESSAGE_CREATE", Data: data}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("generic author ID was accepted: %#v", sink.events)
	}
}

func TestEnterAIORejectsGenericAuthorIDFallback(t *testing.T) {
	sink := &captureEventSink{}
	setActiveEventSink(sink)
	defer clearActiveEventSink(sink)

	data, err := json.Marshal(eventData{Author: eventUser{ID: "generic-author-id"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatchWebhookPayload(webhookPayload{EventID: "event-1", Type: "ENTER_AIO", Data: data}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("generic ENTER_AIO author ID was accepted: %#v", sink.events)
	}
}
