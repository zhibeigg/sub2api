package qqbot

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

type channelCheckMonitorStub struct {
	mu    sync.Mutex
	views []*service.UserMonitorView
	calls int
	err   error
}

func (s *channelCheckMonitorStub) ListUserView(context.Context) ([]*service.UserMonitorView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.views, s.err
}

type channelCheckSettingsStub struct {
	runtime service.ChannelMonitorRuntime
	err     error
}

func (s channelCheckSettingsStub) GetChannelMonitorRuntimeStrict(context.Context) (service.ChannelMonitorRuntime, error) {
	return s.runtime, s.err
}

type channelCheckBindingStub struct {
	found bool
	err   error
	calls int
	appID string
	subj  string
}

func (s *channelCheckBindingStub) HasActiveBoundIdentity(_ context.Context, appID, subject string) (bool, error) {
	s.calls++
	s.appID = appID
	s.subj = subject
	return s.found, s.err
}

type channelCheckLimiterStub struct {
	allowed    bool
	retryAfter time.Duration
	err        error
	calls      int
	scope      string
	token      string
	fetchCalls int
	fetchLimit int
}

func (s *channelCheckLimiterStub) Allow(_ context.Context, _ string, _ int64, _ time.Duration) (bool, time.Duration, error) {
	s.fetchCalls++
	if s.err != nil {
		return false, 0, s.err
	}
	if s.fetchLimit > 0 {
		return s.fetchCalls <= s.fetchLimit, s.retryAfter, nil
	}
	return s.allowed, s.retryAfter, nil
}

func (s *channelCheckLimiterStub) AllowOnce(_ context.Context, scope, token string, _ int64, _ time.Duration) (bool, time.Duration, error) {
	s.calls++
	s.scope = scope
	s.token = token
	return s.allowed, s.retryAfter, s.err
}

func TestChannelCheckPrepareImageURLPermissions(t *testing.T) {
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	manager := channelCheckManager(true)
	signer := &ChannelCheckSigner{rootKey: bytes.Repeat([]byte{0x42}, 32), now: func() time.Time { return now }}
	settings := channelCheckSettingsStub{runtime: service.ChannelMonitorRuntime{Enabled: true}}

	t.Run("whitelisted group does not require a personal binding", func(t *testing.T) {
		limiter := &channelCheckLimiterStub{allowed: true}
		binding := &channelCheckBindingStub{}
		svc := &ChannelCheckService{settings: settings, binding: binding, limiter: limiter, manager: manager, signer: signer}
		imageURL, err := svc.PrepareImageURL(t.Context(), ActiveConfig{AppID: "app", AppSecret: "app-secret", PublicBaseURL: "https://status.example.com"}, InboundEvent{EventID: "event-group", Scene: SceneGroup, SourceID: "group", ProviderSubject: "openid"})
		if err != nil {
			t.Fatal(err)
		}
		if imageURL == "" || binding.calls != 0 {
			t.Fatalf("url=%q binding_calls=%d limiter=%#v", imageURL, binding.calls, limiter)
		}
	})

	t.Run("c2c requires a live active identity", func(t *testing.T) {
		limiter := &channelCheckLimiterStub{allowed: true}
		binding := &channelCheckBindingStub{found: false}
		svc := &ChannelCheckService{settings: settings, binding: binding, limiter: limiter, manager: manager, signer: signer}
		_, err := svc.PrepareImageURL(t.Context(), ActiveConfig{AppID: "app", AppSecret: "app-secret", PublicBaseURL: "https://status.example.com"}, InboundEvent{EventID: "event-c2c", Scene: SceneC2C, ProviderSubject: "openid"})
		if !errors.Is(err, ErrChannelCheckBindingRequired) {
			t.Fatalf("unexpected c2c error: %v", err)
		}
		if binding.appID != "app" || binding.subj != "c2c:openid" {
			t.Fatalf("binding lookup app=%q subject=%q", binding.appID, binding.subj)
		}
	})

}

