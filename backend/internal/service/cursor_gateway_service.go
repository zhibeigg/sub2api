package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/Wei-Shaw/sub2api/internal/config"
	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CursorGatewayService adapts Anthropic Messages, OpenAI Chat Completions and
// OpenAI Responses requests to Cursor's asynchronous Cloud Agents API.
type cursorResponseStore interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
}

type CursorGatewayService struct {
	httpUpstream    HTTPUpstream
	proxyRepo       ProxyRepository
	responseStore   cursorResponseStore
	cfg             *config.Config
	dashboardAuth   *CursorDashboardAuthService
	ideModelMu      sync.RWMutex
	ideModelCache   map[int64]cursorIDEModelCatalogCache
	ideModelRefresh singleflight.Group

	agentSessionMu sync.Mutex
	agentSessions  map[string]*cursorAgentActiveSession
}

func newCursorGatewayService(httpUpstream HTTPUpstream, proxyRepo ProxyRepository, responseStore cursorResponseStore, cfg *config.Config) *CursorGatewayService {
	return &CursorGatewayService{
		httpUpstream: httpUpstream, proxyRepo: proxyRepo, responseStore: responseStore, cfg: cfg,
		ideModelCache: make(map[int64]cursorIDEModelCatalogCache),
		agentSessions: make(map[string]*cursorAgentActiveSession),
	}
}

func (s *CursorGatewayService) SetDashboardAuthService(auth *CursorDashboardAuthService) {
	if s != nil {
		s.dashboardAuth = auth
	}
}

type cursorRequestEnvelope struct {
	Model              string `json:"model"`
	Stream             bool   `json:"stream"`
	PreviousResponseID string `json:"previous_response_id"`
	Store              *bool  `json:"store"`
	Thinking           *struct {
		Type string `json:"type"`
	} `json:"thinking"`
	OutputConfig *struct {
		Effort string `json:"effort"`
	} `json:"output_config"`
	Reasoning *struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type cursorVariantPreference struct {
	Thinking *bool
	Effort   string
	Context  string
	Fast     *bool
}

func (e cursorRequestEnvelope) variantPreference() cursorVariantPreference {
	preference := cursorVariantPreference{}
	if e.Thinking != nil {
		switch strings.ToLower(strings.TrimSpace(e.Thinking.Type)) {
		case "enabled", "adaptive", "auto":
			value := true
			preference.Thinking = &value
		case "disabled", "none", "off":
			value := false
			preference.Thinking = &value
		}
	}
	effort := strings.TrimSpace(e.ReasoningEffort)
	if e.Reasoning != nil && strings.TrimSpace(e.Reasoning.Effort) != "" {
		effort = e.Reasoning.Effort
	}
	if e.OutputConfig != nil && strings.TrimSpace(e.OutputConfig.Effort) != "" {
		effort = e.OutputConfig.Effort
	}
	if normalized := NormalizeClaudeOutputEffort(effort); normalized != nil {
		preference.Effort = *normalized
		if preference.Thinking == nil {
			value := true
			preference.Thinking = &value
		}
	}
	return preference
}

func normalizeCursorCloudModel(model string, preference cursorVariantPreference) (string, cursorVariantPreference) {
	model = strings.TrimSpace(model)
	lower := strings.ToLower(model)
	if lower == "claude-4-sonnet-1m" {
		preference.Context = "1m"
		return "claude-sonnet-4", preference
	}
	parts := strings.Split(lower, "-")
	hasThinking := false
	hasFast := false
	for _, part := range parts {
		hasThinking = hasThinking || part == "thinking"
		hasFast = hasFast || part == "fast"
	}
	effort := cursorIDEVariantEffort(lower)
	if effort == "" && !hasThinking && !hasFast {
		return model, preference
	}
	if preference.Effort == "" && effort != "" {
		preference.Effort = effort
	}
	if preference.Thinking == nil && hasThinking {
		value := true
		preference.Thinking = &value
	}
	if preference.Fast == nil && hasFast {
		value := true
		preference.Fast = &value
	}
	logical := cursorIDEVariantFamily(lower)
	logicalAliases := map[string]string{
		"cursor-grok-4.5":   "grok-4.5",
		"claude-4-sonnet":   "claude-sonnet-4",
		"claude-4.5-haiku":  "claude-haiku-4-5",
		"claude-4.5-opus":   "claude-opus-4-5",
		"claude-4.5-sonnet": "claude-sonnet-4-5",
		"claude-4.6-opus":   "claude-opus-4-6",
		"claude-4.6-sonnet": "claude-sonnet-4-6",
		"claude-4.7-opus":   "claude-opus-4-7",
		"claude-4.8-opus":   "claude-opus-4-8",
	}
	if canonical, ok := logicalAliases[logical]; ok {
		logical = canonical
	}
	return logical, preference
}

const (
	cursorAgentExecMCP         = "mcp"
	cursorAgentExecShell       = "shell"
	cursorAgentExecShellStream = "shell_stream"
)

type cursorAgentPendingMCP struct {
	Kind         string            `json:"kind,omitempty"`
	RequestID    uint64            `json:"request_id"`
	ExecID       string            `json:"exec_id,omitempty"`
	ExecField    int               `json:"exec_field,omitempty"`
	Action       cursorpkg.Action  `json:"action"`
	CursorAction *cursorpkg.Action `json:"cursor_action,omitempty"`
}

type cursorAgentActiveSession struct {
	Stream        *cursorAgentEventAdapter
	Stored        *cursorStoredResponse
	Pending       *cursorAgentPendingMCP
	AccountID     int64
	UpstreamModel string
	refs          []string
	timer         *time.Timer
	closeOnce     sync.Once
}

func (s *cursorAgentActiveSession) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.Stream != nil {
			_ = s.Stream.Close()
		}
	})
}

type cursorStoredResponse struct {
	Owner               string                            `json:"owner"`
	Dialogue            *cursorpkg.Dialogue               `json:"dialogue"`
	AgentConversationID string                            `json:"agent_conversation_id,omitempty"`
	AgentState          *cursorpkg.AgentConversationState `json:"agent_state,omitempty"`
	AgentBlobs          map[string][]byte                 `json:"agent_blobs,omitempty"`
	AgentPendingMCP     *cursorAgentPendingMCP            `json:"agent_pending_mcp,omitempty"`
}

func cursorAgentSessionRef(owner, kind, value string) string {
	value = strings.TrimSpace(value)
	if owner == "" || value == "" {
		return ""
	}
	return owner + "\x00" + kind + "\x00" + value
}

func (s *CursorGatewayService) cursorAgentSessionTTL() time.Duration {
	ttl := durationSeconds(s.cursorConfig().ResponsesTTLSeconds, 600)
	if ttl < 30*time.Second {
		return 30 * time.Second
	}
	if ttl > 10*time.Minute {
		return 10 * time.Minute
	}
	return ttl
}

