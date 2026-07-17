package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	weComOAuthAccessTokenKeyPrefix = "wecom:oauth:access_token:v1:"
	weComOAuthJSAPITicketKeyPrefix = "wecom:oauth:jsapi_ticket:v1:"
	weComOAuthRedisMaxTTL          = 2 * time.Hour
)

type weComOAuthRedisCache struct {
	rdb *redis.Client
}

var _ service.WeComOAuthCache = (*weComOAuthRedisCache)(nil)

// NewWeComOAuthCache 创建企业微信 OAuth 专用 Redis 缓存。
// 缓存键与 GeminiTokenCache 完全隔离，且 TTL 始终被限制为短期值。
func NewWeComOAuthCache(rdb *redis.Client) service.WeComOAuthCache {
	return &weComOAuthRedisCache{rdb: rdb}
}

func (c *weComOAuthRedisCache) GetAccessToken(ctx context.Context, scope string) (string, bool, error) {
	return c.get(ctx, weComOAuthAccessTokenKeyPrefix, scope)
}

func (c *weComOAuthRedisCache) SetAccessToken(ctx context.Context, scope, value string, ttl time.Duration) error {
	return c.set(ctx, weComOAuthAccessTokenKeyPrefix, scope, value, ttl)
}

func (c *weComOAuthRedisCache) DeleteAccessToken(ctx context.Context, scope string) error {
	return c.delete(ctx, weComOAuthAccessTokenKeyPrefix, scope)
}

func (c *weComOAuthRedisCache) GetJSAPITicket(ctx context.Context, scope string) (string, bool, error) {
	return c.get(ctx, weComOAuthJSAPITicketKeyPrefix, scope)
}

func (c *weComOAuthRedisCache) SetJSAPITicket(ctx context.Context, scope, value string, ttl time.Duration) error {
	return c.set(ctx, weComOAuthJSAPITicketKeyPrefix, scope, value, ttl)
}

func (c *weComOAuthRedisCache) DeleteJSAPITicket(ctx context.Context, scope string) error {
	return c.delete(ctx, weComOAuthJSAPITicketKeyPrefix, scope)
}

func (c *weComOAuthRedisCache) get(ctx context.Context, prefix, scope string) (string, bool, error) {
	key, err := c.key(prefix, scope)
	if err != nil {
		return "", false, err
	}
	value, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (c *weComOAuthRedisCache) set(ctx context.Context, prefix, scope, value string, ttl time.Duration) error {
	key, err := c.key(prefix, scope)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("enterprise WeChat cache value is empty")
	}
	if ttl <= 0 {
		return errors.New("enterprise WeChat cache TTL must be positive")
	}
	if ttl > weComOAuthRedisMaxTTL {
		ttl = weComOAuthRedisMaxTTL
	}
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *weComOAuthRedisCache) delete(ctx context.Context, prefix, scope string) error {
	key, err := c.key(prefix, scope)
	if err != nil {
		return err
	}
	return c.rdb.Del(ctx, key).Err()
}

func (c *weComOAuthRedisCache) key(prefix, scope string) (string, error) {
	if c == nil || c.rdb == nil {
		return "", errors.New("enterprise WeChat Redis cache is not configured")
	}
	scope = strings.TrimSpace(scope)
	if scope == "" || len(scope) > 256 {
		return "", errors.New("enterprise WeChat cache scope is invalid")
	}
	return prefix + scope, nil
}
