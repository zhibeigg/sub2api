package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// KiroGatewayService forwards requests to the AWS Kiro / CodeWhisperer upstream,
// presenting Claude models over both the Anthropic Messages and OpenAI Chat
// Completions inbound formats. It is intentionally lighter than the Antigravity
// gateway: Kiro's upstream is a single streaming call with automatic endpoint
// fallback handled inside pkg/kiro.
type KiroGatewayService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *KiroTokenProvider
}

// NewKiroGatewayService constructs a KiroGatewayService.
func NewKiroGatewayService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *KiroTokenProvider,
) *KiroGatewayService {
	return &KiroGatewayService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
	}
}

// GetTokenProvider exposes the token provider for wiring.
func (s *KiroGatewayService) GetTokenProvider() *KiroTokenProvider {
	return s.tokenProvider
}

// IsModelSupported reports whether the requested model can be served by Kiro.
// Kiro provides Claude models; any claude-* (or an aliased model) is accepted.
func (s *KiroGatewayService) IsModelSupported(requestedModel string) bool {
	mapped := kiro.MapModel(requestedModel)
	return strings.HasPrefix(strings.ToLower(mapped), "claude-")
}

// kiroSSEWriter adapts a gin.Context to pkg/kiro's SSEWriter.
type kiroSSEWriter struct {
	c       *gin.Context
	flusher http.Flusher
	wrote   bool
}

func (w *kiroSSEWriter) WriteSSE(event string, data []byte) error {
	frame := kiro.FormatSSE(event, data)
	_, err := w.c.Writer.Write([]byte(frame))
	w.wrote = true
	return err
}

func (w *kiroSSEWriter) Flush() {
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

// Forward converts the inbound request, calls the Kiro upstream and writes the
// response (SSE for stream, JSON otherwise). The inbound format is derived from
// the request path: /v1/messages → Anthropic, /v1/chat/completions → OpenAI.
func (s *KiroGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	start := time.Now()

	if s.tokenProvider == nil {
		return nil, errors.New("kiro token provider is not configured")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	cred := s.buildCredential(ctx, account, accessToken)

	isOpenAI := isOpenAIInboundPath(c)

	var (
		payload      *kiro.KiroPayload
		requestModel string
		stream       bool
	)

	if isOpenAI {
		var req kiro.OpenAIRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("invalid OpenAI request body: " + err.Error())}
		}
		requestModel = req.Model
		stream = req.Stream
		_, thinking := kiro.ParseModelAndThinking(req.Model, kiro.DefaultThinkingSuffix)
		payload = kiro.OpenAIToKiro(&req, thinking)
	} else {
		var req kiro.ClaudeRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("invalid Anthropic request body: " + err.Error())}
		}
		requestModel = req.Model
		stream = req.Stream
		_, thinking := kiro.ResolveClaudeThinkingMode(req.Model, req.Thinking, kiro.DefaultThinkingSuffix)
		payload = kiro.ClaudeToKiro(&req, thinking)
	}

	upstreamModel := kiro.MapModel(requestModel)

	if stream {
		return s.forwardStream(ctx, c, cred, payload, requestModel, upstreamModel, isOpenAI, start)
	}
	return s.forwardNonStream(ctx, c, cred, payload, requestModel, upstreamModel, isOpenAI, start)
}

func (s *KiroGatewayService) buildCredential(ctx context.Context, account *Account, accessToken string) *kiro.Credential {
	proxyURL := ""
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}
	return &kiro.Credential{
		AccessToken:  accessToken,
		RefreshToken: account.GetCredential("refresh_token"),
		ClientID:     account.GetCredential("client_id"),
		ClientSecret: account.GetCredential("client_secret"),
		AuthMethod:   account.GetCredential("auth_method"),
		Region:       account.GetCredential("region"),
		ProfileArn:   account.GetCredential("profile_arn"),
		MachineID:    account.GetCredential("machine_id"),
		ProxyURL:     proxyURL,
	}
}