func (s *CursorGatewayService) detachCursorAgentSessionLocked(session *cursorAgentActiveSession) {
	if session == nil {
		return
	}
	for _, ref := range session.refs {
		if s.agentSessions[ref] == session {
			delete(s.agentSessions, ref)
		}
	}
	session.refs = nil
	if session.timer != nil {
		session.timer.Stop()
		session.timer = nil
	}
}

func (s *CursorGatewayService) storeCursorAgentSession(owner, responseID, toolCallID string, session *cursorAgentActiveSession) {
	if s == nil || session == nil || session.Stream == nil {
		return
	}
	refs := make([]string, 0, 2)
	for _, ref := range []string{
		cursorAgentSessionRef(owner, "response", responseID),
		cursorAgentSessionRef(owner, "tool", toolCallID),
	} {
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		session.Close()
		return
	}

	replacedSet := make(map[*cursorAgentActiveSession]struct{}, len(refs))
	s.agentSessionMu.Lock()
	if s.agentSessions == nil {
		s.agentSessions = make(map[string]*cursorAgentActiveSession)
	}
	for _, ref := range refs {
		if previous := s.agentSessions[ref]; previous != nil && previous != session {
			replacedSet[previous] = struct{}{}
		}
	}
	for previous := range replacedSet {
		s.detachCursorAgentSessionLocked(previous)
	}
	s.detachCursorAgentSessionLocked(session)
	for _, ref := range refs {
		s.agentSessions[ref] = session
	}
	session.refs = append([]string(nil), refs...)
	session.timer = time.AfterFunc(s.cursorAgentSessionTTL(), func() {
		s.expireCursorAgentSession(session)
	})
	s.agentSessionMu.Unlock()
	for previous := range replacedSet {
		previous.Close()
	}
}

func (s *CursorGatewayService) takeCursorAgentSession(owner, responseID string, toolCallIDs []string) (*cursorAgentActiveSession, bool) {
	if s == nil {
		return nil, false
	}
	toolIDs := make(map[string]struct{}, len(toolCallIDs))
	for _, toolCallID := range toolCallIDs {
		if toolCallID = strings.TrimSpace(toolCallID); toolCallID != "" {
			toolIDs[toolCallID] = struct{}{}
		}
	}

	s.agentSessionMu.Lock()
	defer s.agentSessionMu.Unlock()
	responseRef := cursorAgentSessionRef(owner, "response", responseID)
	if responseRef != "" {
		if session := s.agentSessions[responseRef]; session != nil {
			pendingID := ""
			if session.Pending != nil {
				pendingID = strings.TrimSpace(session.Pending.Action.ID)
			}
			if _, matches := toolIDs[pendingID]; pendingID == "" || !matches {
				return nil, true
			}
			s.detachCursorAgentSessionLocked(session)
			return session, true
		}
	}

	for index := len(toolCallIDs) - 1; index >= 0; index-- {
		toolCallID := strings.TrimSpace(toolCallIDs[index])
		if toolCallID == "" {
			continue
		}
		ref := cursorAgentSessionRef(owner, "tool", toolCallID)
		session := s.agentSessions[ref]
		if session == nil {
			continue
		}
		pendingID := ""
		if session.Pending != nil {
			pendingID = strings.TrimSpace(session.Pending.Action.ID)
		}
		if pendingID == "" || pendingID != toolCallID {
			continue
		}
		s.detachCursorAgentSessionLocked(session)
		return session, true
	}
	return nil, false
}

func (s *CursorGatewayService) expireCursorAgentSession(session *cursorAgentActiveSession) {
	if s == nil || session == nil {
		return
	}
	s.agentSessionMu.Lock()
	registered := false
	for _, ref := range session.refs {
		if s.agentSessions[ref] == session {
			registered = true
			break
		}
	}
	if registered {
		s.detachCursorAgentSessionLocked(session)
	}
	s.agentSessionMu.Unlock()
	if registered {
		session.Close()
	}
}

func (s *CursorGatewayService) removeCursorAgentSession(session *cursorAgentActiveSession) bool {
	if s == nil || session == nil {
		return false
	}
	s.agentSessionMu.Lock()
	registered := false
	for _, ref := range session.refs {
		if s.agentSessions[ref] == session {
			registered = true
			break
		}
	}
	if registered {
		s.detachCursorAgentSessionLocked(session)
	}
	s.agentSessionMu.Unlock()
	return registered
}

func (s *CursorGatewayService) closeCursorAgentSessions() {
	if s == nil {
		return
	}
	s.agentSessionMu.Lock()
	unique := make(map[*cursorAgentActiveSession]struct{}, len(s.agentSessions))
	for _, session := range s.agentSessions {
		if session != nil {
			unique[session] = struct{}{}
		}
	}
	for session := range unique {
		s.detachCursorAgentSessionLocked(session)
	}
	s.agentSessionMu.Unlock()
	for session := range unique {
		session.Close()
	}
}

type cursorCollected struct {
	Text         string
	CleanText    string
	Reasoning    string
	Actions      []cursorpkg.Action
	Usage        cursorpkg.Usage
	FinishReason string
}

type CursorDashboardUsageResult struct {
	Usage                 *cursorpkg.DashboardUsage
	RefreshedAccessToken  string
	RefreshedRefreshToken string
}

func (s *CursorGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	protocol := cursorpkg.ProtocolAnthropic
	if isOpenAIInboundPath(c) {
		protocol = cursorpkg.ProtocolOpenAIChat
	}
	return s.forward(ctx, c, account, body, protocol)
}

func (s *CursorGatewayService) ForwardResponses(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	return s.forward(ctx, c, account, body, cursorpkg.ProtocolResponses)
}

