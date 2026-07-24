package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOneBotConfigManagerReloadMasksToken(t *testing.T) {
	storage := defaultOneBotStorageConfig()
	storage.SelfID = "123456789"
	storage.AccessTokenCiphertext = "enc:abcdefghijklmnopqrstuvwxyz012345"
	storage.ConfigVersion = 7
	storage.AutoApproveFriendRequests = true
	storage.AutoApproveGroupRequests = true
	raw, _ := json.Marshal(storage)
	repo := &memorySettingRepo{values: map[string]string{SettingKeyOneBotRuntimeConfig: string(raw)}}
	manager := NewOneBotConfigManager(nil, repo, nil, testEncryptor{})
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	public := manager.Public()
	if !public.AccessTokenConfigured || public.ConfigVersion != 7 || public.SelfID != storage.SelfID || !public.AutoApproveFriendRequests || !public.AutoApproveGroupRequests {
		t.Fatalf("public=%#v", public)
	}
	active, ok := manager.Active()
	if !ok || active.AccessToken != "abcdefghijklmnopqrstuvwxyz012345" {
		t.Fatalf("active=%#v ok=%v", active, ok)
	}
}

func TestOneBotConfigManagerReloadSkipsUnchangedRuntimeConfig(t *testing.T) {
	storage := defaultOneBotStorageConfig()
	storage.Enabled = true
	storage.SelfID = "123456789"
	storage.AccessTokenCiphertext = "enc:abcdefghijklmnopqrstuvwxyz012345"
	storage.ConfigVersion = 7
	raw, _ := json.Marshal(storage)
	repo := &memorySettingRepo{values: map[string]string{SettingKeyOneBotRuntimeConfig: string(raw)}}
	manager := NewOneBotConfigManager(nil, repo, nil, testEncryptor{})
	callbackCount := 0
	manager.SetOnReload(func(context.Context, OneBotActiveConfig) error {
		callbackCount++
		return nil
	})

	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if callbackCount != 1 {
		t.Fatalf("unchanged config reloaded runtime %d times", callbackCount)
	}

	storage.ConfigVersion = 8
	storage.WorkerCount = 3
	raw, _ = json.Marshal(storage)
	repo.values[SettingKeyOneBotRuntimeConfig] = string(raw)
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if callbackCount != 2 {
		t.Fatalf("changed config reload count=%d", callbackCount)
	}
}

func TestOneBotConfigManagerPolicyRefreshDoesNotReloadRuntime(t *testing.T) {
	storage := defaultOneBotStorageConfig()
	storage.Enabled = true
	storage.SelfID = "123456789"
	storage.AccessTokenCiphertext = "enc:abcdefghijklmnopqrstuvwxyz012345"
	raw, _ := json.Marshal(storage)
	repo := &memorySettingRepo{values: map[string]string{SettingKeyOneBotRuntimeConfig: string(raw)}}
	manager := NewOneBotConfigManager(nil, repo, nil, testEncryptor{})
	callbackCount := 0
	manager.SetOnReload(func(context.Context, OneBotActiveConfig) error {
		callbackCount++
		return nil
	})
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}

	storage.ConfigVersion++
	storage.AutoApproveFriendRequests = true
	storage.AutoApproveGroupRequests = true
	raw, _ = json.Marshal(storage)
	repo.values[SettingKeyOneBotRuntimeConfig] = string(raw)
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if callbackCount != 1 {
		t.Fatalf("policy refresh reloaded runtime %d times", callbackCount)
	}
	if policy := manager.RequestPolicy(); !policy.AutoApproveFriendRequests || !policy.AutoApproveGroupRequests {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestOneBotConfigManagerBootstrapsDisabledConfig(t *testing.T) {
	repo := &memorySettingRepo{values: map[string]string{}}
	manager := NewOneBotConfigManager(nil, repo, nil, testEncryptor{})
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	public := manager.Public()
	if public.Enabled || public.ConfigVersion != 1 || public.WorkerCount != DefaultOneBotWorkerCount {
		t.Fatalf("public=%#v", public)
	}
	if repo.values[SettingKeyOneBotRuntimeConfig] == "" {
		t.Fatal("bootstrap config was not persisted")
	}
}

func TestOneBotProbeConfirmationIsCredentialBound(t *testing.T) {
	_, client := newRedisQueue(t)
	manager := &OneBotConfigManager{redis: client}
	cfg := OneBotActiveConfig{SelfID: "123456789", AccessToken: "abcdefghijklmnopqrstuvwxyz012345", ActionTimeoutMS: 10000}
	if err := manager.requireSuccessfulProbe(t.Context(), cfg); !errors.Is(err, ErrProbeRequired) {
		t.Fatalf("require before probe err=%v", err)
	}
	if err := manager.RecordSuccessfulProbe(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := manager.requireSuccessfulProbe(t.Context(), cfg); err != nil {
		t.Fatalf("require after probe err=%v", err)
	}
	cfg.AccessToken = "rotated-abcdefghijklmnopqrstuvwxyz"
	if err := manager.requireSuccessfulProbe(t.Context(), cfg); !errors.Is(err, ErrProbeRequired) {
		t.Fatalf("rotated token reused probe: %v", err)
	}
}

func TestOneBotConfigManagerSaveDisabledTokenThenEnableAfterProbe(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	current := defaultOneBotStorageConfig()
	raw, _ := json.Marshal(current)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).WithArgs(oneBotConfigLockKey).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM settings WHERE key=$1 FOR UPDATE`)).WithArgs(SettingKeyOneBotRuntimeConfig).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	mock.ExpectExec("INSERT INTO settings").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO qqbot_binding_audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	manager := NewOneBotConfigManager(db, &memorySettingRepo{}, nil, testEncryptor{})
	result, err := manager.Save(context.Background(), OneBotUpdateConfigRequest{
		ExpectedConfigVersion: 1,
		Enabled:               false,
		SelfID:                "123456789",
		AccessToken:           "abcdefghijklmnopqrstuvwxyz012345",
		WorkerCount:           2,
		QueueCapacity:         1024,
		ActionTimeoutMS:       10000,
	}, 99)
	if err != nil {
		t.Fatal(err)
	}
	if result.Enabled || !result.AccessTokenConfigured || result.ConfigVersion != 2 {
		t.Fatalf("result=%#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
