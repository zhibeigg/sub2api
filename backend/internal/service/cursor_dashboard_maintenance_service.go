package service

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type CursorDashboardMaintenanceService struct {
	accountRepo AccountRepository
	auth        *CursorDashboardAuthService
	cfg         *config.Config
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

func NewCursorDashboardMaintenanceService(accountRepo AccountRepository, auth *CursorDashboardAuthService, cfg *config.Config) *CursorDashboardMaintenanceService {
	return &CursorDashboardMaintenanceService{accountRepo: accountRepo, auth: auth, cfg: cfg, stopCh: make(chan struct{})}
}

func (s *CursorDashboardMaintenanceService) Start() {
	if s == nil || s.auth == nil || s.accountRepo == nil || s.cfg == nil || !s.cfg.Cursor.DashboardMaintenanceEnabled {
		return
	}
	s.wg.Add(1)
	go s.loop()
}

func (s *CursorDashboardMaintenanceService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *CursorDashboardMaintenanceService) loop() {
	defer s.wg.Done()
	interval := time.Duration(s.cfg.Cursor.DashboardMaintenanceIntervalMins) * time.Minute
	if interval < time.Minute {
		interval = 30 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.runCycle()
	for {
		select {
		case <-ticker.C:
			s.runCycle()
		case <-s.stopCh:
			return
		}
	}
}

func (s *CursorDashboardMaintenanceService) runCycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformCursor)
	if err != nil {
		slog.Warn("cursor_dashboard_maintenance_list_failed", "error", err)
		return
	}
	probeInterval := time.Duration(s.cfg.Cursor.DashboardProbeIntervalMins) * time.Minute
	if probeInterval < time.Minute {
		probeInterval = 6 * time.Hour
	}
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i := range accounts {
		account := accounts[i]
		if !account.IsCursorAPIKey() || account.GetCredential("dashboard_access_token") == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			jitter := time.Duration(rand.IntN(1000)) * time.Millisecond
			select {
			case <-time.After(jitter):
			case <-ctx.Done():
				return
			}
			current := &account
			if refreshed, _, refreshErr := s.auth.RefreshIfNeeded(ctx, current); refreshErr != nil {
				s.auth.markError(ctx, current.ID, classifyCursorDashboardMaintenanceError(refreshErr))
				return
			} else if refreshed != nil {
				current = refreshed
			}
			info := cursorDashboardSessionInfoFromAccount(current)
			if info.LastVerifiedAt != nil && time.Since(*info.LastVerifiedAt) < probeInterval {
				return
			}
			result, fetchErr := s.auth.FetchDashboardUsage(ctx, current)
			if fetchErr != nil || result == nil || result.Usage == nil {
				return
			}
			updates := cursorPlanUsageSnapshotUpdates(cursorPlanUsageFromDashboard(result.Usage, time.Now().UTC()))
			if len(updates) > 0 {
				_ = s.accountRepo.UpdateExtra(ctx, current.ID, updates)
			}
		}()
	}
	wg.Wait()
}

func classifyCursorDashboardMaintenanceError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errCursorDashboardReauthRequired) {
		return "reauth_required"
	}
	return "refresh_error"
}
