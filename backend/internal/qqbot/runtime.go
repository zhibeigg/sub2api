package qqbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	defaultHelpMessage    = "欢迎使用 PokeAPI 账户助手。\n\n绑定账户（白名单群内可直接发送）：/bind 你的邮箱\n查看渠道状态：发送 /check\n查看帮助：发送 /help\n\n验证链接只会发送到 Sub2API 账户邮箱。群内发送邮箱会被群成员看到；普通 QQ 私聊可能受好友限制。数字 QQ 仅作为展示信息，实际身份以机器人 OpenID 为准。"
	defaultWelcomeMessage = "欢迎 {user} 加入 {site}！\n\n可用指令：\n绑定账户（白名单群内可直接发送）：{bind_command}\n查看渠道状态：/check\n查看帮助：/help\n\n安全提示：请勿向任何人提供密码、验证码或 API 密钥；群内发送的邮箱对群成员可见，账户绑定链接只会发送到你的站点账户邮箱。"
	qqBotSiteName         = "PokeAPI"
)

type runtimeGeneration struct {
	config    ActiveConfig
	messenger Messenger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type Runtime struct {
	manager      *ConfigManager
	queue        *ReliableQueue
	binding      *service.QQBotService
	channelCheck *ChannelCheckService

	generation  atomic.Pointer[runtimeGeneration]
	stopping    atomic.Bool
	reloadMu    sync.Mutex
	lifecycleMu sync.Mutex
	root        context.Context
	cancel      context.CancelFunc

	stateMu sync.RWMutex
	state   RuntimeState
}

var _ service.QQBotProactiveC2CTransport = (*Runtime)(nil)

func NewRuntime(manager *ConfigManager, queue *ReliableQueue, binding *service.QQBotService, channelCheck *ChannelCheckService) *Runtime {
	runtime := &Runtime{manager: manager, queue: queue, binding: binding, channelCheck: channelCheck, state: RuntimeState{ProcessStatus: RuntimeDisabled}}
	registerGlobalHandlers()
	if manager != nil {
		manager.SetOnReload(runtime.applyConfig)
	}
	if binding != nil {
		binding.SetProactiveC2CTransport(runtime)
	}
	return runtime
}

func (r *Runtime) ActiveAppID() (string, bool) {
	if r == nil {
		return "", false
	}
	generation := r.generation.Load()
	if generation == nil || !generation.config.Enabled {
		return "", false
	}
	appID := strings.TrimSpace(generation.config.AppID)
	return appID, appID != ""
}

func (r *Runtime) SendProactiveC2C(ctx context.Context, botAppID, openID, content string) error {
	if r == nil {
		return ErrRuntimeUnavailable
	}
	botAppID = strings.TrimSpace(botAppID)
	openID = strings.TrimSpace(openID)
	content = strings.TrimSpace(content)
	if botAppID == "" || openID == "" || content == "" {
		return ErrInvalidConfig
	}
	generation := r.generation.Load()
	if generation == nil || !generation.config.Enabled {
		return ErrRuntimeDisabled
	}
	if strings.TrimSpace(generation.config.AppID) != botAppID || generation.messenger == nil {
		return ErrRuntimeUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sendCtx, cancel := context.WithCancel(ctx)
	stopGenerationCancel := func() bool { return false }
	if generation.ctx != nil {
		stopGenerationCancel = context.AfterFunc(generation.ctx, cancel)
	}
	defer func() {
		stopGenerationCancel()
		cancel()
	}()
	timeout := time.Duration(generation.config.APITimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(DefaultAPITimeoutMS) * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if r.generation.Load() != generation || strings.TrimSpace(generation.config.AppID) != botAppID {
			return ErrRuntimeUnavailable
		}
		attemptCtx, attemptCancel := context.WithTimeout(sendCtx, timeout)
		lastErr = generation.messenger.SendProactiveC2C(attemptCtx, openID, content)
		attemptCancel()
		if lastErr == nil {
			r.markSent()
			return nil
		}
		var definitive interface{ Definitive() bool }
		if errors.As(lastErr, &definitive) && definitive.Definitive() {
			r.recordError("proactive_send_failed")
			return lastErr
		}
		if sendCtx.Err() != nil {
			return sendCtx.Err()
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
			select {
			case <-sendCtx.Done():
				timer.Stop()
				return sendCtx.Err()
			case <-timer.C:
			}
		}
	}
	r.recordError("proactive_send_failed")
	return lastErr
}

func (r *Runtime) Start(ctx context.Context) error {
	if r == nil || r.manager == nil || r.queue == nil {
		return errors.New("qqbot runtime unavailable")
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

func (r *Runtime) Shutdown(ctx context.Context) error {
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
	clearActiveEventSink(r)
	generation := r.generation.Swap(nil)
	workerErr := r.drainAndStop(ctx, generation)
	if cancel != nil {
		cancel()
	}
	r.setStatus(RuntimeDisabled, "")
	return errors.Join(workerErr, managerErr)
}

func (r *Runtime) applyConfig(ctx context.Context, cfg ActiveConfig) error {
	if r == nil {
		return errors.New("qqbot runtime unavailable")
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
	if !cfg.Enabled {
		if r.binding != nil {
			r.binding.SetPublicBaseURL(cfg.PublicBaseURL)
		}
		clearActiveEventSink(r)
		old := r.generation.Swap(nil)
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = r.drainAndStop(stopCtx, old)
		r.stateMu.Lock()
		r.state.ProcessStatus = RuntimeDisabled
		r.state.ActiveConfigVersion = cfg.ConfigVersion
		r.state.WorkerTotal = 0
		r.state.WorkerActive = 0
		r.state.LastErrorCode = ""
		r.state.LastErrorMessage = ""
		r.state.LastErrorAt = nil
		r.stateMu.Unlock()
		return nil
	}
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.AppSecret) == "" || strings.TrimSpace(cfg.WebhookSecret) == "" {
		r.recordError("runtime_credentials_missing")
		return ErrInvalidConfig
	}
	messenger, err := NewBotGoMessenger(cfg.AppID, cfg.AppSecret, cfg.Sandbox, time.Duration(cfg.APITimeoutMS)*time.Millisecond)
	if err != nil {
		r.recordError("messenger_create_failed")
		return err
	}
	messenger.setMediaStore(r.queue)
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.APITimeoutMS)*time.Millisecond)
	_, err = messenger.Probe(probeCtx)
	cancel()
	if err != nil {
		r.recordError("credential_probe_failed")
		return fmt.Errorf("probe qqbot credentials: %w", err)
	}
	if err := r.queue.EnsureGroup(ctx); err != nil {
		r.recordError("queue_group_failed")
		return err
	}
	r.lifecycleMu.Lock()
	root := r.root
	r.lifecycleMu.Unlock()
	if root == nil {
		return errors.New("qqbot runtime not started")
	}
	generationCtx, generationCancel := context.WithCancel(root)
	next := &runtimeGeneration{config: cfg, messenger: messenger, ctx: generationCtx, cancel: generationCancel}
	for i := 0; i < cfg.WorkerCount; i++ {
		next.wg.Add(1)
		go r.worker(next, i)
	}
	old := r.generation.Swap(next)
	if r.binding != nil {
		r.binding.SetPublicBaseURL(cfg.PublicBaseURL)
	}
	setActiveEventSink(r)
	r.stateMu.Lock()
	r.state.ProcessStatus = RuntimeRunning
	r.state.ActiveConfigVersion = cfg.ConfigVersion
	r.state.WorkerTotal = cfg.WorkerCount
	r.state.WorkerActive = cfg.WorkerCount
	r.state.LastErrorCode = ""
	r.state.LastErrorMessage = ""
	r.state.LastErrorAt = nil
	r.stateMu.Unlock()
	if old != nil {
		stopCtx, stopCancel := context.WithTimeout(ctx, 10*time.Second)
		_ = r.drainAndStop(stopCtx, old)
		stopCancel()
	}
	return nil
}

func (r *Runtime) Probe(ctx context.Context) ProbeResult {
	if r == nil || r.manager == nil {
		return failedProbe("runtime_unavailable", "QQBot runtime is unavailable", time.Now())
	}
	cfg, ok := r.manager.Active()
	if !ok {
		return failedProbe("credentials_missing", "QQBot credentials are not configured", time.Now())
	}
	return r.ProbeConfig(ctx, cfg)
}

func (r *Runtime) ProbeConfig(ctx context.Context, cfg ActiveConfig) ProbeResult {
	startedAt := time.Now()
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.AppSecret) == "" || strings.TrimSpace(cfg.WebhookSecret) == "" {
		return failedProbe("credentials_missing", "QQBot credentials are not configured", startedAt)
	}
	messenger, err := NewBotGoMessenger(cfg.AppID, cfg.AppSecret, cfg.Sandbox, time.Duration(cfg.APITimeoutMS)*time.Millisecond)
	if err != nil {
		return failedProbe("messenger_create_failed", "QQBot client initialization failed", startedAt)
	}
	botID, err := messenger.Probe(ctx)
	if err != nil {
		return failedProbe("credential_probe_failed", "QQBot credential probe failed", startedAt)
	}
	return ProbeResult{
		OK:               true,
		Status:           "success",
		Message:          "QQBot credentials are valid",
		LatencyMS:        time.Since(startedAt).Milliseconds(),
		BotIDFingerprint: Fingerprint(botID),
		CheckedAt:        time.Now().UTC(),
	}
}

func failedProbe(code, message string, startedAt time.Time) ProbeResult {
	return ProbeResult{
		Status:    "failed",
		Message:   message,
		ErrorCode: code,
		LatencyMS: time.Since(startedAt).Milliseconds(),
		CheckedAt: time.Now().UTC(),
	}
}

func (r *Runtime) enqueueEvent(event InboundEvent) error {
	generation := r.generation.Load()
	if generation == nil || !generation.config.Enabled {
		return ErrRuntimeDisabled
	}
	if err := r.queue.Enqueue(generation.ctx, event, generation.config.QueueCapacity); err != nil {
		r.recordError("event_enqueue_failed")
		return err
	}
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.LastWebhookAt = &now
	r.stateMu.Unlock()
	return nil
}

func (r *Runtime) worker(generation *runtimeGeneration, workerID int) {
	defer generation.wg.Done()
	consumer := "worker-" + strconv.Itoa(workerID) + "-v" + strconv.FormatInt(generation.config.ConfigVersion, 10)
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

func (r *Runtime) processItems(generation *runtimeGeneration, items []QueuedEvent) {
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
			slog.Warn("qqbot queue payload rejected", "stream_id", shortID(item.ID), "error_code", item.DecodeError)
			continue
		}
		ctx, cancel := context.WithTimeout(generation.ctx, 20*time.Second)
		err := r.process(ctx, generation, item.Event)
		cancel()
		if err == nil {
			if ackErr := r.queue.Ack(generation.ctx, item.ID); ackErr != nil {
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
		if failErr := r.queue.Fail(generation.ctx, item, code); failErr != nil {
			r.recordError("queue_retry_failed")
		} else {
			r.recordError(code)
		}
		slog.Warn("qqbot event processing failed", "event_id", shortID(item.Event.EventID), "scene", item.Event.Scene, "error_code", code)
	}
}

func (r *Runtime) process(ctx context.Context, generation *runtimeGeneration, incoming InboundEvent) error {
	return r.processWith(ctx, generation.config, generation.messenger, r.queue, incoming, r.markSent)
}

func (r *Runtime) processWith(ctx context.Context, cfg ActiveConfig, messenger Messenger, queue *ReliableQueue, incoming InboundEvent, markSent func()) error {
	if r == nil || r.manager == nil || messenger == nil {
		return ErrRuntimeUnavailable
	}
	settings := r.manager.BusinessSettings()
	if !allowed(settings, incoming) {
		return nil
	}
	send := func(content string) error {
		return sendQQBotMessage(ctx, messenger, incoming, content, 1, markSent)
	}
	if incoming.MemberJoined {
		if !settings.WelcomeEnabled {
			return nil
		}
		content := renderWelcome(settings, incoming)
		switch incoming.Scene {
		case SceneGroup:
			if welcomeMessenger, ok := messenger.(GroupWelcomeMessenger); ok {
				if err := welcomeMessenger.SendGroupWelcome(ctx, incoming.SourceID, incoming.ProviderSubject, content); err != nil {
					return err
				}
				if markSent != nil {
					markSent()
				}
				return nil
			}
			return send(content)
		case SceneGuild:
			channelID := strings.TrimSpace(settings.GuildWelcomeChannels[incoming.GuildID])
			if channelID == "" {
				return nil
			}
			incoming.ChannelID = channelID
			return sendQQBotMessage(ctx, messenger, incoming, content, 1, markSent)
		default:
			return nil
		}
	}
	parsed := ParseCommand(incoming.Content)
	if parsed.Kind == CommandNone {
		if (incoming.Scene == SceneGroup || incoming.Scene == SceneC2C) && settings.FirstInteractionEnabled {
			if queue == nil {
				return ErrRuntimeUnavailable
			}
			key := fmt.Sprintf("%s:%s:%s:%s", cfg.AppID, incoming.Scene, incoming.SourceID, incoming.ProviderSubject)
			first, err := queue.BeginWelcome(ctx, key)
			if err != nil || !first {
				return err
			}
			sendErr := send(renderHelp(settings.HelpMessage))
			finishErr := queue.FinishWelcome(ctx, key, sendErr == nil)
			if sendErr != nil {
				return sendErr
			}
			return finishErr
		}
		return nil
	}
	if parsed.Kind == CommandHelp {
		return send(renderHelp(settings.HelpMessage))
	}
	if parsed.Kind == CommandCheck {
		return r.handleChannelCheckWith(ctx, cfg, messenger, incoming, markSent)
	}
	if parsed.Kind != CommandBind {
		return nil
	}
	if !settings.BindingEnabled {
		return send("账户绑定暂未开放。请稍后再试，或联系站点管理员。")
	}
	if !ValidEmail(parsed.Email) {
		return send("邮箱格式不正确。请使用：/bind name@example.com")
	}
	if queue == nil {
		return ErrRuntimeUnavailable
	}
	allowedRequest, retryAfter, err := queue.Allow(ctx, "bind:"+cfg.AppID+":"+string(incoming.Scene)+":"+incoming.ProviderSubject, 3, 5*time.Minute)
	if err != nil {
		return err
	}
	if !allowedRequest {
		minutes := int(retryAfter.Round(time.Minute) / time.Minute)
		if minutes < 1 {
			minutes = 1
		}
		return send(fmt.Sprintf("绑定请求过于频繁。请在约 %d 分钟后重试。", minutes))
	}
	if r.binding == nil {
		return ErrRuntimeUnavailable
	}
	result, err := r.binding.PrepareBinding(ctx, service.QQBotPrepareBindingRequest{EventID: incoming.EventID, MessageID: incoming.MessageID, BotAppID: cfg.AppID, Scene: string(incoming.Scene), ProviderSubject: string(incoming.Scene) + ":" + incoming.ProviderSubject, SourceID: incoming.SourceID, ChannelID: incoming.ChannelID, Email: parsed.Email, DisplayName: incoming.DisplayName})
	if err != nil {
		return send("暂时无法创建绑定请求。请稍后重新发送 /bind。")
	}
	masked := strings.TrimSpace(result.MaskedEmail)
	if masked == "" {
		masked = MaskEmail(parsed.Email)
	}
	if result.AlreadyBound {
		return send("账户已经绑定邮箱 " + masked + "。")
	}
	return send("若该账户存在，验证邮件已发送至 " + masked + "。链接仅在短时间内有效。")
}

func (r *Runtime) handleChannelCheck(ctx context.Context, generation *runtimeGeneration, incoming InboundEvent) error {
	return r.handleChannelCheckWith(ctx, generation.config, generation.messenger, incoming, r.markSent)
}

func (r *Runtime) handleChannelCheckWith(ctx context.Context, cfg ActiveConfig, messenger Messenger, incoming InboundEvent, markSent func()) error {
	if r.channelCheck == nil {
		return sendQQBotMessage(ctx, messenger, incoming, "渠道状态图暂时不可用，请稍后再试。", 1, markSent)
	}
	imageURL, err := r.channelCheck.PrepareImageURL(ctx, cfg, incoming)
	if err != nil {
		message := channelCheckErrorMessage(err)
		if message == "" {
			slog.Warn("qqbot channel check preparation failed", "event_id", shortID(incoming.EventID), "scene", incoming.Scene, "error_code", infraerrors.Reason(err))
			message = "渠道状态图暂时不可用，请稍后再试。"
		}
		return sendQQBotMessage(ctx, messenger, incoming, message, 1, markSent)
	}
	if err := sendQQBotImage(ctx, messenger, incoming, imageURL, markSent); err != nil {
		slog.Warn("qqbot channel check image reply failed", "event_id", shortID(incoming.EventID), "scene", incoming.Scene, "error_code", infraerrors.Reason(err))
		var definitive interface{ Definitive() bool }
		if !errors.As(err, &definitive) || !definitive.Definitive() {
			return err
		}
		fallbackErr := sendQQBotMessage(ctx, messenger, incoming, "渠道状态图发送失败，请稍后重新发送 /check。", 2, markSent)
		if fallbackErr != nil {
			return errors.Join(err, fallbackErr)
		}
	}
	return nil
}

func channelCheckErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrChannelCheckDisabled), errors.Is(err, ErrChannelMonitorDisabled):
		return "渠道状态图暂未开放。"
	case errors.Is(err, ErrChannelCheckBindingRequired):
		return "私聊查看渠道状态前，请先发送 /bind 你的邮箱完成账户绑定。"
	}
	var rateLimitErr *ChannelCheckRateLimitError
	if errors.As(err, &rateLimitErr) {
		seconds := int((rateLimitErr.RetryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		return fmt.Sprintf("请求过于频繁，请在约 %d 秒后重试。", seconds)
	}
	return ""
}

func (r *Runtime) sendImage(ctx context.Context, messenger Messenger, incoming InboundEvent, imageURL string) error {
	return sendQQBotImage(ctx, messenger, incoming, imageURL, r.markSent)
}

func sendQQBotImage(ctx context.Context, messenger Messenger, incoming InboundEvent, imageURL string, markSent func()) error {
	var lastErr error
	const sequence uint32 = 1
	for attempt := 0; attempt < 3; attempt++ {
		switch incoming.Scene {
		case SceneGroup:
			lastErr = messenger.SendGroupImage(ctx, incoming.SourceID, incoming.MessageID, incoming.EventID, imageURL, sequence)
		case SceneC2C:
			lastErr = messenger.SendC2CImage(ctx, incoming.ProviderSubject, incoming.MessageID, incoming.EventID, imageURL, sequence)
		case SceneGuild:
			lastErr = messenger.SendChannelImage(ctx, incoming.ChannelID, incoming.MessageID, incoming.EventID, imageURL, sequence)
		default:
			return errors.New("unsupported qqbot scene")
		}
		if lastErr == nil {
			if markSent != nil {
				markSent()
			}
			return nil
		}
		var definitive interface{ Definitive() bool }
		if errors.As(lastErr, &definitive) && definitive.Definitive() {
			return lastErr
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}

func (r *Runtime) send(ctx context.Context, messenger Messenger, incoming InboundEvent, content string) error {
	return r.sendWithSequence(ctx, messenger, incoming, content, 1)
}

func (r *Runtime) sendWithSequence(ctx context.Context, messenger Messenger, incoming InboundEvent, content string, firstSequence uint32) error {
	return sendQQBotMessage(ctx, messenger, incoming, content, firstSequence, r.markSent)
}

func sendQQBotMessage(ctx context.Context, messenger Messenger, incoming InboundEvent, content string, firstSequence uint32, markSent func()) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		sequence := firstSequence
		switch incoming.Scene {
		case SceneGroup:
			lastErr = messenger.SendGroup(ctx, incoming.SourceID, incoming.MessageID, incoming.EventID, content, sequence)
		case SceneC2C:
			lastErr = messenger.SendC2C(ctx, incoming.ProviderSubject, incoming.MessageID, incoming.EventID, content, sequence)
		case SceneGuild:
			lastErr = messenger.SendChannel(ctx, incoming.ChannelID, incoming.MessageID, incoming.EventID, content, sequence)
		default:
			return errors.New("unsupported qqbot scene")
		}
		if lastErr == nil {
			if markSent != nil {
				markSent()
			}
			return nil
		}
		var definitive interface{ Definitive() bool }
		if errors.As(lastErr, &definitive) && definitive.Definitive() {
			return lastErr
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}

func (r *Runtime) markSent() {
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.LastSendAt = &now
	r.stateMu.Unlock()
}

func (r *Runtime) State(ctx context.Context) RuntimeState {
	state := RuntimeState{ProcessStatus: RuntimeDisabled}
	if r == nil {
		return state
	}
	r.stateMu.RLock()
	state = r.state
	r.stateMu.RUnlock()
	if r.queue != nil {
		state.StreamBacklog, state.StreamPending, state.DeadLetterTotal = r.queue.Stats(ctx)
	}
	return state
}
func (r *Runtime) MarkWebhook() {
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.LastWebhookAt = &now
	r.stateMu.Unlock()
}
func (r *Runtime) recordError(code string) {
	now := time.Now().UTC()
	r.stateMu.Lock()
	r.state.ProcessStatus = RuntimeDegraded
	r.state.LastErrorCode = code
	r.state.LastErrorMessage = code
	r.state.LastErrorAt = &now
	r.stateMu.Unlock()
}
func (r *Runtime) setStatus(status RuntimeStatus, code string) {
	r.stateMu.Lock()
	r.state.ProcessStatus = status
	r.state.LastErrorCode = code
	r.state.LastErrorMessage = code
	if code == "" {
		r.state.LastErrorAt = nil
	}
	r.stateMu.Unlock()
}
func (r *Runtime) drainAndStop(ctx context.Context, generation *runtimeGeneration) error {
	if generation == nil {
		return nil
	}
	if r == nil || r.queue == nil {
		return stopGeneration(ctx, generation)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		backlog, pending, _ := r.queue.Stats(ctx)
		if backlog == 0 && pending == 0 {
			return stopGeneration(ctx, generation)
		}
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return errors.Join(ctx.Err(), stopGeneration(stopCtx, generation))
		case <-ticker.C:
		}
	}
}

func stopGeneration(ctx context.Context, generation *runtimeGeneration) error {
	if generation == nil {
		return nil
	}
	generation.cancel()
	done := make(chan struct{})
	go func() { generation.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func allowed(settings service.QQBotSettings, incoming InboundEvent) bool {
	switch incoming.Scene {
	case SceneC2C:
		return true
	case SceneGroup:
		return contains(settings.AllowedGroupIDs, incoming.SourceID)
	case SceneGuild:
		return contains(settings.AllowedGuildIDs, incoming.GuildID)
	default:
		return false
	}
}
func contains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
func renderHelp(template string) string {
	if strings.TrimSpace(template) == "" {
		return defaultHelpMessage
	}
	return strings.NewReplacer("{bind_command}", "/bind name@example.com", "{site}", qqBotSiteName).Replace(strings.TrimSpace(template))
}

func renderWelcome(settings service.QQBotSettings, incoming InboundEvent) string {
	template := strings.TrimSpace(settings.WelcomeMessage)
	if template == "" {
		template = defaultWelcomeMessage
	}
	result := renderWelcomeTemplate(template, settings, incoming)
	if result == "" && template != defaultWelcomeMessage {
		return renderWelcomeTemplate(defaultWelcomeMessage, settings, incoming)
	}
	return result
}

func renderWelcomeTemplate(template string, settings service.QQBotSettings, incoming InboundEvent) string {
	lines := strings.Split(strings.ReplaceAll(template, "\r\n", "\n"), "\n")
	filtered := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		lower := strings.ToLower(line)
		if !settings.BindingEnabled && (strings.Contains(lower, "{bind_command}") || strings.Contains(lower, "/bind")) {
			continue
		}
		if !settings.ChannelCheckEnabled && strings.Contains(lower, "/check") {
			continue
		}
		blank := strings.TrimSpace(line) == ""
		if blank && (lastBlank || len(filtered) == 0) {
			continue
		}
		filtered = append(filtered, line)
		lastBlank = blank
	}
	for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
		filtered = filtered[:len(filtered)-1]
	}
	return strings.TrimSpace(strings.NewReplacer(
		"{site}", qqBotSiteName,
		"{user}", safeWelcomePlaceholder(incoming.DisplayName, "新成员"),
		"{bind_command}", "/bind name@example.com",
	).Replace(strings.Join(filtered, "\n")))
}

func safeWelcomePlaceholder(value, fallback string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case char == '\r' || char == '\n' || char == '\t':
			builder.WriteByte(' ')
		case unicode.IsControl(char) || unicode.In(char, unicode.Cf):
			continue
		case char == '<':
			builder.WriteRune('‹')
		case char == '>':
			builder.WriteRune('›')
		case char == '/':
			builder.WriteRune('∕')
		default:
			builder.WriteRune(char)
		}
	}
	result := strings.Join(strings.Fields(builder.String()), " ")
	if result == "" {
		result = fallback
	}
	runes := []rune(result)
	if len(runes) > 80 {
		result = string(runes[:80])
	}
	return result
}
