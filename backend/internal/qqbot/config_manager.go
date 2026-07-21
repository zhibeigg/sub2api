package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	qqBotConfigLockKey int64 = 0x5151424F54
	qqBotProbeTTL            = 5 * time.Minute
)

type configSnapshot struct {
	storage  storageConfig
	active   ActiveConfig
	settings service.QQBotSettings
	loadedAt time.Time
}

type ConfigManager struct {
	db                    *sql.DB
	settings              service.SettingRepository
	redis                 *redis.Client
	encryptor             service.SecretEncryptor
	bootstrapPublicURL    string
	stableChannelCheckKey bool

	snapshot atomic.Pointer[configSnapshot]
	expected atomic.Int64

	stateMu       sync.RWMutex
	lastLoadError string
	lastErrorAt   *time.Time
	onReload      func(context.Context, ActiveConfig) error

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewConfigManager(db *sql.DB, settings service.SettingRepository, redisClient *redis.Client, encryptor service.SecretEncryptor, cfg *config.Config) *ConfigManager {
	publicURL := ""
	stableChannelCheckKey := false
	if cfg != nil {
		publicURL = cfg.QQBotIntegration.PublicBaseURL
		stableChannelCheckKey = cfg.Totp.EncryptionKeyConfigured
	}
	return &ConfigManager{db: db, settings: settings, redis: redisClient, encryptor: encryptor, bootstrapPublicURL: publicURL, stableChannelCheckKey: stableChannelCheckKey}
}

func (m *ConfigManager) SetOnReload(callback func(context.Context, ActiveConfig) error) {
	if m == nil {
		return
	}
	m.stateMu.Lock()
	m.onReload = callback
	m.stateMu.Unlock()
}

func (m *ConfigManager) Start(ctx context.Context) error {
	if m == nil {
		return errors.New("qqbot config manager unavailable")
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

func (m *ConfigManager) Shutdown(_ context.Context) error {
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

func (m *ConfigManager) Reload(ctx context.Context) error {
	if m == nil || m.settings == nil {
		return errors.New("qqbot setting repository unavailable")
	}
	keys := append([]string{SettingKeyRuntimeConfig}, qqBotBusinessSettingKeys()...)
	values, err := m.settings.GetMultiple(ctx, keys)
	if err != nil {
		m.recordLoadError("config_load_failed")
		return err
	}
	if strings.TrimSpace(values[SettingKeyRuntimeConfig]) == "" {
		if err := m.bootstrap(ctx); err != nil {
			m.recordLoadError("config_bootstrap_failed")
			return err
		}
		values, err = m.settings.GetMultiple(ctx, keys)
		if err != nil {
			m.recordLoadError("config_load_failed")
			return err
		}
	}
	storage, err := parseStorageConfig(values[SettingKeyRuntimeConfig], m.bootstrapPublicURL)
	if err != nil {
		m.recordLoadError("config_decode_failed")
		return err
	}
	if shouldImportBootstrapEnvironment(storage) {
		if err := m.bootstrap(ctx); err != nil {
			m.recordLoadError("config_bootstrap_failed")
			return err
		}
		values, err = m.settings.GetMultiple(ctx, keys)
		if err != nil {
			m.recordLoadError("config_load_failed")
			return err
		}
		storage, err = parseStorageConfig(values[SettingKeyRuntimeConfig], m.bootstrapPublicURL)
		if err != nil {
			m.recordLoadError("config_decode_failed")
			return err
		}
	}
	active, err := m.activeFromStorage(storage)
	if err != nil {
		m.recordLoadError("config_decrypt_failed")
		return err
	}
	business := parseBusinessSettings(values)
	if err := m.validateChannelCheckActivation(active.Enabled, business.ChannelCheckEnabled, active.PublicBaseURL); err != nil {
		m.recordLoadError("channel_check_config_invalid")
		return err
	}
	now := time.Now().UTC()
	m.expected.Store(storage.ConfigVersion)
	m.snapshot.Store(&configSnapshot{storage: storage, active: active, settings: business, loadedAt: now})
	m.clearLoadError()
	m.stateMu.RLock()
	callback := m.onReload
	m.stateMu.RUnlock()
	if callback != nil {
		if err := callback(ctx, active); err != nil {
			m.recordLoadError("runtime_reload_failed")
			return err
		}
	}
	return nil
}

func (m *ConfigManager) Active() (ActiveConfig, bool) {
	if m == nil {
		return ActiveConfig{}, false
	}
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		return ActiveConfig{}, false
	}
	return snapshot.active, true
}

func (m *ConfigManager) Public() PublicConfig {
	if m == nil {
		return publicFromStorage(defaultStorageConfig(""), defaultBusinessSettings())
	}
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		return publicFromStorage(defaultStorageConfig(m.bootstrapPublicURL), defaultBusinessSettings())
	}
	return publicFromStorage(snapshot.storage, cloneBusinessSettings(snapshot.settings))
}

func (m *ConfigManager) BusinessSettings() service.QQBotSettings {
	if m == nil {
		return defaultBusinessSettings()
	}
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		return defaultBusinessSettings()
	}
	return cloneBusinessSettings(snapshot.settings)
}

func (m *ConfigManager) ResolveProbeConfig(req ProbeRequest) (ActiveConfig, error) {
	if m == nil {
		return ActiveConfig{}, ErrRuntimeUnavailable
	}
	if len(req.AppSecret) > 4096 || len(req.WebhookSecret) > 4096 || len(req.PublicBaseURL) > 2048 {
		return ActiveConfig{}, ErrInvalidConfig
	}
	current, _ := m.Active()
	webhookConfiguredSeparately := false
	if snapshot := m.snapshot.Load(); snapshot != nil {
		webhookConfiguredSeparately = snapshot.storage.WebhookSecretCiphertext != ""
	}
	candidate := current
	candidate.AppID = strings.TrimSpace(req.AppID)
	candidate.Sandbox = req.Sandbox
	candidate.PublicBaseURL = strings.TrimRight(strings.TrimSpace(req.PublicBaseURL), "/")
	candidate.APITimeoutMS = req.APITimeoutMS
	if candidate.APITimeoutMS == 0 {
		candidate.APITimeoutMS = current.APITimeoutMS
	}
	if candidate.APITimeoutMS == 0 {
		candidate.APITimeoutMS = DefaultAPITimeoutMS
	}
	if value := strings.TrimSpace(req.AppSecret); value != "" {
		candidate.AppSecret = value
	}
	if value := strings.TrimSpace(req.WebhookSecret); value != "" {
		candidate.WebhookSecret = value
	} else if !webhookConfiguredSeparately || candidate.WebhookSecret == "" {
		candidate.WebhookSecret = candidate.AppSecret
	}
	validation := storageConfig{
		Enabled:             true,
		AppID:               candidate.AppID,
		PublicBaseURL:       candidate.PublicBaseURL,
		WorkerCount:         1,
		QueueCapacity:       16,
		APITimeoutMS:        candidate.APITimeoutMS,
		AppSecretCiphertext: "configured",
	}
	if candidate.AppSecret == "" || candidate.WebhookSecret == "" || validateStorageConfig(validation, true) != nil {
		return ActiveConfig{}, ErrInvalidConfig
	}
	channelCheckEnabled := m.BusinessSettings().ChannelCheckEnabled
	if req.ChannelCheckEnabled != nil {
		channelCheckEnabled = *req.ChannelCheckEnabled
	}
	if err := m.validateChannelCheckActivation(true, channelCheckEnabled, candidate.PublicBaseURL); err != nil {
		return ActiveConfig{}, err
	}
	return candidate, nil
}

func (m *ConfigManager) validateChannelCheckActivation(runtimeEnabled, channelCheckEnabled bool, publicBaseURL string) error {
	if !runtimeEnabled || !channelCheckEnabled {
		return nil
	}
	if m == nil || !m.stableChannelCheckKey {
		return ErrInvalidConfig
	}
	if _, err := validateChannelCheckPublicBaseURL(publicBaseURL); err != nil {
		return ErrInvalidConfig
	}
	return nil
}

func (m *ConfigManager) RecordSuccessfulProbe(ctx context.Context, cfg ActiveConfig) error {
	if m == nil || m.redis == nil {
		return ErrRuntimeUnavailable
	}
	return m.redis.Set(ctx, m.probeKey(cfg), "1", qqBotProbeTTL).Err()
}

func (m *ConfigManager) requireSuccessfulProbe(ctx context.Context, cfg ActiveConfig) error {
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

func (m *ConfigManager) probeKey(cfg ActiveConfig) string {
	return "sub2api:qqbot:probe:" + credentialProbeFingerprint(cfg)
}

func (m *ConfigManager) RuntimeState() (expected, active int64, loadedAt *time.Time, loadError string, errorAt *time.Time) {
	if m == nil {
		return 1, 0, nil, "config_manager_unavailable", nil
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

func (m *ConfigManager) Save(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
	if m == nil || m.db == nil || m.encryptor == nil {
		return PublicConfig{}, errors.New("qqbot config persistence unavailable")
	}
	if req.ExpectedConfigVersion < 1 || actorID <= 0 || len(req.AppSecret) > 4096 || len(req.WebhookSecret) > 4096 || len(req.PublicBaseURL) > 2048 {
		return PublicConfig{}, ErrInvalidConfig
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PublicConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, qqBotConfigLockKey); err != nil {
		return PublicConfig{}, err
	}

	values := make(map[string]string)
	for _, key := range append([]string{SettingKeyRuntimeConfig}, qqBotBusinessSettingKeys()...) {
		var value string
		err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, key).Scan(&value)
		if err == nil {
			values[key] = value
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return PublicConfig{}, err
		}
	}
	current, err := parseStorageConfig(values[SettingKeyRuntimeConfig], m.bootstrapPublicURL)
	if err != nil {
		return PublicConfig{}, err
	}
	if current.ConfigVersion != req.ExpectedConfigVersion {
		return PublicConfig{}, ErrConfigConflict
	}
	business, err := applyBusinessUpdate(parseBusinessSettings(values), req.businessUpdate())
	if err != nil {
		return PublicConfig{}, err
	}
	next := current
	next.Enabled = req.Enabled
	next.AppID = strings.TrimSpace(req.AppID)
	next.Sandbox = req.Sandbox
	next.PublicBaseURL = strings.TrimRight(strings.TrimSpace(req.PublicBaseURL), "/")
	next.WorkerCount = req.WorkerCount
	next.QueueCapacity = req.QueueCapacity
	next.APITimeoutMS = req.APITimeoutMS
	changedSecrets := make([]string, 0, 2)
	if strings.TrimSpace(req.AppSecret) != "" {
		next.AppSecretCiphertext, err = m.encryptor.Encrypt(strings.TrimSpace(req.AppSecret))
		if err != nil {
			return PublicConfig{}, err
		}
		changedSecrets = append(changedSecrets, "app_secret")
	}
	if strings.TrimSpace(req.WebhookSecret) != "" {
		next.WebhookSecretCiphertext, err = m.encryptor.Encrypt(strings.TrimSpace(req.WebhookSecret))
		if err != nil {
			return PublicConfig{}, err
		}
		changedSecrets = append(changedSecrets, "webhook_secret")
	}
	normalizeStorageConfig(&next)
	if err := validateStorageConfig(next, next.Enabled); err != nil {
		return PublicConfig{}, err
	}
	if err := m.validateChannelCheckActivation(next.Enabled, business.ChannelCheckEnabled, next.PublicBaseURL); err != nil {
		return PublicConfig{}, err
	}
	credentialsChanged := (!current.Enabled && next.Enabled) || current.AppID != next.AppID || current.Sandbox != next.Sandbox || strings.TrimSpace(req.AppSecret) != "" || strings.TrimSpace(req.WebhookSecret) != ""
	if credentialsChanged {
		candidate, candidateErr := m.activeFromStorage(next)
		if candidateErr != nil {
			return PublicConfig{}, candidateErr
		}
		if err := m.requireSuccessfulProbe(ctx, candidate); err != nil {
			return PublicConfig{}, err
		}
	}
	next.ConfigVersion = current.ConfigVersion + 1
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorID
	next.ChangeSummary = configChangeSummary(next, business, changedSecrets)
	rawNext, err := json.Marshal(next)
	if err != nil {
		return PublicConfig{}, err
	}
	updates := businessSettingsValues(business)
	updates[SettingKeyRuntimeConfig] = string(rawNext)
	for key, value := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key,value,updated_at) VALUES ($1,$2,NOW()) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=EXCLUDED.updated_at`, key, value); err != nil {
			return PublicConfig{}, err
		}
	}
	metadata, _ := json.Marshal(map[string]any{"config_version": next.ConfigVersion, "summary": json.RawMessage(next.ChangeSummary)})
	if _, err := tx.ExecContext(ctx, `INSERT INTO qqbot_binding_audit_logs (action,status,actor_type,actor_subject,reason,metadata) VALUES ('settings','success','admin',$1,'', $2::jsonb)`, Fingerprint(strconv.FormatInt(actorID, 10)), string(metadata)); err != nil {
		return PublicConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return PublicConfig{}, err
	}
	active, err := m.activeFromStorage(next)
	if err != nil {
		return PublicConfig{}, err
	}
	m.expected.Store(next.ConfigVersion)
	m.snapshot.Store(&configSnapshot{storage: next, active: active, settings: cloneBusinessSettings(business), loadedAt: time.Now().UTC()})
	m.clearLoadError()
	m.stateMu.RLock()
	callback := m.onReload
	m.stateMu.RUnlock()
	if callback != nil {
		if err := callback(ctx, active); err != nil {
			m.recordLoadError("runtime_reload_failed")
		}
	}
	if m.redis != nil {
		_ = m.redis.Publish(ctx, ConfigInvalidationChannel, strconv.FormatInt(next.ConfigVersion, 10)).Err()
	}
	return publicFromStorage(next, business), nil
}

func (m *ConfigManager) activeFromStorage(storage storageConfig) (ActiveConfig, error) {
	active := ActiveConfig{Enabled: storage.Enabled, AppID: storage.AppID, Sandbox: storage.Sandbox, PublicBaseURL: storage.PublicBaseURL, WorkerCount: storage.WorkerCount, QueueCapacity: storage.QueueCapacity, APITimeoutMS: storage.APITimeoutMS, ConfigVersion: storage.ConfigVersion, UpdatedAt: storage.UpdatedAt, UpdatedBy: storage.UpdatedBy}
	var err error
	if storage.AppSecretCiphertext != "" {
		active.AppSecret, err = m.encryptor.Decrypt(storage.AppSecretCiphertext)
		if err != nil {
			return ActiveConfig{}, fmt.Errorf("decrypt qqbot app secret: %w", err)
		}
	}
	if storage.WebhookSecretCiphertext != "" {
		active.WebhookSecret, err = m.encryptor.Decrypt(storage.WebhookSecretCiphertext)
		if err != nil {
			return ActiveConfig{}, fmt.Errorf("decrypt qqbot webhook secret: %w", err)
		}
	} else {
		active.WebhookSecret = active.AppSecret
	}
	return active, nil
}

func (m *ConfigManager) bootstrap(ctx context.Context) error {
	cfg := defaultStorageConfig(firstNonEmpty(os.Getenv("QQBOT_PUBLIC_BASE_URL"), legacyQQBotPublicURL(), m.bootstrapPublicURL))
	cfg.AppID = strings.TrimSpace(os.Getenv("QQBOT_APP_ID"))
	cfg.Sandbox, _ = strconv.ParseBool(strings.TrimSpace(os.Getenv("QQBOT_SANDBOX")))
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("QQBOT_WORKER_COUNT"))); err == nil && value > 0 {
		cfg.WorkerCount = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("QQBOT_QUEUE_SIZE"))); err == nil && value > 0 {
		cfg.QueueCapacity = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("QQBOT_API_TIMEOUT_MS"))); err == nil && value > 0 {
		cfg.APITimeoutMS = value
	} else if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("QQBOT_API_TIMEOUT"))); err == nil && value > 0 {
		cfg.APITimeoutMS = int(value / time.Millisecond)
	}
	var err error
	if secret := strings.TrimSpace(os.Getenv("QQBOT_APP_SECRET")); secret != "" {
		cfg.AppSecretCiphertext, err = m.encryptor.Encrypt(secret)
		if err != nil {
			return err
		}
	}
	if secret := strings.TrimSpace(os.Getenv("QQBOT_WEBHOOK_SECRET")); secret != "" {
		cfg.WebhookSecretCiphertext, err = m.encryptor.Encrypt(secret)
		if err != nil {
			return err
		}
	}
	if cfg.WebhookSecretCiphertext == "" {
		cfg.WebhookSecretCiphertext = cfg.AppSecretCiphertext
	}
	cfg.Enabled = false
	cfg.ChangeSummary = `{"bootstrap":true,"enabled":false}`
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return m.settings.Set(ctx, SettingKeyRuntimeConfig, string(raw))
}

func legacyQQBotPublicURL() string {
	for _, key := range []string{"QQBOT_APP_ID", "QQBOT_APP_SECRET", "QQBOT_WEBHOOK_SECRET"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL"))
		}
	}
	return ""
}

func shouldImportBootstrapEnvironment(cfg storageConfig) bool {
	if cfg.ConfigVersion != 1 || cfg.UpdatedBy != 0 || cfg.AppID != "" || cfg.AppSecretCiphertext != "" || cfg.WebhookSecretCiphertext != "" || strings.Contains(cfg.ChangeSummary, `"bootstrap":true`) {
		return false
	}
	for _, key := range []string{"QQBOT_APP_ID", "QQBOT_APP_SECRET", "QQBOT_WEBHOOK_SECRET", "QQBOT_PUBLIC_BASE_URL"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func (m *ConfigManager) refreshLoop(ctx context.Context) {
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
func (m *ConfigManager) subscribeLoop(ctx context.Context) {
	defer m.wg.Done()
	pubsub := m.redis.Subscribe(ctx, ConfigInvalidationChannel)
	defer func() { _ = pubsub.Close() }()
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-channel:
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
func (m *ConfigManager) recordLoadError(code string) {
	now := time.Now().UTC()
	m.stateMu.Lock()
	m.lastLoadError = code
	m.lastErrorAt = &now
	m.stateMu.Unlock()
}
func (m *ConfigManager) clearLoadError() {
	m.stateMu.Lock()
	m.lastLoadError = ""
	m.lastErrorAt = nil
	m.stateMu.Unlock()
}

func qqBotBusinessSettingKeys() []string {
	return []string{service.SettingKeyQQBotBindingEnabled, service.SettingKeyQQBotFirstBindBonus, service.SettingKeyQQBotLinkTTLMinutes, service.SettingKeyQQBotWelcomeEnabled, service.SettingKeyQQBotFirstInteractionEnabled, service.SettingKeyQQBotChannelCheckEnabled, service.SettingKeyQQBotHelpMessage, service.SettingKeyQQBotAllowedGroupIDs, service.SettingKeyQQBotAllowedGuildIDs, service.SettingKeyQQBotGuildWelcomeChannels}
}
func defaultBusinessSettings() service.QQBotSettings {
	return service.QQBotSettings{BindingEnabled: true, FirstBindBonus: 5, LinkTTLMinutes: 15, WelcomeEnabled: true, FirstInteractionEnabled: true, ChannelCheckEnabled: false, HelpMessage: defaultHelpMessage, AllowedGroupIDs: []string{}, AllowedGuildIDs: []string{}, GuildWelcomeChannels: map[string]string{}}
}
func parseBusinessSettings(values map[string]string) service.QQBotSettings {
	cfg := defaultBusinessSettings()
	if value, ok := values[service.SettingKeyQQBotBindingEnabled]; ok {
		cfg.BindingEnabled, _ = strconv.ParseBool(value)
	}
	if value, err := strconv.ParseFloat(values[service.SettingKeyQQBotFirstBindBonus], 64); err == nil && value >= 0 {
		cfg.FirstBindBonus = value
	}
	if value, err := strconv.Atoi(values[service.SettingKeyQQBotLinkTTLMinutes]); err == nil && value >= 5 && value <= 1440 {
		cfg.LinkTTLMinutes = value
	}
	if value, ok := values[service.SettingKeyQQBotWelcomeEnabled]; ok {
		cfg.WelcomeEnabled, _ = strconv.ParseBool(value)
	}
	if value, ok := values[service.SettingKeyQQBotFirstInteractionEnabled]; ok {
		cfg.FirstInteractionEnabled, _ = strconv.ParseBool(value)
	}
	if value, ok := values[service.SettingKeyQQBotChannelCheckEnabled]; ok {
		cfg.ChannelCheckEnabled, _ = strconv.ParseBool(value)
	}
	if value := strings.TrimSpace(values[service.SettingKeyQQBotHelpMessage]); value != "" {
		cfg.HelpMessage = value
	}
	_ = json.Unmarshal([]byte(values[service.SettingKeyQQBotAllowedGroupIDs]), &cfg.AllowedGroupIDs)
	_ = json.Unmarshal([]byte(values[service.SettingKeyQQBotAllowedGuildIDs]), &cfg.AllowedGuildIDs)
	_ = json.Unmarshal([]byte(values[service.SettingKeyQQBotGuildWelcomeChannels]), &cfg.GuildWelcomeChannels)
	cfg.AllowedGroupIDs = normalizeIDs(cfg.AllowedGroupIDs)
	cfg.AllowedGuildIDs = normalizeIDs(cfg.AllowedGuildIDs)
	cfg.GuildWelcomeChannels = normalizeChannelMap(cfg.GuildWelcomeChannels)
	return cfg
}
func applyBusinessUpdate(current service.QQBotSettings, update service.QQBotSettingsUpdate) (service.QQBotSettings, error) {
	if update.BindingEnabled != nil {
		current.BindingEnabled = *update.BindingEnabled
	}
	if update.FirstBindBonus != nil {
		current.FirstBindBonus = *update.FirstBindBonus
	}
	if update.LinkTTLMinutes != nil {
		current.LinkTTLMinutes = *update.LinkTTLMinutes
	}
	if update.WelcomeEnabled != nil {
		current.WelcomeEnabled = *update.WelcomeEnabled
	}
	if update.FirstInteractionEnabled != nil {
		current.FirstInteractionEnabled = *update.FirstInteractionEnabled
	}
	if update.ChannelCheckEnabled != nil {
		current.ChannelCheckEnabled = *update.ChannelCheckEnabled
	}
	if update.HelpMessage != nil {
		current.HelpMessage = strings.TrimSpace(*update.HelpMessage)
	}
	if update.AllowedGroupIDs != nil {
		current.AllowedGroupIDs = normalizeIDs(*update.AllowedGroupIDs)
	}
	if update.AllowedGuildIDs != nil {
		current.AllowedGuildIDs = normalizeIDs(*update.AllowedGuildIDs)
	}
	if update.GuildWelcomeChannels != nil {
		current.GuildWelcomeChannels = normalizeChannelMap(*update.GuildWelcomeChannels)
	}
	if current.FirstBindBonus < 0 || current.FirstBindBonus > 1_000_000 || current.LinkTTLMinutes < 5 || current.LinkTTLMinutes > 1440 || len([]rune(current.HelpMessage)) > 4000 || len(current.AllowedGroupIDs) > 500 || len(current.AllowedGuildIDs) > 500 || len(current.GuildWelcomeChannels) > 500 {
		return service.QQBotSettings{}, ErrInvalidConfig
	}
	return current, nil
}
func businessSettingsValues(cfg service.QQBotSettings) map[string]string {
	groups, _ := json.Marshal(cfg.AllowedGroupIDs)
	guilds, _ := json.Marshal(cfg.AllowedGuildIDs)
	channels, _ := json.Marshal(cfg.GuildWelcomeChannels)
	return map[string]string{service.SettingKeyQQBotBindingEnabled: strconv.FormatBool(cfg.BindingEnabled), service.SettingKeyQQBotFirstBindBonus: strconv.FormatFloat(cfg.FirstBindBonus, 'f', -1, 64), service.SettingKeyQQBotLinkTTLMinutes: strconv.Itoa(cfg.LinkTTLMinutes), service.SettingKeyQQBotWelcomeEnabled: strconv.FormatBool(cfg.WelcomeEnabled), service.SettingKeyQQBotFirstInteractionEnabled: strconv.FormatBool(cfg.FirstInteractionEnabled), service.SettingKeyQQBotChannelCheckEnabled: strconv.FormatBool(cfg.ChannelCheckEnabled), service.SettingKeyQQBotHelpMessage: cfg.HelpMessage, service.SettingKeyQQBotAllowedGroupIDs: string(groups), service.SettingKeyQQBotAllowedGuildIDs: string(guilds), service.SettingKeyQQBotGuildWelcomeChannels: string(channels)}
}
func normalizeIDs(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && len(value) <= 255 {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func normalizeChannelMap(values map[string]string) map[string]string {
	result := map[string]string{}
	for guild, channel := range values {
		guild = strings.TrimSpace(guild)
		channel = strings.TrimSpace(channel)
		if guild != "" && channel != "" && len(guild) <= 255 && len(channel) <= 255 {
			result[guild] = channel
		}
	}
	return result
}
func cloneBusinessSettings(cfg service.QQBotSettings) service.QQBotSettings {
	cfg.AllowedGroupIDs = append([]string{}, cfg.AllowedGroupIDs...)
	cfg.AllowedGuildIDs = append([]string{}, cfg.AllowedGuildIDs...)
	channels := make(map[string]string, len(cfg.GuildWelcomeChannels))
	for key, value := range cfg.GuildWelcomeChannels {
		channels[key] = value
	}
	cfg.GuildWelcomeChannels = channels
	return cfg
}