func (s *CursorGatewayService) Probe(ctx context.Context, account *Account, _, _ string) (string, error) {
	if s == nil || s.httpUpstream == nil {
		return "", errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorAPIKey() {
		return "", errors.New("a Cursor account is required")
	}
	if err := ValidateCursorAccountCredentials(account.Type, account.Credentials); err != nil {
		return "", err
	}
	if s.cursorTransportMode(account) == CursorTransportIDEChat {
		if s.cursorConfig().AgentRPCEnabled {
			return s.probeCursorIDE(ctx, account)
		}
		rawMode := cursorAccountSetting(account, "cursor_transport_mode")
		if strings.TrimSpace(rawMode) == "" {
			rawMode = s.cursorConfig().DefaultTransportMode
		}
		if NormalizeCursorTransportMode(rawMode) != CursorTransportAuto || strings.TrimSpace(account.GetCredential("api_key")) == "" {
			return "", errors.New("cursor Agent RPC is disabled")
		}
	}
	client, err := s.newCloudClient(ctx, account)
	if err != nil {
		return "", err
	}
	identity, err := client.Me(ctx)
	if err != nil {
		return "", err
	}
	if identity.UserEmail != "" {
		return identity.UserEmail, nil
	}
	if identity.APIKeyName != "" {
		return identity.APIKeyName, nil
	}
	return "Cursor Cloud Agent API key verified", nil
}

func (s *CursorGatewayService) FetchDashboardUsage(ctx context.Context, account *Account) (*CursorDashboardUsageResult, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorAPIKey() {
		return nil, errors.New("a Cursor API key account is required")
	}
	accessToken := strings.TrimSpace(account.GetCredential("dashboard_access_token"))
	if accessToken == "" {
		return nil, errors.New("cursor Dashboard access token is missing")
	}
	client, err := s.newDashboardClient(ctx, account, accessToken)
	if err != nil {
		return nil, err
	}
	usage, err := client.FetchUsage(ctx)
	if err == nil {
		return &CursorDashboardUsageResult{Usage: usage}, nil
	}
	if !cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) {
		return nil, err
	}
	refreshToken := strings.TrimSpace(account.GetCredential("dashboard_refresh_token"))
	if refreshToken == "" {
		return nil, err
	}
	refreshed, refreshErr := client.RefreshAccessToken(ctx, refreshToken)
	if refreshErr != nil {
		return nil, refreshErr
	}
	if refreshed.ShouldLogout {
		return nil, errCursorDashboardReauthRequired
	}
	retryClient, clientErr := s.newDashboardClient(ctx, account, refreshed.AccessToken)
	if clientErr != nil {
		return nil, clientErr
	}
	usage, err = retryClient.FetchUsage(ctx)
	if err != nil {
		return nil, err
	}
	return &CursorDashboardUsageResult{
		Usage:                 usage,
		RefreshedAccessToken:  refreshed.AccessToken,
		RefreshedRefreshToken: refreshed.RefreshToken,
	}, nil
}

func (s *CursorGatewayService) CountTokens(body []byte, protocol cursorpkg.Protocol) (int, error) {
	dialogue, err := cursorpkg.ParseRequest(protocol, body)
	if err != nil {
		return 0, err
	}
	return estimateCursorDialogueTokens(dialogue), nil
}

func (s *CursorGatewayService) forward(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol cursorpkg.Protocol) (*ForwardResult, error) {
	if s.cursorForwardTransportMode(account) != CursorTransportIDEChat {
		return s.forwardCloud(ctx, c, account, body, protocol)
	}
	if !s.cursorConfig().AgentRPCEnabled {
		rawMode := cursorAccountSetting(account, "cursor_transport_mode")
		if strings.TrimSpace(rawMode) == "" {
			rawMode = s.cursorConfig().DefaultTransportMode
		}
		if account != nil && strings.TrimSpace(account.GetCredential("api_key")) != "" && NormalizeCursorTransportMode(rawMode) == CursorTransportAuto {
			return s.forwardCloud(ctx, c, account, body, protocol)
		}
		return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte("Cursor Agent RPC is disabled")}
	}

	result, err := s.forwardIDE(ctx, c, account, body, protocol)
	if err == nil {
		return result, nil
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	requestModel := cursorRequestModelForLog(body)
	statusCode, upstreamErrorCode := cursorFailoverErrorMetadata(err)
	shouldFallback, blockedReason := s.shouldFallbackCursorIDEToCloud(c, account, body, protocol, err)
	if !shouldFallback {
		if blockedReason != "" {
			slog.Warn("cursor_ide_cloud_fallback_skipped",
				"account_id", accountID,
				"model", requestModel,
				"status_code", statusCode,
				"upstream_error_code", upstreamErrorCode,
				"reason", blockedReason,
				"error", err.Error(),
			)
		}
		return result, err
	}

	slog.Warn("cursor_ide_auto_fallback",
		"account_id", accountID,
		"model", requestModel,
		"status_code", statusCode,
		"upstream_error_code", upstreamErrorCode,
		"fallback", CursorTransportCloudAgent,
		"error", err.Error(),
	)
	return s.forwardCloud(ctx, c, account, body, protocol)
}

func cursorRequestModelForLog(body []byte) string {
	var envelope cursorRequestEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Model)
}

func cursorFailoverErrorMetadata(err error) (int, string) {
	var upstreamErr *UpstreamFailoverError
	if !errors.As(err, &upstreamErr) || upstreamErr == nil {
		return 0, ""
	}
	var envelope struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(upstreamErr.ResponseBody, &envelope) != nil {
		return upstreamErr.StatusCode, ""
	}
	code := strings.TrimSpace(envelope.Error.Type)
	if code == "" {
		code = strings.TrimSpace(envelope.Error.Code)
	}
	return upstreamErr.StatusCode, code
}

func (s *CursorGatewayService) shouldFallbackCursorIDEToCloud(c *gin.Context, account *Account, body []byte, protocol cursorpkg.Protocol, err error) (bool, string) {
	if !s.cursorConfig().AgentCloudFallbackEnabled || err == nil || account == nil || strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return false, ""
	}
	if c != nil && c.Writer != nil && c.Writer.Written() {
		return false, ""
	}
	rawMode := cursorAccountSetting(account, "cursor_transport_mode")
	if strings.TrimSpace(rawMode) == "" {
		rawMode = s.cursorConfig().DefaultTransportMode
	}
	if NormalizeCursorTransportMode(rawMode) != CursorTransportAuto {
		return false, ""
	}
	var upstreamErr *UpstreamFailoverError
	if !errors.As(err, &upstreamErr) {
		return false, ""
	}
	if upstreamErr.StatusCode != http.StatusTooManyRequests && upstreamErr.StatusCode < http.StatusInternalServerError {
		return false, ""
	}
	if required, reason := cursorRequestRequiresAgentRPC(body, protocol); required {
		return false, reason
	}
	return true, ""
}

func cursorRequestRequiresAgentRPC(body []byte, protocol cursorpkg.Protocol) (bool, string) {
	var envelope cursorRequestEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return true, "request_parse_failed"
	}
	if protocol == cursorpkg.ProtocolResponses && strings.TrimSpace(envelope.PreviousResponseID) != "" {
		return true, "responses_continuation"
	}
	dialogue, err := cursorpkg.ParseRequest(protocol, body)
	if err != nil {
		return true, "request_parse_failed"
	}
	if len(dialogue.Tools) > 0 {
		return true, "client_tools"
	}
	for _, message := range dialogue.Messages {
		if len(message.Images) > 0 {
			return true, "inline_images"
		}
		if len(message.ToolCalls) > 0 || strings.TrimSpace(message.ToolCallID) != "" || strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			return true, "tool_continuation"
		}
	}
	return false, ""
}

func cursorCloudPromptImages(dialogue *cursorpkg.Dialogue) []cursorpkg.CloudImage {
	if dialogue == nil {
		return nil
	}
	var images []cursorpkg.CloudImage
	for _, message := range dialogue.Messages {
		for _, image := range message.Images {
			if len(image.Data) == 0 || strings.TrimSpace(image.MIMEType) == "" {
				continue
			}
			images = append(images, cursorpkg.CloudImage{
				Data:     base64.StdEncoding.EncodeToString(image.Data),
				MimeType: image.MIMEType,
			})
		}
	}
	return images
}

