package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/firefly"
)

const adobeVideoTaskKeyPrefix = "adobe:video:task:"

var (
	ErrAdobeVideoTaskNotFound          = errors.New("adobe video task not found")
	ErrAdobeVideoTaskOwnerMismatch     = errors.New("adobe video task owner mismatch")
	ErrAdobeVideoTaskImmutableConflict = errors.New("adobe video task immutable fields conflict")
	ErrAdobeVideoTaskSettlementLocked  = errors.New("adobe video task settlement is locked")
)

type AdobeVideoTaskStatus string

const (
	AdobeVideoTaskPending    AdobeVideoTaskStatus = "pending"
	AdobeVideoTaskProcessing AdobeVideoTaskStatus = "processing"
	AdobeVideoTaskCompleted  AdobeVideoTaskStatus = "completed"
	AdobeVideoTaskFailed     AdobeVideoTaskStatus = "failed"
	AdobeVideoTaskCanceled   AdobeVideoTaskStatus = "canceled"
)

type AdobeVideoSettlementStatus string

const (
	AdobeVideoSettlementPending AdobeVideoSettlementStatus = "pending"
	AdobeVideoSettlementSettled AdobeVideoSettlementStatus = "settled"
	AdobeVideoSettlementFailed  AdobeVideoSettlementStatus = "failed"
)

// AdobeVideoTask keeps immutable submission identity together with the last upstream
// state. PollURL is stored in full and must never be reconstructed during status calls.
type AdobeVideoTask struct {
	TaskID         string `json:"task_id"`
	PollURL        string `json:"poll_url"`
	AccountID      int64  `json:"account_id"`
	UserID         int64  `json:"user_id"`
	APIKeyID       int64  `json:"api_key_id"`
	GroupID        int64  `json:"group_id"`
	SubscriptionID *int64 `json:"subscription_id,omitempty"`

	RequestedModel  string                    `json:"requested_model"`
	ChannelModel    string                    `json:"channel_model,omitempty"`
	UpstreamModel   string                    `json:"upstream_model"`
	Resolution      string                    `json:"resolution"`
	DurationSeconds int                       `json:"duration_seconds"`
	ReferenceMode   string                    `json:"reference_mode,omitempty"`
	PricingSnapshot AdobeMediaPricingSnapshot `json:"pricing_snapshot"`
	SnapshotHash    string                    `json:"snapshot_hash"`
	CreatedAt       time.Time                 `json:"created_at"`

	Status           AdobeVideoTaskStatus       `json:"status"`
	ResultURLs       []string                   `json:"result_urls,omitempty"`
	UpstreamResponse json.RawMessage            `json:"upstream_response,omitempty"`
	SettlementStatus AdobeVideoSettlementStatus `json:"settlement_status"`
	SettledAt        *time.Time                 `json:"settled_at,omitempty"`
	LastError        string                     `json:"last_error,omitempty"`
	UpdatedAt        time.Time                  `json:"updated_at"`
}

func (t *AdobeVideoTask) Validate() error {
	if t == nil || strings.TrimSpace(t.TaskID) == "" || t.AccountID <= 0 || t.UserID <= 0 || t.APIKeyID <= 0 || t.GroupID <= 0 {
		return ErrAdobeMediaSnapshotInvalid
	}
	if err := firefly.ValidateStatusURL(t.PollURL); err != nil {
		return ErrAdobeMediaSnapshotInvalid
	}
	if err := t.PricingSnapshot.Validate(); err != nil {
		return err
	}
	if t.PricingSnapshot.BillingMode != string(BillingModeVideo) || t.PricingSnapshot.GroupID != t.GroupID || t.PricingSnapshot.Quantity != t.DurationSeconds {
		return ErrAdobeMediaSnapshotInvalid
	}
	if t.SnapshotHash == "" {
		t.SnapshotHash = t.PricingSnapshot.Hash
	}
	if !strings.EqualFold(t.SnapshotHash, t.PricingSnapshot.Hash) {
		return ErrAdobeMediaSnapshotConflict
	}
	if t.Status == "" {
		t.Status = AdobeVideoTaskPending
	}
	switch t.Status {
	case AdobeVideoTaskPending, AdobeVideoTaskProcessing, AdobeVideoTaskFailed, AdobeVideoTaskCanceled:
	case AdobeVideoTaskCompleted:
		if len(t.ResultURLs) == 0 {
			return ErrAdobeMediaSnapshotInvalid
		}
	default:
		return ErrAdobeMediaSnapshotInvalid
	}
	if t.SettlementStatus == "" {
		t.SettlementStatus = AdobeVideoSettlementPending
	}
	switch t.SettlementStatus {
	case AdobeVideoSettlementPending, AdobeVideoSettlementFailed:
	case AdobeVideoSettlementSettled:
		if t.Status != AdobeVideoTaskCompleted || len(t.ResultURLs) == 0 {
			return ErrAdobeMediaSnapshotInvalid
		}
	default:
		return ErrAdobeMediaSnapshotInvalid
	}
	return nil
}

func (t *AdobeVideoTask) IsTerminal() bool {
	return t != nil && (t.Status == AdobeVideoTaskCompleted || t.Status == AdobeVideoTaskFailed || t.Status == AdobeVideoTaskCanceled)
}

func (t *AdobeVideoTask) CanExposeResult() bool {
	return t != nil && t.Status == AdobeVideoTaskCompleted && t.SettlementStatus == AdobeVideoSettlementSettled
}

