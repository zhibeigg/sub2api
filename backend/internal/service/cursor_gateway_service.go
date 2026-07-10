package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CursorGatewayService adapts Anthropic Messages, OpenAI Chat Completions and
// OpenAI Responses requests to Cursor's documentation-chat SSE endpoint.
type CursorGatewayService struct {
	httpUpstream        HTTPUpstream
	proxyRepo           ProxyRepository
	tlsFPProfileService *TLSFingerprintProfileService
	redisClient         *redis.Client
	cfg                 *config.Config
}

func NewCursorGatewayService(httpUpstream HTTPUpstream, proxyRepo ProxyRepository, tlsFPProfileService *TLSFingerprintProfileService, redisClient *redis.Client, cfg *config.Config) *CursorGatewayService {
	return &CursorGatewayService{httpUpstream: httpUpstream, proxyRepo: proxyRepo, tlsFPProfileService: tlsFPProfileService, redisClient: redisClient, cfg: cfg}
}

type cursorRequestEnvelope struct {
	Model              string `json:"model"`
	Stream             bool   `json:"stream"`
	PreviousResponseID string `json:"previous_response_id"`
	Store              *bool  `json:"store"`
}

type cursorStoredResponse struct {
	Owner    string              `json:"owner"`
	Dialogue *cursorpkg.Dialogue `json:"dialogue"`
}

type cursorCollected struct {
	Text         string
	CleanText    string
	Actions      []cursorpkg.Action
	Usage        cursorpkg.Usage
	FinishReason string
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

func (s *CursorGatewayService) Probe(ctx context.Context, account *Account, model, prompt string) (string, error) {
	if s == nil || s.httpUpstream == nil {
		return "", errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorCookie() {
		return "", errors.New("a Cursor cookie account is required")
	}
	if err := ValidateCursorAccountCredentials(account.Type, account.Credentials); err != nil {
		return "", err
	}
	if strings.TrimSpace(model) == "" {
		model = "cursor-chat"
	}
	upstreamModel, _ := account.ResolveMappedModel(model)
	if override := cursorAccountSetting(account, "cursor_upstream_model"); override != "" {
		upstreamModel = override
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Reply with OK."
	}
	client, err := s.newClient(ctx, account, upstreamModel)
	if err != nil {
		return "", err
	}
	payload, err := client.BuildPayload(&cursorpkg.Dialogue{Messages: []cursorpkg.DialogueMessage{{Role: "user", Text: prompt}}}, cursorpkg.BuildOptions{Model: upstreamModel, ConversationID: uuid.NewString(), MaxHistoryMessages: 4, MaxHistoryTokens: 2048})
	if err != nil {
		return "", err
	}
	collected, _, err := collectCursorResponse(ctx, client, payload, time.Now())
	if err != nil {
		return "", err
	}
	return collected.CleanText, nil
}

func (s *CursorGatewayService) CountTokens(body []byte, protocol cursorpkg.Protocol) (int, error) {
	dialogue, err := cursorpkg.ParseRequest(protocol, body)
	if err != nil {
		return 0, err
	}
	model := "cursor-chat"
	var envelope cursorRequestEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && strings.TrimSpace(envelope.Model) != "" {
		model = envelope.Model
	}
	payload, err := cursorpkg.BuildPayload(dialogue, cursorpkg.BuildOptions{Model: model})
	if err != nil {
		return 0, err
	}
	return estimateCursorPayloadTokens(payload), nil
}

func (s *CursorGatewayService) forward(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol cursorpkg.Protocol) (*ForwardResult, error) {
	start := time.Now()
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorCookie() {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("a Cursor cookie account is required")}
	}
	if err := ValidateCursorAccountCredentials(account.Type, account.Credentials); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, ResponseBody: []byte(err.Error())}
	}

	var envelope cursorRequestEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("invalid request body: " + err.Error())}
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

	dialogue, err := cursorpkg.ParseRequest(protocol, body)
	if err != nil {
		return nil, mapCursorError(err)
	}
	if protocol == cursorpkg.ProtocolResponses && strings.TrimSpace(envelope.PreviousResponseID) != "" {
		previous, loadErr := s.loadCursorResponse(ctx, c, envelope.PreviousResponseID)
		if loadErr != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(loadErr.Error())}
		}
		if strings.TrimSpace(dialogue.System) == "" {
			dialogue.System = previous.System
		}
		dialogue.Messages = append(append([]cursorpkg.DialogueMessage(nil), previous.Messages...), dialogue.Messages...)
	}
	client, err := s.newClient(ctx, account, upstreamModel)
	if err != nil {
		return nil, mapCursorError(err)
	}
	payload, err := client.BuildPayload(dialogue, cursorpkg.BuildOptions{
		Model:              upstreamModel,
		ConversationID:     uuid.NewString(),
		MaxHistoryMessages: s.cursorConfig().MaxHistoryMessages,
		MaxHistoryTokens:   s.cursorConfig().MaxHistoryTokens,
	})
	if err != nil {
		return nil, mapCursorError(err)
	}
	estimatedInput := estimateCursorPayloadTokens(payload)
	collected, firstTokenMs, err := collectCursorResponse(ctx, client, payload, start)
	if err != nil {
		return nil, mapCursorError(err)
	}
	if err := validateCursorToolResult(dialogue, collected.Actions); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
	}
	if collected.Usage.InputTokens <= 0 {
		collected.Usage.InputTokens = estimatedInput
	}
	if collected.Usage.OutputTokens <= 0 {
		collected.Usage.OutputTokens = cursorpkg.EstimateTokens(collected.CleanText) + estimateCursorActionTokens(collected.Actions)
	}
	collected.Usage.TotalTokens = collected.Usage.InputTokens + collected.Usage.OutputTokens

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
	return &ForwardResult{
		RequestID:     responseID,
		Usage:         ClaudeUsage{InputTokens: collected.Usage.InputTokens, OutputTokens: collected.Usage.OutputTokens},
		Model:         requestModel,
		UpstreamModel: differentOrEmpty(requestModel, upstreamModel),
		Stream:        envelope.Stream,
		Duration:      time.Since(start),
		FirstTokenMs:  firstTokenMs,
	}, nil
}

