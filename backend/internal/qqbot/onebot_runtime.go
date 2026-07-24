package qqbot

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type oneBotRuntimeGeneration struct {
	config    OneBotActiveConfig
	hub       *OneBotHub
	messenger *OneBotMessenger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	accepting atomic.Bool
}

type OneBotChannelCheckDiagnostics struct {
	LastRecognizedAt      *time.Time `json:"last_recognized_at,omitempty"`
	LastURLIssuedAt       *time.Time `json:"last_url_issued_at,omitempty"`
	LastImageActionSentAt *time.Time `json:"last_image_action_sent_at,omitempty"`
	LastFailureStage      string     `json:"last_failure_stage,omitempty"`
	LastFailureErrorCode  string     `json:"last_failure_error_code,omitempty"`
	LastFailureAt         *time.Time `json:"last_failure_at,omitempty"`
}

type OneBotRuntimeState struct {
	RuntimeState
	Connected               bool                          `json:"connected"`
	ConnectionID            uint64                        `json:"connection_id,omitempty"`
	SelfIDFingerprint       string                        `json:"self_id_fingerprint,omitempty"`
	PendingActions          int                           `json:"pending_actions"`
	ConnectedAt             *time.Time                    `json:"connected_at,omitempty"`
	LastActionAt            *time.Time                    `json:"last_action_at,omitempty"`
	LastDisconnectAt        *time.Time                    `json:"last_disconnect_at,omitempty"`
	ChannelCheckDiagnostics OneBotChannelCheckDiagnostics `json:"channel_check_diagnostics"`
}

type OneBotRuntime struct {
	manager   *OneBotConfigManager
	queue     *OneBotQueue
	processor *Runtime

	transportMu   sync.RWMutex
	transportMode TransportMode

	generation atomic.Pointer[oneBotRuntimeGeneration]
	stopping   atomic.Bool
	reloadMu   sync.Mutex

	generationsMu sync.RWMutex
	generations   map[int64]*oneBotRuntimeGeneration

	lifecycleMu sync.Mutex
	root        context.Context
	cancel      context.CancelFunc

	stateMu                 sync.RWMutex
	state                   RuntimeState
	channelCheckDiagnostics OneBotChannelCheckDiagnostics
}

func NewOneBotRuntime(manager *OneBotConfigManager, queue *OneBotQueue, processor *Runtime) *OneBotRuntime {
	runtime := &OneBotRuntime{
		manager:       manager,
		queue:         queue,
		processor:     processor,
		transportMode: TransportModeBotGo,
		generations:   make(map[int64]*oneBotRuntimeGeneration),
		state:         RuntimeState{ProcessStatus: RuntimeDisabled},
	}
	if manager != nil {
		manager.SetOnReload(runtime.applyConfig)
	}
	if processor != nil {
		processor.SetOneBotRuntime(runtime)
	}
	return runtime
}

func (r *OneBotRuntime) SyncTransportMode(ctx context.Context, mode TransportMode) error {
	if r == nil {
		return ErrInvalidConfig
	}
	if r.manager == nil {
		return ErrRuntimeUnavailable
	}
	r.transportMu.Lock()
	r.transportMode = normalizeTransportMode(mode)
	r.transportMu.Unlock()
	cfg, ok := r.manager.Active()
	if !ok {
		return nil
	}
	r.lifecycleMu.Lock()
	started := r.root != nil
	r.lifecycleMu.Unlock()
	if !started {
		return nil
	}
	return r.applyConfig(ctx, cfg)
}

func (r *OneBotRuntime) transportSelected() bool {
	if r == nil {
		return false
	}
	r.transportMu.RLock()
	selected := r.transportMode == TransportModeOneBot
	r.transportMu.RUnlock()
	return selected
}

func (r *OneBotRuntime) Start(ctx context.Context) error {
	if r == nil || r.manager == nil || r.queue == nil || r.processor == nil {
		return errors.New("onebot runtime unavailable")
	}
	r.lifecycleMu.Lock()
	if r.cancel != nil {
		r.lifecycleMu.Unlock()
		return nil
	}
	r.root, r.cancel = context.WithCancel(ctx)
	r.stopping.Store(false)
	r.lifecycleMu.Unlock()
	return r.manager.Start(r.root)
}

