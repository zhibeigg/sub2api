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

func TestOneBotRuntimeHandoffAcceptsOldGenerationEvent(t *testing.T) {
	queue, client := newRedisQueue(t)
	ctx := t.Context()
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}

	old := &oneBotRuntimeGeneration{config: OneBotActiveConfig{Enabled: true, QueueCapacity: 64, ConfigVersion: 1}}
	old.accepting.Store(true)
	next := &oneBotRuntimeGeneration{config: OneBotActiveConfig{Enabled: true, QueueCapacity: 64, ConfigVersion: 2}}
	runtime := &OneBotRuntime{queue: &OneBotQueue{ReliableQueue: queue}, generations: make(map[int64]*oneBotRuntimeGeneration)}
	runtime.trackGeneration(old)
	runtime.trackGeneration(next)
	runtime.generation.Store(next)

	event := InboundEvent{EventID: "handoff-event", Scene: SceneGroup, ProviderSubject: "group-1", Content: "/help"}
	if err := runtime.enqueueEvent(ctx, old, event); err != nil {
		t.Fatal(err)
	}
	if length, _ := client.XLen(ctx, queue.stream()).Result(); length != 1 {
		t.Fatalf("stream length=%d", length)
	}
	items, err := queue.Read(ctx, "handoff-inspector", 1, time.Millisecond)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if items[0].Event.RuntimeConfigVersion != 1 {
		t.Fatalf("runtime config version=%d", items[0].Event.RuntimeConfigVersion)
	}
	if target := runtime.generationForEvent(next, items[0].Event); target != old {
		t.Fatal("handoff event was not routed to its source generation")
	}

	old.accepting.Store(false)
	if err := runtime.enqueueEvent(ctx, old, InboundEvent{EventID: "retired-event", Scene: SceneGroup, ProviderSubject: "group-1"}); err != nil {
		t.Fatal(err)
	}
	if length, _ := client.XLen(ctx, queue.stream()).Result(); length != 1 {
		t.Fatalf("retired generation enqueued event, stream length=%d", length)
	}
}

