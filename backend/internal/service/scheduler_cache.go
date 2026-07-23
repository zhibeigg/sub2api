package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	SchedulerModeSingle = "single"
	SchedulerModeMixed  = "mixed"
	SchedulerModeForced = "forced"
)

var (
	ErrSchedulerBucketRetired              = errors.New("scheduler bucket retired")
	ErrSchedulerBucketWriteFenced          = errors.New("scheduler bucket write fenced")
	ErrSchedulerGroupLifecycleLeaseInvalid = errors.New("scheduler group lifecycle lease invalid")
	ErrSchedulerGroupLifecycleLeaseLost    = errors.New("scheduler group lifecycle lease lost")
)

// SchedulerBucketWriteToken fences a snapshot writer to one bucket epoch.
// Tokens must be captured before any database load or queued rebuild work.
type SchedulerBucketWriteToken struct {
	Bucket SchedulerBucket
	Epoch  int64
}

func (t SchedulerBucketWriteToken) ValidFor(bucket SchedulerBucket) bool {
	return t.Epoch > 0 && t.Bucket == bucket
}

// SchedulerGroupLifecycleLease identifies one owner of a group's short-lived
// retirement/reopen critical section.
type SchedulerGroupLifecycleLease struct {
	GroupID    int64
	OwnerToken string
}

func (l SchedulerGroupLifecycleLease) ValidFor(groupID int64) bool {
	return groupID > 0 && l.GroupID == groupID && l.OwnerToken != ""
}

const SchedulerBucketSchemaVersion = 2

// SchedulerBucket identifies one provider-independent scheduler candidate set.
// Protocol buckets are keyed by ingress protocol plus an optional exact account
// provider restriction. Platform is retained only for legacy callers that do not
// carry an endpoint protocol in context; new gateway paths must use Protocol.
type SchedulerBucket struct {
	GroupID        int64
	Protocol       EndpointProtocol
	ForcedPlatform string
	Mode           string
	Platform       string
}

func NewSchedulerBucket(groupID int64, protocol EndpointProtocol, forcedPlatform string) (SchedulerBucket, bool) {
	protocol = NormalizeEndpointProtocol(protocol)
	if !IsValidEndpointProtocol(protocol) {
		return SchedulerBucket{}, false
	}
	forcedPlatform = NormalizePlatform(forcedPlatform)
	if forcedPlatform != "" && !IsValidPlatform(forcedPlatform) {
		return SchedulerBucket{}, false
	}
	mode := SchedulerModeMixed
	candidatePlatforms := CandidateAccountPlatforms(protocol, forcedPlatform)
	if len(candidatePlatforms) == 0 {
		return SchedulerBucket{}, false
	}
	if forcedPlatform != "" {
		mode = SchedulerModeForced
	} else if len(candidatePlatforms) == 1 {
		mode = SchedulerModeSingle
	}
	return SchedulerBucket{
		GroupID:        groupID,
		Protocol:       protocol,
		ForcedPlatform: forcedPlatform,
		Mode:           mode,
	}, true
}

func (b SchedulerBucket) IsProtocolBucket() bool {
	return IsValidEndpointProtocol(b.Protocol)
}

func (b SchedulerBucket) RequestDescriptor() RequestDescriptor {
	return RequestDescriptor{
		Protocol:       NormalizeEndpointProtocol(b.Protocol),
		ForcedPlatform: NormalizePlatform(b.ForcedPlatform),
	}
}

func (b SchedulerBucket) String() string {
	if b.IsProtocolBucket() {
		forcedPlatform := NormalizePlatform(b.ForcedPlatform)
		if forcedPlatform == "" {
			forcedPlatform = "-"
		}
		return fmt.Sprintf("v%d:%d:%s:%s:%s", SchedulerBucketSchemaVersion, b.GroupID, NormalizeEndpointProtocol(b.Protocol), forcedPlatform, b.Mode)
	}
	return fmt.Sprintf("%d:%s:%s", b.GroupID, NormalizePlatform(b.Platform), b.Mode)
}

