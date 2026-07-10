package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/firefly"
)

type AdobeFireflyClientFactory interface {
	ForAccount(account *Account) *firefly.Client
}

type adobeFireflyClientFactory struct{ cfg *config.Config }

func NewAdobeFireflyClientFactory(cfg *config.Config) AdobeFireflyClientFactory {
	return &adobeFireflyClientFactory{cfg: cfg}
}

func (f *adobeFireflyClientFactory) ForAccount(account *Account) *firefly.Client {
	proxyURL := ""
	if account != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	clientCfg := firefly.ClientConfig{ProxyURL: proxyURL}
	if f != nil && f.cfg != nil {
		if f.cfg.Adobe.RequestTimeoutSeconds > 0 {
			clientCfg.RequestTimeout = time.Duration(f.cfg.Adobe.RequestTimeoutSeconds) * time.Second
		}
		if f.cfg.Adobe.ImagePollIntervalSeconds > 0 {
			clientCfg.PollInterval = time.Duration(f.cfg.Adobe.ImagePollIntervalSeconds) * time.Second
		}
		if f.cfg.Adobe.ImageMaxPollAttempts > 0 {
			clientCfg.MaxPollAttempts = f.cfg.Adobe.ImageMaxPollAttempts
		}
	}
	return firefly.NewClient(clientCfg)
}

type AdobeFireflyAdapter struct {
	factory AdobeFireflyClientFactory
	tokens  *AdobeTokenProvider
}

func NewAdobeFireflyAdapter(factory AdobeFireflyClientFactory, tokens *AdobeTokenProvider) *AdobeFireflyAdapter {
	return &AdobeFireflyAdapter{factory: factory, tokens: tokens}
}

func (a *AdobeFireflyAdapter) SubmitVideo(ctx context.Context, account *Account, request AdobeVideoSubmitRequest) (*AdobeVideoSubmitResult, error) {
	params, err := firefly.ResolveVideoModel(request.Model, request.Resolution, request.DurationSeconds, request.AspectRatio, &request.GenerateAudio, request.ReferenceMode)
	if err != nil {
		return nil, err
	}
	var result *firefly.SubmitResult
	err = a.withAuthRetry(ctx, account, func(client *firefly.Client, token string) error {
		var callErr error
		result, callErr = client.SubmitVideo(ctx, token, params, request.Prompt, request.ReferenceAssets)
		return callErr
	})
	if err != nil {
		return nil, err
	}
	return &AdobeVideoSubmitResult{TaskID: result.TaskID, PollURL: result.StatusURL, Status: AdobeVideoTaskPending}, nil
}

func (a *AdobeFireflyAdapter) PollVideo(ctx context.Context, account *Account, pollURL string) (*AdobeVideoPollResult, error) {
	var result *firefly.PollResult
	err := a.withAuthRetry(ctx, account, func(client *firefly.Client, token string) error {
		var callErr error
		result, callErr = client.Poll(ctx, token, pollURL)
		return callErr
	})
	if err != nil {
		var providerErr *firefly.ProviderError
		if result != nil && normalizeAdobeVideoStatus(result.Status, result.OutputURL) == AdobeVideoTaskFailed {
			message := result.ErrorMessage
			if errors.As(err, &providerErr) && message == "" {
				message = providerErr.Message
			}
			return &AdobeVideoPollResult{Status: AdobeVideoTaskFailed, ErrorMessage: message}, nil
		}
		if errors.As(err, &providerErr) && providerErr.Kind == firefly.ErrorRequest && strings.Contains(strings.ToLower(providerErr.Code), "generation") {
			return &AdobeVideoPollResult{Status: AdobeVideoTaskFailed, ErrorMessage: providerErr.Message}, nil
		}
		return nil, err
	}
	status := normalizeAdobeVideoStatus(result.Status, result.OutputURL)
	out := &AdobeVideoPollResult{Status: status, ErrorMessage: result.ErrorMessage}
	if result.OutputURL != "" {
		out.ResultURLs = []string{result.OutputURL}
	}
	return out, nil
}

func (a *AdobeFireflyAdapter) GenerateImage(ctx context.Context, account *Account, params *firefly.ResolvedParams, prompt string, refs []string) (*firefly.ImageResult, error) {
	var result *firefly.ImageResult
	err := a.withAuthRetry(ctx, account, func(client *firefly.Client, token string) error {
		var callErr error
		result, callErr = client.GenerateImage(ctx, token, params, prompt, refs)
		return callErr
	})
	return result, err
}

func (a *AdobeFireflyAdapter) UploadAsset(ctx context.Context, account *Account, name, contentType string, data []byte) (string, error) {
	var assetID string
	err := a.withAuthRetry(ctx, account, func(client *firefly.Client, token string) error {
		var callErr error
		assetID, callErr = client.UploadAsset(ctx, token, name, contentType, data)
		return callErr
	})
	return assetID, err
}

func (a *AdobeFireflyAdapter) withAuthRetry(ctx context.Context, account *Account, call func(*firefly.Client, string) error) error {
	if a == nil || a.factory == nil || a.tokens == nil || account == nil {
		return ErrAdobeMediaUpstreamUnavailable
	}
	client := a.factory.ForAccount(account)
	token, err := a.tokens.GetAccessToken(ctx, account)
	if err != nil {
		return err
	}
	if err = call(client, token); !firefly.IsAuthError(err) {
		return err
	}
	// Frozen rule: 401/403 auth failures force refresh and retry exactly once.
	token, refreshErr := a.tokens.ForceRefresh(ctx, account)
	if refreshErr != nil {
		return refreshErr
	}
	return call(client, token)
}

// HandleAdobeAccountFailure applies a short account-local cooldown only for
// failures that are safe to route around. Request/content/business errors are
// deliberately left untouched so they cannot trigger blind account churn.
func (s *OpenAIGatewayService) HandleAdobeAccountFailure(ctx context.Context, accountID int64, err error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || err == nil || ctx == nil || ctx.Err() != nil {
		return
	}
	var providerErr *firefly.ProviderError
	if !errors.As(err, &providerErr) {
		return
	}
	now := time.Now()
	switch providerErr.Kind {
	case firefly.ErrorRateLimited:
		cooldown := providerErr.RetryAfter
		if cooldown <= 0 {
			cooldown = time.Minute
		}
		_ = s.accountRepo.SetRateLimited(ctx, accountID, now.Add(cooldown))
	case firefly.ErrorAuth:
		_ = s.accountRepo.SetTempUnschedulable(ctx, accountID, now.Add(5*time.Minute), "adobe_auth")
	case firefly.ErrorTemporary, firefly.ErrorProviderBlocked:
		_ = s.accountRepo.SetOverloaded(ctx, accountID, now.Add(30*time.Second))
	}
}

func normalizeAdobeVideoStatus(status, outputURL string) AdobeVideoTaskStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "SUCCEEDED", "SUCCESS":
		return AdobeVideoTaskCompleted
	case "FAILED", "ERROR", "REJECTED":
		return AdobeVideoTaskFailed
	case "CANCELLED", "CANCELED":
		return AdobeVideoTaskCanceled
	case "RUNNING", "PROCESSING", "IN_PROGRESS":
		return AdobeVideoTaskProcessing
	default:
		if strings.TrimSpace(outputURL) != "" {
			return AdobeVideoTaskCompleted
		}
		return AdobeVideoTaskPending
	}
}