func (s *CursorGatewayService) forwardCloud(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol cursorpkg.Protocol) (*ForwardResult, error) {
	start := time.Now()
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorAPIKey() {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("a Cursor API key account is required")}
	}
	if err := ValidateCursorAccountCredentials(account.Type, account.Credentials); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, ResponseBody: []byte(err.Error())}
	}

	var envelope cursorRequestEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, cursorRequestFailoverError("invalid request body: " + err.Error())
	}
	requestModel := strings.TrimSpace(envelope.Model)
	if requestModel == "" {
		requestModel = "cursor-chat"
	}
	upstreamModel, _ := account.ResolveMappedModel(requestModel)
	if override := cursorAccountSetting(account, "cursor_upstream_model"); override != "" {
		upstreamModel = override
	}
	if upstreamModel == "" {
		upstreamModel = s.cursorConfig().DefaultModel
	}
	variantPreference := envelope.variantPreference()
	upstreamModel, variantPreference = normalizeCursorCloudModel(upstreamModel, variantPreference)

	dialogue, err := cursorpkg.ParseRequest(protocol, body)
	if err != nil {
		return nil, mapCursorError(err)
	}
	if protocol == cursorpkg.ProtocolResponses && strings.TrimSpace(envelope.PreviousResponseID) != "" {
		previous, loadErr := s.loadCursorResponse(ctx, c, envelope.PreviousResponseID)
		if loadErr != nil {
			return nil, cursorRequestFailoverError(loadErr.Error())
		}
		if strings.TrimSpace(dialogue.System) == "" {
			dialogue.System = previous.System
		}
		dialogue.Messages = append(append([]cursorpkg.DialogueMessage(nil), previous.Messages...), dialogue.Messages...)
	}
	client, err := s.newCloudClient(ctx, account)
	if err != nil {
		return nil, mapCursorError(err)
	}
	payload, err := cursorpkg.BuildPayload(dialogue, cursorpkg.BuildOptions{
		Model:              upstreamModel,
		ConversationID:     uuid.NewString(),
		MaxHistoryMessages: s.cursorConfig().MaxHistoryMessages,
		MaxHistoryTokens:   s.cursorConfig().MaxHistoryTokens,
	})
	if err != nil {
		return nil, mapCursorError(err)
	}
	estimatedInput := estimateCursorPayloadTokens(payload)
	modelRef := cursorCloudModelRef(account, upstreamModel, variantPreference)
	if modelRef != nil && len(modelRef.Params) > 0 {
		models, listErr := client.ListModels(ctx)
		if listErr != nil {
			return nil, mapCursorError(listErr)
		}
		modelRef, err = completeCursorCloudModelRef(modelRef, models)
		if err != nil {
			return nil, cursorRequestFailoverError(err.Error())
		}
	}
	created, err := client.CreateAgent(ctx, cursorpkg.CreateAgentRequest{
		Prompt: cursorpkg.CloudPrompt{
			Text:   cursorpkg.RenderAgentPrompt(payload),
			Images: cursorCloudPromptImages(dialogue),
		},
		Model: modelRef,
		Name:  "Sub2API compatibility request",
	})
	if err != nil {
		return nil, mapCursorError(err)
	}
	completed := false
	defer func() { s.cleanupCloudAgent(client, created.Agent.ID, created.Run.ID, !completed) }()
	collected, firstTokenMs, err := collectCloudResponse(ctx, client, created, start)
	if err != nil {
		return nil, mapCursorError(err)
	}
	completed = true
	if err := validateCursorToolResult(dialogue, collected.Actions); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
	}
	if collected.Usage.InputTokens <= 0 {
		collected.Usage.InputTokens = estimatedInput
	}
	if collected.Usage.OutputTokens <= 0 {
		collected.Usage.OutputTokens = cursorpkg.EstimateTokens(collected.CleanText) + estimateCursorActionTokens(collected.Actions)
	}
	collected.Usage.TotalTokens = collected.Usage.InputTokens + collected.Usage.OutputTokens + collected.Usage.CacheWriteTokens + collected.Usage.CacheReadTokens

	responseID := cursorResponseID(protocol)
	if protocol == cursorpkg.ProtocolResponses && (envelope.Store == nil || *envelope.Store) {
		storedDialogue := &cursorpkg.Dialogue{System: dialogue.System, Tools: dialogue.Tools, ToolChoice: dialogue.ToolChoice, Messages: append([]cursorpkg.DialogueMessage(nil), dialogue.Messages...)}
		storedDialogue.Messages = append(storedDialogue.Messages, cursorpkg.DialogueMessage{Role: "assistant", Text: collected.CleanText, ToolCalls: collected.Actions})
		if saveErr := s.saveCursorResponse(ctx, c, responseID, storedDialogue); saveErr != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte("failed to store Cursor response continuation: " + saveErr.Error())}
		}
	}
	if envelope.Stream {
		if err := writeCursorStream(c, protocol, responseID, requestModel, collected); err != nil {
			return nil, err
		}
	} else {
		writeCursorJSON(c, protocol, responseID, requestModel, envelope.PreviousResponseID, collected)
	}
	result := &ForwardResult{
		RequestID: responseID,
		Usage: ClaudeUsage{
			InputTokens:              collected.Usage.InputTokens,
			OutputTokens:             collected.Usage.OutputTokens,
			CacheCreationInputTokens: collected.Usage.CacheWriteTokens,
			CacheReadInputTokens:     collected.Usage.CacheReadTokens,
		},
		Model:         requestModel,
		UpstreamModel: differentOrEmpty(requestModel, upstreamModel),
		Stream:        envelope.Stream,
		Duration:      time.Since(start),
		FirstTokenMs:  firstTokenMs,
	}
	if variantPreference.Effort != "" {
		result.ReasoningEffort = &variantPreference.Effort
	}
	return result, nil
}

