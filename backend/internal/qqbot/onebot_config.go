package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	SettingKeyOneBotRuntimeConfig         = "qqbot_onebot_runtime_config"
	OneBotConfigInvalidationChannel       = "sub2api:qqbot:onebot:config:invalidate"
	DefaultOneBotWorkerCount              = 2
	DefaultOneBotQueueCapacity            = 1024
	DefaultOneBotActionTimeoutMS          = 10000
	oneBotConfigLockKey             int64 = 0x514F4E45424F54
	oneBotProbeTTL                        = 5 * time.Minute
)

type oneBotStorageConfig struct {
	Enabled               bool      `json:"enabled"`
	SelfID                string    `json:"self_id"`
	AccessTokenCiphertext string    `json:"access_token_ciphertext,omitempty"`
	WorkerCount           int       `json:"worker_count"`
	QueueCapacity         int       `json:"queue_capacity"`
	ActionTimeoutMS       int       `json:"action_timeout_ms"`
	ConfigVersion         int64     `json:"config_version"`
	UpdatedAt             time.Time `json:"updated_at"`
	UpdatedBy             int64     `json:"updated_by"`
	ChangeSummary         string    `json:"change_summary"`
}

type OneBotActiveConfig struct {
	Enabled         bool
	SelfID          string
	AccessToken     string
	WorkerCount     int
	QueueCapacity   int
	ActionTimeoutMS int
	ConfigVersion   int64
	UpdatedAt       time.Time
	UpdatedBy       int64
}

type OneBotPublicConfig struct {
	Enabled               bool      `json:"enabled"`
	SelfID                string    `json:"self_id"`
	AccessTokenConfigured bool      `json:"access_token_configured"`
	WorkerCount           int       `json:"worker_count"`
	QueueCapacity         int       `json:"queue_capacity"`
	ActionTimeoutMS       int       `json:"action_timeout_ms"`
	ReverseWSURL          string    `json:"reverse_ws_url"`
	ConfigVersion         int64     `json:"config_version"`
	UpdatedAt             time.Time `json:"updated_at"`
	UpdatedBy             int64     `json:"updated_by"`
	ChangeSummary         string    `json:"change_summary"`
}

type OneBotUpdateConfigRequest struct {
	ExpectedConfigVersion int64  `json:"expected_config_version" binding:"required"`
	Enabled               bool   `json:"enabled"`
	SelfID                string `json:"self_id"`
	AccessToken           string `json:"access_token,omitempty"`
	WorkerCount           int    `json:"worker_count"`
	QueueCapacity         int    `json:"queue_capacity"`
	ActionTimeoutMS       int    `json:"action_timeout_ms"`
}

type OneBotProbeRequest struct {
	SelfID          string `json:"self_id"`
	AccessToken     string `json:"access_token,omitempty"`
	ActionTimeoutMS int    `json:"action_timeout_ms"`
}

type oneBotConfigSnapshot struct {
	storage  oneBotStorageConfig
	active   OneBotActiveConfig
	loadedAt time.Time
}

type OneBotConfigManager struct {
	db        *sql.DB
	settings  service.SettingRepository
	redis     *redis.Client
	encryptor service.SecretEncryptor

	snapshot atomic.Pointer[oneBotConfigSnapshot]
	expected atomic.Int64

	stateMu       sync.RWMutex
	lastLoadError string
	lastErrorAt   *time.Time
	onReload      func(context.Context, OneBotActiveConfig) error

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewOneBotConfigManager(db *sql.DB, settings service.SettingRepository, redisClient *redis.Client, encryptor service.SecretEncryptor) *OneBotConfigManager {
	return &OneBotConfigManager{db: db, settings: settings, redis: redisClient, encryptor: encryptor}
}

func defaultOneBotStorageConfig() oneBotStorageConfig {
	return oneBotStorageConfig{
		WorkerCount:     DefaultOneBotWorkerCount,
		QueueCapacity:   DefaultOneBotQueueCapacity,
		ActionTimeoutMS: DefaultOneBotActionTimeoutMS,
		ConfigVersion:   1,
		ChangeSummary:   `{"bootstrap":true,"enabled":false}`,
	}
}

func parseOneBotStorageConfig(raw string) (oneBotStorageConfig, error) {
	cfg := defaultOneBotStorageConfig()
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return oneBotStorageConfig{}, fmt.Errorf("decode onebot runtime config: %w", err)
		}
	}
	normalizeOneBotStorageConfig(&cfg)
	if err := validateOneBotStorageConfig(cfg, cfg.Enabled); err != nil {
		return oneBotStorageConfig{}, err
	}
	return cfg, nil
}