func (s *CursorGatewayService) newClient(ctx context.Context, account *Account, model string) (*cursorpkg.Client, error) {
	cfg := s.cursorConfig()
	endpoint, err := cursorEndpoint(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	} else if account.ProxyID != nil && s.proxyRepo != nil {
		if proxy, proxyErr := s.proxyRepo.GetByID(ctx, *account.ProxyID); proxyErr == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	var profile *tlsfingerprint.Profile
	if s.tlsFPProfileService != nil {
		profile = s.tlsFPProfileService.ResolveTLSProfile(account)
	}
	httpClient := &http.Client{Transport: &cursorRoundTripper{upstream: s.httpUpstream, proxyURL: proxyURL, accountID: account.ID, concurrency: account.Concurrency, profile: profile}}
	referer := cursorAccountSetting(account, "cursor_referer")
	if referer == "" {
		referer = cfg.Referer
	}
	return cursorpkg.NewClient(httpClient, cursorpkg.Credential{Cookie: account.GetCredential("cookie")}, cursorpkg.ClientConfig{
		BaseURL:           endpoint,
		Model:             model,
		Referer:           referer,
		UserAgent:         cfg.UserAgent,
		RequestTimeout:    durationSeconds(cfg.RequestTimeoutSeconds, 120),
		StreamIdleTimeout: durationSeconds(cfg.StreamIdleTimeoutSeconds, 60),
		MaxErrorBody:      8 << 10,
	})
}

func (s *CursorGatewayService) cursorConfig() config.CursorConfig {
	if s != nil && s.cfg != nil {
		return s.cfg.Cursor
	}
	return config.CursorConfig{BaseURL: "https://cursor.com", DefaultModel: "google/gemini-3-flash", Referer: "https://cursor.com/docs", UserAgent: "sub2api-cursor/1", RequestTimeoutSeconds: 120, StreamIdleTimeoutSeconds: 60, MaxHistoryTokens: 24000, MaxHistoryMessages: 100}
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
		raw = "https://cursor.com"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("Cursor base_url must be a valid HTTPS URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "cursor.com" && !strings.HasSuffix(host, ".cursor.com") {
		return "", fmt.Errorf("Cursor base_url host must be cursor.com or its subdomain")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/api/chat") {
		parsed.Path += "/api/chat"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

type cursorRoundTripper struct {
	upstream    HTTPUpstream
	proxyURL    string
	accountID   int64
	concurrency int
	profile     *tlsfingerprint.Profile
}

func (t *cursorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.upstream.DoWithTLS(req, t.proxyURL, t.accountID, t.concurrency, t.profile)
}

func collectCursorResponse(ctx context.Context, client *cursorpkg.Client, payload *cursorpkg.Request, start time.Time) (cursorCollected, *int, error) {
	var out cursorCollected
	var text strings.Builder
	var firstTokenMs *int
	err := client.Stream(ctx, payload, func(event cursorpkg.SSEEvent) error {
		if event.Delta != "" {
			if firstTokenMs == nil {
				ms := int(time.Since(start).Milliseconds())
				firstTokenMs = &ms
			}
			text.WriteString(event.Delta)
		}
		if usage := event.EventUsage(); usage != nil {
			out.Usage = *usage
		}
		if event.FinishReason != "" {
			out.FinishReason = event.FinishReason
		}
		return nil
	})
	if err != nil {
		return out, firstTokenMs, err
	}
	out.Text = text.String()
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
			return fmt.Errorf("Cursor compatibility mode returned unknown tool %q", action.Name)
		}
	}
	if (dialogue.ToolChoice.Mode == "any" || dialogue.ToolChoice.Mode == "required" || dialogue.ToolChoice.Mode == "tool" || dialogue.ToolChoice.Mode == "function") && len(actions) == 0 {
		return errors.New("Cursor compatibility mode did not return the required tool call")
	}
	if (dialogue.ToolChoice.Mode == "tool" || dialogue.ToolChoice.Mode == "function") && dialogue.ToolChoice.Name != "" {
		for _, action := range actions {
			if action.Name != dialogue.ToolChoice.Name {
				return fmt.Errorf("Cursor compatibility mode returned tool %q instead of required tool %q", action.Name, dialogue.ToolChoice.Name)
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
	if protocol == cursorpkg.ProtocolOpenAIChat {
		prefix = "chatcmpl"
	} else if protocol == cursorpkg.ProtocolResponses {
		prefix = "resp"
	}
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func mapCursorError(err error) error {
	var cursorErr *cursorpkg.Error
	if errors.As(err, &cursorErr) {
		status := cursorErr.StatusCode
		if status == 0 {
			if cursorErr.Kind == cursorpkg.ErrorBadRequest {
				status = http.StatusBadRequest
			} else {
				status = http.StatusBadGateway
			}
		}
		return &UpstreamFailoverError{StatusCode: status, ResponseBody: []byte(cursorErr.Error())}
	}
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
}

func (s *CursorGatewayService) saveCursorResponse(ctx context.Context, c *gin.Context, responseID string, dialogue *cursorpkg.Dialogue) error {
	if s == nil || s.redisClient == nil {
		return errors.New("Redis is not configured")
	}
	owner, err := cursorResponseOwner(c)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(cursorStoredResponse{Owner: owner, Dialogue: dialogue})
	if err != nil {
		return err
	}
	ttl := durationSeconds(s.cursorConfig().ResponsesTTLSeconds, 86400)
	return s.redisClient.Set(ctx, "cursor:responses:"+responseID, encoded, ttl).Err()
}

func (s *CursorGatewayService) loadCursorResponse(ctx context.Context, c *gin.Context, responseID string) (*cursorpkg.Dialogue, error) {
	if s == nil || s.redisClient == nil {
		return nil, errors.New("Redis is not configured")
	}
	owner, err := cursorResponseOwner(c)
	if err != nil {
		return nil, err
	}
	encoded, err := s.redisClient.Get(ctx, "cursor:responses:"+strings.TrimSpace(responseID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("previous_response_id %q was not found or expired", responseID)
	}
	if err != nil {
		return nil, err
	}
	var stored cursorStoredResponse
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, err
	}
	if stored.Owner != owner {
		return nil, errors.New("previous_response_id does not belong to this API key")
	}
	if stored.Dialogue == nil {
		return nil, errors.New("previous_response_id has no stored dialogue")
	}
	return stored.Dialogue, nil
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

func sortedCursorActions(actions []cursorpkg.Action) []cursorpkg.Action {
	out := append([]cursorpkg.Action(nil), actions...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