func (s *CursorGatewayService) newCloudClient(ctx context.Context, account *Account) (*cursorpkg.CloudClient, error) {
	cfg := s.cursorConfig()
	baseURL, err := cursorEndpoint(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	httpClient := s.newCursorHTTPClient(ctx, account)
	return cursorpkg.NewCloudClient(httpClient, account.GetCredential("api_key"), cursorpkg.CloudClientConfig{
		BaseURL:           baseURL,
		RequestTimeout:    durationSeconds(cfg.RequestTimeoutSeconds, 120),
		StreamIdleTimeout: durationSeconds(cfg.StreamIdleTimeoutSeconds, 60),
		MaxErrorBody:      8 << 10,
	})
}

func (s *CursorGatewayService) newDashboardClient(ctx context.Context, account *Account, accessToken string) (*cursorpkg.DashboardClient, error) {
	cfg := s.cursorConfig()
	baseURL, err := cursorDashboardEndpoint(cfg.DashboardBaseURL)
	if err != nil {
		return nil, err
	}
	return cursorpkg.NewDashboardClient(s.newCursorHTTPClient(ctx, account), accessToken, cursorpkg.DashboardClientConfig{
		BaseURL:        baseURL,
		RequestTimeout: durationSeconds(cfg.RequestTimeoutSeconds, 120),
		MaxErrorBody:   8 << 10,
	})
}

func (s *CursorGatewayService) newDashboardAuthClient(ctx context.Context, account *Account) (*cursorpkg.DashboardAuthClient, error) {
	cfg := s.cursorConfig()
	baseURL, err := cursorDashboardEndpoint(cfg.DashboardBaseURL)
	if err != nil {
		return nil, err
	}
	websiteURL, err := cursorDashboardWebsiteEndpoint(cfg.DashboardAuthWebsiteURL)
	if err != nil {
		return nil, err
	}
	return cursorpkg.NewDashboardAuthClient(s.newCursorHTTPClient(ctx, account), cursorpkg.DashboardAuthClientConfig{
		BaseURL:        baseURL,
		WebsiteURL:     websiteURL,
		RequestTimeout: durationSeconds(cfg.RequestTimeoutSeconds, 120),
		MaxErrorBody:   8 << 10,
	})
}

func (s *CursorGatewayService) newCursorHTTPClient(ctx context.Context, account *Account) *http.Client {
	return s.newCursorHTTPClientWithProfile(ctx, account, HTTPUpstreamProfileDefault)
}

func (s *CursorGatewayService) newCursorIDEHTTPClient(ctx context.Context, account *Account) *http.Client {
	return s.newCursorHTTPClientWithProfile(ctx, account, HTTPUpstreamProfileCursorH2)
}

func (s *CursorGatewayService) newCursorHTTPClientWithProfile(ctx context.Context, account *Account, profile HTTPUpstreamProfile) *http.Client {
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	} else if account.ProxyID != nil && s.proxyRepo != nil {
		if proxy, proxyErr := s.proxyRepo.GetByID(ctx, *account.ProxyID); proxyErr == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	return &http.Client{Transport: &cursorRoundTripper{upstream: s.httpUpstream, proxyURL: proxyURL, accountID: account.ID, concurrency: account.Concurrency, profile: profile}}
}

func (s *CursorGatewayService) cursorConfig() config.CursorConfig {
	if s != nil && s.cfg != nil {
		return s.cfg.Cursor
	}
	return config.CursorConfig{
		BaseURL: cursorpkg.DefaultCloudBaseURL, ChatBaseURL: cursorpkg.DefaultDashboardBaseURL,
		DefaultTransportMode: CursorTransportAuto, ClientVersion: "3.11.13",
		MaxFrameBytes: 8 << 20, MaxBufferedBytes: 16 << 20,
		ResponseHeaderTimeoutSeconds: 60, IDEStreamIdleTimeoutSeconds: 60,
		AgentRPCEnabled: true, AgentCloudFallbackEnabled: true,
		AgentModelCacheTTLSeconds: 300, AgentModelStaleTTLSeconds: 1800, AgentModelProbeTimeoutSeconds: 5,
		AgentModelPrewarmEnabled: true, AgentModelPrewarmConcurrency: 3,
		DashboardBaseURL: cursorpkg.DefaultDashboardBaseURL, DashboardAuthWebsiteURL: cursorpkg.DefaultDashboardWebsiteURL,
		DashboardMaintenanceEnabled: true, DashboardMaintenanceIntervalMins: 30, DashboardProbeIntervalMins: 360,
		DashboardRefreshBeforeExpiryHours: 1272, DashboardLoginSessionTTLMins: 5,
		DefaultModel: "auto", RequestTimeoutSeconds: 120, StreamIdleTimeoutSeconds: 60,
		MaxHistoryTokens: 24000, MaxHistoryMessages: 100, ResponsesTTLSeconds: 86400,
	}
}

func (s *CursorGatewayService) cursorTransportMode(account *Account) string {
	raw := cursorAccountSetting(account, "cursor_transport_mode")
	if strings.TrimSpace(raw) == "" {
		raw = s.cursorConfig().DefaultTransportMode
	}
	mode := NormalizeCursorTransportMode(raw)
	if mode == "" {
		mode = CursorTransportAuto
	}
	if mode == CursorTransportAuto {
		if account != nil && strings.TrimSpace(account.GetCredential("dashboard_access_token")) != "" {
			return CursorTransportIDEChat
		}
		return CursorTransportCloudAgent
	}
	return mode
}

func (s *CursorGatewayService) cursorForwardTransportMode(account *Account) string {
	mode := s.cursorTransportMode(account)
	if mode != CursorTransportIDEChat || strings.TrimSpace(cursorAccountSetting(account, "cursor_machine_id")) != "" {
		return mode
	}
	raw := cursorAccountSetting(account, "cursor_transport_mode")
	if strings.TrimSpace(raw) == "" {
		raw = s.cursorConfig().DefaultTransportMode
	}
	if NormalizeCursorTransportMode(raw) == CursorTransportAuto {
		accountID := int64(0)
		if account != nil {
			accountID = account.ID
		}
		slog.Warn("cursor_ide_machine_id_missing_fallback", "account_id", accountID, "fallback", CursorTransportCloudAgent)
		return CursorTransportCloudAgent
	}
	return mode
}

func durationSeconds(value, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}

func cursorEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = cursorpkg.DefaultCloudBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("cursor base_url must be a valid HTTPS URL")
	}
	if !strings.EqualFold(parsed.Hostname(), "api.cursor.com") {
		return "", fmt.Errorf("cursor base_url host must be api.cursor.com")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func cursorDashboardEndpoint(raw string) (string, error) {
	return cursorAPI2Endpoint(raw, "dashboard_base_url")
}

func cursorChatEndpoint(raw string) (string, error) {
	return cursorAPI2Endpoint(raw, "chat_base_url")
}

func cursorAPI2Endpoint(raw, configKey string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = cursorpkg.DefaultDashboardBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("cursor %s must be a valid HTTPS URL", configKey)
	}
	if !strings.EqualFold(parsed.Hostname(), "api2.cursor.sh") {
		return "", fmt.Errorf("cursor %s host must be api2.cursor.sh", configKey)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func cursorDashboardWebsiteEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = cursorpkg.DefaultDashboardWebsiteURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("cursor dashboard_auth_website_url must be a valid HTTPS URL")
	}
	if !strings.EqualFold(parsed.Hostname(), "cursor.com") {
		return "", fmt.Errorf("cursor dashboard_auth_website_url host must be cursor.com")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

type cursorRoundTripper struct {
	upstream    HTTPUpstream
	proxyURL    string
	accountID   int64
	concurrency int
	profile     HTTPUpstreamProfile
}

func (t *cursorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.upstream == nil {
		return nil, errors.New("cursor HTTP upstream is not configured")
	}
	if req != nil && t.profile != HTTPUpstreamProfileDefault {
		req = req.Clone(WithHTTPUpstreamProfile(req.Context(), t.profile))
	}
	return t.upstream.Do(req, t.proxyURL, t.accountID, t.concurrency)
}

func collectCloudResponse(ctx context.Context, client *cursorpkg.CloudClient, created *cursorpkg.CreateAgentResponse, start time.Time) (cursorCollected, *int, error) {
	var out cursorCollected
	var streamed strings.Builder
	var finalText string
	var firstTokenMs *int
	err := client.StreamRun(ctx, created.Agent.ID, created.Run.ID, func(event cursorpkg.CloudSSEEvent) error {
		name := strings.ToLower(strings.TrimSpace(event.Event))
		if usage := cloudEventUsage(event.Data); usage != nil {
			out.Usage = *usage
		}
		switch name {
		case "error":
			message := cloudEventError(event.Data)
			if message == "" {
				message = "Cursor Cloud Agent run failed"
			}
			return cursorpkg.HTTPError(http.StatusBadGateway, "stream run", message)
		case "result":
			if text := cloudEventText(event.Data); text != "" {
				finalText = text
			}
			out.FinishReason = "stop"
		case "interaction_update":
			if text := cloudEventDelta(event.Data); text != "" {
				if firstTokenMs == nil {
					ms := int(time.Since(start).Milliseconds())
					firstTokenMs = &ms
				}
				_, _ = streamed.WriteString(text)
			}
		case "assistant":
			if streamed.Len() == 0 {
				if text := cloudEventText(event.Data); text != "" {
					_, _ = streamed.WriteString(text)
				}
			}
		}
		return nil
	})
	if err != nil {
		return out, firstTokenMs, err
	}
	if finalText == "" {
		finalText = streamed.String()
	}
	if finalText == "" {
		run, waitErr := waitForCloudRun(ctx, client, created.Agent.ID, created.Run.ID)
		if waitErr != nil {
			return out, firstTokenMs, waitErr
		}
		finalText = cloudEventText(run.Result)
		if run.Usage != nil {
			out.Usage = cursorpkg.Usage{
				InputTokens:      run.Usage.InputTokens,
				OutputTokens:     run.Usage.OutputTokens,
				CacheWriteTokens: run.Usage.CacheWriteTokens,
				CacheReadTokens:  run.Usage.CacheReadTokens,
				ReasoningTokens:  run.Usage.ReasoningTokens,
				TotalTokens:      run.Usage.TotalTokens,
			}
		}
		if finalText == "" {
			return out, firstTokenMs, cursorpkg.HTTPError(http.StatusBadGateway, "get run", "Cursor run finished without a result")
		}
	}
	if firstTokenMs == nil && finalText != "" {
		ms := int(time.Since(start).Milliseconds())
		firstTokenMs = &ms
	}
	out.Text = finalText
	out.FinishReason = "stop"
	actions, clean, err := cursorpkg.ParseActions(out.Text)
	if err != nil {
		return out, firstTokenMs, err
	}
	for i := range actions {
		if actions[i].ID == "" {
			actions[i].ID = fmt.Sprintf("call_%s_%d", uuid.NewString()[:8], i)
		}
	}
	out.Actions = actions
	out.CleanText = clean
	return out, firstTokenMs, nil
}

func waitForCloudRun(ctx context.Context, client *cursorpkg.CloudClient, agentID, runID string) (*cursorpkg.CloudRun, error) {
	const pollInterval = 500 * time.Millisecond
	for {
		run, err := client.GetRun(ctx, agentID, runID)
		if err != nil {
			return nil, err
		}
		switch strings.ToUpper(strings.TrimSpace(run.Status)) {
		case "FINISHED", "COMPLETED":
			return run, nil
		case "ERROR", "CANCELLED", "EXPIRED":
			return nil, cursorpkg.HTTPError(http.StatusBadGateway, "get run", "Cursor run ended with status "+run.Status)
		case "CREATING", "RUNNING", "":
		default:
			return nil, cursorpkg.HTTPError(http.StatusBadGateway, "get run", "Cursor returned unknown run status "+run.Status)
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func cursorCloudModelRef(account *Account, model string, preference cursorVariantPreference) *cursorpkg.ModelRef {
	model = strings.TrimSpace(model)
	if model == "" || strings.EqualFold(model, "auto") {
		return nil
	}
	ref := &cursorpkg.ModelRef{ID: model}
	params := make(map[string]string)
	if account != nil && account.Credentials != nil {
		if raw, ok := account.Credentials["cursor_model_params"]; ok {
			var configured []cursorpkg.ModelParam
			encoded, err := json.Marshal(raw)
			if err == nil && json.Unmarshal(encoded, &configured) == nil {
				for _, item := range configured {
					id := strings.TrimSpace(item.ID)
					if id != "" {
						params[id] = strings.TrimSpace(item.Value)
					}
				}
			}
		}
	}
	if preference.Thinking != nil {
		params["thinking"] = strconv.FormatBool(*preference.Thinking)
	}
	if preference.Effort != "" {
		params["effort"] = preference.Effort
	}
	if preference.Context != "" {
		params["context"] = preference.Context
	}
	if preference.Fast != nil {
		params["fast"] = strconv.FormatBool(*preference.Fast)
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ref.Params = append(ref.Params, cursorpkg.ModelParam{ID: key, Value: params[key]})
	}
	return ref
}

func completeCursorCloudModelRef(ref *cursorpkg.ModelRef, models []cursorpkg.CloudModel) (*cursorpkg.ModelRef, error) {
	if ref == nil || len(ref.Params) == 0 {
		return ref, nil
	}
	var selectedModel *cursorpkg.CloudModel
	for index := range models {
		model := &models[index]
		if strings.EqualFold(strings.TrimSpace(model.ID), strings.TrimSpace(ref.ID)) {
			selectedModel = model
			break
		}
		for _, alias := range model.Aliases {
			if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(ref.ID)) {
				selectedModel = model
				break
			}
		}
		if selectedModel != nil {
			break
		}
	}
	if selectedModel == nil || len(selectedModel.Variants) == 0 {
		return ref, nil
	}

	desired := make(map[string]string, len(ref.Params))
	for _, param := range ref.Params {
		desired[strings.TrimSpace(param.ID)] = strings.TrimSpace(param.Value)
	}
	base := selectedModel.Variants[0]
	for _, variant := range selectedModel.Variants {
		if variant.IsDefault {
			base = variant
			break
		}
	}
	target := cursorModelParamMap(base.Params)
	for key, value := range desired {
		target[key] = value
	}
	for _, variant := range selectedModel.Variants {
		if cursorModelParamMapsEqual(target, cursorModelParamMap(variant.Params)) {
			return &cursorpkg.ModelRef{ID: selectedModel.ID, Params: append([]cursorpkg.ModelParam(nil), variant.Params...)}, nil
		}
	}
	for _, variant := range selectedModel.Variants {
		params := cursorModelParamMap(variant.Params)
		matches := true
		for key, value := range desired {
			if params[key] != value {
				matches = false
				break
			}
		}
		if matches {
			return &cursorpkg.ModelRef{ID: selectedModel.ID, Params: append([]cursorpkg.ModelParam(nil), variant.Params...)}, nil
		}
	}
	return nil, fmt.Errorf("cursor model %q has no variant matching the requested parameters", ref.ID)
}

func cursorModelParamMap(params []cursorpkg.ModelParam) map[string]string {
	result := make(map[string]string, len(params))
	for _, param := range params {
		result[strings.TrimSpace(param.ID)] = strings.TrimSpace(param.Value)
	}
	return result
}

func cursorModelParamMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func (s *CursorGatewayService) cleanupCloudAgent(client *cursorpkg.CloudClient, agentID, runID string, cancelRun bool) {
	if client == nil || agentID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cancelRun && runID != "" {
			_ = client.CancelRun(ctx, agentID, runID)
		}

		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			if err := client.DeleteAgent(ctx, agentID); err == nil {
				return
			} else {
				lastErr = err
			}
			if attempt == 3 {
				break
			}
			timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				lastErr = ctx.Err()
				attempt = 3
			case <-timer.C:
			}
		}
		slog.Warn("cursor_agent_cleanup_failed", "agent_id", agentID, "run_id", runID, "error", lastErr)
	}()
}

func cloudEventDelta(data json.RawMessage) string {
	var value any
	if len(data) == 0 || json.Unmarshal(data, &value) != nil {
		return ""
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"delta", "textDelta", "text_delta"} {
			if text, ok := object[key].(string); ok {
				return text
			}
		}
		if eventType, _ := object["type"].(string); strings.Contains(strings.ToLower(eventType), "text") {
			if text, ok := object["text"].(string); ok {
				return text
			}
		}
	}
	return ""
}

