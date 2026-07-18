package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConfigManagerReloadMasksSecrets(t *testing.T) {
	storage := defaultStorageConfig("https://qq.example.com")
	storage.AppID = "123456"
	storage.AppSecretCiphertext = "enc:app-secret"
	storage.WebhookSecretCiphertext = "enc:webhook-secret"
	storage.ConfigVersion = 7
	raw, _ := json.Marshal(storage)
	repo := &memorySettingRepo{values: map[string]string{SettingKeyRuntimeConfig: string(raw)}}
	manager := NewConfigManager(nil, repo, nil, testEncryptor{}, nil)
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	public := manager.Public()
	if !public.AppSecretConfigured || !public.WebhookSecretConfigured || public.ConfigVersion != 7 {
		t.Fatalf("public=%#v", public)
	}
	active, ok := manager.Active()
	if !ok || active.AppSecret != "app-secret" || active.WebhookSecret != "webhook-secret" {
		t.Fatalf("active=%#v ok=%v", active, ok)
	}
}

func TestParseStorageConfigKeepsBootstrapPublicURL(t *testing.T) {
	storage, err := parseStorageConfig(`{"enabled":false,"public_base_url":"","config_version":1}`, "https://qq.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if storage.PublicBaseURL != "https://qq.example.com" {
		t.Fatalf("public base URL=%q", storage.PublicBaseURL)
	}
}

func TestBootstrapEnvironmentImportRunsOnlyForPristineMigrationConfig(t *testing.T) {
	t.Setenv("QQBOT_APP_ID", "123456")
	storage := defaultStorageConfig("https://qq.example.com")
	storage.ChangeSummary = `{"bootstrap":false,"enabled":false}`
	if !shouldImportBootstrapEnvironment(storage) {
		t.Fatal("pristine migration config did not import environment")
	}
	storage.ChangeSummary = `{"bootstrap":true,"enabled":false}`
	if shouldImportBootstrapEnvironment(storage) {
		t.Fatal("bootstrap environment would be imported repeatedly")
	}
}

func TestPublicConfigJSONUsesFlatBusinessFields(t *testing.T) {
	storage := defaultStorageConfig("https://qq.example.com")
	settings := defaultBusinessSettings()
	raw, err := json.Marshal(publicFromStorage(storage, settings))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, nested := payload["settings"]; nested {
		t.Fatalf("unexpected nested settings: %s", raw)
	}
	if payload["binding_enabled"] != true || payload["link_ttl_minutes"] != float64(15) {
		t.Fatalf("missing flat business fields: %s", raw)
	}
}

func TestProbeConfirmationIsBoundToCredentialFingerprint(t *testing.T) {
	_, client := newRedisQueue(t)
	manager := &ConfigManager{redis: client}
	cfg := ActiveConfig{AppID: "app", AppSecret: "app-secret", WebhookSecret: "webhook-secret", Sandbox: false}
	if err := manager.requireSuccessfulProbe(t.Context(), cfg); !errors.Is(err, ErrProbeRequired) {
		t.Fatalf("require before probe err=%v", err)
	}
	if err := manager.RecordSuccessfulProbe(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := manager.requireSuccessfulProbe(t.Context(), cfg); err != nil {
		t.Fatalf("require after probe err=%v", err)
	}
	cfg.AppSecret = "rotated"
	if err := manager.requireSuccessfulProbe(t.Context(), cfg); !errors.Is(err, ErrProbeRequired) {
		t.Fatalf("rotated credentials reused probe: %v", err)
	}
}

func TestResolveProbeConfigUsesAppSecretAsWebhookFallback(t *testing.T) {
	manager := &ConfigManager{}
	manager.snapshot.Store(&configSnapshot{
		storage: storageConfig{AppSecretCiphertext: "enc:old-app"},
		active:  ActiveConfig{AppID: "123456", AppSecret: "old-app", WebhookSecret: "old-app", APITimeoutMS: 10000},
	})
	candidate, err := manager.ResolveProbeConfig(ProbeRequest{AppID: "123456", AppSecret: "new-app", PublicBaseURL: "https://qq.example.com", APITimeoutMS: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.AppSecret != "new-app" || candidate.WebhookSecret != "new-app" {
		t.Fatalf("candidate=%#v", candidate)
	}
}

func TestConfigManagerSaveRetainsEmptySecrets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	mock.MatchExpectationsInOrder(false)
	current := defaultStorageConfig("https://qq.example.com")
	current.Enabled = true
	current.AppID = "123456"
	current.AppSecretCiphertext = "enc:old-app"
	current.WebhookSecretCiphertext = "enc:old-webhook"
	current.ConfigVersion = 3
	raw, _ := json.Marshal(current)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(qqBotConfigLockKey).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, key := range append([]string{SettingKeyRuntimeConfig}, qqBotBusinessSettingKeys()...) {
		expectation := mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM settings WHERE key=$1 FOR UPDATE`)).WithArgs(key)
		if key == SettingKeyRuntimeConfig {
			expectation.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
		} else {
			expectation.WillReturnError(sql.ErrNoRows)
		}
	}
	for range append([]string{SettingKeyRuntimeConfig}, qqBotBusinessSettingKeys()...) {
		mock.ExpectExec("INSERT INTO settings").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("INSERT INTO qqbot_binding_audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	manager := NewConfigManager(db, &memorySettingRepo{}, nil, testEncryptor{}, nil)
	result, err := manager.Save(context.Background(), UpdateConfigRequest{ExpectedConfigVersion: 3, Enabled: true, AppID: "123456", PublicBaseURL: "https://qq.example.com", WorkerCount: 4, QueueCapacity: 256, APITimeoutMS: 10000, BindingEnabled: true, FirstBindBonus: 5, LinkTTLMinutes: 15, WelcomeEnabled: true, FirstInteractionEnabled: true, HelpMessage: defaultHelpMessage, AllowedGroupIDs: []string{}, AllowedGuildIDs: []string{}, GuildWelcomeChannels: map[string]string{}}, 99)
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigVersion != 4 || !result.AppSecretConfigured || !result.WebhookSecretConfigured {
		t.Fatalf("result=%#v", result)
	}
	active, _ := manager.Active()
	if active.AppSecret != "old-app" || active.WebhookSecret != "old-webhook" {
		t.Fatalf("secrets were not retained: %#v", active)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