func TestOneBotRuntimeDrainWaitsForBufferedHubEvents(t *testing.T) {
	queue, _ := newRedisQueue(t)
	started := make(chan struct{})
	release := make(chan struct{})
	hub, err := NewOneBotHub(OneBotHubOptions{
		SelfID:      "3944007489",
		AccessToken: testOneBotToken,
		EventBuffer: 1,
		EventHandler: func(context.Context, InboundEvent) error {
			close(started)
			<-release
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	generationCtx, generationCancel := context.WithCancel(context.Background())
	generation := &oneBotRuntimeGeneration{
		config: OneBotActiveConfig{Enabled: true},
		hub:    hub,
		ctx:    generationCtx,
		cancel: generationCancel,
	}
	runtime := &OneBotRuntime{queue: &OneBotQueue{ReliableQueue: queue}}
	hub.events <- InboundEvent{EventID: "buffered"}
	<-started
	hub.StopAccepting()

	done := make(chan error, 1)
	go func() {
		drainCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		done <- runtime.drainAndStop(drainCtx, generation)
	}()
	select {
	case err := <-done:
		t.Fatalf("drain returned before hub event completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if generationCtx.Err() == nil {
		t.Fatal("generation was not cancelled after drain")
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
	mu         sync.Mutex
	contents   []string
	groupIDs   []string
	channelIDs []string
}

func (m *recordingMessenger) Probe(context.Context) (string, error) { return "bot", nil }
func (m *recordingMessenger) record(content string) error {
	m.mu.Lock()
	m.contents = append(m.contents, content)
	m.mu.Unlock()
	return nil
}
func (m *recordingMessenger) SendGroup(_ context.Context, groupID, _, _, content string, _ uint32) error {
	m.mu.Lock()
	m.groupIDs = append(m.groupIDs, groupID)
	m.contents = append(m.contents, content)
	m.mu.Unlock()
	return nil
}
func (m *recordingMessenger) SendC2C(_ context.Context, _, _, _, content string, _ uint32) error {
	return m.record(content)
}
func (m *recordingMessenger) SendChannel(_ context.Context, channelID, _, _, content string, _ uint32) error {
	m.mu.Lock()
	m.channelIDs = append(m.channelIDs, channelID)
	m.contents = append(m.contents, content)
	m.mu.Unlock()
	return nil
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
	event := InboundEvent{EventID: "event-check", MessageID: "message", Scene: SceneC2C, ProviderSubject: "openid", FriendConversation: true, Content: "/check"}
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

func TestRuntimeGroupMemberJoinedUsesWelcomeMessageAndAllowlist(t *testing.T) {
	settings := defaultBusinessSettings()
	settings.BindingEnabled = false
	settings.ChannelCheckEnabled = false
	settings.AllowedGroupIDs = []string{"allowed-group"}
	settings.WelcomeMessage = "欢迎 {user} 加入 {site}\n绑定：{bind_command}\n状态：/check\n帮助：/help"
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{settings: settings})
	runtime := &Runtime{manager: manager, state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &recordingMessenger{}
	generation := &runtimeGeneration{messenger: messenger}

	event := InboundEvent{EventID: "event-group-join", Scene: SceneGroup, SourceID: "allowed-group", DisplayName: "Alice\n/bind <@all>", MemberJoined: true}
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}

	messenger.mu.Lock()
	contents := append([]string(nil), messenger.contents...)
	groupIDs := append([]string(nil), messenger.groupIDs...)
	messenger.mu.Unlock()
	if len(contents) != 1 || len(groupIDs) != 1 || groupIDs[0] != "allowed-group" {
		t.Fatalf("group deliveries=%v contents=%v", groupIDs, contents)
	}
	if strings.Contains(contents[0], "/bind") || strings.Contains(contents[0], "/check") || strings.Contains(contents[0], "<@") {
		t.Fatalf("disabled or unsafe content leaked: %q", contents[0])
	}
	if !strings.Contains(contents[0], "Alice ∕bind ‹@all›") || !strings.Contains(contents[0], "PokeAPI") || !strings.Contains(contents[0], "/help") {
		t.Fatalf("welcome placeholders were not rendered safely: %q", contents[0])
	}

	event.EventID = "event-group-denied"
	event.SourceID = "other-group"
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}
	messenger.mu.Lock()
	count := len(messenger.contents)
	messenger.mu.Unlock()
	if count != 1 {
		t.Fatalf("unlisted group received welcome, count=%d", count)
	}

	settings.WelcomeEnabled = false
	manager.snapshot.Store(&configSnapshot{settings: settings})
	event.EventID = "event-group-disabled"
	event.SourceID = "allowed-group"
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}
	messenger.mu.Lock()
	count = len(messenger.contents)
	messenger.mu.Unlock()
	if count != 1 {
		t.Fatalf("disabled welcome sent, count=%d", count)
	}
}

func TestRuntimeGuildMemberJoinedKeepsWelcomeChannelMapping(t *testing.T) {
	settings := defaultBusinessSettings()
	settings.AllowedGuildIDs = []string{"guild-1"}
	settings.GuildWelcomeChannels = map[string]string{"guild-1": "welcome-channel"}
	settings.WelcomeMessage = "欢迎 {user}"
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{settings: settings})
	runtime := &Runtime{manager: manager, state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &recordingMessenger{}
	generation := &runtimeGeneration{messenger: messenger}

	event := InboundEvent{EventID: "event-guild-join", Scene: SceneGuild, SourceID: "guild-1", GuildID: "guild-1", ChannelID: "original-channel", DisplayName: "Bob", MemberJoined: true}
	if err := runtime.process(t.Context(), generation, event); err != nil {
		t.Fatal(err)
	}

	messenger.mu.Lock()
	contents := append([]string(nil), messenger.contents...)
	channelIDs := append([]string(nil), messenger.channelIDs...)
	messenger.mu.Unlock()
	if len(contents) != 1 || contents[0] != "欢迎 Bob" || len(channelIDs) != 1 || channelIDs[0] != "welcome-channel" {
		t.Fatalf("channel deliveries=%v contents=%v", channelIDs, contents)
	}
}

func TestRenderWelcomeDefaultAdvertisesOnlyEnabledCommands(t *testing.T) {
	settings := defaultBusinessSettings()
	settings.BindingEnabled = false
	settings.ChannelCheckEnabled = false
	message := renderWelcome(settings, InboundEvent{DisplayName: ""})
	if strings.Contains(message, "/bind") || strings.Contains(message, "/check") {
		t.Fatalf("disabled commands advertised: %q", message)
	}
	if !strings.Contains(message, "新成员") || !strings.Contains(message, "/help") || !strings.Contains(message, "安全提示") {
		t.Fatalf("default welcome is incomplete: %q", message)
	}

	settings.BindingEnabled = true
	settings.ChannelCheckEnabled = true
	message = renderWelcome(settings, InboundEvent{DisplayName: "Carol"})
	if !strings.Contains(message, "/bind name@example.com") || !strings.Contains(message, "/check") || !strings.Contains(message, "/help") {
		t.Fatalf("enabled commands missing: %q", message)
	}
}

func TestRuntimeLeavesOrdinaryGroupMessagesSilent(t *testing.T) {
	settings := defaultBusinessSettings()
	settings.AllowedGroupIDs = []string{"group"}
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{storage: defaultStorageConfig("https://example.com"), active: ActiveConfig{Enabled: true, AppID: "app"}, settings: settings})
	runtime := &Runtime{manager: manager, state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &recordingMessenger{}
	generation := &runtimeGeneration{config: ActiveConfig{Enabled: true, AppID: "app"}, messenger: messenger}
	if err := runtime.process(t.Context(), generation, InboundEvent{EventID: "event", Scene: SceneGroup, SourceID: "group", ProviderSubject: "20001", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	messenger.mu.Lock()
	defer messenger.mu.Unlock()
	if len(messenger.contents) != 0 || len(messenger.groupIDs) != 0 {
		t.Fatalf("ordinary group message produced output: groups=%v contents=%v", messenger.groupIDs, messenger.contents)
	}
}

func TestRuntimeFriendOpeningIsMarkedOnlyAfterSuccessfulSend(t *testing.T) {
	queue, _ := newRedisQueue(t)
	settings := defaultBusinessSettings()
	settings.FirstInteractionEnabled = false
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{storage: defaultStorageConfig("https://example.com"), active: ActiveConfig{Enabled: true, AppID: "app"}, settings: settings})
	runtime := &Runtime{manager: manager, queue: queue, state: RuntimeState{ProcessStatus: RuntimeRunning}}
	messenger := &recordingMessenger{}
	generation := &runtimeGeneration{config: ActiveConfig{Enabled: true, AppID: "app"}, messenger: messenger}
	event := InboundEvent{EventID: "event-1", Scene: SceneC2C, ProviderSubject: "20001", FriendConversation: true, FriendAdded: true}
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
		t.Fatalf("friend opening send count=%d", count)
	}
}

var _ service.SecretEncryptor = testEncryptor{}