func ParseSchedulerBucket(raw string) (SchedulerBucket, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) == 5 && parts[0] == fmt.Sprintf("v%d", SchedulerBucketSchemaVersion) {
		groupID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return SchedulerBucket{}, false
		}
		protocol := NormalizeEndpointProtocol(EndpointProtocol(parts[2]))
		if !IsValidEndpointProtocol(protocol) {
			return SchedulerBucket{}, false
		}
		forcedPlatform := parts[3]
		if forcedPlatform == "-" {
			forcedPlatform = ""
		}
		bucket, ok := NewSchedulerBucket(groupID, protocol, forcedPlatform)
		if !ok || bucket.Mode != parts[4] {
			return SchedulerBucket{}, false
		}
		return bucket, true
	}
	if len(parts) != 3 {
		return SchedulerBucket{}, false
	}
	groupID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return SchedulerBucket{}, false
	}
	platform := NormalizePlatform(parts[1])
	if platform == "" || parts[2] == "" {
		return SchedulerBucket{}, false
	}
	return SchedulerBucket{
		GroupID:  groupID,
		Platform: platform,
		Mode:     parts[2],
	}, true
}

// SchedulerCache 负责调度快照与账号快照的缓存读写。
type SchedulerCache interface {
	// GetSnapshot 读取快照并返回命中与否（ready + active + 数据完整）。
	GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error)
	// CaptureBucketWriteToken captures the current open epoch without changing
	// retirement state. A tombstoned bucket returns ErrSchedulerBucketRetired.
	CaptureBucketWriteToken(ctx context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error)
	// SetSnapshot 写入快照并切换激活版本。token 必须在 DB load/任务排队前取得。
	SetSnapshot(ctx context.Context, bucket SchedulerBucket, token SchedulerBucketWriteToken, accounts []Account) error
	// RetireBucket persistently tombstones a bucket and fences every older writer.
	// Readers that captured the active version before retirement may finish; new
	// readers observe ready/active as absent.
	RetireBucket(ctx context.Context, bucket SchedulerBucket) error
	// ReopenBucket is the only operation allowed to clear a tombstone. It returns
	// the retirement generation established by RetireBucket; repeated calls for
	// the same generation are idempotent. Callers must serialize a fresh authority
	// check through ReopenBucket with RetireBucket under the same group lifecycle
	// lease; ordinary rebuild paths never call ReopenBucket.
	ReopenBucket(ctx context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error)
	// TryAcquireGroupLifecycleLease serializes authoritative retirement/reopen
	// decisions for one non-zero group across instances.
	TryAcquireGroupLifecycleLease(ctx context.Context, groupID int64, ttl time.Duration) (SchedulerGroupLifecycleLease, bool, error)
	// ReleaseGroupLifecycleLease releases the lease only if its owner token still
	// matches, so an expired holder cannot delete a successor's lease. Missing,
	// expired, mismatched, and already released leases return
	// ErrSchedulerGroupLifecycleLeaseLost.
	ReleaseGroupLifecycleLease(ctx context.Context, lease SchedulerGroupLifecycleLease) error
	// GetAccount 获取单账号快照。
	GetAccount(ctx context.Context, accountID int64) (*Account, error)
	// SetAccount 写入单账号快照（包含不可调度状态）。
	SetAccount(ctx context.Context, account *Account) error
	// DeleteAccount 删除单账号快照。
	DeleteAccount(ctx context.Context, accountID int64) error
	// UpdateLastUsed 批量更新账号的最后使用时间。
	UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	// TryLockBucket 尝试获取分桶重建锁。
	TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error)
	// UnlockBucket 释放分桶重建锁。
	UnlockBucket(ctx context.Context, bucket SchedulerBucket) error
	// ListBuckets 返回已注册的分桶集合。
	ListBuckets(ctx context.Context) ([]SchedulerBucket, error)
	// GetOutboxWatermark 读取 outbox 水位。
	GetOutboxWatermark(ctx context.Context) (int64, error)
	// SetOutboxWatermark 保存 outbox 水位。
	SetOutboxWatermark(ctx context.Context, id int64) error
}