func normalizeOneBotStorageConfig(cfg *oneBotStorageConfig) {
	if cfg == nil {
		return
	}
	cfg.SelfID = strings.TrimSpace(cfg.SelfID)
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = DefaultOneBotWorkerCount
	}
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = DefaultOneBotQueueCapacity
	}
	if cfg.ActionTimeoutMS == 0 {
		cfg.ActionTimeoutMS = DefaultOneBotActionTimeoutMS
	}
	if cfg.ConfigVersion < 1 {
		cfg.ConfigVersion = 1
	}
}

func validOneBotSelfID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 5 || len(value) > 20 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value[0] != '0'
}

func validateOneBotStorageConfig(cfg oneBotStorageConfig, requireCredentials bool) error {
	if cfg.WorkerCount < 1 || cfg.WorkerCount > 64 || cfg.QueueCapacity < 16 || cfg.QueueCapacity > 100000 || cfg.ActionTimeoutMS < 500 || cfg.ActionTimeoutMS > 30000 {
		return ErrInvalidConfig
	}
	if cfg.SelfID != "" && !validOneBotSelfID(cfg.SelfID) {
		return ErrInvalidConfig
	}
	if requireCredentials && (cfg.SelfID == "" || cfg.AccessTokenCiphertext == "") {
		return ErrInvalidConfig
	}
	return nil
}

func (m *OneBotConfigManager) SetOnReload(callback func(context.Context, OneBotActiveConfig) error) {
	if m == nil {
		return
	}
	m.stateMu.Lock()
	m.onReload = callback
	m.stateMu.Unlock()
}

func (m *OneBotConfigManager) Start(ctx context.Context) error {
	if m == nil || m.settings == nil {
		return errors.New("onebot config manager unavailable")
	}
	m.lifecycleMu.Lock()
	if m.cancel != nil {
		m.lifecycleMu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.lifecycleMu.Unlock()
	loadErr := m.Reload(runCtx)
	m.wg.Add(1)
	go m.refreshLoop(runCtx)
	if m.redis != nil {
		m.wg.Add(1)
		go m.subscribeLoop(runCtx)
	}
	return loadErr
}

func (m *OneBotConfigManager) Shutdown(context.Context) error {
	if m == nil {
		return nil
	}
	m.lifecycleMu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
	return nil
}

func (m *OneBotConfigManager) Reload(ctx context.Context) error {
	if m == nil || m.settings == nil {
		return errors.New("onebot setting repository unavailable")
	}
	setting, err := m.settings.Get(ctx, SettingKeyOneBotRuntimeConfig)
	if errors.Is(err, service.ErrSettingNotFound) {
		cfg := defaultOneBotStorageConfig()
		encoded, _ := json.Marshal(cfg)
		if err := m.settings.Set(ctx, SettingKeyOneBotRuntimeConfig, string(encoded)); err != nil {
			m.recordLoadError("onebot_config_bootstrap_failed")
			return err
		}
		setting = &service.Setting{Key: SettingKeyOneBotRuntimeConfig, Value: string(encoded)}
	} else if err != nil {
		m.recordLoadError("onebot_config_load_failed")
		return err
	}
	raw := ""
	if setting != nil {
		raw = setting.Value
	}
	if strings.TrimSpace(raw) == "" {
		cfg := defaultOneBotStorageConfig()
		encoded, _ := json.Marshal(cfg)
		if err := m.settings.Set(ctx, SettingKeyOneBotRuntimeConfig, string(encoded)); err != nil {
			m.recordLoadError("onebot_config_bootstrap_failed")
			return err
		}
		raw = string(encoded)
	}
	storage, err := parseOneBotStorageConfig(raw)
	if err != nil {
		m.recordLoadError("onebot_config_decode_failed")
		return err
	}
	active, err := m.activeFromStorage(storage)
	if err != nil {
		m.recordLoadError("onebot_config_decrypt_failed")
		return err
	}
	previous := m.snapshot.Load()
	now := time.Now().UTC()
	m.expected.Store(storage.ConfigVersion)
	m.snapshot.Store(&oneBotConfigSnapshot{storage: storage, active: active, loadedAt: now})
	m.clearLoadError()
	if previous != nil && sameOneBotActiveConfig(previous.active, active) {
		return nil
	}
	m.stateMu.RLock()
	callback := m.onReload
	m.stateMu.RUnlock()
	if callback != nil {
		if err := callback(ctx, active); err != nil {
			m.recordLoadError("onebot_runtime_reload_failed")
			return err
		}
	}
	return nil
}

func (m *OneBotConfigManager) Active() (OneBotActiveConfig, bool) {
	if m == nil {
		return OneBotActiveConfig{}, false
	}
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		return OneBotActiveConfig{}, false
	}
	return snapshot.active, true
}