func (s *KiroGatewayService) forwardStream(
	ctx context.Context, c *gin.Context, cred *kiro.Credential, payload *kiro.KiroPayload,
	requestModel, upstreamModel string, isOpenAI bool, start time.Time,
) (*ForwardResult, error) {
	flusher, _ := c.Writer.(http.Flusher)
	w := &kiroSSEWriter{c: c, flusher: flusher}

	if isOpenAI {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	} else {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	}
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)

	var firstTokenMs *int
	markFirst := func() {
		if firstTokenMs == nil {
			ms := int(time.Since(start).Milliseconds())
			firstTokenMs = &ms
		}
	}

	var usage kiro.ClaudeUsage
	if isOpenAI {
		state := kiro.NewOpenAIStreamState(w, requestModel)
		cb := state.Callback()
		wrapText := cb.OnText
		cb.OnText = func(t string, thinking bool) { markFirst(); wrapText(t, thinking) }
		origComplete := cb.OnComplete
		cb.OnComplete = func(in, out int) {
			usage.InputTokens, usage.OutputTokens = in, out
			if origComplete != nil {
				origComplete(in, out)
			}
		}
		if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
			return nil, mapKiroError(err)
		}
		state.Finish()
	} else {
		state := kiro.NewClaudeStreamState(w, requestModel)
		cb := state.Callback()
		wrapText := cb.OnText
		cb.OnText = func(t string, thinking bool) { markFirst(); wrapText(t, thinking) }
		origComplete := cb.OnComplete
		cb.OnComplete = func(in, out int) {
			usage.InputTokens, usage.OutputTokens = in, out
			if origComplete != nil {
				origComplete(in, out)
			}
		}
		if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
			return nil, mapKiroError(err)
		}
		state.Finish()
	}
	w.Flush()

	return &ForwardResult{
		Model:         requestModel,
		UpstreamModel: differentOrEmpty(requestModel, upstreamModel),
		Stream:        true,
		Duration:      time.Since(start),
		FirstTokenMs:  firstTokenMs,
		Usage:         ClaudeUsage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens},
	}, nil
}

func (s *KiroGatewayService) forwardNonStream(
	ctx context.Context, c *gin.Context, cred *kiro.Credential, payload *kiro.KiroPayload,
	requestModel, upstreamModel string, isOpenAI bool, start time.Time,
) (*ForwardResult, error) {
	agg := &kiro.Aggregator{}
	if err := kiro.CallKiroAPI(ctx, cred, payload, agg.Callback()); err != nil {
		return nil, mapKiroError(err)
	}

	if isOpenAI {
		resp := kiro.KiroToOpenAIResponse(agg.Text.String(), agg.ToolUses, agg.InputTokens, agg.OutputTokens, requestModel)
		c.JSON(http.StatusOK, resp)
	} else {
		resp := kiro.KiroToClaudeResponse(agg.Text.String(), agg.Thinking.String(), false, agg.ToolUses, agg.InputTokens, agg.OutputTokens, requestModel)
		c.JSON(http.StatusOK, resp)
	}

	return &ForwardResult{
		Model:         requestModel,
		UpstreamModel: differentOrEmpty(requestModel, upstreamModel),
		Stream:        false,
		Duration:      time.Since(start),
		Usage:         ClaudeUsage{InputTokens: agg.InputTokens, OutputTokens: agg.OutputTokens},
	}, nil
}

func differentOrEmpty(model, upstream string) string {
	if model == upstream {
		return ""
	}
	return upstream
}

// mapKiroError maps a pkg/kiro upstream error to a sub2api failover error.
func mapKiroError(err error) error {
	var apiErr *kiro.APIError
	if errors.As(err, &apiErr) {
		return &UpstreamFailoverError{
			StatusCode:   apiErr.StatusCode,
			ResponseBody: []byte(apiErr.Body),
		}
	}
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
}

// isOpenAIInboundPath reports whether the request arrived on an OpenAI-style
// chat completions endpoint (vs the Anthropic messages endpoint).
func isOpenAIInboundPath(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	path := c.Request.URL.Path
	return strings.Contains(path, "chat/completions")
}