func (r *OneBotRuntime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.stopping.Store(true)
	r.lifecycleMu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.root = nil
	r.lifecycleMu.Unlock()
	var managerErr error
	if r.manager != nil {
		managerErr = r.manager.Shutdown(ctx)
	}
	generation := r.generation.Swap(nil)
	if generation != nil && generation.hub != nil {
		generation.hub.StopAccepting()
	}
	workerErr := r.drainAndStop(ctx, generation)
	if generation != nil {
		generation.accepting.Store(false)
		r.untrackGeneration(generation)
	}
	if cancel != nil {
		cancel()
	}
	r.stateMu.Lock()
	r.state.ProcessStatus = RuntimeDisabled
	r.state.WorkerTotal = 0
	r.state.WorkerActive = 0
	r.stateMu.Unlock()
	return errors.Join(workerErr, managerErr)
}

func (r *OneBotRuntime) applyConfig(ctx context.Context, cfg OneBotActiveConfig) error {
	if r == nil {
		return errors.New("onebot runtime unavailable")
	}
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()
	if r.stopping.Load() {
		return context.Canceled
	}

	r.stateMu.Lock()
	r.state.DesiredConfigVersion = cfg.ConfigVersion
	if r.generation.Load() == nil {
		r.state.ProcessStatus = RuntimeStarting
	} else {
		r.state.ProcessStatus = RuntimeReloading
	}
	r.stateMu.Unlock()

	selected := r.transportSelected()
	if !selected || strings.TrimSpace(cfg.SelfID) == "" || strings.TrimSpace(cfg.AccessToken) == "" {
		old := r.generation.Swap(nil)
		if old != nil && old.hub != nil {
			old.hub.StopAccepting()
		}
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = r.drainAndStop(stopCtx, old)
		cancel()
		if old != nil {
			old.accepting.Store(false)
			r.untrackGeneration(old)
		}
		r.stateMu.Lock()
		r.state.ActiveConfigVersion = cfg.ConfigVersion
		r.state.WorkerTotal = 0
		r.state.WorkerActive = 0
		if cfg.Enabled && selected {
			r.state.ProcessStatus = RuntimeDegraded
			r.state.LastErrorCode = "runtime_credentials_missing"
			r.state.LastErrorMessage = "runtime_credentials_missing"
			now := time.Now().UTC()
			r.state.LastErrorAt = &now
		} else {
			r.state.ProcessStatus = RuntimeDisabled
			r.state.LastErrorCode = ""
			r.state.LastErrorMessage = ""
			r.state.LastErrorAt = nil
		}
		r.stateMu.Unlock()
		if cfg.Enabled && selected {
			return ErrInvalidConfig
		}
		return nil
	}

	r.lifecycleMu.Lock()
	root := r.root
	r.lifecycleMu.Unlock()
	if root == nil {
		return errors.New("onebot runtime not started")
	}
	generationCtx, generationCancel := context.WithCancel(root)
	next := &oneBotRuntimeGeneration{config: cfg, ctx: generationCtx, cancel: generationCancel}
	hub, err := NewOneBotHub(OneBotHubOptions{
		SelfID:        cfg.SelfID,
		AccessToken:   cfg.AccessToken,
		ActionTimeout: time.Duration(cfg.ActionTimeoutMS) * time.Millisecond,
		EventHandler: func(eventCtx context.Context, event InboundEvent) error {
			return r.enqueueEvent(eventCtx, next, event)
		},
	})
	if err != nil {
		generationCancel()
		r.recordError("hub_create_failed")
		return err
	}
	messenger, err := NewOneBotMessenger(hub)
	if err != nil {
		_ = hub.Close()
		generationCancel()
		r.recordError("messenger_create_failed")
		return err
	}
	next.hub = hub
	next.messenger = messenger

	if cfg.Enabled {
		if err := r.queue.EnsureGroup(ctx); err != nil {
			_ = hub.Close()
			generationCancel()
			r.recordError("queue_group_failed")
			return err
		}
	}

	// Publish the fully constructed generation before retiring the previous one.
	// Existing old-generation WebSockets remain accepting while the shared queue
	// drains, so events already read during the handoff are not discarded. All new
	// connections immediately bind to the new generation.
	next.accepting.Store(true)
	r.trackGeneration(next)
	old := r.generation.Swap(next)
	if cfg.Enabled {
		for workerID := 0; workerID < cfg.WorkerCount; workerID++ {
			next.wg.Add(1)
			go r.worker(next, workerID)
		}
	}
	if old != nil {
		if old.hub != nil {
			old.hub.StopAccepting()
		}
		stopCtx, stopCancel := context.WithTimeout(ctx, 10*time.Second)
		_ = r.drainAndStop(stopCtx, old)
		stopCancel()
		old.accepting.Store(false)
		r.untrackGeneration(old)
	}
	r.stateMu.Lock()
	r.state.ActiveConfigVersion = cfg.ConfigVersion
	r.state.WorkerTotal = 0
	r.state.WorkerActive = 0
	if cfg.Enabled {
		r.state.ProcessStatus = RuntimeRunning
		r.state.WorkerTotal = cfg.WorkerCount
		r.state.WorkerActive = cfg.WorkerCount
	} else {
		r.state.ProcessStatus = RuntimeDisabled
	}
	r.state.LastErrorCode = ""
	r.state.LastErrorMessage = ""
	r.state.LastErrorAt = nil
	r.stateMu.Unlock()
	return nil
}