func (m *OneBotConfigManager) Public() OneBotPublicConfig {
	if m == nil || m.snapshot.Load() == nil {
		return publicOneBotConfig(defaultOneBotStorageConfig())
	}
	return publicOneBotConfig(m.snapshot.Load().storage)
}

func publicOneBotConfig(cfg oneBotStorageConfig) OneBotPublicConfig {
	return OneBotPublicConfig{
		Enabled:               cfg.Enabled,
		SelfID:                cfg.SelfID,
		AccessTokenConfigured: cfg.AccessTokenCiphertext != "",
		WorkerCount:           cfg.WorkerCount,
		QueueCapacity:         cfg.QueueCapacity,
		ActionTimeoutMS:       cfg.ActionTimeoutMS,
		ReverseWSURL:          "ws://127.0.0.1:8080/webhooks/qq/onebot",
		ConfigVersion:         cfg.ConfigVersion,
		UpdatedAt:             cfg.UpdatedAt,
		UpdatedBy:             cfg.UpdatedBy,
		ChangeSummary:         cfg.ChangeSummary,
	}
}

func (m *OneBotConfigManager) ResolveProbeConfig(req OneBotProbeRequest) (OneBotActiveConfig, error) {
	if m == nil {
		return OneBotActiveConfig{}, ErrRuntimeUnavailable
	}
	if len(req.AccessToken) > 4096 {
		return OneBotActiveConfig{}, ErrInvalidConfig
	}
	candidate, _ := m.Active()
	candidate.SelfID = strings.TrimSpace(req.SelfID)
	if value := strings.TrimSpace(req.AccessToken); value != "" {
		candidate.AccessToken = value
	}
	if req.ActionTimeoutMS != 0 {
		candidate.ActionTimeoutMS = req.ActionTimeoutMS
	}
	if candidate.ActionTimeoutMS == 0 {
		candidate.ActionTimeoutMS = DefaultOneBotActionTimeoutMS
	}
	validation := oneBotStorageConfig{
		SelfID:                candidate.SelfID,
		AccessTokenCiphertext: "configured",
		WorkerCount:           1,
		QueueCapacity:         16,
		ActionTimeoutMS:       candidate.ActionTimeoutMS,
		ConfigVersion:         1,
	}
	if candidate.AccessToken == "" || validateOneBotStorageConfig(validation, true) != nil {
		return OneBotActiveConfig{}, ErrInvalidConfig
	}
	return candidate, nil
}

func (m *OneBotConfigManager) RecordSuccessfulProbe(ctx context.Context, cfg OneBotActiveConfig) error {
	if m == nil || m.redis == nil {
		return ErrRuntimeUnavailable
	}
	return m.redis.Set(ctx, m.probeKey(cfg), "1", oneBotProbeTTL).Err()
}

func (m *OneBotConfigManager) requireSuccessfulProbe(ctx context.Context, cfg OneBotActiveConfig) error {
	if m == nil || m.redis == nil {
		return ErrRuntimeUnavailable
	}
	exists, err := m.redis.Exists(ctx, m.probeKey(cfg)).Result()
	if err != nil {
		return err
	}
	if exists != 1 {
		return ErrProbeRequired
	}
	return nil
}

