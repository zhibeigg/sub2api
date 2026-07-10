package service

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newAdobeVideoTaskForTest(t *testing.T) *AdobeVideoTask {
	t.Helper()
	snapshot := AdobeMediaPricingSnapshot{
		Version: adobeMediaSnapshotVersion, Platform: adobePlatformName, BillingMode: string(BillingModeVideo),
		Tier: VideoBillingResolution720P, Unit: AdobeMediaUnitSecond, Quantity: 5,
		GroupID: 2, PriceSource: "group", UnitPrice: 0.1, RequestedModel: "veo3",
		UpstreamModel: "firefly-video", GroupMultiplier: 1, PeakMultiplier: 1,
		MediaMultiplier: 1, AccountMultiplier: 1, SubscriptionMultiplier: 1,
		BaseCost: 0.5, ActualCost: 0.5, QuotaCost: 0.5, AccountQuotaCost: 0.5,
	}
	require.NoError(t, snapshot.Seal())
	return &AdobeVideoTask{
		TaskID: "firefly-task/raw:id", PollURL: "https://firefly-3p.ff.adobe.io/v2/status/firefly-task/raw:id",
		AccountID: 10, UserID: 11, APIKeyID: 12, GroupID: 2,
		RequestedModel: "veo3", UpstreamModel: "firefly-video", Resolution: VideoBillingResolution720P,
		DurationSeconds: 5, PricingSnapshot: snapshot, SnapshotHash: snapshot.Hash,
		Status: AdobeVideoTaskPending, SettlementStatus: AdobeVideoSettlementPending,
	}
}

func TestAdobeVideoTaskStore_PreservesFullURLAndRawTaskID(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewAdobeVideoTaskStore(client, 72*time.Hour, 24*time.Hour)
	task := newAdobeVideoTaskForTest(t)

	require.NoError(t, store.Create(context.Background(), task))
	loaded, err := store.Get(context.Background(), task.TaskID)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, loaded.TaskID)
	require.Equal(t, task.PollURL, loaded.PollURL)
	require.Equal(t, 72*time.Hour, server.TTL(adobeVideoTaskKey(task.TaskID)))
}

func TestAdobeVideoTaskStore_AbsoluteActiveAndTerminalTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewAdobeVideoTaskStore(client, 72*time.Hour, 24*time.Hour)
	task := newAdobeVideoTaskForTest(t)
	require.NoError(t, store.Create(context.Background(), task))

	server.FastForward(time.Hour)
	task.Status = AdobeVideoTaskProcessing
	require.NoError(t, store.Update(context.Background(), task))
	require.Equal(t, 71*time.Hour, server.TTL(adobeVideoTaskKey(task.TaskID)), "active polls must not slide the absolute TTL")

	task.Status = AdobeVideoTaskCompleted
	task.ResultURLs = []string{"https://storage.adobe.io/result.mp4"}
	require.NoError(t, store.Update(context.Background(), task))
	require.Equal(t, 24*time.Hour, server.TTL(adobeVideoTaskKey(task.TaskID)))

	server.FastForward(time.Hour)
	task.SettlementStatus = AdobeVideoSettlementSettled
	require.NoError(t, store.Update(context.Background(), task))
	require.Equal(t, 23*time.Hour, server.TTL(adobeVideoTaskKey(task.TaskID)), "settlement updates must not slide terminal TTL")

	conflict := *task
	conflict.AccountID++
	require.ErrorIs(t, store.Update(context.Background(), &conflict), ErrAdobeVideoTaskImmutableConflict)
}

func TestAdobeVideoTask_RejectsUntrustedOrNonStatusPollURL(t *testing.T) {
	task := newAdobeVideoTaskForTest(t)
	for _, pollURL := range []string{
		"https://127.0.0.1/v2/status/task",
		"https://evil.ff.adobe.io/v2/status/task",
		"https://firefly-3p.ff.adobe.io/profile",
		"https://firefly-3p.ff.adobe.io/v2/storage/image",
	} {
		task.PollURL = pollURL
		require.Error(t, task.Validate(), pollURL)
	}
}

func TestAdobeVideoTaskStore_SettlementLock(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewAdobeVideoTaskStore(client, 72*time.Hour, 24*time.Hour)
	unlock, err := store.AcquireSettlementLock(context.Background(), "task-1", time.Minute)
	require.NoError(t, err)
	_, err = store.AcquireSettlementLock(context.Background(), "task-1", time.Minute)
	require.ErrorIs(t, err, ErrAdobeVideoTaskSettlementLocked)
	require.NoError(t, unlock(context.Background()))
	_, err = store.AcquireSettlementLock(context.Background(), "task-1", time.Minute)
	require.NoError(t, err)
}
