package qqbot

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func TestReliableQueueAtomicDedupAndAck(t *testing.T) {
	queue, client := newRedisQueue(t)
	ctx := t.Context()
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	event := InboundEvent{EventID: "event-1", Scene: SceneC2C, ProviderSubject: "openid", Content: "/help"}
	if err := queue.Enqueue(ctx, event, 64); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(ctx, event, 64); err != nil {
		t.Fatal(err)
	}
	if length, _ := client.XLen(ctx, queue.stream()).Result(); length != 1 {
		t.Fatalf("stream length=%d", length)
	}
	items, err := queue.Read(ctx, "consumer", 1, time.Millisecond)
	if err != nil || len(items) != 1 || items[0].Event.EventID != event.EventID {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if err := queue.Ack(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
	backlog, pending, dead := queue.Stats(ctx)
	if backlog != 0 || pending != 0 || dead != 0 {
		t.Fatalf("backlog=%d pending=%d dead=%d", backlog, pending, dead)
	}
}

func TestReliableQueueMovesMalformedPayloadToDeadLetter(t *testing.T) {
	queue, client := newRedisQueue(t)
	ctx := t.Context()
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: queue.stream(), Values: map[string]any{"payload": "enc:not-json"}}).Err(); err != nil {
		t.Fatal(err)
	}
	items, err := queue.Read(ctx, "consumer", 1, time.Millisecond)
	if err != nil || len(items) != 1 || items[0].DecodeError != "queue_payload_decode_failed" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if err := queue.DeadLetter(ctx, items[0], items[0].DecodeError); err != nil {
		t.Fatal(err)
	}
	backlog, pending, dead := queue.Stats(ctx)
	if backlog != 0 || pending != 0 || dead != 1 {
		t.Fatalf("backlog=%d pending=%d dead=%d", backlog, pending, dead)
	}
}

func TestReliableQueuePersistsEncryptedMediaFileInfo(t *testing.T) {
	queue, client := newRedisQueue(t)
	ctx := t.Context()
	if err := queue.SetMediaFileInfo(ctx, "app\x00group\x00event", "opaque-file-info"); err != nil {
		t.Fatal(err)
	}
	stored, err := client.Get(ctx, queue.prefix+":media:"+Fingerprint("app\x00group\x00event")).Result()
	if err != nil {
		t.Fatal(err)
	}
	if stored == "opaque-file-info" || !strings.HasPrefix(stored, "enc:") {
		t.Fatalf("media file info was not encrypted: %q", stored)
	}
	fileInfo, found, err := queue.GetMediaFileInfo(ctx, "app\x00group\x00event")
	if err != nil || !found || fileInfo != "opaque-file-info" {
		t.Fatalf("file_info=%q found=%v err=%v", fileInfo, found, err)
	}
}

