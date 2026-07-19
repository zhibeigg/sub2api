package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	opencodepkg "github.com/Wei-Shaw/sub2api/internal/pkg/opencode"
	"github.com/gin-gonic/gin"
)

const opencodeErrorBodyLimit int64 = 1 << 20

type OpenCodeGatewayService struct {
	httpUpstream HTTPUpstream
	proxyRepo    ProxyRepository
	cfg          *config.Config
}

func NewOpenCodeGatewayService(httpUpstream HTTPUpstream, proxyRepo ProxyRepository, cfg *config.Config) *OpenCodeGatewayService {
	return &OpenCodeGatewayService{httpUpstream: httpUpstream, proxyRepo: proxyRepo, cfg: cfg}
}

func (s *OpenCodeGatewayService) ForwardMessages(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	return s.forward(ctx, c, account, body, opencodepkg.ProtocolMessages)
}

func (s *OpenCodeGatewayService) ForwardChatCompletions(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	return s.forward(ctx, c, account, body, opencodepkg.ProtocolChatCompletions)
}

func (s *OpenCodeGatewayService) ForwardResponses(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	return s.forward(ctx, c, account, body, opencodepkg.ProtocolResponses)
}

func (s *OpenCodeGatewayService) forward(ctx context.Context, c *gin.Context, account *Account, body []byte, inbound opencodepkg.Protocol) (*ForwardResult, error) {
	start := time.Now()
	if c == nil {
		return nil, fmt.Errorf("gin context is required")
	}
	if account == nil {
		return nil, opencodeRequestError(http.StatusBadRequest, []byte("account is required"))
	}
	if s == nil || s.httpUpstream == nil {
		return nil, fmt.Errorf("OpenCode HTTP upstream is not configured")
	}
	requestCtx, cancel := s.withInferenceTimeout(ctx)
	defer cancel()

	meta, endpoint, err := s.prepareRequest(account, body, inbound)
	if err != nil {
		return nil, err
	}
	upstreamBody, err := opencodepkg.TransformRequest(body, meta)
	if err != nil {
		return nil, opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}
	request.Header.Set("Content-Type", "application/json")
	if account != nil {
		account.ApplyHeaderOverrides(request.Header)
	}
	// Authentication is authoritative and follows the selected upstream protocol.
	// OpenCode's Chat Completions endpoint accepts Bearer auth, while its Anthropic
	// Messages endpoint follows the Anthropic SDK contract (x-api-key + version).
	request.Header.Del("Authorization")
	request.Header.Del("X-Api-Key")
	request.Header.Del("Anthropic-Version")
	if meta.UpstreamProtocol == opencodepkg.ProtocolMessages {
		request.Header.Set("X-Api-Key", opencodeAPIKey(account))
		request.Header.Set("Anthropic-Version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+opencodeAPIKey(account))
	}
	if meta.Stream {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
	}

	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, opencodeAccountFailure(http.StatusBadGateway, []byte(err.Error()), nil)
	}
	upstreamStart := time.Now()
	response, err := s.httpUpstream.Do(request, proxyURL, account.ID, opencodeAccountConcurrency(account))
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, opencodeNetworkFailure(err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, opencodeHTTPFailure(response)
	}
	notifyOpenCodeUpstreamAccepted(c)
	copyOpenCodeResponseHeaders(c.Writer.Header(), response.Header)

	result := &ForwardResult{
		RequestID:        opencodeRequestID(response.Header),
		Model:            meta.RequestedModel,
		BillingModel:     opencodepkg.ModelPrefix + meta.BillingModel,
		UpstreamModel:    meta.UpstreamModel,
		UpstreamEndpoint: openCodeEndpointPath(meta.UpstreamProtocol),
		Stream:           meta.Stream,
	}
	if opencodepkg.NormalizeModelID(result.UpstreamModel) == opencodepkg.NormalizeModelID(result.Model) {
		result.UpstreamModel = ""
	}
	if meta.Stream {
		if err := s.forwardStream(c, response.Body, meta, result, start); err != nil {
			return nil, err
		}
		result.Duration = time.Since(start)
		return result, nil
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, opencodeNetworkFailure(err)
	}
	converted, usage, responseID, err := opencodepkg.TransformResponse(responseBody, meta)
	if err != nil {
		return nil, opencodeNetworkFailure(err)
	}
	if responseID == "" {
		responseID = opencodeRequestID(response.Header)
	}
	result.RequestID = responseID
	result.Usage = openCodeClaudeUsage(usage)
	result.Duration = time.Since(start)
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	if _, err := c.Writer.Write(converted); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *OpenCodeGatewayService) prepareRequest(account *Account, body []byte, inbound opencodepkg.Protocol) (opencodepkg.RequestMeta, string, error) {
	requestedModel, stream, err := opencodepkg.InspectRequest(body, inbound)
	if err != nil {
		return opencodepkg.RequestMeta{}, "", opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}
	resolution, err := ResolveOpenCodeModel(account, requestedModel)
	if err != nil {
		return opencodepkg.RequestMeta{}, "", opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}
	if opencodeAPIKey(account) == "" {
		return opencodepkg.RequestMeta{}, "", opencodeAccountFailure(http.StatusUnauthorized, []byte("OpenCode API key is missing"), nil)
	}
	baseURL := s.openCodeBaseURL(account)
	endpoint, err := opencodepkg.Endpoint(baseURL, resolution.Protocol)
	if err != nil {
		return opencodepkg.RequestMeta{}, "", opencodeAccountFailure(http.StatusBadGateway, []byte(err.Error()), nil)
	}
	meta := opencodepkg.RequestMeta{
		InboundProtocol: inbound, UpstreamProtocol: resolution.Protocol,
		RequestedModel: requestedModel, BillingModel: resolution.BillingModel,
		UpstreamModel: resolution.UpstreamModel, Stream: stream,
	}
	if err := opencodepkg.PopulateResponseMetadata(body, &meta); err != nil {
		return opencodepkg.RequestMeta{}, "", opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}
	return meta, endpoint, nil
}

