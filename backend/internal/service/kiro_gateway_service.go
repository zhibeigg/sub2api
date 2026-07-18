package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// KiroGatewayService forwards requests to the AWS Kiro / CodeWhisperer upstream,
// presenting Claude models over both the Anthropic Messages and OpenAI Chat
// Completions inbound formats. It is intentionally lighter than the Antigravity
// gateway: Kiro's upstream is a single streaming call with automatic endpoint
// fallback handled inside pkg/kiro.
type KiroGatewayService struct {
	accountRepo       AccountRepository
	proxyRepo         ProxyRepository
	tokenProvider     *KiroTokenProvider
	responsesStore    *kiro.ResponsesStore
	promptCache       *kiro.PromptCacheTracker
	profileArnCache   sync.Map
	profileArnFlight  singleflight.Group
	resolveProfileArn func(context.Context, *kiro.Credential) (string, error)
	callKiroAPI       func(context.Context, *kiro.Credential, *kiro.KiroPayload, *kiro.StreamCallback) error
}

// NewKiroGatewayService constructs a KiroGatewayService.
func NewKiroGatewayService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *KiroTokenProvider,
) *KiroGatewayService {
	return &KiroGatewayService{
		accountRepo:       accountRepo,
		proxyRepo:         proxyRepo,
		tokenProvider:     tokenProvider,
		responsesStore:    kiro.NewResponsesStore(24 * time.Hour),
		promptCache:       kiro.NewPromptCacheTracker(),
		resolveProfileArn: kiro.ResolveProfileArn,
		callKiroAPI:       kiro.CallKiroAPI,
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

	isOpenAI := isOpenAIInboundPath(c)

	var (
		payload        *kiro.KiroPayload
		requestModel   string
		stream         bool
		estimatedInput int
		cacheProfile   *kiro.PromptCacheProfile
	)

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
		accountingModel, thinking := kiro.ResolveClaudeThinkingMode(req.Model, req.Thinking, kiro.DefaultThinkingSuffix)
		effectiveReq := kiro.CloneClaudeRequestForThinking(&req, thinking)
		effectiveReq.Model = accountingModel
		estimatedInput = kiro.EstimateClaudeRequestInputTokens(effectiveReq)
		if s.promptCache != nil {
			cacheProfile = s.promptCache.BuildClaudeProfile(effectiveReq, estimatedInput)
		}
		payload = kiro.ClaudeToKiro(&req, thinking)
	}

	upstreamModel := kiro.MapModel(requestModel)
	cred := s.prepareCredential(ctx, account, accessToken)

	if stream {
		return s.forwardStream(ctx, c, account, cred, payload, requestModel, upstreamModel, isOpenAI, estimatedInput, cacheProfile, start)
	}
	return s.forwardNonStream(ctx, c, account, cred, payload, requestModel, upstreamModel, isOpenAI, estimatedInput, cacheProfile, start)
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
	cred := s.prepareCredential(ctx, account, accessToken)
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

	if err := s.callKiroWithAuthRetry(ctx, account, cred, payload, cb); err != nil {
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
	if err := s.callKiroWithAuthRetry(ctx, account, cred, payload, cb); err != nil {
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

func (s *KiroGatewayService) prepareCredential(ctx context.Context, account *Account, accessToken string) *kiro.Credential {
	cred := s.buildCredential(ctx, account, accessToken)
	s.ensureProfileArn(ctx, account, cred)
	return cred
}

func (s *KiroGatewayService) ensureProfileArn(ctx context.Context, account *Account, cred *kiro.Credential) {
	if s == nil || account == nil || cred == nil {
		return
	}
	cacheKey := kiroProfileArnCacheKey(account)
	if profileArn := strings.TrimSpace(cred.ProfileArn); profileArn != "" {
		s.profileArnCache.Store(cacheKey, profileArn)
		return
	}
	if cached, ok := s.profileArnCache.Load(cacheKey); ok {
		if profileArn, ok := cached.(string); ok && strings.TrimSpace(profileArn) != "" {
			cred.ProfileArn = profileArn
			return
		}
	}

	resolver := s.resolveProfileArn
	if resolver == nil {
		resolver = kiro.ResolveProfileArn
	}
	resolved, err, _ := s.profileArnFlight.Do(cacheKey, func() (any, error) {
		if cached, ok := s.profileArnCache.Load(cacheKey); ok {
			if profileArn, ok := cached.(string); ok && strings.TrimSpace(profileArn) != "" {
				return profileArn, nil
			}
		}
		profileArn, resolveErr := resolver(ctx, cred)
		profileArn = strings.TrimSpace(profileArn)
		if resolveErr != nil {
			return "", resolveErr
		}
		if profileArn == "" {
			return "", errors.New("kiro profile ARN resolution returned an empty value")
		}
		s.profileArnCache.Store(cacheKey, profileArn)
		if persistErr := persistKiroProfileArn(ctx, s.accountRepo, account, profileArn); persistErr != nil {
			slog.Warn("kiro profile ARN persistence failed", "account_id", account.ID, "error", persistErr)
		}
		return profileArn, nil
	})
	if err != nil {
		if kiro.IsProfileArnResolutionSoftError(err) {
			slog.Debug("kiro profile ARN unavailable; using credential region", "account_id", account.ID, "region", cred.Region, "error", err)
		} else {
			slog.Warn("kiro profile ARN resolution failed; falling back to credential region", "account_id", account.ID, "region", cred.Region, "error", err)
		}
		return
	}
	if profileArn, ok := resolved.(string); ok {
		cred.ProfileArn = profileArn
	}
}

func kiroProfileArnCacheKey(account *Account) string {
	if account == nil {
		return "0"
	}
	return strconv.FormatInt(account.ID, 10) + "\x00" + account.GetCredential("region") + "\x00" + account.GetCredential("client_id")
}

func persistKiroProfileArn(ctx context.Context, repo accountUpdater, account *Account, profileArn string) error {
	profileArn = strings.TrimSpace(profileArn)
	if repo == nil || account == nil || profileArn == "" || account.GetCredential("profile_arn") == profileArn {
		return nil
	}
	accountCopy := *account
	credentials := MergeCredentials(account.Credentials, map[string]any{"profile_arn": profileArn})
	return persistAccountCredentials(ctx, repo, &accountCopy, credentials)
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
	requestModel, upstreamModel string, isOpenAI bool, estimatedInput int, cacheProfile *kiro.PromptCacheProfile, start time.Time,
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
				_, _ = thinkingBuf.WriteString(t)
			} else {
				_, _ = textBuf.WriteString(t)
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

	cacheUsage := s.computePromptCacheUsage(account, cacheProfile)
	initialUsage := buildKiroClaudeUsage(estimatedInput, 0, cacheUsage)

	var callback *kiro.StreamCallback
	var finish func(ClaudeUsage)
	if isOpenAI {
		state := kiro.NewOpenAIStreamState(w, requestModel)
		callback = state.Callback()
		finish = func(usage ClaudeUsage) {
			state.OnComplete(usage.InputTokens, usage.OutputTokens)
			state.Finish()
		}
	} else {
		state := kiro.NewClaudeStreamState(w, requestModel)
		state.SetUsage(toKiroClaudeUsage(initialUsage))
		callback = state.Callback()
		finish = func(usage ClaudeUsage) {
			state.SetUsage(toKiroClaudeUsage(usage))
			state.Finish()
		}
	}

	instrument(callback)
	if err := s.callKiroWithAuthRetry(ctx, account, cred, payload, callback); err != nil {
		return nil, mapKiroError(err)
	}

	totalInputTokens, outputTokens := s.resolveKiroUsage(
		requestModel, upstreamIn, upstreamOut, estimatedInput, contextPct,
		textBuf.String(), thinkingBuf.String(), toolUses, isOpenAI,
	)
	usage := buildKiroClaudeUsage(totalInputTokens, outputTokens, cacheUsage)
	finish(usage)
	w.Flush()

	s.updatePromptCache(account, cacheProfile)
	s.recordContextUsage(ctx, account, contextPct)

	return &ForwardResult{
		Model:               requestModel,
		UpstreamModel:       differentOrEmpty(requestModel, upstreamModel),
		Stream:              true,
		Duration:            time.Since(start),
		FirstTokenMs:        firstTokenMs,
		Usage:               usage,
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

func (s *KiroGatewayService) computePromptCacheUsage(account *Account, profile *kiro.PromptCacheProfile) kiro.PromptCacheUsage {
	if s == nil || s.promptCache == nil || account == nil || profile == nil {
		return kiro.PromptCacheUsage{}
	}
	return s.promptCache.Compute(strconv.FormatInt(account.ID, 10), profile)
}

func (s *KiroGatewayService) updatePromptCache(account *Account, profile *kiro.PromptCacheProfile) {
	if s == nil || s.promptCache == nil || account == nil || profile == nil {
		return
	}
	s.promptCache.Update(strconv.FormatInt(account.ID, 10), profile)
}

func buildKiroClaudeUsage(totalInputTokens, outputTokens int, cacheUsage kiro.PromptCacheUsage) ClaudeUsage {
	cacheUsage = normalizeKiroPromptCacheUsage(totalInputTokens, cacheUsage)
	return ClaudeUsage{
		InputTokens:              cacheUsage.UncachedInputTokens(totalInputTokens),
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: cacheUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     cacheUsage.CacheReadInputTokens,
		CacheCreation5mTokens:    cacheUsage.CacheCreation5mInputTokens,
		CacheCreation1hTokens:    cacheUsage.CacheCreation1hInputTokens,
	}
}

func normalizeKiroPromptCacheUsage(totalInputTokens int, usage kiro.PromptCacheUsage) kiro.PromptCacheUsage {
	if totalInputTokens <= 0 {
		return kiro.PromptCacheUsage{}
	}

	usage.CacheReadInputTokens = max(usage.CacheReadInputTokens, 0)
	usage.CacheCreationInputTokens = max(usage.CacheCreationInputTokens, 0)
	usage.CacheCreation5mInputTokens = max(usage.CacheCreation5mInputTokens, 0)
	usage.CacheCreation1hInputTokens = max(usage.CacheCreation1hInputTokens, 0)

	if usage.CacheReadInputTokens > totalInputTokens {
		usage.CacheReadInputTokens = totalInputTokens
	}
	remaining := totalInputTokens - usage.CacheReadInputTokens
	if usage.CacheCreationInputTokens > remaining {
		usage.CacheCreationInputTokens = remaining
	}

	breakdownTotal := usage.CacheCreation5mInputTokens + usage.CacheCreation1hInputTokens
	switch {
	case usage.CacheCreationInputTokens == 0:
		usage.CacheCreation5mInputTokens = 0
		usage.CacheCreation1hInputTokens = 0
	case breakdownTotal == 0:
		usage.CacheCreation5mInputTokens = usage.CacheCreationInputTokens
	case breakdownTotal != usage.CacheCreationInputTokens:
		usage.CacheCreation5mInputTokens = usage.CacheCreationInputTokens * usage.CacheCreation5mInputTokens / breakdownTotal
		usage.CacheCreation1hInputTokens = usage.CacheCreationInputTokens - usage.CacheCreation5mInputTokens
	}

	return usage
}

func toKiroClaudeUsage(usage ClaudeUsage) kiro.ClaudeUsage {
	result := kiro.ClaudeUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
	}
	if usage.CacheCreation5mTokens > 0 || usage.CacheCreation1hTokens > 0 {
		result.CacheCreation = &kiro.ClaudeCacheCreationUsage{
			Ephemeral5mInputTokens: usage.CacheCreation5mTokens,
			Ephemeral1hInputTokens: usage.CacheCreation1hTokens,
		}
	}
	return result
}

func (s *KiroGatewayService) forwardNonStream(
	ctx context.Context, c *gin.Context, account *Account, cred *kiro.Credential, payload *kiro.KiroPayload,
	requestModel, upstreamModel string, isOpenAI bool, estimatedInput int, cacheProfile *kiro.PromptCacheProfile, start time.Time,
) (*ForwardResult, error) {
	agg := &kiro.Aggregator{}
	cb := agg.Callback()
	var credits, contextPct float64
	cb.OnCredits = func(c float64) { credits += c }
	cb.OnContextUsage = func(pct float64) { contextPct = pct }
	cacheUsage := s.computePromptCacheUsage(account, cacheProfile)
	if err := s.callKiroWithAuthRetry(ctx, account, cred, payload, cb); err != nil {
		return nil, mapKiroError(err)
	}

	totalInputTokens, outputTokens := s.resolveKiroUsage(
		requestModel, agg.InputTokens, agg.OutputTokens, estimatedInput, contextPct,
		agg.Text.String(), agg.Thinking.String(), agg.ToolUses, isOpenAI,
	)
	usage := buildKiroClaudeUsage(totalInputTokens, outputTokens, cacheUsage)

	if isOpenAI {
		resp := kiro.KiroToOpenAIResponse(agg.Text.String(), agg.ToolUses, usage.InputTokens, usage.OutputTokens, requestModel)
		c.JSON(http.StatusOK, resp)
	} else {
		resp := kiro.KiroToClaudeResponse(agg.Text.String(), agg.Thinking.String(), false, agg.ToolUses, usage.InputTokens, usage.OutputTokens, requestModel)
		resp.Usage = toKiroClaudeUsage(usage)
		c.JSON(http.StatusOK, resp)
	}

	s.updatePromptCache(account, cacheProfile)
	s.recordContextUsage(ctx, account, contextPct)

	return &ForwardResult{
		Model:               requestModel,
		UpstreamModel:       differentOrEmpty(requestModel, upstreamModel),
		Stream:              false,
		Duration:            time.Since(start),
		Usage:               usage,
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

func (s *KiroGatewayService) callKiroWithAuthRetry(
	ctx context.Context,
	account *Account,
	cred *kiro.Credential,
	payload *kiro.KiroPayload,
	callback *kiro.StreamCallback,
) error {
	call := s.callKiroAPI
	if call == nil {
		call = kiro.CallKiroAPI
	}

	err := call(ctx, cred, payload, callback)
	if err != nil {
		logKiroUpstreamCallError("initial", account, cred, payload, err)
	}
	if !isKiroUnauthorized(err) || s.tokenProvider == nil || account == nil || cred == nil {
		return err
	}

	accessToken, refreshErr := s.tokenProvider.ForceRefreshAccessToken(ctx, account)
	if refreshErr != nil {
		slog.Warn("kiro upstream 401 forced refresh failed",
			"account_id", account.ID,
			"error", logredact.RedactText(refreshErr.Error()),
		)
		return err
	}

	cred.AccessToken = accessToken
	slog.Info("kiro upstream 401 retrying after forced refresh", "account_id", account.ID)
	retryErr := call(ctx, cred, payload, callback)
	if retryErr != nil {
		logKiroUpstreamCallError("after_forced_refresh", account, cred, payload, retryErr)
	}
	return retryErr
}

func logKiroUpstreamCallError(phase string, account *Account, cred *kiro.Credential, payload *kiro.KiroPayload, err error) {
	attrs := []any{
		"phase", phase,
		"error_type", fmt.Sprintf("%T", err),
		"error", logredact.RedactText(err.Error()),
	}
	if account != nil {
		attrs = append(attrs, "account_id", account.ID)
	}
	if cred != nil {
		attrs = append(attrs,
			"access_token_hash", kiroDiagnosticHash(cred.AccessToken),
			"profile_arn_hash", kiroDiagnosticHash(cred.ProfileArn),
			"region", cred.Region,
		)
	}
	if payload != nil {
		userInput := payload.ConversationState.CurrentMessage.UserInputMessage
		attrs = append(attrs,
			"model", userInput.ModelID,
			"conversation_id", payload.ConversationState.ConversationID,
			"agent_task_type", payload.ConversationState.AgentTaskType,
			"has_agent_continuation_id", strings.TrimSpace(payload.ConversationState.AgentContinuationId) != "",
		)
		if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
			attrs = append(attrs, "payload_hash", kiroDiagnosticHash(string(encoded)), "payload_bytes", len(encoded))
		}
	}
	var apiErr *kiro.APIError
	if errors.As(err, &apiErr) {
		attrs = append(attrs,
			"upstream_status", apiErr.StatusCode,
			"upstream_endpoint", apiErr.Endpoint,
			"upstream_body", logredact.RedactText(apiErr.Body),
		)
	}
	slog.Warn("kiro upstream call failed", attrs...)
}

func kiroDiagnosticHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:6])
}

func isKiroUnauthorized(err error) bool {
	var apiErr *kiro.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
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
