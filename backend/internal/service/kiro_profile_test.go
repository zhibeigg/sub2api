package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

type kiroProfileRepoStub struct {
	updateCalls int
	updatedID   int64
	credentials map[string]any
}

func (r *kiroProfileRepoStub) Update(_ context.Context, account *Account) error {
	r.updateCalls++
	r.updatedID = account.ID
	r.credentials = shallowCopyMap(account.Credentials)
	return nil
}

func (r *kiroProfileRepoStub) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	r.updateCalls++
	r.updatedID = id
	r.credentials = shallowCopyMap(credentials)
	return nil
}

func TestPersistKiroProfileArnPreservesCredentials(t *testing.T) {
	repo := &kiroProfileRepoStub{}
	account := &Account{
		ID: 143,
		Credentials: map[string]any{
			"access_token":  "access",
			"refresh_token": "refresh",
			"model_mapping": map[string]any{"claude-opus-4-8": "claude-opus-4.8"},
		},
	}

	profileArn := "arn:aws:codewhisperer:eu-central-1:123456789012:profile/test"
	if err := persistKiroProfileArn(context.Background(), repo, account, profileArn); err != nil {
		t.Fatalf("persist profile ARN: %v", err)
	}
	if repo.updateCalls != 1 || repo.updatedID != account.ID {
		t.Fatalf("unexpected persistence calls=%d id=%d", repo.updateCalls, repo.updatedID)
	}
	if got := repo.credentials["profile_arn"]; got != profileArn {
		t.Fatalf("profile_arn=%v, want %s", got, profileArn)
	}
	if repo.credentials["access_token"] != "access" || repo.credentials["refresh_token"] != "refresh" {
		t.Fatalf("existing tokens were not preserved: %#v", repo.credentials)
	}
	if _, mutated := account.Credentials["profile_arn"]; mutated {
		t.Fatal("persisting profile ARN must not mutate the shared account instance")
	}
}

func TestKiroGatewayPrepareCredentialResolvesAndCachesProfileArn(t *testing.T) {
	var calls atomic.Int32
	profileArn := "arn:aws:codewhisperer:eu-central-1:123456789012:profile/test"
	svc := &KiroGatewayService{
		resolveProfileArn: func(_ context.Context, cred *kiro.Credential) (string, error) {
			calls.Add(1)
			cred.ProfileArn = profileArn
			return profileArn, nil
		},
	}
	account := &Account{ID: 143, Credentials: map[string]any{"region": "eu-central-1"}}

	first := svc.prepareCredential(context.Background(), account, "token-1")
	if first.ProfileArn != profileArn {
		t.Fatalf("first profile ARN=%q", first.ProfileArn)
	}
	second := svc.prepareCredential(context.Background(), account, "token-2")
	if second.ProfileArn != profileArn {
		t.Fatalf("cached profile ARN=%q", second.ProfileArn)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls=%d, want 1", got)
	}
}

func TestKiroGatewayProfileCacheChangesWithReauthentication(t *testing.T) {
	var calls atomic.Int32
	svc := &KiroGatewayService{
		resolveProfileArn: func(_ context.Context, cred *kiro.Credential) (string, error) {
			calls.Add(1)
			return "arn:aws:codewhisperer:eu-central-1:123456789012:profile/" + cred.ClientID, nil
		},
	}
	firstAccount := &Account{ID: 143, Credentials: map[string]any{"region": "eu-central-1", "client_id": "client-a"}}
	secondAccount := &Account{ID: 143, Credentials: map[string]any{"region": "eu-central-1", "client_id": "client-b"}}

	first := svc.prepareCredential(context.Background(), firstAccount, "token-a")
	second := svc.prepareCredential(context.Background(), secondAccount, "token-b")
	if first.ProfileArn == second.ProfileArn {
		t.Fatalf("profile cache was reused across reauthentication: %q", first.ProfileArn)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("resolver calls=%d, want 2", got)
	}
}

func TestKiroGatewayProfileResolutionUsesSingleflight(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	profileArn := "arn:aws:codewhisperer:eu-central-1:123456789012:profile/test"
	svc := &KiroGatewayService{
		resolveProfileArn: func(_ context.Context, _ *kiro.Credential) (string, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			return profileArn, nil
		},
	}
	account := &Account{ID: 143, Credentials: map[string]any{"region": "eu-central-1"}}

	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cred := svc.prepareCredential(context.Background(), account, "token")
			results <- cred.ProfileArn
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)

	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls=%d, want 1", got)
	}
	for got := range results {
		if got != profileArn {
			t.Fatalf("resolved profile ARN=%q", got)
		}
	}
}

func TestKiroGatewayProfileResolutionFailureKeepsRegionFallback(t *testing.T) {
	svc := &KiroGatewayService{
		resolveProfileArn: func(_ context.Context, _ *kiro.Credential) (string, error) {
			return "", errors.New("profile lookup unavailable")
		},
	}
	account := &Account{ID: 143, Credentials: map[string]any{"region": "eu-central-1"}}

	cred := svc.prepareCredential(context.Background(), account, "token")
	if cred.ProfileArn != "" {
		t.Fatalf("profile ARN=%q, want empty", cred.ProfileArn)
	}
	if cred.Region != "eu-central-1" {
		t.Fatalf("region=%q, want eu-central-1", cred.Region)
	}
}
