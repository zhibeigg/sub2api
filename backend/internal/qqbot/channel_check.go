package qqbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"golang.org/x/sync/singleflight"
)

const (
	channelCheckRateLimitWindow = 30 * time.Second
	channelCheckRenderCacheTTL  = 15 * time.Second
	channelCheckRenderWorkers   = 2
	channelCheckMaxImageFetches = 4
)

var (
	ErrChannelCheckDisabled        = errors.New("qqbot channel check disabled")
	ErrChannelMonitorDisabled      = errors.New("channel monitor disabled")
	ErrChannelCheckBindingRequired = errors.New("qqbot channel check binding required")
	ErrChannelCheckUnavailable     = errors.New("qqbot channel check unavailable")
	ErrChannelCheckFetchLimit      = errors.New("qqbot channel check image fetch limit exceeded")
)

type ChannelCheckRateLimitError struct {
	RetryAfter time.Duration
}

func (e *ChannelCheckRateLimitError) Error() string {
	return "qqbot channel check rate limited"
}

type channelCheckMonitorReader interface {
	ListUserView(ctx context.Context) ([]*service.UserMonitorView, error)
}

type channelCheckSettingsReader interface {
	GetChannelMonitorRuntimeStrict(ctx context.Context) (service.ChannelMonitorRuntime, error)
}

type channelCheckBindingReader interface {
	HasActiveBoundIdentity(ctx context.Context, botAppID, providerSubject string) (bool, error)
}

type channelCheckLimiter interface {
	Allow(ctx context.Context, scope string, limit int64, window time.Duration) (bool, time.Duration, error)
	AllowOnce(ctx context.Context, scope, token string, limit int64, window time.Duration) (bool, time.Duration, error)
}

type ChannelCheckService struct {
	monitor  channelCheckMonitorReader
	settings channelCheckSettingsReader
	binding  channelCheckBindingReader
	limiter  channelCheckLimiter
	manager  *ConfigManager
	signer   *ChannelCheckSigner
	renderer *ChannelStatusRenderer

	now         func() time.Time
	renderSlots chan struct{}
	cacheMu     sync.RWMutex
	cacheUntil  time.Time
	cachePNG    []byte
	renderGroup singleflight.Group
}

func NewChannelCheckService(
	monitor *service.ChannelMonitorService,
	settings *service.SettingService,
	binding *service.QQBotService,
	queue *ReliableQueue,
	manager *ConfigManager,
	signer *ChannelCheckSigner,
	renderer *ChannelStatusRenderer,
) *ChannelCheckService {
	return &ChannelCheckService{
		monitor:     monitor,
		settings:    settings,
		binding:     binding,
		limiter:     queue,
		manager:     manager,
		signer:      signer,
		renderer:    renderer,
		now:         time.Now,
		renderSlots: make(chan struct{}, channelCheckRenderWorkers),
	}
}

func (s *ChannelCheckService) PrepareImageURL(ctx context.Context, cfg ActiveConfig, incoming InboundEvent) (string, error) {
	if s == nil || s.manager == nil || s.signer == nil || s.limiter == nil {
		return "", ErrChannelCheckUnavailable
	}
	if !s.manager.BusinessSettings().ChannelCheckEnabled {
		return "", ErrChannelCheckDisabled
	}
	if s.settings == nil {
		return "", ErrChannelCheckUnavailable
	}
	monitorRuntime, err := s.settings.GetChannelMonitorRuntimeStrict(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: channel monitor settings", ErrChannelCheckUnavailable)
	}
	if !monitorRuntime.Enabled {
		return "", ErrChannelMonitorDisabled
	}

	identity := strings.TrimSpace(incoming.ProviderSubject)
	if identity == "" || strings.TrimSpace(cfg.AppID) == "" {
		return "", ErrChannelCheckUnavailable
	}
	limitScope := strings.Join([]string{"check", cfg.AppID, string(incoming.Scene), identity}, ":")
	allowedRequest, retryAfter, err := s.limiter.AllowOnce(ctx, limitScope, incoming.EventID, 1, channelCheckRateLimitWindow)
	if err != nil {
		return "", fmt.Errorf("limit qqbot channel check: %w", err)
	}
	if !allowedRequest {
		return "", &ChannelCheckRateLimitError{RetryAfter: retryAfter}
	}

	if incoming.Scene == SceneC2C {
		if s.binding == nil {
			return "", ErrChannelCheckUnavailable
		}
		bound, err := s.binding.HasActiveBoundIdentity(ctx, cfg.AppID, string(SceneC2C)+":"+identity)
		if err != nil {
			return "", fmt.Errorf("resolve qqbot channel check binding: %w", err)
		}
		if !bound {
			return "", ErrChannelCheckBindingRequired
		}
	}
	return s.signer.IssueURL(cfg.PublicBaseURL, cfg.AppID)
}