func cloudEventText(data json.RawMessage) string {
	var value any
	if len(data) == 0 || json.Unmarshal(data, &value) != nil {
		return ""
	}
	return cloudTextValue(value)
}

func cloudTextValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			_, _ = builder.WriteString(cloudTextValue(item))
		}
		return builder.String()
	case map[string]any:
		for _, key := range []string{"text", "result", "output", "content", "message"} {
			if child, ok := typed[key]; ok {
				if text := cloudTextValue(child); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func cloudEventUsage(data json.RawMessage) *cursorpkg.Usage {
	var value any
	if len(data) == 0 || json.Unmarshal(data, &value) != nil {
		return nil
	}
	object := findCloudObject(value, "usage")
	if object == nil {
		object, _ = value.(map[string]any)
	}
	if object == nil {
		return nil
	}
	usage := &cursorpkg.Usage{
		InputTokens:      cloudInt(object, "inputTokens", "input_tokens"),
		OutputTokens:     cloudInt(object, "outputTokens", "output_tokens"),
		CacheWriteTokens: cloudInt(object, "cacheWriteTokens", "cache_write_tokens", "cacheCreationTokens", "cache_creation_tokens"),
		CacheReadTokens:  cloudInt(object, "cacheReadTokens", "cache_read_tokens"),
		ReasoningTokens:  cloudInt(object, "reasoningTokens", "reasoning_tokens"),
		TotalTokens:      cloudInt(object, "totalTokens", "total_tokens"),
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CacheWriteTokens == 0 && usage.CacheReadTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

func findCloudObject(value any, key string) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if child, ok := typed[key].(map[string]any); ok {
			return child
		}
		for _, child := range typed {
			if found := findCloudObject(child, key); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findCloudObject(child, key); found != nil {
				return found
			}
		}
	}
	return nil
}

func cloudInt(object map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, ok := object[key].(float64); ok {
			return int(value)
		}
	}
	return 0
}

func cloudEventError(data json.RawMessage) string {
	var value any
	if len(data) == 0 || json.Unmarshal(data, &value) != nil {
		return ""
	}
	return cloudErrorValue(value)
}

func cloudErrorValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range []string{"message", "detail", "error"} {
			if child, ok := typed[key]; ok {
				if text := cloudErrorValue(child); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func validateCursorToolResult(dialogue *cursorpkg.Dialogue, actions []cursorpkg.Action) error {
	if dialogue == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(dialogue.Tools))
	for _, tool := range dialogue.Tools {
		allowed[tool.Name] = struct{}{}
	}
	for _, action := range actions {
		if _, ok := allowed[action.Name]; !ok {
			return fmt.Errorf("cursor compatibility mode returned unknown tool %q", action.Name)
		}
	}
	if (dialogue.ToolChoice.Mode == "any" || dialogue.ToolChoice.Mode == "required" || dialogue.ToolChoice.Mode == "tool" || dialogue.ToolChoice.Mode == "function") && len(actions) == 0 {
		return errors.New("cursor compatibility mode did not return the required tool call")
	}
	if (dialogue.ToolChoice.Mode == "tool" || dialogue.ToolChoice.Mode == "function") && dialogue.ToolChoice.Name != "" {
		for _, action := range actions {
			if action.Name != dialogue.ToolChoice.Name {
				return fmt.Errorf("cursor compatibility mode returned tool %q instead of required tool %q", action.Name, dialogue.ToolChoice.Name)
			}
		}
	}
	return nil
}

func estimateCursorPayloadTokens(payload *cursorpkg.Request) int {
	if payload == nil {
		return 0
	}
	total := 0
	for _, message := range payload.Messages {
		total += cursorpkg.EstimateMessageTokens(message)
	}
	return total
}

func estimateCursorActionTokens(actions []cursorpkg.Action) int {
	total := 0
	for _, action := range actions {
		encoded, _ := json.Marshal(action.Arguments)
		total += cursorpkg.EstimateTokens(action.Name) + cursorpkg.EstimateTokens(string(encoded))
	}
	return total
}

func cursorResponseID(protocol cursorpkg.Protocol) string {
	prefix := "msg"
	switch protocol {
	case cursorpkg.ProtocolOpenAIChat:
		prefix = "chatcmpl"
	case cursorpkg.ProtocolResponses:
		prefix = "resp"
	}
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func mapCursorError(err error) error {
	if err == nil {
		return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte("Cursor request failed")}
	}
	var cursorErr *cursorpkg.Error
	if errors.As(err, &cursorErr) {
		if cursorErrorIndicatesPayloadTooLarge(cursorErr.Error()) {
			return cursorRequestPayloadTooLargeError("Cursor request payload exceeds the Agent frame limit")
		}
		status := cursorErr.StatusCode
		if status == 0 {
			if cursorErr.Kind == cursorpkg.ErrorBadRequest {
				status = http.StatusBadRequest
			} else {
				status = http.StatusBadGateway
			}
		}
		failure := &UpstreamFailoverError{StatusCode: status, ResponseBody: []byte(cursorErr.Error())}
		if cursorErr.Kind == cursorpkg.ErrorBadRequest {
			clientMessage := cursorBadRequestClientMessage(cursorErr)
			failure.ResponseBody = []byte(clientMessage)
			failure.Stage = GatewayFailureStageInference
			failure.Scope = GatewayFailureScopeRequest
			failure.Reason = GatewayFailureReason("cursor_invalid_request")
			failure.NextAccountAction = NextAccountStop
			failure.ClientStatusCode = http.StatusBadRequest
			failure.ClientMessage = clientMessage
		}
		return failure
	}
	if cursorErrorIndicatesPayloadTooLarge(err.Error()) {
		return cursorRequestPayloadTooLargeError("Cursor request payload exceeds the Agent frame limit")
	}
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
}

func cursorBadRequestClientMessage(err *cursorpkg.Error) string {
	if err == nil {
		return "Invalid Cursor request"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unsupported remote image source"):
		return "unsupported remote image source; use base64 image data"
	case strings.Contains(message, "invalid base64 image data"):
		return "invalid base64 image data"
	case strings.Contains(message, "image source is required"):
		return "image source is required"
	case strings.Contains(message, "image data is required"):
		return "image data is required"
	case strings.Contains(message, "image data is empty"):
		return "image data is empty"
	case strings.Contains(message, "image data does not match media_type"):
		return "image data does not match media_type"
	case strings.Contains(message, "image media_type must be one of"):
		return "image media_type must be one of image/png, image/jpeg, image/gif, or image/webp"
	case strings.Contains(message, "image content is only valid for user messages"):
		return "image content is only valid for user messages"
	default:
		return "Invalid Cursor request"
	}
}

func cursorErrorIndicatesPayloadTooLarge(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "inline image count exceeds") ||
		strings.Contains(message, "inline image exceeds") ||
		strings.Contains(message, "inline images exceed") ||
		strings.Contains(message, "agent frame size") ||
		(strings.Contains(message, "encode ide chat frame") && strings.Contains(message, "frame size") && strings.Contains(message, "exceeds"))
}

func cursorRequestFailoverError(message string) *UpstreamFailoverError {
	return cursorRequestScopedError(http.StatusBadRequest, GatewayFailureReason("cursor_invalid_request"), message)
}

func cursorRequestPayloadTooLargeError(message string) *UpstreamFailoverError {
	return cursorRequestScopedError(http.StatusRequestEntityTooLarge, GatewayFailureReason("cursor_request_too_large"), message)
}

func cursorRequestScopedError(status int, reason GatewayFailureReason, message string) *UpstreamFailoverError {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Invalid Cursor request"
	}
	return &UpstreamFailoverError{
		StatusCode:        status,
		ResponseBody:      []byte(message),
		Stage:             GatewayFailureStageInference,
		Scope:             GatewayFailureScopeRequest,
		Reason:            reason,
		NextAccountAction: NextAccountStop,
		ClientStatusCode:  status,
		ClientMessage:     message,
	}
}