func (r *OneBotRuntime) ServeReverseWebSocket(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if request == nil || request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !trustedOneBotPeer(request) {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	generation := r.generation.Load()
	if generation == nil || generation.hub == nil {
		http.Error(writer, "onebot configuration unavailable", http.StatusServiceUnavailable)
		return
	}
	generation.hub.ServeHTTP(writer, request)
}

func trustedOneBotPeer(request *http.Request) bool {
	if request == nil {
		return false
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP"} {
		if strings.TrimSpace(request.Header.Get(header)) != "" {
			return false
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(request.RemoteAddr)
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func (r *OneBotRuntime) enqueueEvent(ctx context.Context, generation *oneBotRuntimeGeneration, event InboundEvent) error {
	if r == nil || generation == nil || !generation.accepting.Load() || !generation.config.Enabled {
		return nil
	}
	if ctx == nil {
		ctx = generation.ctx
	}
	event.RuntimeConfigVersion = generation.config.ConfigVersion
	if err := r.queue.Enqueue(ctx, event, generation.config.QueueCapacity); err != nil {
		r.recordError("event_enqueue_failed")
		return err
	}
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.LastWebhookAt = &now
	r.stateMu.Unlock()
	return nil
}

func (r *OneBotRuntime) trackGeneration(generation *oneBotRuntimeGeneration) {
	if r == nil || generation == nil || generation.config.ConfigVersion == 0 {
		return
	}
	r.generationsMu.Lock()
	if r.generations == nil {
		r.generations = make(map[int64]*oneBotRuntimeGeneration)
	}
	r.generations[generation.config.ConfigVersion] = generation
	r.generationsMu.Unlock()
}

func (r *OneBotRuntime) untrackGeneration(generation *oneBotRuntimeGeneration) {
	if r == nil || generation == nil || generation.config.ConfigVersion == 0 {
		return
	}
	r.generationsMu.Lock()
	if r.generations[generation.config.ConfigVersion] == generation {
		delete(r.generations, generation.config.ConfigVersion)
	}
	r.generationsMu.Unlock()
}

func (r *OneBotRuntime) generationForEvent(worker *oneBotRuntimeGeneration, event InboundEvent) *oneBotRuntimeGeneration {
	target := worker
	if r != nil && event.RuntimeConfigVersion != 0 {
		r.generationsMu.RLock()
		if candidate := r.generations[event.RuntimeConfigVersion]; candidate != nil {
			target = candidate
		}
		r.generationsMu.RUnlock()
	}
	if target != nil && target.hub != nil && !target.hub.Snapshot().Connected {
		current := r.generation.Load()
		if current != nil && current.hub != nil && current.hub.Snapshot().Connected {
			target = current
		}
	}
	return target
}

func (r *OneBotRuntime) worker(generation *oneBotRuntimeGeneration, workerID int) {
	defer generation.wg.Done()
	consumer := "onebot-worker-" + strconv.Itoa(workerID) + "-v" + strconv.FormatInt(generation.config.ConfigVersion, 10)
	reclaimTicker := time.NewTicker(15 * time.Second)
	defer reclaimTicker.Stop()
	for {
		select {
		case <-generation.ctx.Done():
			return
		case <-reclaimTicker.C:
			items, err := r.queue.Claim(generation.ctx, consumer, 30*time.Second, 10)
			if err == nil {
				r.processItems(generation, items)
			}
		default:
			items, err := r.queue.Read(generation.ctx, consumer, 10, 2*time.Second)
			if err != nil {
				if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					continue
				}
				r.recordError("queue_read_failed")
				continue
			}
			r.processItems(generation, items)
		}
	}
}

func (r *OneBotRuntime) processItems(generation *oneBotRuntimeGeneration, items []QueuedEvent) {
	for _, item := range items {
		if generation.ctx.Err() != nil {
			return
		}
		if item.DecodeError != "" {
			if err := r.queue.DeadLetter(generation.ctx, item, item.DecodeError); err != nil {
				r.recordError("queue_dead_letter_failed")
			} else {
				r.recordError(item.DecodeError)
			}
			slog.Warn("onebot queue payload rejected", "stream_id", shortID(item.ID), "error_code", item.DecodeError)
			continue
		}
		target := r.generationForEvent(generation, item.Event)
		if target == nil || target.messenger == nil {
			r.recordError("event_generation_unavailable")
			continue
		}
		processCtx, cancel := context.WithTimeout(target.ctx, 20*time.Second)
		var err error
		approvalAttempted := false
		if item.Event.OneBotRequest != nil {
			approvalAttempted, err = r.processRequestApproval(processCtx, target.messenger, item.Event)
		} else {
			err = r.processor.processWith(processCtx, r.activeProcessingConfig(target.config), target.messenger, r.queue.ReliableQueue, item.Event, r.markSent, r)
		}
		cancel()
		if approvalAttempted {
			auditCtx, auditCancel := context.WithTimeout(target.ctx, 5*time.Second)
			if auditErr := r.manager.RecordRequestApproval(auditCtx, target.config.SelfID, item.Event, err); auditErr != nil {
				slog.Warn("onebot request approval audit failed", "event_id", shortID(item.Event.EventID), "error_code", "request_approval_audit_failed")
			}
			auditCancel()
		}
		if err == nil {
			if ackErr := r.queue.Ack(target.ctx, item.ID); ackErr != nil {
				r.recordError("queue_ack_failed")
			}
			now := time.Now().UTC()
			r.stateMu.Lock()
			r.state.LastEventAt = &now
			r.stateMu.Unlock()
			continue
		}
		code := infraerrors.Reason(err)
		if code == "" {
			code = "event_process_failed"
		}
		if failErr := r.queue.Fail(target.ctx, item, code); failErr != nil {
			r.recordError("queue_retry_failed")
		} else {
			r.recordError(code)
		}
		slog.Warn("onebot event processing failed", "event_id", shortID(item.Event.EventID), "scene", item.Event.Scene, "error_code", code)
	}
}

func (r *OneBotRuntime) processRequestApproval(ctx context.Context, messenger *OneBotMessenger, incoming InboundEvent) (bool, error) {
	if r == nil || r.manager == nil || messenger == nil || incoming.OneBotRequest == nil {
		return false, ErrRuntimeUnavailable
	}
	request := incoming.OneBotRequest
	policy := r.manager.RequestPolicy()
	switch request.Kind {
	case "friend":
		if !policy.AutoApproveFriendRequests {
			return false, nil
		}
		return true, messenger.ApproveFriendRequest(ctx, request.Flag)
	case "group":
		if !policy.AutoApproveGroupRequests || request.SubType != "add" || r.processor == nil || r.processor.manager == nil {
			return false, nil
		}
		settings := r.processor.manager.BusinessSettings()
		if !contains(settings.AllowedGroupIDs, incoming.SourceID) {
			return false, nil
		}
		return true, messenger.ApproveGroupRequest(ctx, request.Flag, request.SubType)
	default:
		return false, nil
	}
}

func (r *OneBotRuntime) activeProcessingConfig(cfg OneBotActiveConfig) ActiveConfig {
	active := ActiveConfig{
		Enabled:       cfg.Enabled,
		AppID:         strings.TrimSpace(cfg.SelfID),
		WorkerCount:   cfg.WorkerCount,
		QueueCapacity: cfg.QueueCapacity,
		APITimeoutMS:  cfg.ActionTimeoutMS,
		ConfigVersion: cfg.ConfigVersion,
	}
	if r != nil && r.processor != nil && r.processor.manager != nil {
		if botGo, ok := r.processor.manager.Active(); ok {
			active.PublicBaseURL = botGo.PublicBaseURL
		}
	}
	return active
}

func (r *OneBotRuntime) Probe(ctx context.Context) ProbeResult {
	if r == nil || r.manager == nil {
		return failedProbe("runtime_unavailable", "OneBot runtime is unavailable", time.Now())
	}
	cfg, ok := r.manager.Active()
	if !ok {
		return failedProbe("credentials_missing", "OneBot credentials are not configured", time.Now())
	}
	return r.ProbeConfig(ctx, cfg)
}

func (r *OneBotRuntime) ProbeConfig(ctx context.Context, cfg OneBotActiveConfig) ProbeResult {
	startedAt := time.Now()
	if strings.TrimSpace(cfg.SelfID) == "" || strings.TrimSpace(cfg.AccessToken) == "" {
		return failedProbe("credentials_missing", "OneBot credentials are not configured", startedAt)
	}
	generation := r.generation.Load()
	if generation == nil || generation.hub == nil || generation.messenger == nil || generation.config.SelfID != cfg.SelfID || !sameSecret(generation.config.AccessToken, cfg.AccessToken) {
		return failedProbe("configuration_not_active", "Save the OneBot configuration before probing", startedAt)
	}
	timeout := time.Duration(cfg.ActionTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultOneBotActionTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	botID, err := generation.messenger.Probe(probeCtx)
	cancel()
	if err != nil {
		code := "onebot_probe_failed"
		if errors.Is(err, ErrOneBotDisconnected) {
			code = "reverse_ws_disconnected"
		}
		return failedProbe(code, "OneBot reverse WebSocket probe failed", startedAt)
	}
	if strings.TrimSpace(botID) != strings.TrimSpace(cfg.SelfID) {
		return failedProbe("self_id_mismatch", "OneBot login self ID does not match configuration", startedAt)
	}
	if err := r.manager.RecordSuccessfulProbe(ctx, cfg); err != nil {
		return failedProbe("probe_record_failed", "OneBot probe succeeded but could not be recorded", startedAt)
	}
	return ProbeResult{
		OK:               true,
		Status:           "success",
		Message:          "OneBot reverse WebSocket is connected",
		LatencyMS:        time.Since(startedAt).Milliseconds(),
		BotIDFingerprint: Fingerprint(botID),
		CheckedAt:        time.Now().UTC(),
	}
}

func sameSecret(left, right string) bool {
	leftDigest := sha256.Sum256([]byte(left))
	rightDigest := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftDigest[:], rightDigest[:]) == 1
}

func (r *OneBotRuntime) State(ctx context.Context) OneBotRuntimeState {
	result := OneBotRuntimeState{RuntimeState: RuntimeState{ProcessStatus: RuntimeDisabled}}
	if r == nil {
		return result
	}
	r.stateMu.RLock()
	result.RuntimeState = r.state
	result.ChannelCheckDiagnostics = copyOneBotChannelCheckDiagnostics(r.channelCheckDiagnostics)
	r.stateMu.RUnlock()
	if r.queue != nil {
		result.StreamBacklog, result.StreamPending, result.DeadLetterTotal = r.queue.Stats(ctx)
	}
	generation := r.generation.Load()
	if generation == nil || generation.hub == nil {
		return result
	}
	hubState := generation.hub.Snapshot()
	result.Connected = hubState.Connected
	result.ConnectionID = hubState.ConnectionID
	result.SelfIDFingerprint = hubState.SelfIDFingerprint
	result.PendingActions = hubState.PendingActions
	result.ConnectedAt = timePointer(hubState.ConnectedAt)
	result.LastActionAt = timePointer(hubState.LastActionAt)
	result.LastDisconnectAt = timePointer(hubState.LastDisconnectAt)
	if result.LastWebhookAt == nil {
		result.LastWebhookAt = timePointer(hubState.LastEventAt)
	}
	if generation.config.Enabled && !hubState.Connected && result.ProcessStatus == RuntimeRunning {
		result.ProcessStatus = RuntimeDegraded
		result.LastErrorCode = "reverse_ws_disconnected"
		result.LastErrorMessage = "reverse_ws_disconnected"
	}
	if result.LastErrorCode == "" && hubState.LastErrorCode != "" && hubState.LastErrorCode != "read_failed" {
		result.LastErrorCode = hubState.LastErrorCode
		result.LastErrorMessage = hubState.LastErrorCode
	}
	return result
}

func (r *OneBotRuntime) recordChannelCheckLifecycle(stage, eventID string, scene Scene, errorCode string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.stateMu.Lock()
	switch stage {
	case channelCheckStageRecognized:
		r.channelCheckDiagnostics.LastRecognizedAt = &now
		r.channelCheckDiagnostics.LastFailureStage = ""
		r.channelCheckDiagnostics.LastFailureErrorCode = ""
		r.channelCheckDiagnostics.LastFailureAt = nil
	case channelCheckStageURLIssued:
		r.channelCheckDiagnostics.LastURLIssuedAt = &now
	case channelCheckStageImageActionSent:
		r.channelCheckDiagnostics.LastImageActionSentAt = &now
	case channelCheckStagePrepareFailed, channelCheckStageImageFailed:
		r.channelCheckDiagnostics.LastFailureStage = stage
		r.channelCheckDiagnostics.LastFailureErrorCode = strings.TrimSpace(errorCode)
		r.channelCheckDiagnostics.LastFailureAt = &now
	}
	r.stateMu.Unlock()

	attributes := []any{"event_id", shortID(eventID), "scene", scene, "stage", stage}
	if errorCode != "" {
		attributes = append(attributes, "error_code", errorCode)
		slog.Warn("onebot channel check lifecycle", attributes...)
		return
	}
	slog.Info("onebot channel check lifecycle", attributes...)
}

func copyOneBotChannelCheckDiagnostics(value OneBotChannelCheckDiagnostics) OneBotChannelCheckDiagnostics {
	result := value
	result.LastRecognizedAt = timePointerValue(value.LastRecognizedAt)
	result.LastURLIssuedAt = timePointerValue(value.LastURLIssuedAt)
	result.LastImageActionSentAt = timePointerValue(value.LastImageActionSentAt)
	result.LastFailureAt = timePointerValue(value.LastFailureAt)
	return result
}

func timePointerValue(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copyValue := value
	return &copyValue
}

func (r *OneBotRuntime) markSent() {
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.LastSendAt = &now
	r.stateMu.Unlock()
}

func (r *OneBotRuntime) recordError(code string) {
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.ProcessStatus = RuntimeDegraded
	r.state.LastErrorCode = code
	r.state.LastErrorMessage = code
	r.state.LastErrorAt = &now
	r.stateMu.Unlock()
}

func (r *OneBotRuntime) drainAndStop(ctx context.Context, generation *oneBotRuntimeGeneration) error {
	if generation == nil {
		return nil
	}
	if r == nil || r.queue == nil || !generation.config.Enabled {
		return stopOneBotGeneration(ctx, generation)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		backlog, pending, _ := r.queue.Stats(ctx)
		hubDrained := generation.hub == nil || generation.hub.EventsDrained()
		if backlog == 0 && pending == 0 && hubDrained {
			return stopOneBotGeneration(ctx, generation)
		}
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return errors.Join(ctx.Err(), stopOneBotGeneration(stopCtx, generation))
		case <-ticker.C:
		}
	}
}

func stopOneBotGeneration(ctx context.Context, generation *oneBotRuntimeGeneration) error {
	if generation == nil {
		return nil
	}
	generation.cancel()
	done := make(chan struct{})
	go func() {
		generation.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return generation.hub.Close()
	case <-ctx.Done():
		_ = generation.hub.Close()
		return ctx.Err()
	}
}
