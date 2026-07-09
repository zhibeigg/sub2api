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
	accountRepo    AccountRepository
	proxyRepo      ProxyRepository
	tokenProvider  *KiroTokenProvider
	responsesStore *kiro.ResponsesStore
}

// NewKiroGatewayService constructs a KiroGatewayService.
func NewKiroGatewayService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *KiroTokenProvider,
) *KiroGatewayService {
	return &KiroGatewayService{
		accountRepo:    accountRepo,
		proxyRepo:      proxyRepo,
		tokenProvider:  tokenProvider,
		responsesStore: kiro.NewResponsesStore(24 * time.Hour),
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

	var estimatedInput int

	if isOpenAI {
		var req kiro.OpenAIRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("invalid OpenAI request body: " + err.Error())}
		}
		requestModel = req.Model
		stream = req.Stream
		_, thinking := kiro.ParseModelAndThinking(req.Model, kiro.DefaultThinkingSuffix)
		estimatedInput = kiro.EstimateOpenAIRequestInputTokens(&req)
		payload = kiro.OpenAIToKiro(&req, thinking)
	} else {
		var req kiro.ClaudeRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("invalid Anthropic request body: " + err.Error())}
		}
		requestModel = req.Model
		stream = req.Stream
		_, thinking := kiro.ResolveClaudeThinkingMode(req.Model, req.Thinking, kiro.DefaultThinkingSuffix)
		estimatedInput = kiro.EstimateClaudeRequestInputTokens(&req)
		payload = kiro.ClaudeToKiro(&req, thinking)
	}

	upstreamModel := kiro.MapModel(requestModel)

	if stream {
		return s.forwardStream(ctx, c, account, cred, payload, requestModel, upstreamModel, isOpenAI, estimatedInput, start)
	}
	return s.forwardNonStream(ctx, c, account, cred, payload, requestModel, upstreamModel, isOpenAI, estimatedInput, start)
}

// ForwardResponses handles an OpenAI Responses API (/v1/responses) request by
// converting it to the Kiro (Claude) payload, calling the upstream, and
// emitting Responses-format output (SSE for stream, JSON otherwise).
func (s *KiroGatewayService) ForwardResponses(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	start := time.Now()

	if s.tokenProvider == nil {
		return nil, errors.New("kiro token provider is not configured")
	}

	var req kiro.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("invalid responses request body: " + err.Error())}
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = kiro.DefaultResponsesModel
	}
	storeResponse := req.Store == nil || *req.Store
	storedInput := append(json.RawMessage(nil), req.Input...)

	messages, err := s.responsesStore.BuildResponsesMessages(&req)
	if err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(err.Error())}
	}
	if len(messages) == 0 {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("input must contain at least one message")}
	}

	requestModel := req.Model
	_, thinking := kiro.ParseModelAndThinking(req.Model, kiro.DefaultThinkingSuffix)
	openaiReq := &kiro.OpenAIRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   req.Stream,
		Tools:    req.Tools,
	}
	if req.Temperature != nil {
		openaiReq.Temperature = *req.Temperature
	}
	if req.MaxOutputTokens != nil {
		openaiReq.MaxTokens = *req.MaxOutputTokens
	}
	payload := kiro.OpenAIToKiro(openaiReq, thinking)
	upstreamModel := kiro.MapModel(requestModel)

	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	cred := s.buildCredential(ctx, account, accessToken)
	respID := kiro.GenerateResponseID()

	if req.Stream {
		return s.forwardResponsesStream(ctx, c, account, cred, payload, &req, requestModel, upstreamModel, thinking, respID, storedInput, storeResponse, start)
	}
	return s.forwardResponsesNonStream(ctx, c, account, cred, payload, &req, requestModel, upstreamModel, thinking, respID, storedInput, storeResponse, start)
}

