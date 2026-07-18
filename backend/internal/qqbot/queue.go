package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	queueGroup          = "qqbot-runtime"
	eventTTL            = 24 * time.Hour
	welcomeTTL          = 180 * 24 * time.Hour
	maxDeliveryAttempts = 5
)

var enqueueScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return '' end
local id = redis.call('XADD', KEYS[2], 'MAXLEN', '~', ARGV[3], '*', 'payload', ARGV[2])
redis.call('SET', KEYS[1], '1', 'PX', ARGV[1])
return id
`)

var beginOnceScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
if redis.call('SET', KEYS[2], '1', 'NX', 'PX', ARGV[1]) then return 1 end
return 0
`)

var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
return {count, ttl}
`)

type QueuedEvent struct {
	ID          string
	Event       InboundEvent
	Payload     string
	DecodeError string
}

type ReliableQueue struct {
	redis     *redis.Client
	encryptor service.SecretEncryptor
	prefix    string
}

func NewReliableQueue(redisClient *redis.Client, encryptor service.SecretEncryptor) *ReliableQueue {
	return &ReliableQueue{redis: redisClient, encryptor: encryptor, prefix: "sub2api:qqbot"}
}

func (q *ReliableQueue) stream() string { return q.prefix + ":events" }
func (q *ReliableQueue) dead() string   { return q.prefix + ":events:dead" }

func (q *ReliableQueue) EnsureGroup(ctx context.Context) error {
	if q == nil || q.redis == nil {
		return errors.New("qqbot redis queue unavailable")
	}
	err := q.redis.XGroupCreateMkStream(ctx, q.stream(), queueGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (q *ReliableQueue) Enqueue(ctx context.Context, event InboundEvent, capacity int) error {
	if q == nil || q.redis == nil || q.encryptor == nil {
		return errors.New("qqbot redis queue unavailable")
	}
	if strings.TrimSpace(event.EventID) == "" {
		return errors.New("qqbot event id is required")
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	ciphertext, err := q.encryptor.Encrypt(string(raw))
	if err != nil {
		return fmt.Errorf("encrypt qqbot event: %w", err)
	}
	if capacity < 16 {
		capacity = DefaultQueueCapacity
	}
	_, err = enqueueScript.Run(ctx, q.redis, []string{q.prefix + ":event:" + Fingerprint(event.EventID), q.stream()}, eventTTL.Milliseconds(), ciphertext, capacity).Text()
	return err
}

func (q *ReliableQueue) Read(ctx context.Context, consumer string, count int64, block time.Duration) ([]QueuedEvent, error) {
	streams, err := q.redis.XReadGroup(ctx, &redis.XReadGroupArgs{Group: queueGroup, Consumer: consumer, Streams: []string{q.stream(), ">"}, Count: count, Block: block}).Result()
	if err != nil {
		return nil, err
	}
	return q.decodeStreams(streams)
}

func (q *ReliableQueue) Claim(ctx context.Context, consumer string, minIdle time.Duration, count int64) ([]QueuedEvent, error) {
	messages, _, err := q.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: q.stream(), Group: queueGroup, Consumer: consumer, MinIdle: minIdle, Start: "0-0", Count: count}).Result()
	if err != nil {
		return nil, err
	}
	return q.decodeMessages(messages)
}

func (q *ReliableQueue) decodeStreams(streams []redis.XStream) ([]QueuedEvent, error) {
	var messages []redis.XMessage
	for _, stream := range streams {
		messages = append(messages, stream.Messages...)
	}
	return q.decodeMessages(messages)
}

func (q *ReliableQueue) decodeMessages(messages []redis.XMessage) ([]QueuedEvent, error) {
	result := make([]QueuedEvent, 0, len(messages))
	for _, message := range messages {
		payload, ok := message.Values["payload"].(string)
		if !ok || payload == "" {
			result = append(result, QueuedEvent{ID: message.ID, DecodeError: "queue_payload_missing"})
			continue
		}
		plain, err := q.encryptor.Decrypt(payload)
		if err != nil {
			result = append(result, QueuedEvent{ID: message.ID, Payload: payload, DecodeError: "queue_payload_decrypt_failed"})
			continue
		}
		var event InboundEvent
		if err := json.Unmarshal([]byte(plain), &event); err != nil {
			result = append(result, QueuedEvent{ID: message.ID, Payload: payload, DecodeError: "queue_payload_decode_failed"})
			continue
		}
		result = append(result, QueuedEvent{ID: message.ID, Event: event, Payload: payload})
	}
	return result, nil
}

func (q *ReliableQueue) Ack(ctx context.Context, id string) error {
	pipe := q.redis.TxPipeline()
	pipe.XAck(ctx, q.stream(), queueGroup, id)
	pipe.XDel(ctx, q.stream(), id)
	pipe.Del(ctx, q.prefix+":retry:"+id)
	_, err := pipe.Exec(ctx)
	return err
}

func (q *ReliableQueue) Fail(ctx context.Context, item QueuedEvent, errorCode string) error {
	retryKey := q.prefix + ":retry:" + item.ID
	attempts, err := q.redis.Incr(ctx, retryKey).Result()
	if err != nil {
		return err
	}
	_ = q.redis.Expire(ctx, retryKey, eventTTL).Err()
	if attempts < maxDeliveryAttempts {
		return nil
	}
	return q.moveToDeadLetter(ctx, item, errorCode, attempts)
}

func (q *ReliableQueue) DeadLetter(ctx context.Context, item QueuedEvent, errorCode string) error {
	return q.moveToDeadLetter(ctx, item, errorCode, 1)
}

func (q *ReliableQueue) moveToDeadLetter(ctx context.Context, item QueuedEvent, errorCode string, attempts int64) error {
	pipe := q.redis.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: q.dead(), MaxLen: 10000, Approx: true, Values: map[string]any{"payload": item.Payload, "source_id": item.ID, "error_code": errorCode, "attempts": attempts, "failed_at": time.Now().UTC().Format(time.RFC3339Nano)}})
	pipe.XAck(ctx, q.stream(), queueGroup, item.ID)
	pipe.XDel(ctx, q.stream(), item.ID)
	pipe.Del(ctx, q.prefix+":retry:"+item.ID)
	_, err := pipe.Exec(ctx)
	return err
}

func (q *ReliableQueue) BeginWelcome(ctx context.Context, key string) (bool, error) {
	fingerprint := Fingerprint(key)
	value, err := beginOnceScript.Run(ctx, q.redis, []string{q.prefix + ":welcome:done:" + fingerprint, q.prefix + ":welcome:lock:" + fingerprint}, (60 * time.Second).Milliseconds()).Int()
	return value == 1, err
}
func (q *ReliableQueue) FinishWelcome(ctx context.Context, key string, success bool) error {
	fingerprint := Fingerprint(key)
	lockKey := q.prefix + ":welcome:lock:" + fingerprint
	if !success {
		return q.redis.Del(ctx, lockKey).Err()
	}
	pipe := q.redis.TxPipeline()
	pipe.Set(ctx, q.prefix+":welcome:done:"+fingerprint, "1", welcomeTTL)
	pipe.Del(ctx, lockKey)
	_, err := pipe.Exec(ctx)
	return err
}

func (q *ReliableQueue) Allow(ctx context.Context, scope string, limit int64, window time.Duration) (bool, time.Duration, error) {
	if q == nil || q.redis == nil || limit < 1 || window <= 0 {
		return false, 0, errors.New("qqbot rate limiter unavailable")
	}
	key := q.prefix + ":limit:" + Fingerprint(scope)
	values, err := rateLimitScript.Run(ctx, q.redis, []string{key}, window.Milliseconds()).Int64Slice()
	if err != nil || len(values) != 2 {
		if err == nil {
			err = errors.New("qqbot rate limiter returned an invalid response")
		}
		return false, 0, err
	}
	ttl := time.Duration(values[1]) * time.Millisecond
	if ttl < 0 {
		ttl = window
	}
	return values[0] <= limit, ttl, nil
}

func (q *ReliableQueue) Stats(ctx context.Context) (int64, int64, int64) {
	if q == nil || q.redis == nil {
		return 0, 0, 0
	}
	backlog, _ := q.redis.XLen(ctx, q.stream()).Result()
	pending := int64(0)
	if value, err := q.redis.XPending(ctx, q.stream(), queueGroup).Result(); err == nil && value != nil {
		pending = value.Count
	}
	dead, _ := q.redis.XLen(ctx, q.dead()).Result()
	return backlog, pending, dead
}