type AdobeVideoTaskStore interface {
	Healthy(ctx context.Context) error
	Create(ctx context.Context, task *AdobeVideoTask) error
	Get(ctx context.Context, taskID string) (*AdobeVideoTask, error)
	Update(ctx context.Context, task *AdobeVideoTask) error
	AcquireSettlementLock(ctx context.Context, taskID string, ttl time.Duration) (func(context.Context) error, error)
}

type adobeVideoTaskCache interface {
	Ping(ctx context.Context) error
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) ([]byte, error)
	Watch(ctx context.Context, key string, update func([]byte, time.Duration) ([]byte, time.Duration, error)) error
	TryLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, key, token string) error
}

type RedisAdobeVideoTaskStore struct {
	client      adobeVideoTaskCache
	activeTTL   time.Duration
	terminalTTL time.Duration
}

func newAdobeVideoTaskStore(client adobeVideoTaskCache, activeTTL, terminalTTL time.Duration) *RedisAdobeVideoTaskStore {
	if activeTTL <= 0 {
		activeTTL = 72 * time.Hour
	}
	if terminalTTL <= 0 {
		terminalTTL = 24 * time.Hour
	}
	return &RedisAdobeVideoTaskStore{client: client, activeTTL: activeTTL, terminalTTL: terminalTTL}
}

func (s *RedisAdobeVideoTaskStore) Healthy(ctx context.Context) error {
	if s == nil || s.client == nil {
		return errors.New("adobe video task redis is unavailable")
	}
	return s.client.Ping(ctx)
}

func adobeVideoTaskKey(taskID string) string {
	return adobeVideoTaskKeyPrefix + strings.TrimSpace(taskID)
}

func (s *RedisAdobeVideoTaskStore) Create(ctx context.Context, task *AdobeVideoTask) error {
	if err := task.Validate(); err != nil {
		return err
	}
	if s == nil || s.client == nil {
		return errors.New("adobe video task redis is unavailable")
	}
	now := time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}
	created, err := s.client.SetNX(ctx, adobeVideoTaskKey(task.TaskID), body, s.activeTTL)
	if err != nil {
		return err
	}
	if !created {
		existing, getErr := s.Get(ctx, task.TaskID)
		if getErr != nil {
			return getErr
		}
		if existing.SnapshotHash != task.SnapshotHash || existing.AccountID != task.AccountID || existing.UserID != task.UserID || existing.GroupID != task.GroupID || existing.PollURL != task.PollURL {
			return ErrAdobeVideoTaskImmutableConflict
		}
	}
	return nil
}

func (s *RedisAdobeVideoTaskStore) Get(ctx context.Context, taskID string) (*AdobeVideoTask, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("adobe video task redis is unavailable")
	}
	body, err := s.client.Get(ctx, adobeVideoTaskKey(taskID))
	if err != nil {
		return nil, err
	}
	var task AdobeVideoTask
	if err := json.Unmarshal(body, &task); err != nil {
		return nil, fmt.Errorf("decode adobe video task: %w", err)
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	return &task, nil
}

// Update uses WATCH to avoid losing a concurrent settlement marker and verifies all
// immutable identity fields before replacing the cached document.
func (s *RedisAdobeVideoTaskStore) Update(ctx context.Context, task *AdobeVideoTask) error {
	if err := task.Validate(); err != nil {
		return err
	}
	if s == nil || s.client == nil {
		return errors.New("adobe video task redis is unavailable")
	}
	key := adobeVideoTaskKey(task.TaskID)
	return s.client.Watch(ctx, key, func(body []byte, remainingTTL time.Duration) ([]byte, time.Duration, error) {
		var existing AdobeVideoTask
		if err := json.Unmarshal(body, &existing); err != nil {
			return nil, 0, err
		}
		if existing.SnapshotHash != task.SnapshotHash || existing.AccountID != task.AccountID || existing.UserID != task.UserID || existing.APIKeyID != task.APIKeyID || existing.GroupID != task.GroupID || existing.PollURL != task.PollURL {
			return nil, 0, ErrAdobeVideoTaskImmutableConflict
		}
		// A stale poll result must not undo any terminal state or a completed settlement.
		if existing.IsTerminal() {
			statusChanged := task.Status != existing.Status
			task.Status = existing.Status
			task.ResultURLs = append([]string(nil), existing.ResultURLs...)
			task.UpstreamResponse = cloneRawJSON(existing.UpstreamResponse)
			if statusChanged {
				task.LastError = existing.LastError
			}
		}
		if existing.SettlementStatus == AdobeVideoSettlementSettled && task.SettlementStatus != AdobeVideoSettlementSettled {
			task.SettlementStatus = existing.SettlementStatus
			task.SettledAt = existing.SettledAt
		}
		if remainingTTL <= 0 {
			return nil, 0, ErrAdobeVideoTaskNotFound
		}
		ttl := remainingTTL
		if !existing.IsTerminal() && task.IsTerminal() {
			ttl = s.terminalTTL
		}
		task.UpdatedAt = time.Now().UTC()
		encoded, err := json.Marshal(task)
		if err != nil {
			return nil, 0, err
		}
		return encoded, ttl, nil
	})
}

func (s *RedisAdobeVideoTaskStore) AcquireSettlementLock(ctx context.Context, taskID string, ttl time.Duration) (func(context.Context) error, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("adobe video task redis is unavailable")
	}
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	lockKey := adobeVideoTaskKey(taskID) + ":settle-lock"
	token := fmt.Sprintf("%d", time.Now().UnixNano())
	ok, err := s.client.TryLock(ctx, lockKey, token, ttl)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAdobeVideoTaskSettlementLocked
	}
	unlock := func(unlockCtx context.Context) error {
		return s.client.Unlock(unlockCtx, lockKey, token)
	}
	return unlock, nil
}