func (m *OneBotConfigManager) probeKey(cfg OneBotActiveConfig) string {
	return "sub2api:qqbot:onebot:probe:" + Fingerprint(strings.Join([]string{cfg.SelfID, cfg.AccessToken, strconv.Itoa(cfg.ActionTimeoutMS)}, "\x00"))
}

func (m *OneBotConfigManager) Save(ctx context.Context, req OneBotUpdateConfigRequest, actorID int64) (OneBotPublicConfig, error) {
	if m == nil || m.db == nil || m.encryptor == nil || req.ExpectedConfigVersion < 1 || actorID <= 0 || len(req.AccessToken) > 4096 {
		return OneBotPublicConfig{}, ErrInvalidConfig
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return OneBotPublicConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, oneBotConfigLockKey); err != nil {
		return OneBotPublicConfig{}, err
	}
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, SettingKeyOneBotRuntimeConfig).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		cfg := defaultOneBotStorageConfig()
		encoded, _ := json.Marshal(cfg)
		raw = string(encoded)
	} else if err != nil {
		return OneBotPublicConfig{}, err
	}
	current, err := parseOneBotStorageConfig(raw)
	if err != nil {
		return OneBotPublicConfig{}, err
	}
	if current.ConfigVersion != req.ExpectedConfigVersion {
		return OneBotPublicConfig{}, ErrConfigConflict
	}
	next := current
	next.Enabled = req.Enabled
	next.SelfID = strings.TrimSpace(req.SelfID)
	next.WorkerCount = req.WorkerCount
	next.QueueCapacity = req.QueueCapacity
	next.ActionTimeoutMS = req.ActionTimeoutMS
	changedSecrets := false
	if token := strings.TrimSpace(req.AccessToken); token != "" {
		if len([]byte(token)) < 32 {
			return OneBotPublicConfig{}, ErrInvalidConfig
		}
		next.AccessTokenCiphertext, err = m.encryptor.Encrypt(token)
		if err != nil {
			return OneBotPublicConfig{}, err
		}
		changedSecrets = true
	}
	normalizeOneBotStorageConfig(&next)
	if err := validateOneBotStorageConfig(next, next.Enabled); err != nil {
		return OneBotPublicConfig{}, err
	}
	credentialsChanged := (!current.Enabled && next.Enabled) || current.SelfID != next.SelfID || changedSecrets
	if credentialsChanged && next.Enabled {
		candidate, candidateErr := m.activeFromStorage(next)
		if candidateErr != nil {
			return OneBotPublicConfig{}, candidateErr
		}
		if err := m.requireSuccessfulProbe(ctx, candidate); err != nil {
			return OneBotPublicConfig{}, err
		}
	}
	next.ConfigVersion = current.ConfigVersion + 1
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorID
	next.ChangeSummary = oneBotConfigChangeSummary(next, changedSecrets)
	encoded, err := json.Marshal(next)
	if err != nil {
		return OneBotPublicConfig{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key,value,updated_at) VALUES ($1,$2,NOW()) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=EXCLUDED.updated_at`, SettingKeyOneBotRuntimeConfig, string(encoded)); err != nil {
		return OneBotPublicConfig{}, err
	}
	metadata, _ := json.Marshal(map[string]any{"config_version": next.ConfigVersion, "summary": json.RawMessage(next.ChangeSummary)})
	if _, err := tx.ExecContext(ctx, `INSERT INTO qqbot_binding_audit_logs (action,status,actor_type,actor_subject,reason,metadata) VALUES ('onebot_settings','success','admin',$1,'', $2::jsonb)`, Fingerprint(strconv.FormatInt(actorID, 10)), string(metadata)); err != nil {
		return OneBotPublicConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return OneBotPublicConfig{}, err
	}
	active, err := m.activeFromStorage(next)
	if err != nil {
		return OneBotPublicConfig{}, err
	}
	m.expected.Store(next.ConfigVersion)
	m.snapshot.Store(&oneBotConfigSnapshot{storage: next, active: active, loadedAt: time.Now().UTC()})
	m.clearLoadError()
	m.stateMu.RLock()
	callback := m.onReload
	m.stateMu.RUnlock()
	if callback != nil {
		if err := callback(ctx, active); err != nil {
			m.recordLoadError("onebot_runtime_reload_failed")
		}
	}
	if m.redis != nil {
		_ = m.redis.Publish(ctx, OneBotConfigInvalidationChannel, strconv.FormatInt(next.ConfigVersion, 10)).Err()
	}
	return publicOneBotConfig(next), nil
}

func oneBotConfigChangeSummary(cfg oneBotStorageConfig, changedSecret bool) string {
	payload := map[string]any{
		"enabled":           cfg.Enabled,
		"self_id_hash":      Fingerprint(cfg.SelfID),
		"worker_count":      cfg.WorkerCount,
		"queue_capacity":    cfg.QueueCapacity,
		"action_timeout_ms": cfg.ActionTimeoutMS,
		"token_changed":     changedSecret,
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func (m *OneBotConfigManager) activeFromStorage(storage oneBotStorageConfig) (OneBotActiveConfig, error) {
	active := OneBotActiveConfig{
		Enabled:         storage.Enabled,
		SelfID:          storage.SelfID,
		WorkerCount:     storage.WorkerCount,
		QueueCapacity:   storage.QueueCapacity,
		ActionTimeoutMS: storage.ActionTimeoutMS,
		ConfigVersion:   storage.ConfigVersion,
		UpdatedAt:       storage.UpdatedAt,
		UpdatedBy:       storage.UpdatedBy,
	}
	if storage.AccessTokenCiphertext != "" {
		value, err := m.encryptor.Decrypt(storage.AccessTokenCiphertext)
		if err != nil {
			return OneBotActiveConfig{}, fmt.Errorf("decrypt onebot access token: %w", err)
		}
		active.AccessToken = value
	}
	return active, nil
}

func sameOneBotActiveConfig(left, right OneBotActiveConfig) bool {
	return left.Enabled == right.Enabled &&
		left.SelfID == right.SelfID &&
		sameSecret(left.AccessToken, right.AccessToken) &&
		left.WorkerCount == right.WorkerCount &&
		left.QueueCapacity == right.QueueCapacity &&
		left.ActionTimeoutMS == right.ActionTimeoutMS &&
		left.ConfigVersion == right.ConfigVersion
}

func (m *OneBotConfigManager) RuntimeState() (expected, active int64, loadedAt *time.Time, loadError string, errorAt *time.Time) {
	if m == nil {
		return 1, 0, nil, "onebot_config_manager_unavailable", nil
	}
	expected = m.expected.Load()
	if expected < 1 {
		expected = 1
	}
	if snapshot := m.snapshot.Load(); snapshot != nil {
		active = snapshot.active.ConfigVersion
		value := snapshot.loadedAt
		loadedAt = &value
	}
	m.stateMu.RLock()
	loadError = m.lastLoadError
	if m.lastErrorAt != nil {
		value := *m.lastErrorAt
		errorAt = &value
	}
	m.stateMu.RUnlock()
	return
}

func (m *OneBotConfigManager) refreshLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = m.Reload(ctx)
		}
	}
}

func (m *OneBotConfigManager) subscribeLoop(ctx context.Context) {
	defer m.wg.Done()
	pubsub := m.redis.Subscribe(ctx, OneBotConfigInvalidationChannel)
	defer func() { _ = pubsub.Close() }()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-pubsub.Channel():
			if !ok {
				return
			}
			version, err := strconv.ParseInt(strings.TrimSpace(message.Payload), 10, 64)
			if err == nil && version > 0 {
				m.expected.Store(version)
				_ = m.Reload(ctx)
			}
		}
	}
}

func (m *OneBotConfigManager) recordLoadError(code string) {
	now := time.Now().UTC()
	m.stateMu.Lock()
	m.lastLoadError = code
	m.lastErrorAt = &now
	m.stateMu.Unlock()
}

func (m *OneBotConfigManager) clearLoadError() {
	m.stateMu.Lock()
	m.lastLoadError = ""
	m.lastErrorAt = nil
	m.stateMu.Unlock()
}