func (s *KiroGatewayService) forwardResponsesStream(
	ctx context.Context, c *gin.Context, account *Account, cred *kiro.Credential, payload *kiro.KiroPayload,
	req *kiro.ResponsesRequest, requestModel, upstreamModel string, thinking bool, respID string,
	storedInput []byte, storeResponse bool, start time.Time,
) (*ForwardResult, error) {
	flusher, _ := c.Writer.(http.Flusher)
	w := &kiroSSEWriter{c: c, flusher: flusher}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)

	var firstTokenMs *int
	createdAt := start.Unix()
	state := kiro.NewResponsesStreamState(w, requestModel, respID, req, createdAt)
	cb := state.Callback()
	wrapText := cb.OnText
	cb.OnText = func(t string, thinkingChunk bool) {
		if firstTokenMs == nil {
			ms := int(time.Since(start).Milliseconds())
			firstTokenMs = &ms
		}
		wrapText(t, thinkingChunk)
	}

	if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
		if !state.Started() {
			return nil, mapKiroError(err)
		}
		state.EmitFailed(err.Error())
		w.Flush()
		return nil, mapKiroError(err)
	}

	obj, inputTokens, outputTokens, credits := state.Finish(thinking, 0)
	w.Flush()

	if storeResponse && obj != nil {
		obj.StoredInput = storedInput
		obj.Instructions = req.Instructions
		s.responsesStore.Save(obj)
	}
	s.recordContextUsage(ctx, account, 0)

	return &ForwardResult{
		Model:         requestModel,
		UpstreamModel: differentOrEmpty(requestModel, upstreamModel),
		Stream:        true,
		Duration:      time.Since(start),
		FirstTokenMs:  firstTokenMs,
		Usage:         ClaudeUsage{InputTokens: inputTokens, OutputTokens: outputTokens},
		KiroCredits:   credits,
	}, nil
}

func (s *KiroGatewayService) forwardResponsesNonStream(
	ctx context.Context, c *gin.Context, account *Account, cred *kiro.Credential, payload *kiro.KiroPayload,
	req *kiro.ResponsesRequest, requestModel, upstreamModel string, thinking bool, respID string,
	storedInput []byte, storeResponse bool, start time.Time,
) (*ForwardResult, error) {
	agg := &kiro.Aggregator{}
	cb := agg.Callback()
	var credits float64
	cb.OnCredits = func(c float64) { credits += c }
	if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
		return nil, mapKiroError(err)
	}

	finalContent, _ := kiro.ExtractThinkingFromContent(agg.Text.String())
	obj := kiro.BuildResponsesObject(respID, requestModel, finalContent, agg.ToolUses, agg.InputTokens, agg.OutputTokens, req)
	if storeResponse {
		obj.StoredInput = storedInput
		obj.Instructions = req.Instructions
		s.responsesStore.Save(obj)
	}
	c.JSON(http.StatusOK, obj)
	s.recordContextUsage(ctx, account, 0)

	return &ForwardResult{
		Model:         requestModel,
		UpstreamModel: differentOrEmpty(requestModel, upstreamModel),
		Stream:        false,
		Duration:      time.Since(start),
		Usage:         ClaudeUsage{InputTokens: agg.InputTokens, OutputTokens: agg.OutputTokens},
		KiroCredits:   credits,
	}, nil
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
	ctx context.Context, c *gin.Context, account *Account, cred *kiro.Credential, payload *kiro.KiroPayload,
	requestModel, upstreamModel string, isOpenAI bool, estimatedInput int, start time.Time,
) (*ForwardResult, error) {
	flusher, _ := c.Writer.(http.Flusher)
	w := &kiroSSEWriter{c: c, flusher: flusher}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
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

	// Aggregate output content locally so token counts can be estimated when the
	// Kiro upstream does not report them (which is the common case).
	var textBuf, thinkingBuf strings.Builder
	var toolUses []kiro.KiroToolUse
	var upstreamIn, upstreamOut int
	var credits, contextPct float64
	instrument := func(cb *kiro.StreamCallback) {
		wrapText := cb.OnText
		cb.OnText = func(t string, thinking bool) {
			markFirst()
			if thinking {
				thinkingBuf.WriteString(t)
			} else {
				textBuf.WriteString(t)
			}
			wrapText(t, thinking)
		}
		wrapTool := cb.OnToolUse
		cb.OnToolUse = func(tu kiro.KiroToolUse) {
			toolUses = append(toolUses, tu)
			if wrapTool != nil {
				wrapTool(tu)
			}
		}
		origComplete := cb.OnComplete
		cb.OnComplete = func(in, out int) {
			upstreamIn, upstreamOut = in, out
			if origComplete != nil {
				origComplete(in, out)
			}
		}
		cb.OnCredits = func(c float64) { credits += c }
		cb.OnContextUsage = func(pct float64) { contextPct = pct }
	}

	if isOpenAI {
		state := kiro.NewOpenAIStreamState(w, requestModel)
		cb := state.Callback()
		instrument(cb)
		if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
			return nil, mapKiroError(err)
		}
		state.Finish()
	} else {
		state := kiro.NewClaudeStreamState(w, requestModel)
		cb := state.Callback()
		instrument(cb)
		if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
			return nil, mapKiroError(err)
		}
		state.Finish()
	}
	w.Flush()

	inputTokens, outputTokens := s.resolveKiroUsage(
		requestModel, upstreamIn, upstreamOut, estimatedInput, contextPct,
		textBuf.String(), thinkingBuf.String(), toolUses, isOpenAI,
	)

	s.recordContextUsage(ctx, account, contextPct)

	return &ForwardResult{
		Model:               requestModel,
		UpstreamModel:       differentOrEmpty(requestModel, upstreamModel),
		Stream:              true,
		Duration:            time.Since(start),
		FirstTokenMs:        firstTokenMs,
		Usage:               ClaudeUsage{InputTokens: inputTokens, OutputTokens: outputTokens},
		KiroCredits:         credits,
		KiroContextUsagePct: contextPct,
	}, nil
}