func (s *ChannelCheckService) RenderSignedPNG(ctx context.Context, version, expires, nonce, signature string) ([]byte, error) {
	if s == nil || s.manager == nil || s.signer == nil || s.renderer == nil || s.monitor == nil || s.limiter == nil {
		return nil, ErrChannelCheckUnavailable
	}
	active, ok := s.manager.Active()
	if !ok || !active.Enabled {
		return nil, ErrChannelCheckUnavailable
	}
	if err := s.signer.Verify(active.AppID, version, expires, nonce, signature); err != nil {
		return nil, ErrInvalidChannelCheckSignature
	}
	if !s.manager.BusinessSettings().ChannelCheckEnabled {
		return nil, ErrChannelCheckDisabled
	}
	if s.settings == nil {
		return nil, ErrChannelCheckUnavailable
	}
	monitorRuntime, err := s.settings.GetChannelMonitorRuntimeStrict(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: channel monitor settings", ErrChannelCheckUnavailable)
	}
	if !monitorRuntime.Enabled {
		return nil, ErrChannelMonitorDisabled
	}
	expiresAtUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return nil, ErrInvalidChannelCheckSignature
	}
	fetchWindow := time.Unix(expiresAtUnix, 0).Sub(s.currentTime())
	if fetchWindow <= 0 {
		return nil, ErrInvalidChannelCheckSignature
	}
	allowedFetch, _, err := s.limiter.Allow(ctx, "check-image:"+nonce, channelCheckMaxImageFetches, fetchWindow)
	if err != nil {
		return nil, fmt.Errorf("%w: image fetch limiter", ErrChannelCheckUnavailable)
	}
	if !allowedFetch {
		return nil, ErrChannelCheckFetchLimit
	}
	if cached := s.cachedPNG(); cached != nil {
		return cached, nil
	}

	resultChannel := s.renderGroup.DoChan("channel-status", func() (any, error) {
		return s.renderPNG(ctx)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			return nil, result.Err
		}
		imageBytes, ok := result.Val.([]byte)
		if !ok {
			return nil, ErrChannelCheckUnavailable
		}
		return append([]byte(nil), imageBytes...), nil
	}
}

func (s *ChannelCheckService) renderPNG(ctx context.Context) ([]byte, error) {
	if cached := s.cachedPNG(); cached != nil {
		return cached, nil
	}
	select {
	case s.renderSlots <- struct{}{}:
		defer func() { <-s.renderSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if cached := s.cachedPNG(); cached != nil {
		return cached, nil
	}
	views, err := s.monitor.ListUserView(ctx)
	if err != nil {
		return nil, fmt.Errorf("load qqbot channel status: %w", err)
	}
	generatedAt := s.currentTime()
	imageBytes, err := s.renderer.Render(ctx, views, generatedAt)
	if err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	s.cachePNG = append(s.cachePNG[:0], imageBytes...)
	s.cacheUntil = generatedAt.Add(channelCheckRenderCacheTTL)
	s.cacheMu.Unlock()
	return append([]byte(nil), imageBytes...), nil
}

func (s *ChannelCheckService) cachedPNG() []byte {
	now := s.currentTime()
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if len(s.cachePNG) == 0 || !s.cacheUntil.After(now) {
		return nil
	}
	return append([]byte(nil), s.cachePNG...)
}

func (s *ChannelCheckService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
