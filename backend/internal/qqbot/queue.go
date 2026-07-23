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
	mediaFileInfoTTL    = 24 * time.Hour
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

var idempotentRateLimitScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
  local ttl = redis.call('PTTL', KEYS[2])
  return {1, ttl}
end
local count = redis.call('INCR', KEYS[2])
if count == 1 then
  redis.call('PEXPIRE', KEYS[2], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[2])
if count <= tonumber(ARGV[2]) then
  redis.call('SET', KEYS[1], '1', 'PX', ARGV[3])
  return {1, ttl}
end
return {0, ttl}
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
	group     string
}

type OneBotQueue struct{ *ReliableQueue }

func NewReliableQueue(redisClient *redis.Client, encryptor service.SecretEncryptor) *ReliableQueue {
	return newReliableQueue(redisClient, encryptor, "sub2api:qqbot", queueGroup)
}

func NewOneBotQueue(redisClient *redis.Client, encryptor service.SecretEncryptor) *OneBotQueue {
	return &OneBotQueue{ReliableQueue: newReliableQueue(redisClient, encryptor, "sub2api:qqbot:onebot", queueGroup+"-onebot")}
}

func newReliableQueue(redisClient *redis.Client, encryptor service.SecretEncryptor, prefix, group string) *ReliableQueue {
	return &ReliableQueue{redis: redisClient, encryptor: encryptor, prefix: strings.TrimRight(prefix, ":"), group: group}
}

func (q *ReliableQueue) consumerGroup() string {
	if q != nil && strings.TrimSpace(q.group) != "" {
		return q.group
	}
	return queueGroup
}

func (q *ReliableQueue) stream() string { return q.prefix + ":events" }
func (q *ReliableQueue) dead() string   { return q.prefix + ":events:dead" }

func (q *ReliableQueue) EnsureGroup(ctx context.Context) error {
	if q == nil || q.redis == nil {
		return errors.New("qqbot redis queue unavailable")
	}
	err := q.redis.XGroupCreateMkStream(ctx, q.stream(), q.consumerGroup(), "0").Err()
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
	streams, err := q.redis.XReadGroup(ctx, &redis.XReadGroupArgs{Group: q.consumerGroup(), Consumer: consumer, Streams: []string{q.stream(), ">"}, Count: count, Block: block}).Result()
	if err != nil {
		return nil, err
	}
	return q.decodeStreams(streams)
}

func (q *ReliableQueue) Claim(ctx context.Context, consumer string, minIdle time.Duration, count int64) ([]QueuedEvent, error) {
	messages, _, err := q.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: q.stream(), Group: q.consumerGroup(), Consumer: consumer, MinIdle: minIdle, Start: "0-0", Count: count}).Result()
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
	pipe.XAck(ctx, q.stream(), q.consumerGroup(), id)
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
	pipe.XAck(ctx, q.stream(), q.consumerGroup(), item.ID)
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

func (q *ReliableQueue) AllowOnce(ctx context.Context, scope, token string, limit int64, window time.Duration) (bool, time.Duration, error) {
	if q == nil || q.redis == nil || strings.TrimSpace(token) == "" || limit < 1 || window <= 0 {
		return false, 0, errors.New("qqbot idempotent rate limiter unavailable")
	}
	keys := []string{
		q.prefix + ":limit:accepted:" + Fingerprint(scope+":"+token),
		q.prefix + ":limit:" + Fingerprint(scope),
	}
	values, err := idempotentRateLimitScript.Run(ctx, q.redis, keys, window.Milliseconds(), limit, eventTTL.Milliseconds()).Int64Slice()
	if err != nil || len(values) != 2 {
		if err == nil {
			err = errors.New("qqbot idempotent rate limiter returned an invalid response")
		}
		return false, 0, err
	}
	ttl := time.Duration(values[1]) * time.Millisecond
	if ttl < 0 {
		ttl = window
	}
	return values[0] == 1, ttl, nil
}

func (q *ReliableQueue) GetMediaFileInfo(ctx context.Context, key string) (string, bool, error) {
	if q == nil || q.redis == nil || q.encryptor == nil || strings.TrimSpace(key) == "" {
		return "", false, errors.New("qqbot media store unavailable")
	}
	ciphertext, err := q.redis.Get(ctx, q.prefix+":media:"+Fingerprint(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, err := q.encryptor.Decrypt(ciphertext)
	if err != nil {
		return "", false, fmt.Errorf("decrypt qqbot media file info: %w", err)
	}
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", false, nil
	}
	return plain, true, nil
}

func (q *ReliableQueue) SetMediaFileInfo(ctx context.Context, key, fileInfo string) error {
	if q == nil || q.redis == nil || q.encryptor == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(fileInfo) == "" {
		return errors.New("qqbot media store unavailable")
	}
	ciphertext, err := q.encryptor.Encrypt(strings.TrimSpace(fileInfo))
	if err != nil {
		return fmt.Errorf("encrypt qqbot media file info: %w", err)
	}
	return q.redis.Set(ctx, q.prefix+":media:"+Fingerprint(key), ciphertext, mediaFileInfoTTL).Err()
}

func (q *ReliableQueue) Stats(ctx context.Context) (int64, int64, int64) {
	if q == nil || q.redis == nil {
		return 0, 0, 0
	}
	backlog, _ := q.redis.XLen(ctx, q.stream()).Result()
	pending := int64(0)
	if value, err := q.redis.XPending(ctx, q.stream(), q.consumerGroup()).Result(); err == nil && value != nil {
		pending = value.Count
	}
	dead, _ := q.redis.XLen(ctx, q.dead()).Result()
	return backlog, pending, dead
}