func TestChannelCheckPrepareImageURLHonorsBothSwitches(t *testing.T) {
	limiter := &channelCheckLimiterStub{allowed: true}
	signer := &ChannelCheckSigner{rootKey: bytes.Repeat([]byte{0x33}, 32)}
	input := InboundEvent{EventID: "event", Scene: SceneGroup, ProviderSubject: "openid"}
	cfg := ActiveConfig{AppID: "app", AppSecret: "app-secret", PublicBaseURL: "https://status.example.com"}

	svc := &ChannelCheckService{settings: channelCheckSettingsStub{runtime: service.ChannelMonitorRuntime{Enabled: true}}, limiter: limiter, manager: channelCheckManager(false), signer: signer}
	if _, err := svc.PrepareImageURL(t.Context(), cfg, input); !errors.Is(err, ErrChannelCheckDisabled) {
		t.Fatalf("qqbot switch error=%v", err)
	}
	if limiter.calls != 0 {
		t.Fatal("disabled command consumed rate limit")
	}

	svc = &ChannelCheckService{settings: channelCheckSettingsStub{runtime: service.ChannelMonitorRuntime{Enabled: false}}, limiter: limiter, manager: channelCheckManager(true), signer: signer}
	if _, err := svc.PrepareImageURL(t.Context(), cfg, input); !errors.Is(err, ErrChannelMonitorDisabled) {
		t.Fatalf("monitor switch error=%v", err)
	}

	svc = &ChannelCheckService{settings: channelCheckSettingsStub{err: errors.New("settings unavailable")}, limiter: limiter, manager: channelCheckManager(true), signer: signer}
	if _, err := svc.PrepareImageURL(t.Context(), cfg, input); !errors.Is(err, ErrChannelCheckUnavailable) {
		t.Fatalf("settings failure did not fail closed: %v", err)
	}
}

func TestChannelCheckRenderSignedPNGCachesSafeUserView(t *testing.T) {
	parsedFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	signer := &ChannelCheckSigner{rootKey: bytes.Repeat([]byte{0x55}, 32), now: func() time.Time { return now }}
	monitor := &channelCheckMonitorStub{views: []*service.UserMonitorView{{Name: "Public monitor", Provider: "openai", PrimaryStatus: "operational", Availability7d: 100}}}
	limiter := &channelCheckLimiterStub{allowed: true, fetchLimit: channelCheckMaxImageFetches}
	svc := &ChannelCheckService{
		monitor:     monitor,
		settings:    channelCheckSettingsStub{runtime: service.ChannelMonitorRuntime{Enabled: true}},
		limiter:     limiter,
		manager:     channelCheckManager(true),
		signer:      signer,
		renderer:    newChannelStatusRendererWithFonts(parsedFont, parsedFont),
		now:         func() time.Time { return now },
		renderSlots: make(chan struct{}, 1),
	}
	issued, err := signer.IssueURL("https://status.example.com", "app")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(issued)
	query := parsed.Query()

	first, err := svc.RenderSignedPNG(t.Context(), query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.RenderSignedPNG(t.Context(), query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("cached PNG changed")
	}
	if _, err := png.Decode(bytes.NewReader(first)); err != nil {
		t.Fatalf("generated PNG is invalid: %v", err)
	}
	monitor.mu.Lock()
	calls := monitor.calls
	monitor.mu.Unlock()
	if calls != 1 {
		t.Fatalf("monitor data loaded %d times", calls)
	}
	for fetch := 0; fetch < channelCheckMaxImageFetches-2; fetch++ {
		if _, err := svc.RenderSignedPNG(t.Context(), query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig")); err != nil {
			t.Fatalf("allowed image refetch %d failed: %v", fetch+3, err)
		}
	}
	if _, err := svc.RenderSignedPNG(t.Context(), query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig")); !errors.Is(err, ErrChannelCheckFetchLimit) {
		t.Fatalf("excessive image replay was accepted: %v", err)
	}

	if _, err := svc.RenderSignedPNG(t.Context(), query.Get("v"), query.Get("exp"), query.Get("nonce"), "tampered"); !errors.Is(err, ErrInvalidChannelCheckSignature) {
		t.Fatalf("tampered signature accepted: %v", err)
	}
}

func channelCheckManager(enabled bool) *ConfigManager {
	settings := defaultBusinessSettings()
	settings.ChannelCheckEnabled = enabled
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{active: ActiveConfig{Enabled: true, AppID: "app", AppSecret: "app-secret", PublicBaseURL: "https://status.example.com"}, settings: settings})
	return manager
}