func (s *CursorGatewayService) saveCursorResponse(ctx context.Context, c *gin.Context, responseID string, dialogue *cursorpkg.Dialogue) error {
	return s.saveCursorStoredResponse(ctx, c, responseID, &cursorStoredResponse{Dialogue: dialogue})
}

func cloneCursorAgentPendingMCP(pending *cursorAgentPendingMCP) *cursorAgentPendingMCP {
	if pending == nil {
		return nil
	}
	cloned := *pending
	cloned.Action.Arguments = shallowCopyAnyMap(pending.Action.Arguments)
	if pending.CursorAction != nil {
		cursorAction := *pending.CursorAction
		cursorAction.Arguments = shallowCopyAnyMap(pending.CursorAction.Arguments)
		cloned.CursorAction = &cursorAction
	}
	return &cloned
}

func shallowCopyAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneCursorAgentBlobs(blobs map[string][]byte) map[string][]byte {
	if len(blobs) == 0 {
		return nil
	}
	cloned := make(map[string][]byte, len(blobs))
	for key, value := range blobs {
		cloned[key] = append([]byte(nil), value...)
	}
	return cloned
}

func (s *CursorGatewayService) saveCursorStoredResponse(ctx context.Context, c *gin.Context, responseID string, stored *cursorStoredResponse) error {
	if s == nil || s.responseStore == nil {
		return errors.New("redis is not configured")
	}
	if stored == nil {
		return errors.New("cursor response state is unavailable")
	}
	owner, err := cursorResponseOwner(c)
	if err != nil {
		return err
	}
	stored.Owner = owner
	encoded, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	ttl := durationSeconds(s.cursorConfig().ResponsesTTLSeconds, 86400)
	return s.responseStore.Set(ctx, "cursor:responses:"+responseID, encoded, ttl)
}