func TestReliableQueueAllowOnceIsIdempotentPerEvent(t *testing.T) {
	queue, _ := newRedisQueue(t)
	ctx := t.Context()
	allowed, _, err := queue.AllowOnce(ctx, "check:app:c2c:openid", "event-1", 1, 30*time.Second)
	if err != nil || !allowed {
		t.Fatalf("first request allowed=%v err=%v", allowed, err)
	}
	allowed, _, err = queue.AllowOnce(ctx, "check:app:c2c:openid", "event-1", 1, 30*time.Second)
	if err != nil || !allowed {
		t.Fatalf("same event retry allowed=%v err=%v", allowed, err)
	}
	allowed, retryAfter, err := queue.AllowOnce(ctx, "check:app:c2c:openid", "event-2", 1, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if allowed || retryAfter <= 0 {
		t.Fatalf("new event allowed=%v retry_after=%s", allowed, retryAfter)
	}
}

func TestRuntimeDrainTimeoutPreservesPendingEvent(t *testing.T) {
	queue, _ := newRedisQueue(t)
	ctx := t.Context()
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	event := InboundEvent{EventID: "event-pending", Scene: SceneC2C, ProviderSubject: "openid", Content: "/help"}
	if err := queue.Enqueue(ctx, event, 64); err != nil {
		t.Fatal(err)
	}
	items, err := queue.Read(ctx, "consumer", 1, time.Millisecond)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	generationCtx, generationCancel := context.WithCancel(context.Background())
	generation := &runtimeGeneration{ctx: generationCtx, cancel: generationCancel}
	runtime := &Runtime{queue: queue}
	drainCtx, drainCancel := context.WithTimeout(ctx, 25*time.Millisecond)
	defer drainCancel()
	if err := runtime.drainAndStop(drainCtx, generation); err == nil {
		t.Fatal("drain unexpectedly succeeded with a pending event")
	}
	if generationCtx.Err() == nil {
		t.Fatal("generation was not cancelled after drain timeout")
	}
	_, pending, _ := queue.Stats(ctx)
	if pending != 1 {
		t.Fatalf("pending=%d", pending)
	}
}

type recordingMessenger struct {
	mu       sync.Mutex
	contents []string
}

func (m *recordingMessenger) Probe(context.Context) (string, error) { return "bot", nil }
func (m *recordingMessenger) record(content string) error {
	m.mu.Lock()
	m.contents = append(m.contents, content)
	m.mu.Unlock()
	return nil
}
func (m *recordingMessenger) SendGroup(_ context.Context, _, _, _, content string, _ uint32) error {
	return m.record(content)
}
func (m *recordingMessenger) SendC2C(_ context.Context, _, _, _, content string, _ uint32) error {
	return m.record(content)
}
func (m *recordingMessenger) SendChannel(_ context.Context, _, _, _, content string, _ uint32) error {
	return m.record(content)
}
func (m *recordingMessenger) SendGroupImage(_ context.Context, _, _, _, imageURL string, _ uint32) error {
	return m.record(imageURL)
}
func (m *recordingMessenger) SendC2CImage(_ context.Context, _, _, _, imageURL string, _ uint32) error {
	return m.record(imageURL)
}
func (m *recordingMessenger) SendChannelImage(_ context.Context, _, _, _, imageURL string, _ uint32) error {
	return m.record(imageURL)
}

type retryImageMessenger struct {
	recordingMessenger
	sequences []uint32
	attempts  int
}

func (m *retryImageMessenger) SendC2CImage(_ context.Context, _, _, _ string, _ string, sequence uint32) error {
	m.sequences = append(m.sequences, sequence)
	m.attempts++
	if m.attempts < 3 {
		return errors.New("temporary send failure")
	}
	return nil
}

func TestRuntimeImageRetriesReuseMessageSequence(t *testing.T) {
	runtime := &Runtime{state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &retryImageMessenger{}
	event := InboundEvent{EventID: "event", MessageID: "message", Scene: SceneC2C, ProviderSubject: "openid"}
	if err := runtime.sendImage(t.Context(), messenger, event, "https://status.example.com/image.png"); err != nil {
		t.Fatal(err)
	}
	if len(messenger.sequences) != 3 || messenger.sequences[0] != 1 || messenger.sequences[1] != 1 || messenger.sequences[2] != 1 {
		t.Fatalf("image retry sequences=%v", messenger.sequences)
	}
}

func TestRuntimeChannelCheckUsesImageReplyForActiveC2CBinding(t *testing.T) {
	settings := defaultBusinessSettings()
	settings.ChannelCheckEnabled = true
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{settings: settings})
	binding := &channelCheckBindingStub{found: true}
	limiter := &channelCheckLimiterStub{allowed: true}
	channelCheck := &ChannelCheckService{
		settings: channelCheckSettingsStub{runtime: service.ChannelMonitorRuntime{Enabled: true}},
		binding:  binding,
		limiter:  limiter,
		manager:  manager,
		signer:   &ChannelCheckSigner{rootKey: bytes.Repeat([]byte{0x71}, 32)},
	}
	runtime := &Runtime{manager: manager, channelCheck: channelCheck, state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &recordingMessenger{}
	generation := &runtimeGeneration{config: ActiveConfig{Enabled: true, AppID: "app", AppSecret: "app-secret", PublicBaseURL: "https://status.example.com"}, messenger: messenger}
	event := InboundEvent{EventID: "event-check", MessageID: "message", Scene: SceneC2C, ProviderSubject: "openid", Content: "/check"}
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}
	messenger.mu.Lock()
	contents := append([]string(nil), messenger.contents...)
	messenger.mu.Unlock()
	if len(contents) != 1 || !strings.HasPrefix(contents[0], "https://status.example.com/api/v1/public/qqbot/channel-status.png?") {
		t.Fatalf("unexpected replies: %#v", contents)
	}
	if binding.subj != "c2c:openid" {
		t.Fatalf("binding subject=%q", binding.subj)
	}
}

func TestRuntimeWelcomeIsMarkedOnlyAfterSuccessfulSend(t *testing.T) {
	queue, _ := newRedisQueue(t)
	settings := defaultBusinessSettings()
	settings.AllowedGroupIDs = []string{"group"}
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{storage: defaultStorageConfig("https://example.com"), active: ActiveConfig{Enabled: true, AppID: "app"}, settings: settings})
	runtime := &Runtime{manager: manager, queue: queue, state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &recordingMessenger{}
	generation := &runtimeGeneration{config: ActiveConfig{Enabled: true, AppID: "app"}, messenger: messenger}
	event := InboundEvent{EventID: "event-1", Scene: SceneGroup, SourceID: "group", ProviderSubject: "openid", Content: "hello"}
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}
	event.EventID = "event-2"
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}
	messenger.mu.Lock()
	count := len(messenger.contents)
	messenger.mu.Unlock()
	if count != 1 {
		t.Fatalf("welcome send count=%d", count)
	}
}

var _ service.SecretEncryptor = testEncryptor{}
