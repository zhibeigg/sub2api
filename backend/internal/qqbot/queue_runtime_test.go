package qqbot

import (
	"context"
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