func (s *CursorGatewayService) loadCursorResponse(ctx context.Context, c *gin.Context, responseID string) (*cursorpkg.Dialogue, error) {
	stored, err := s.loadCursorStoredResponse(ctx, c, responseID)
	if err != nil {
		return nil, err
	}
	if stored.Dialogue == nil {
		return nil, errors.New("previous_response_id has no stored dialogue")
	}
	return stored.Dialogue, nil
}

func (s *CursorGatewayService) loadCursorStoredResponse(ctx context.Context, c *gin.Context, responseID string) (*cursorStoredResponse, error) {
	if s == nil || s.responseStore == nil {
		return nil, errors.New("redis is not configured")
	}
	owner, err := cursorResponseOwner(c)
	if err != nil {
		return nil, err
	}
	encoded, found, err := s.responseStore.Get(ctx, "cursor:responses:"+strings.TrimSpace(responseID))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("previous_response_id %q was not found or expired", responseID)
	}
	var stored cursorStoredResponse
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, err
	}
	if stored.Owner != owner {
		return nil, errors.New("previous_response_id does not belong to this API key")
	}
	return &stored, nil
}

func cursorAccountSetting(account *Account, key string) string {
	if account == nil {
		return ""
	}
	if value := strings.TrimSpace(account.GetExtraString(key)); value != "" {
		return value
	}
	return strings.TrimSpace(account.GetCredential(key))
}

func cursorResponseOwner(c *gin.Context) (string, error) {
	if c == nil {
		return "", errors.New("request context is unavailable")
	}
	value, exists := c.Get("api_key")
	if !exists {
		return "", errors.New("authenticated API key is unavailable")
	}
	apiKey, ok := value.(*APIKey)
	if !ok || apiKey == nil || apiKey.ID <= 0 {
		return "", errors.New("authenticated API key is invalid")
	}
	return fmt.Sprintf("user:%d:key:%d", apiKey.UserID, apiKey.ID), nil
}
