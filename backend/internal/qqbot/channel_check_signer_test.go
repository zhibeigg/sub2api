package qqbot

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestChannelCheckSignerIssuesAndVerifiesShortLivedURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.Totp.EncryptionKey = strings.Repeat("11", 32)
	cfg.Totp.EncryptionKeyConfigured = true
	signer, err := NewChannelCheckSigner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return now }

	issued, err := signer.IssueURL("https://status.example.com", "app")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(issued)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "status.example.com" || parsed.Path != ChannelCheckImagePath {
		t.Fatalf("unexpected signed URL: %s", issued)
	}
	query := parsed.Query()
	if err := signer.Verify("app", query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig")); err != nil {
		t.Fatalf("verify signed URL: %v", err)
	}

	if err := signer.Verify("app", query.Get("v"), query.Get("exp"), query.Get("nonce")+"x", query.Get("sig")); !errors.Is(err, ErrInvalidChannelCheckSignature) {
		t.Fatalf("tampered nonce accepted: %v", err)
	}
	signer.now = func() time.Time { return now.Add(channelCheckURLTTL + time.Second) }
	if err := signer.Verify("app", query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig")); !errors.Is(err, ErrInvalidChannelCheckSignature) {
		t.Fatalf("expired URL accepted: %v", err)
	}
}

func TestChannelCheckSignerRequiresStableRootKey(t *testing.T) {
	unconfigured := &config.Config{}
	unconfigured.Totp.EncryptionKey = strings.Repeat("aa", 32)
	unconfigured.Totp.EncryptionKeyConfigured = false
	signer, err := NewChannelCheckSigner(unconfigured)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signer.IssueURL("https://status.example.com", "app"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unconfigured root key was accepted: %v", err)
	}

	configuredA := &config.Config{}
	configuredA.Totp.EncryptionKey = strings.Repeat("bb", 32)
	configuredA.Totp.EncryptionKeyConfigured = true
	configuredB := &config.Config{}
	configuredB.Totp.EncryptionKey = configuredA.Totp.EncryptionKey
	configuredB.Totp.EncryptionKeyConfigured = true
	signerA, err := NewChannelCheckSigner(configuredA)
	if err != nil {
		t.Fatal(err)
	}
	signerB, err := NewChannelCheckSigner(configuredB)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	signerA.now = func() time.Time { return now }
	signerB.now = func() time.Time { return now }
	issued, err := signerA.IssueURL("https://status.example.com", "app")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(issued)
	query := parsed.Query()
	if err := signerB.Verify("app", query.Get("v"), query.Get("exp"), query.Get("nonce"), query.Get("sig")); err != nil {
		t.Fatalf("second instance rejected shared root key: %v", err)
	}
}

func TestChannelCheckSignerRejectsUnsafeConfiguration(t *testing.T) {
	cfg := &config.Config{}
	cfg.Totp.EncryptionKey = "bad-key"
	cfg.Totp.EncryptionKeyConfigured = true
	if _, err := NewChannelCheckSigner(cfg); err == nil {
		t.Fatal("invalid signing root key accepted")
	}

	cfg.Totp.EncryptionKey = strings.Repeat("22", 32)
	signer, err := NewChannelCheckSigner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, baseURL := range []string{
		"http://status.example.com",
		"https://user:pass@status.example.com",
		"https://status.example.com?token=secret",
		"https://status.example.com/proxy",
		"https://localhost",
		"https://127.0.0.1",
		"https://10.0.0.1",
		"https://status.internal",
		"https://singlelabel",
		"//status.example.com",
	} {
		if _, err := signer.IssueURL(baseURL, "app"); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("unsafe base URL %q accepted: %v", baseURL, err)
		}
	}
}