// resolveKiroUsage applies the token-count fallback priority shared by the
// Kiro stream and non-stream paths:
//
//	input  = contextUsageEvent reverse-derived → upstream usage → request estimate
//	output = upstream usage → local estimate
//
// The Kiro upstream almost never reports token counts, so without these
// fallbacks input/output would be 0 and billing/usage would show nothing.
func (s *KiroGatewayService) resolveKiroUsage(
	model string, upstreamIn, upstreamOut, estimatedInput int, contextPct float64,
	text, thinking string, toolUses []kiro.KiroToolUse, isOpenAI bool,
) (int, int) {
	inputTokens := 0
	if contextPct > 0 {
		inputTokens = int(contextPct * float64(kiro.GetContextWindowSize(model)) / 100.0)
	}
	if inputTokens <= 0 {
		inputTokens = upstreamIn
	}
	if inputTokens <= 0 {
		inputTokens = estimatedInput
	}

	outputTokens := upstreamOut
	if outputTokens <= 0 {
		if isOpenAI {
			outputTokens = kiro.EstimateOpenAIOutputTokens(text, thinking, toolUses)
		} else {
			outputTokens = kiro.EstimateClaudeOutputTokens(text, thinking, toolUses)
		}
	}
	return inputTokens, outputTokens
}

func (s *KiroGatewayService) forwardNonStream(
	ctx context.Context, c *gin.Context, account *Account, cred *kiro.Credential, payload *kiro.KiroPayload,
	requestModel, upstreamModel string, isOpenAI bool, estimatedInput int, start time.Time,
) (*ForwardResult, error) {
	agg := &kiro.Aggregator{}
	cb := agg.Callback()
	var credits, contextPct float64
	cb.OnCredits = func(c float64) { credits += c }
	cb.OnContextUsage = func(pct float64) { contextPct = pct }
	if err := kiro.CallKiroAPI(ctx, cred, payload, cb); err != nil {
		return nil, mapKiroError(err)
	}

	inputTokens, outputTokens := s.resolveKiroUsage(
		requestModel, agg.InputTokens, agg.OutputTokens, estimatedInput, contextPct,
		agg.Text.String(), agg.Thinking.String(), agg.ToolUses, isOpenAI,
	)

	if isOpenAI {
		resp := kiro.KiroToOpenAIResponse(agg.Text.String(), agg.ToolUses, inputTokens, outputTokens, requestModel)
		c.JSON(http.StatusOK, resp)
	} else {
		resp := kiro.KiroToClaudeResponse(agg.Text.String(), agg.Thinking.String(), false, agg.ToolUses, inputTokens, outputTokens, requestModel)
		c.JSON(http.StatusOK, resp)
	}

	s.recordContextUsage(ctx, account, contextPct)

	return &ForwardResult{
		Model:               requestModel,
		UpstreamModel:       differentOrEmpty(requestModel, upstreamModel),
		Stream:              false,
		Duration:            time.Since(start),
		Usage:               ClaudeUsage{InputTokens: inputTokens, OutputTokens: outputTokens},
		KiroCredits:         credits,
		KiroContextUsagePct: contextPct,
	}, nil
}

// recordContextUsage persists the latest context-usage percentage to the
// account's Kiro snapshot as an observability signal. It is best-effort and
// runs asynchronously so it never blocks the response path.
func (s *KiroGatewayService) recordContextUsage(_ context.Context, account *Account, pct float64) {
	if s.accountRepo == nil || account == nil || pct <= 0 {
		return
	}
	accountID := account.ID
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(bg, accountID, map[string]any{
			kiroContextUsageExtraKey: pct,
		})
	}()
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