func (s *OpenCodeGatewayService) CountTokens(_ context.Context, c *gin.Context, _ *Account, body []byte) (*ForwardResult, error) {
	if c == nil {
		return nil, fmt.Errorf("gin context is required")
	}
	var envelope struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}
	tokens := estimateOpenCodeTokens(body)
	c.JSON(http.StatusOK, gin.H{"input_tokens": tokens})
	return &ForwardResult{
		Model: envelope.Model, BillingModel: opencodepkg.NormalizeModelID(envelope.Model),
		Usage: ClaudeUsage{InputTokens: tokens},
	}, nil
}

func (s *OpenCodeGatewayService) FetchModels(ctx context.Context, c *gin.Context, account *Account) (*ForwardResult, error) {
	if c == nil {
		return nil, fmt.Errorf("gin context is required")
	}
	response, endpoint, err := s.fetchModels(ctx, account)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, opencodeNetworkFailure(err)
	}
	copyOpenCodeResponseHeaders(c.Writer.Header(), response.Header)
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	if _, err := c.Writer.Write(body); err != nil {
		return nil, err
	}
	return &ForwardResult{RequestID: opencodeRequestID(response.Header), UpstreamEndpoint: endpoint}, nil
}

// ListModels returns the model IDs from OpenCode Go's authenticated /v1/models endpoint.
func (s *OpenCodeGatewayService) ListModels(ctx context.Context, account *Account) ([]string, error) {
	response, _, err := s.fetchModels(ctx, account)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, upstreamModelsBodyLimit+1))
	if err != nil {
		return nil, opencodeNetworkFailure(err)
	}
	if int64(len(body)) > upstreamModelsBodyLimit {
		return nil, opencodeRequestError(http.StatusBadGateway, []byte("OpenCode model list response is too large"))
	}
	models, err := extractUpstreamModelIDs(body)
	if err != nil {
		return nil, opencodeRequestError(http.StatusBadGateway, []byte(err.Error()))
	}
	if len(models) == 0 {
		return nil, opencodeRequestError(http.StatusBadGateway, []byte("OpenCode returned no supported models"))
	}
	return models, nil
}

func (s *OpenCodeGatewayService) TestConnection(ctx context.Context, account *Account) (*ForwardResult, error) {
	response, endpoint, err := s.fetchModels(ctx, account)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return nil, opencodeNetworkFailure(err)
	}
	return &ForwardResult{RequestID: opencodeRequestID(response.Header), UpstreamEndpoint: endpoint}, nil
}

