package qqbot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testRuntimeWithConfig(cfg ActiveConfig) *Runtime {
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{storage: storageConfig{Enabled: cfg.Enabled, AppID: cfg.AppID, WebhookSecretCiphertext: "configured", ConfigVersion: cfg.ConfigVersion}, active: cfg, settings: defaultBusinessSettings()})
	return &Runtime{manager: manager, state: RuntimeState{ProcessStatus: RuntimeRunning}}
}

func TestWebhookValidationAndHeartbeat(t *testing.T) {
	cfg := ActiveConfig{Enabled: true, AppID: "123456", WebhookSecret: "0123456789abcdef0123456789abcdef", ConfigVersion: 2}
	runtime := testRuntimeWithConfig(cfg)

	validationBody := []byte(`{"op":13,"d":{"plain_token":"plain-token","event_ts":"1720000000"}}`)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/qq", bytes.NewReader(validationBody))
	request.Header.Set("X-Bot-Appid", cfg.AppID)
	recorder := httptest.NewRecorder()
	runtime.ServeWebhook(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("validation status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var validation struct {
		PlainToken string `json:"plain_token"`
		Signature  string `json:"signature"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &validation); err != nil {
		t.Fatal(err)
	}
	verified, err := verifySignature(cfg.WebhookSecret, "1720000000", validation.Signature, []byte(validation.PlainToken))
	if err != nil || !verified {
		t.Fatalf("validation signature rejected: %v", err)
	}

	generationCtx := request.Context()
	runtime.generation.Store(&runtimeGeneration{config: cfg, ctx: generationCtx})
	heartbeatBody := []byte(`{"op":1,"d":42}`)
	heartbeat := httptest.NewRequest(http.MethodPost, "/webhooks/qq", bytes.NewReader(heartbeatBody))
	heartbeat.Header.Set(headerTimestamp, "1720000001")
	heartbeatSignature, _ := generateSignature(cfg.WebhookSecret, "1720000001", heartbeatBody)
	heartbeat.Header.Set(headerSignature, heartbeatSignature)
	heartbeatRecorder := httptest.NewRecorder()
	runtime.ServeWebhook(heartbeatRecorder, heartbeat)
	if heartbeatRecorder.Code != http.StatusOK || !bytes.Contains(heartbeatRecorder.Body.Bytes(), []byte(`"op":11`)) {
		t.Fatalf("heartbeat response=%d %s", heartbeatRecorder.Code, heartbeatRecorder.Body.String())
	}
}

func TestWebhookValidationDoesNotRejectMismatchedAppIDHeader(t *testing.T) {
	cfg := ActiveConfig{Enabled: true, AppID: "123456", WebhookSecret: "0123456789abcdef0123456789abcdef", ConfigVersion: 2}
	runtime := testRuntimeWithConfig(cfg)
	body := []byte(`{"d":{"plain_token":"plain-token","event_ts":"1720000000"},"op":13}`)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/qq", bytes.NewReader(body))
	request.Header.Set("X-Bot-Appid", "654321")
	recorder := httptest.NewRecorder()

	runtime.ServeWebhook(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("validation status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		PlainToken string `json:"plain_token"`
		Signature  string `json:"signature"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	verified, err := verifySignature(cfg.WebhookSecret, "1720000000", response.Signature, []byte(response.PlainToken))
	if err != nil || !verified {
		t.Fatalf("validation signature rejected: %v", err)
	}
}

func TestWebhookDispatchFailsClosedWhenQueueUnavailable(t *testing.T) {
	cfg := ActiveConfig{Enabled: true, AppID: "123456", WebhookSecret: "0123456789abcdef0123456789abcdef", ConfigVersion: 2, QueueCapacity: 64}
	runtime := testRuntimeWithConfig(cfg)
	runtime.generation.Store(&runtimeGeneration{config: cfg, ctx: t.Context()})
	setActiveEventSink(runtime)
	defer clearActiveEventSink(runtime)
	body := []byte(`{"op":0,"id":"event-1","t":"GROUP_AT_MESSAGE_CREATE","d":{"id":"message-1","content":"/help","group_openid":"group-secret","author":{"member_openid":"openid-secret"}}}`)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/qq", bytes.NewReader(body))
	request.Header.Set(headerTimestamp, "1720000002")
	signature, _ := generateSignature(cfg.WebhookSecret, "1720000002", body)
	request.Header.Set(headerSignature, signature)
	recorder := httptest.NewRecorder()
	runtime.ServeWebhook(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"d":1`)) {
		t.Fatalf("dispatch response=%d %s", recorder.Code, recorder.Body.String())
	}
}
