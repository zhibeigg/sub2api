package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	defaultQQBotTimestampToleranceSeconds = 300
	defaultQQBotNonceTTLSeconds           = 600
)

// QQBotIntegrationConfig secures the private API used by the standalone QQBot service.
// The HMAC secret must be injected through QQBOT_INTEGRATION_HMAC_SECRET in production.
type QQBotIntegrationConfig struct {
	Enabled                   bool   `mapstructure:"enabled"`
	KeyID                     string `mapstructure:"key_id"`
	HMACSecret                string `mapstructure:"hmac_secret"`
	PublicBaseURL             string `mapstructure:"public_base_url"`
	TimestampToleranceSeconds int    `mapstructure:"timestamp_tolerance_seconds"`
	NonceTTLSeconds           int    `mapstructure:"nonce_ttl_seconds"`
}

func (c QQBotIntegrationConfig) TimestampTolerance() time.Duration {
	seconds := c.TimestampToleranceSeconds
	if seconds <= 0 {
		seconds = defaultQQBotTimestampToleranceSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (c QQBotIntegrationConfig) NonceTTL() time.Duration {
	seconds := c.NonceTTLSeconds
	if seconds <= 0 {
		seconds = defaultQQBotNonceTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (c *QQBotIntegrationConfig) normalizeAndValidate() error {
	if c == nil {
		return nil
	}
	c.KeyID = strings.TrimSpace(c.KeyID)
	c.HMACSecret = strings.TrimSpace(c.HMACSecret)
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
	if c.TimestampToleranceSeconds <= 0 {
		c.TimestampToleranceSeconds = defaultQQBotTimestampToleranceSeconds
	}
	if c.NonceTTLSeconds <= 0 {
		c.NonceTTLSeconds = defaultQQBotNonceTTLSeconds
	}
	if c.TimestampToleranceSeconds < 30 || c.TimestampToleranceSeconds > 900 {
		return fmt.Errorf("qqbot_integration.timestamp_tolerance_seconds must be between 30 and 900")
	}
	if c.NonceTTLSeconds < c.TimestampToleranceSeconds || c.NonceTTLSeconds > 3600 {
		return fmt.Errorf("qqbot_integration.nonce_ttl_seconds must be between timestamp_tolerance_seconds and 3600")
	}
	if !c.Enabled {
		return nil
	}
	if c.KeyID == "" {
		return fmt.Errorf("qqbot_integration.key_id is required when enabled")
	}
	if len([]byte(c.HMACSecret)) < 32 {
		return fmt.Errorf("qqbot_integration.hmac_secret must be at least 32 bytes when enabled")
	}
	if c.PublicBaseURL == "" {
		return fmt.Errorf("qqbot_integration.public_base_url is required when enabled")
	}
	u, err := url.Parse(c.PublicBaseURL)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("qqbot_integration.public_base_url must be an absolute http(s) URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("qqbot_integration.public_base_url must not contain credentials, query, or fragment")
	}
	return nil
}