func (s *OpenCodeGatewayService) fetchModels(ctx context.Context, account *Account) (*http.Response, string, error) {
	if account == nil {
		return nil, "", opencodeRequestError(http.StatusBadRequest, []byte("account is required"))
	}
	apiKey := opencodeAPIKey(account)
	if apiKey == "" {
		return nil, "", opencodeAccountFailure(http.StatusUnauthorized, []byte("OpenCode API key is missing"), nil)
	}
	baseURL := s.openCodeBaseURL(account)
	endpoint, err := opencodepkg.ModelsEndpoint(baseURL)
	if err != nil {
		return nil, "", opencodeAccountFailure(http.StatusBadGateway, []byte(err.Error()), nil)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", opencodeRequestError(http.StatusBadRequest, []byte(err.Error()))
	}
	request.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(request.Header)
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Del("X-Api-Key")
	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, "", opencodeAccountFailure(http.StatusBadGateway, []byte(err.Error()), nil)
	}
	response, err := s.httpUpstream.Do(request, proxyURL, account.ID, opencodeAccountConcurrency(account))
	if err != nil {
		return nil, "", opencodeNetworkFailure(err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		return nil, "", opencodeHTTPFailure(response)
	}
	return response, endpoint, nil
}

func (s *OpenCodeGatewayService) resolveProxyURL(ctx context.Context, account *Account) (string, error) {
	if account == nil || account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy != nil && account.Proxy.ID == *account.ProxyID {
		return account.Proxy.URL(), nil
	}
	if s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil {
		return "", fmt.Errorf("resolve OpenCode proxy: %w", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func (s *OpenCodeGatewayService) openCodeBaseURL(account *Account) string {
	baseURL := opencodepkg.DefaultBaseURL
	if s != nil && s.cfg != nil {
		if configured := strings.TrimSpace(s.cfg.OpenCode.BaseURL); configured != "" {
			baseURL = configured
		}
	}
	if account != nil {
		if configured := strings.TrimSpace(account.GetCredential("base_url")); configured != "" {
			baseURL = configured
		}
	}
	return baseURL
}

func (s *OpenCodeGatewayService) withInferenceTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	seconds := 600
	if s != nil && s.cfg != nil && s.cfg.OpenCode.InferenceTimeoutSeconds > 0 {
		seconds = s.cfg.OpenCode.InferenceTimeoutSeconds
	}
	return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}

func openCodeEndpointPath(protocol opencodepkg.Protocol) string {
	switch protocol {
	case opencodepkg.ProtocolChatCompletions:
		return "/v1/chat/completions"
	case opencodepkg.ProtocolMessages:
		return "/v1/messages"
	case opencodepkg.ProtocolResponses:
		return "/v1/responses"
	default:
		return ""
	}
}

func notifyOpenCodeUpstreamAccepted(c *gin.Context) {
	if c == nil {
		return
	}
	value, ok := c.Get("parsed_request")
	if !ok {
		return
	}
	parsed, ok := value.(*ParsedRequest)
	if !ok || parsed == nil || parsed.OnUpstreamAccepted == nil {
		return
	}
	parsed.OnUpstreamAccepted()
	parsed.OnUpstreamAccepted = nil
}

func opencodeAPIKey(account *Account) string {
	if account == nil {
		return ""
	}
	return account.GetOpenCodeAPIKey()
}

func opencodeAccountConcurrency(account *Account) int {
	if account != nil && account.Concurrency > 0 {
		return account.Concurrency
	}
	return 1
}

func estimateOpenCodeTokens(body []byte) int {
	characters := utf8.RuneCount(body)
	if characters == 0 {
		return 0
	}
	return (characters + 3) / 4
}

func openCodeClaudeUsage(usage opencodepkg.Usage) ClaudeUsage {
	return ClaudeUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
	}
}

func opencodeRequestID(headers http.Header) string {
	for _, key := range []string{"X-Request-Id", "Request-Id", "X-Amzn-Requestid", "Cf-Ray"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func copyOpenCodeResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		switch strings.ToLower(key) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "content-length", "content-encoding":
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
