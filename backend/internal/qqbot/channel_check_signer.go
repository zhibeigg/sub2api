package qqbot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	ChannelCheckImagePath        = "/api/v1/public/qqbot/channel-status.png"
	channelCheckSignatureVersion = "1"
	channelCheckURLTTL           = 2 * time.Minute
	channelCheckMaxURLTTL        = 5 * time.Minute
	channelCheckSigningPurpose   = "sub2api/qqbot/channel-check-url/v1"
)

var ErrInvalidChannelCheckSignature = errors.New("invalid qqbot channel check signature")

type ChannelCheckSigner struct {
	rootKey []byte
	now     func() time.Time
}

func NewChannelCheckSigner(cfg *config.Config) (*ChannelCheckSigner, error) {
	if cfg == nil {
		return nil, errors.New("qqbot channel check signer configuration unavailable")
	}
	signer := &ChannelCheckSigner{now: time.Now}
	if !cfg.Totp.EncryptionKeyConfigured {
		return signer, nil
	}
	rootKey, err := hex.DecodeString(strings.TrimSpace(cfg.Totp.EncryptionKey))
	if err != nil {
		return nil, fmt.Errorf("decode qqbot channel check signing key: %w", err)
	}
	if len(rootKey) != 32 {
		return nil, fmt.Errorf("qqbot channel check signing key must be 32 bytes, got %d", len(rootKey))
	}
	signer.rootKey = rootKey
	return signer, nil
}

func (s *ChannelCheckSigner) IssueURL(publicBaseURL, appID string) (string, error) {
	if s == nil {
		return "", ErrInvalidChannelCheckSignature
	}
	base, err := validateChannelCheckPublicBaseURL(publicBaseURL)
	if err != nil {
		return "", err
	}
	signingKey, err := s.signingKey(appID)
	if err != nil {
		return "", err
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("generate qqbot channel check nonce: %w", err)
	}
	now := s.currentTime()
	expires := now.Add(channelCheckURLTTL).Unix()
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	signature := signChannelCheck(signingKey, channelCheckSignatureVersion, expires, nonce)

	base.Path = ChannelCheckImagePath
	query := base.Query()
	query.Set("v", channelCheckSignatureVersion)
	query.Set("exp", strconv.FormatInt(expires, 10))
	query.Set("nonce", nonce)
	query.Set("sig", signature)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (s *ChannelCheckSigner) Verify(appID, version, expiresRaw, nonce, signature string) error {
	if s == nil || version != channelCheckSignatureVersion {
		return ErrInvalidChannelCheckSignature
	}
	if len(expiresRaw) == 0 || len(expiresRaw) > 20 || len(nonce) < 20 || len(nonce) > 64 || len(signature) < 32 || len(signature) > 128 {
		return ErrInvalidChannelCheckSignature
	}
	expires, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil {
		return ErrInvalidChannelCheckSignature
	}
	now := s.currentTime()
	expiresAt := time.Unix(expires, 0)
	if !expiresAt.After(now) || expiresAt.After(now.Add(channelCheckMaxURLTTL)) {
		return ErrInvalidChannelCheckSignature
	}
	signingKey, err := s.signingKey(appID)
	if err != nil {
		return ErrInvalidChannelCheckSignature
	}
	expected := signChannelCheck(signingKey, version, expires, nonce)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrInvalidChannelCheckSignature
	}
	return nil
}

func (s *ChannelCheckSigner) signingKey(appID string) ([]byte, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" || len(s.rootKey) != 32 {
		return nil, ErrInvalidConfig
	}
	derive := hmac.New(sha256.New, s.rootKey)
	_, _ = derive.Write([]byte(channelCheckSigningPurpose + "\napp_id=" + appID))
	return derive.Sum(nil), nil
}

func signChannelCheck(signingKey []byte, version string, expires int64, nonce string) string {
	payload := strings.Join([]string{
		"GET",
		ChannelCheckImagePath,
		"v=" + version,
		"exp=" + strconv.FormatInt(expires, 10),
		"nonce=" + nonce,
	}, "\n")
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validateChannelCheckPublicBaseURL(publicBaseURL string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || !base.IsAbs() || base.Host == "" || base.Scheme != "https" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, ErrInvalidConfig
	}
	if (base.Path != "" && base.Path != "/") || (base.RawPath != "" && base.RawPath != "/") {
		return nil, ErrInvalidConfig
	}
	hostname := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(base.Hostname()), "."))
	if hostname == "" || hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") || strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") {
		return nil, ErrInvalidConfig
	}
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return nil, ErrInvalidConfig
		}
	} else if !strings.Contains(hostname, ".") {
		return nil, ErrInvalidConfig
	}
	base.Path = ""
	base.RawPath = ""
	return base, nil
}

func (s *ChannelCheckSigner) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
