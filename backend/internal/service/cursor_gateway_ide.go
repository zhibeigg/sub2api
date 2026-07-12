package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type cursorIDEModel struct {
	Name             string
	ServerName       string
	DefaultOn        bool
	SupportsThinking bool
	LegacySlugs      []string
}

type cursorIDEModelSelection struct {
	ServerName string
	Thinking   bool
	Effort     string
}

type cursorIDEModelCatalogCache struct {
	TokenHash  [32]byte
	FreshUntil time.Time
	StaleUntil time.Time
	Models     []cursorIDEModel
}

type cursorIDEModelCatalogState uint8

const (
	cursorIDEModelCatalogMiss cursorIDEModelCatalogState = iota
	cursorIDEModelCatalogFresh
	cursorIDEModelCatalogStale
)

type cursorIDEModelCatalogLoader func(context.Context) ([]cursorIDEModel, error)

func (s *CursorGatewayService) cursorIDEModelCatalogTTLs() (time.Duration, time.Duration) {
	cfg := s.cursorConfig()
	freshSeconds := cfg.AgentModelCacheTTLSeconds
	if freshSeconds <= 0 {
		freshSeconds = 300
	}
	staleSeconds := cfg.AgentModelStaleTTLSeconds
	if staleSeconds < freshSeconds {
		staleSeconds = 1800
		if staleSeconds < freshSeconds {
			staleSeconds = freshSeconds
		}
	}
	return time.Duration(freshSeconds) * time.Second, time.Duration(staleSeconds) * time.Second
}

func cursorIDEModelCatalogKey(account *Account) string {
	if account == nil {
		return ""
	}
	tokenHash := sha256.Sum256([]byte(strings.TrimSpace(account.GetCredential("dashboard_access_token"))))
	return fmt.Sprintf("%d:%x", account.ID, tokenHash)
}

func (s *CursorGatewayService) cachedIDEModelCatalogState(account *Account) ([]cursorIDEModel, cursorIDEModelCatalogState) {
	if s == nil || account == nil || account.ID <= 0 {
		return nil, cursorIDEModelCatalogMiss
	}
	tokenHash := sha256.Sum256([]byte(strings.TrimSpace(account.GetCredential("dashboard_access_token"))))
	s.ideModelMu.RLock()
	cached, ok := s.ideModelCache[account.ID]
	s.ideModelMu.RUnlock()
	if !ok || cached.TokenHash != tokenHash {
		return nil, cursorIDEModelCatalogMiss
	}
	now := time.Now()
	if now.Before(cached.FreshUntil) {
		return append([]cursorIDEModel(nil), cached.Models...), cursorIDEModelCatalogFresh
	}
	if now.Before(cached.StaleUntil) {
		return append([]cursorIDEModel(nil), cached.Models...), cursorIDEModelCatalogStale
	}
	return nil, cursorIDEModelCatalogMiss
}

func (s *CursorGatewayService) cachedIDEModelCatalog(account *Account) ([]cursorIDEModel, bool) {
	models, state := s.cachedIDEModelCatalogState(account)
	return models, state != cursorIDEModelCatalogMiss
}

func (s *CursorGatewayService) invalidateIDEModelCatalog(account *Account) {
	if s == nil || account == nil || account.ID <= 0 {
		return
	}
	s.ideModelMu.Lock()
	delete(s.ideModelCache, account.ID)
	s.ideModelMu.Unlock()
}

func (s *CursorGatewayService) storeIDEModelCatalog(account *Account, models []cursorIDEModel) {
	if s == nil || account == nil || account.ID <= 0 || len(models) == 0 {
		return
	}
	freshTTL, staleTTL := s.cursorIDEModelCatalogTTLs()
	now := time.Now()
	entry := cursorIDEModelCatalogCache{
		TokenHash:  sha256.Sum256([]byte(strings.TrimSpace(account.GetCredential("dashboard_access_token")))),
		FreshUntil: now.Add(freshTTL),
		StaleUntil: now.Add(staleTTL),
		Models:     append([]cursorIDEModel(nil), models...),
	}
	s.ideModelMu.Lock()
	if s.ideModelCache == nil {
		s.ideModelCache = make(map[int64]cursorIDEModelCatalogCache)
	}
	s.ideModelCache[account.ID] = entry
	s.ideModelMu.Unlock()
}

func (s *CursorGatewayService) refreshIDEModelCatalog(ctx context.Context, account *Account, loader cursorIDEModelCatalogLoader) ([]cursorIDEModel, error) {
	if s == nil || account == nil || loader == nil {
		return nil, errors.New("Cursor Agent model catalog loader is unavailable")
	}
	key := cursorIDEModelCatalogKey(account)
	value, err, _ := s.ideModelRefresh.Do(key, func() (any, error) {
		models, loadErr := loader(ctx)
		if loadErr != nil {
			return nil, loadErr
		}
		if len(models) == 0 {
			return nil, errors.New("Cursor Agent returned no supported models")
		}
		s.storeIDEModelCatalog(account, models)
		return append([]cursorIDEModel(nil), models...), nil
	})
	if err != nil {
		return nil, err
	}
	models, ok := value.([]cursorIDEModel)
	if !ok || len(models) == 0 {
		return nil, errors.New("Cursor Agent returned an invalid model catalog")
	}
	return append([]cursorIDEModel(nil), models...), nil
}

func (s *CursorGatewayService) refreshIDEModelCatalogAsync(account *Account, loader cursorIDEModelCatalogLoader) {
	if s == nil || account == nil || loader == nil {
		return
	}
	cfg := s.cursorConfig()
	timeoutSeconds := cfg.AgentModelProbeTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
		if _, err := s.refreshIDEModelCatalog(ctx, account, loader); err != nil {
			slog.Debug("cursor Agent model catalog background refresh failed", "account_id", account.ID, "error", err)
		}
	}()
}

func (s *CursorGatewayService) PrewarmIDEModelCatalog(account *Account) {
	if s == nil || account == nil || account.ID <= 0 || !s.cursorConfig().AgentModelPrewarmEnabled {
		return
	}
	if _, state := s.cachedIDEModelCatalogState(account); state == cursorIDEModelCatalogFresh {
		return
	}
	s.refreshIDEModelCatalogAsync(account, func(ctx context.Context) ([]cursorIDEModel, error) {
		return s.fetchIDEModelCatalogUncached(ctx, account)
	})
}

func (s *CursorGatewayService) FetchIDEModels(ctx context.Context, account *Account) ([]string, error) {
	catalog, err := s.fetchIDEModelCatalog(ctx, account)
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(catalog))
	seen := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.ServerName)
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		models = append(models, name)
	}
	if len(models) == 0 {
		return nil, errors.New("Cursor IDE returned no supported models")
	}
	sort.Strings(models)
	return models, nil
}

func (s *CursorGatewayService) FetchIDELogicalModels(ctx context.Context, account *Account) ([]string, error) {
	catalog, err := s.forceFetchIDEModelCatalog(ctx, account)
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(catalog))
	seen := make(map[string]struct{}, len(catalog))
	appendModel := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || strings.EqualFold(model, "default") {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for _, item := range catalog {
		for _, slug := range item.LegacySlugs {
			appendModel(slug)
		}
	}
	if len(models) == 0 {
		for _, item := range catalog {
			logical, _ := normalizeCursorCloudModel(item.Name, cursorVariantPreference{})
			appendModel(logical)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("Cursor IDE returned no logical models")
	}
	sort.Strings(models)
	return models, nil
}

func (s *CursorGatewayService) forceFetchIDEModelCatalog(ctx context.Context, account *Account) ([]cursorIDEModel, error) {
	s.invalidateIDEModelCatalog(account)
	return s.fetchIDEModelCatalog(ctx, account)
}

func (s *CursorGatewayService) fetchIDEModelCatalog(ctx context.Context, account *Account) ([]cursorIDEModel, error) {
	activeAccount := account
	if s.dashboardAuth != nil && activeAccount != nil && activeAccount.ID > 0 {
		refreshed, _, err := s.dashboardAuth.RefreshIfNeeded(ctx, activeAccount)
		if err != nil {
			return nil, err
		}
		if refreshed != nil {
			activeAccount = refreshed
		}
	}

	cached, state := s.cachedIDEModelCatalogState(activeAccount)
	loader := func(loadCtx context.Context) ([]cursorIDEModel, error) {
		return s.fetchIDEModelCatalogUncached(loadCtx, activeAccount)
	}
	switch state {
	case cursorIDEModelCatalogFresh:
		return cached, nil
	case cursorIDEModelCatalogStale:
		s.refreshIDEModelCatalogAsync(activeAccount, loader)
		return cached, nil
	default:
		return s.refreshIDEModelCatalog(ctx, activeAccount, loader)
	}
}

func (s *CursorGatewayService) fetchIDEModelCatalogUncached(ctx context.Context, activeAccount *Account) ([]cursorIDEModel, error) {
	client, credential, err := s.newCursorAgentClient(ctx, activeAccount)
	if err != nil {
		return nil, err
	}
	models, err := client.GetUsableModels(ctx, credential, nil)
	if err != nil && cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) && s.dashboardAuth != nil && activeAccount.ID > 0 {
		activeAccount, err = s.dashboardAuth.forceRefresh(ctx, activeAccount)
		if err == nil {
			client, credential, err = s.newCursorAgentClient(ctx, activeAccount)
		}
		if err == nil {
			models, err = client.GetUsableModels(ctx, credential, nil)
		}
	}
	if err != nil {
		return nil, err
	}
	catalog := make([]cursorIDEModel, 0, len(models))
	for _, model := range models {
		serverName := strings.TrimSpace(model.ID)
		name := strings.TrimSpace(model.DisplayID)
		if name == "" {
			name = serverName
		}
		if serverName == "" {
			serverName = name
		}
		if serverName == "" {
			continue
		}
		aliases := append([]string(nil), model.Aliases...)
		logical, _ := normalizeCursorCloudModel(serverName, cursorVariantPreference{})
		if logical != "" && !strings.EqualFold(logical, serverName) && !containsCursorIDEAlias(aliases, logical) {
			aliases = append(aliases, logical)
		}
		catalog = append(catalog, cursorIDEModel{
			Name: name, ServerName: serverName,
			SupportsThinking: model.SupportsThinking || cursorIDEVariantNameHasThinking(name) || cursorIDEVariantNameHasThinking(serverName),
			LegacySlugs:      aliases,
		})
	}
	if len(catalog) == 0 {
		return nil, cursorpkg.HTTPError(http.StatusBadGateway, "Agent models request", "Cursor Agent returned no supported models")
	}
	return catalog, nil
}

func (s *CursorGatewayService) resolveCursorIDEModel(ctx context.Context, account *Account, requested string, preference cursorVariantPreference) (cursorIDEModelSelection, error) {
	requested = strings.TrimSpace(requested)
	fallback := cursorIDEModelSelection{ServerName: requested}
	catalog, state := s.cachedIDEModelCatalogState(account)
	if state == cursorIDEModelCatalogMiss {
		// Model discovery is deliberately kept off the chat hot path. Agent Run can
		// accept the requested model directly while startup/authorization prewarm
		// fills the catalog for later variant resolution.
		return fallback, nil
	}
	if state == cursorIDEModelCatalogStale {
		s.refreshIDEModelCatalogAsync(account, func(loadCtx context.Context) ([]cursorIDEModel, error) {
			return s.fetchIDEModelCatalogUncached(loadCtx, account)
		})
	}

	// Concrete IDE variant names remain valid for backwards compatibility.
	for _, item := range catalog {
		if strings.EqualFold(requested, item.Name) || strings.EqualFold(requested, item.ServerName) {
			return cursorIDESelection(item), nil
		}
	}

	candidates := make([]cursorIDEModel, 0)
	families := make(map[string]struct{})
	for _, item := range catalog {
		matched := containsCursorIDEAlias(item.LegacySlugs, requested)
		if !matched {
			logicalName, _ := normalizeCursorCloudModel(item.Name, cursorVariantPreference{})
			logicalServerName, _ := normalizeCursorCloudModel(item.ServerName, cursorVariantPreference{})
			matched = strings.EqualFold(requested, logicalName) || strings.EqualFold(requested, logicalServerName)
		}
		if matched {
			candidates = append(candidates, item)
			families[cursorIDEVariantFamily(item.Name)] = struct{}{}
		}
	}
	if len(families) > 0 {
		seen := make(map[string]struct{}, len(candidates))
		for _, item := range candidates {
			seen[item.ServerName] = struct{}{}
		}
		for _, item := range catalog {
			if _, ok := families[cursorIDEVariantFamily(item.Name)]; !ok {
				continue
			}
			if _, ok := seen[item.ServerName]; ok {
				continue
			}
			seen[item.ServerName] = struct{}{}
			candidates = append(candidates, item)
		}
	}
	if len(candidates) == 0 {
		return fallback, nil
	}

	filtered := candidates
	if preference.Thinking != nil {
		matching := make([]cursorIDEModel, 0, len(filtered))
		for _, item := range filtered {
			if cursorIDEModelSupportsThinking(item) == *preference.Thinking {
				matching = append(matching, item)
			}
		}
		if len(matching) > 0 {
			filtered = matching
		}
	}
	if preference.Effort != "" {
		matching := make([]cursorIDEModel, 0, len(filtered))
		for _, item := range filtered {
			if cursorIDEVariantEffort(item.Name) == preference.Effort {
				matching = append(matching, item)
			}
		}
		if len(matching) > 0 {
			filtered = matching
		}
	}
	for _, item := range filtered {
		if item.DefaultOn {
			return cursorIDESelection(item), nil
		}
	}
	if preference.Thinking == nil {
		nonThinking := make([]cursorIDEModel, 0, len(filtered))
		for _, item := range filtered {
			if !cursorIDEModelSupportsThinking(item) {
				nonThinking = append(nonThinking, item)
			}
		}
		if len(nonThinking) > 0 {
			filtered = nonThinking
		}
	}
	if preference.Effort == "" {
		defaultEffort := ""
		for _, item := range candidates {
			if item.DefaultOn {
				defaultEffort = cursorIDEVariantEffort(item.Name)
				break
			}
		}
		if defaultEffort == "" {
			defaultEffort = "medium"
		}
		for _, item := range filtered {
			if cursorIDEVariantEffort(item.Name) == defaultEffort {
				return cursorIDESelection(item), nil
			}
		}
	}
	return cursorIDESelection(filtered[0]), nil
}

func containsCursorIDEAlias(aliases []string, requested string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(requested)) {
			return true
		}
	}
	return false
}

func cursorIDEVariantNameHasThinking(name string) bool {
	for _, part := range strings.Split(strings.ToLower(strings.TrimSpace(name)), "-") {
		if part == "thinking" {
			return true
		}
	}
	return false
}

func cursorIDEModelSupportsThinking(item cursorIDEModel) bool {
	return item.SupportsThinking || cursorIDEVariantNameHasThinking(item.Name) || cursorIDEVariantNameHasThinking(item.ServerName)
}

func cursorIDESelection(item cursorIDEModel) cursorIDEModelSelection {
	return cursorIDEModelSelection{
		ServerName: item.ServerName,
		Thinking:   cursorIDEModelSupportsThinking(item),
		Effort:     cursorIDEVariantEffort(item.Name),
	}
}

func cursorIDEVariantFamily(name string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(name)), "-")
	effortIndex, _ := cursorIDEVariantEffortParts(parts)
	family := make([]string, 0, len(parts))
	for index, part := range parts {
		if index == effortIndex || part == "thinking" || part == "fast" {
			continue
		}
		family = append(family, part)
	}
	return strings.Join(family, "-")
}

func cursorIDEVariantEffort(name string) string {
	_, effort := cursorIDEVariantEffortParts(strings.Split(strings.ToLower(strings.TrimSpace(name)), "-"))
	return effort
}

func cursorIDEVariantEffortParts(parts []string) (int, string) {
	for index := len(parts) - 1; index >= 0; index-- {
		part := parts[index]
		if part == "thinking" || part == "fast" {
			continue
		}
		switch part {
		case "low", "medium", "high", "xhigh":
			return index, part
		case "max":
			// gpt-5.1-codex-max is a logical model ID; a trailing effort on
			// one of its variants appears after this token (for example ...-max-high).
			if index > 0 && parts[index-1] == "codex" {
				return -1, ""
			}
			return index, part
		default:
			return -1, ""
		}
	}
	return -1, ""
}

func decodeCursorIDEModelsResponse(contentType string, body []byte, cfg config.CursorConfig) ([]cursorIDEModel, error) {
	trimmed := bytes.TrimSpace(body)
	if strings.Contains(strings.ToLower(contentType), "json") || (len(trimmed) > 0 && trimmed[0] == '{') {
		var payload struct {
			Models []struct {
				Name             string   `json:"name"`
				ServerModelName  string   `json:"serverModelName"`
				DefaultOn        bool     `json:"defaultOn"`
				SupportsThinking bool     `json:"supportsThinking"`
				LegacySlugs      []string `json:"legacySlugs"`
			} `json:"models"`
		}
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return nil, cursorpkg.HTTPError(http.StatusBadGateway, "IDE models request", "invalid JSON model catalog response: "+err.Error())
		}
		models := make([]cursorIDEModel, 0, len(payload.Models))
		for _, item := range payload.Models {
			name := strings.TrimSpace(item.Name)
			serverName := strings.TrimSpace(item.ServerModelName)
			if name == "" {
				name = serverName
			}
			if serverName == "" {
				serverName = name
			}
			if serverName != "" {
				models = append(models, cursorIDEModel{
					Name: name, ServerName: serverName,
					DefaultOn: item.DefaultOn, SupportsThinking: item.SupportsThinking,
					LegacySlugs: append([]string(nil), item.LegacySlugs...),
				})
			}
		}
		return models, nil
	}
	models, err := cursorpkg.DecodeIDEAvailableModels(body, cfg.MaxFrameBytes, cfg.MaxBufferedBytes)
	if err != nil {
		return nil, err
	}
	catalog := make([]cursorIDEModel, 0, len(models))
	for _, name := range models {
		name = strings.TrimSpace(name)
		if name != "" {
			catalog = append(catalog, cursorIDEModel{Name: name, ServerName: name})
		}
	}
	return catalog, nil
}

func (s *CursorGatewayService) probeCursorIDE(ctx context.Context, account *Account) (string, error) {
	catalog, err := s.forceFetchIDEModelCatalog(ctx, account)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Cursor Agent RPC connected (%d models)", len(catalog)), nil
}

type cursorIDEEventSource interface {
	Next() (cursorpkg.IDEEvent, error)
	Close() error
}

type cursorAgentEventAdapter struct {
	stream     *cursorpkg.AgentStream
	checkpoint *cursorpkg.AgentConversationState
	blobs      map[string][]byte
	pendingMCP *cursorAgentPendingMCP
	emitted    map[string]struct{}
	sawTool    bool
}

func newCursorAgentEventAdapter(stream *cursorpkg.AgentStream, blobs map[string][]byte) *cursorAgentEventAdapter {
	return &cursorAgentEventAdapter{
		stream: stream, blobs: cloneCursorAgentBlobs(blobs), emitted: make(map[string]struct{}),
	}
}

func (s *cursorAgentEventAdapter) Next() (cursorpkg.IDEEvent, error) {
	if s == nil || s.stream == nil {
		return cursorpkg.IDEEvent{}, io.EOF
	}
	for {
		event, err := s.stream.Next()
		if err != nil {
			return cursorpkg.IDEEvent{}, err
		}
		switch event.Type {
		case cursorpkg.AgentEventText:
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventText, Text: event.Text}, nil
		case cursorpkg.AgentEventThinking:
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventThinking, Thinking: event.Thinking}, nil
		case cursorpkg.AgentEventUsage:
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventUsage, Usage: event.Usage}, nil
		case cursorpkg.AgentEventToolCompleted:
			if mapped, ok := s.mapTool(event.Tool); ok {
				return mapped, nil
			}
		case cursorpkg.AgentEventExecMCP:
			if event.ExecMCP != nil {
				action := *event.ExecMCP
				action.Arguments = shallowCopyAnyMap(event.ExecMCP.Arguments)
				s.pendingMCP = &cursorAgentPendingMCP{RequestID: event.ExecRequestID, ExecID: event.ExecID, Action: action}
			}
			if mapped, ok := s.mapTool(event.ExecMCP); ok {
				return mapped, nil
			}
		case cursorpkg.AgentEventCheckpoint:
			if event.Checkpoint != nil {
				s.checkpoint = event.Checkpoint
			}
		case cursorpkg.AgentEventKVSet:
			if event.KV == nil {
				continue
			}
			if s.blobs == nil {
				s.blobs = make(map[string][]byte)
			}
			key := base64.RawURLEncoding.EncodeToString(event.KV.BlobID)
			s.blobs[key] = append([]byte(nil), event.KV.BlobData...)
			if err := s.stream.SendKVSetResult(event.KV.ID); err != nil {
				return cursorpkg.IDEEvent{}, err
			}
		case cursorpkg.AgentEventKVGet:
			if event.KV == nil {
				continue
			}
			key := base64.RawURLEncoding.EncodeToString(event.KV.BlobID)
			blob := append([]byte(nil), s.blobs[key]...)
			if err := s.stream.SendKVGetResult(event.KV.ID, blob); err != nil {
				return cursorpkg.IDEEvent{}, err
			}
		case cursorpkg.AgentEventTurnEnded:
			reason := strings.TrimSpace(event.FinishReason)
			if s.sawTool {
				reason = "tool_calls"
			} else if reason == "" {
				reason = "stop"
			}
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventFinish, FinishReason: reason}, nil
		case cursorpkg.AgentEventFinish:
			reason := strings.TrimSpace(event.FinishReason)
			if s.sawTool {
				reason = "tool_calls"
			} else if reason == "" {
				reason = "stop"
			}
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventFinish, FinishReason: reason}, nil
		case cursorpkg.AgentEventUnsupportedExec:
			field := 0
			if event.Unsupported != nil {
				field = event.Unsupported.Field
			}
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventError, Error: &cursorpkg.IDEStreamError{
				Code: "unsupported_local_tool", Message: fmt.Sprintf("Cursor Agent requested unsupported local execution tool field %d", field),
			}}, nil
		case cursorpkg.AgentEventError:
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventError, Error: event.Error}, nil
		}
	}
}

func (s *cursorAgentEventAdapter) mapTool(action *cursorpkg.Action) (cursorpkg.IDEEvent, bool) {
	if action == nil || strings.TrimSpace(action.Name) == "" {
		return cursorpkg.IDEEvent{}, false
	}
	key := strings.TrimSpace(action.ID)
	if key == "" {
		encoded, _ := json.Marshal(action.Arguments)
		key = action.Name + ":" + string(encoded)
	}
	if _, ok := s.emitted[key]; ok {
		return cursorpkg.IDEEvent{}, false
	}
	s.emitted[key] = struct{}{}
	s.sawTool = true
	copyAction := *action
	return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventToolCall, ToolCall: &copyAction}, true
}

func (s *cursorAgentEventAdapter) Close() error {
	if s == nil || s.stream == nil {
		return nil
	}
	return s.stream.Close()
}

func (s *cursorAgentEventAdapter) Checkpoint() *cursorpkg.AgentConversationState {
	if s == nil {
		return nil
	}
	return s.checkpoint
}

func (s *cursorAgentEventAdapter) Blobs() map[string][]byte {
	if s == nil {
		return nil
	}
	return cloneCursorAgentBlobs(s.blobs)
}

func (s *cursorAgentEventAdapter) PendingMCP() *cursorAgentPendingMCP {
	if s == nil {
		return nil
	}
	return cloneCursorAgentPendingMCP(s.pendingMCP)
}

func (s *cursorAgentEventAdapter) SendMCPResult(pending *cursorAgentPendingMCP, result cursorpkg.DialogueMessage) error {
	if s == nil || s.stream == nil || pending == nil {
		return errors.New("Cursor Agent MCP resume state is unavailable")
	}
	return s.stream.SendMCPResult(pending.RequestID, pending.ExecID, result.Text, result.IsError)
}

func cursorAgentToolResult(dialogue *cursorpkg.Dialogue, pending *cursorAgentPendingMCP) (cursorpkg.DialogueMessage, bool) {
	if dialogue == nil || pending == nil || strings.TrimSpace(pending.Action.ID) == "" {
		return cursorpkg.DialogueMessage{}, false
	}
	for index := len(dialogue.Messages) - 1; index >= 0; index-- {
		message := dialogue.Messages[index]
		if message.Role == "tool" && strings.TrimSpace(message.ToolCallID) == strings.TrimSpace(pending.Action.ID) {
			return message, true
		}
	}
	return cursorpkg.DialogueMessage{}, false
}

func (s *CursorGatewayService) forwardIDE(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol cursorpkg.Protocol) (*ForwardResult, error) {
	start := time.Now()
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorAPIKey() {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("a Cursor account is required")}
	}
	if strings.TrimSpace(cursorAccountSetting(account, "cursor_machine_id")) == "" {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, ResponseBody: []byte("Cursor IDE session is missing its login-bound machine ID; reconnect the Dashboard session")}
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
	conversationID := uuid.NewString()
	var agentState *cursorpkg.AgentConversationState
	var agentBlobs map[string][]byte
	var pendingMCP *cursorAgentPendingMCP
	if protocol == cursorpkg.ProtocolResponses && strings.TrimSpace(envelope.PreviousResponseID) != "" {
		previous, loadErr := s.loadCursorStoredResponse(ctx, c, envelope.PreviousResponseID)
		if loadErr != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(loadErr.Error())}
		}
		if previous.Dialogue == nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("previous_response_id has no stored dialogue")}
		}
		if strings.TrimSpace(dialogue.System) == "" {
			dialogue.System = previous.Dialogue.System
		}
		if len(dialogue.Tools) == 0 && len(previous.Dialogue.Tools) > 0 {
			dialogue.Tools = append([]cursorpkg.ToolDefinition(nil), previous.Dialogue.Tools...)
		}
		dialogue.Messages = append(append([]cursorpkg.DialogueMessage(nil), previous.Dialogue.Messages...), dialogue.Messages...)
		if strings.TrimSpace(previous.AgentConversationID) != "" {
			conversationID = strings.TrimSpace(previous.AgentConversationID)
		}
		agentState = previous.AgentState
		agentBlobs = cloneCursorAgentBlobs(previous.AgentBlobs)
		pendingMCP = cloneCursorAgentPendingMCP(previous.AgentPendingMCP)
	} else {
		for index := len(dialogue.Messages) - 1; index >= 0; index-- {
			message := dialogue.Messages[index]
			if message.Role != "tool" || strings.TrimSpace(message.ToolCallID) == "" {
				continue
			}
			if previous, loadErr := s.loadCursorStoredResponse(ctx, c, "tool:"+message.ToolCallID); loadErr == nil {
				if strings.TrimSpace(previous.AgentConversationID) != "" {
					conversationID = strings.TrimSpace(previous.AgentConversationID)
				}
				agentState = previous.AgentState
				agentBlobs = cloneCursorAgentBlobs(previous.AgentBlobs)
				pendingMCP = cloneCursorAgentPendingMCP(previous.AgentPendingMCP)
			}
			break
		}
	}
	trimCursorDialogue(dialogue, s.cursorConfig().MaxHistoryMessages, s.cursorConfig().MaxHistoryTokens)
	mode := prepareCursorAgentMode(dialogue)
	estimatedInput := estimateCursorDialogueTokens(dialogue)
	variantPreference := envelope.variantPreference()
	selection := cursorIDEModelSelection{ServerName: upstreamModel}
	if variantPreference.Thinking != nil {
		selection.Thinking = *variantPreference.Thinking
	}
	resolvedSelection, resolveErr := s.resolveCursorIDEModel(ctx, account, upstreamModel, variantPreference)
	if resolveErr != nil {
		slog.Warn("cursor_ide_model_resolution_failed", "account_id", account.ID, "model", upstreamModel, "error", resolveErr.Error())
	} else if resolvedSelection.ServerName != "" {
		selection = resolvedSelection
		upstreamModel = resolvedSelection.ServerName
	}

	toolResult, resumeAttempt := cursorAgentToolResult(dialogue, pendingMCP)
	resumeAttempt = resumeAttempt && strings.TrimSpace(conversationID) != ""
	runOptions := cursorpkg.AgentRunOptions{
		Model: upstreamModel, DisplayModel: requestModel, ConversationID: conversationID, Mode: mode,
		ConversationState: agentState, Resume: resumeAttempt, MCPProviderIdentifier: "sub2api",
		RequestContext: cursorpkg.AgentRequestContext{
			OSVersion: runtime.GOOS, TimeZone: time.Now().Location().String(), MCPInfoComplete: true, EnvInfoComplete: true,
		},
	}
	activeAccount, resp, stream, err := s.openCursorAgentStream(ctx, account, dialogue, runOptions, agentBlobs)
	if err == nil && resumeAttempt {
		err = stream.SendMCPResult(pendingMCP, toolResult)
	}
	if err != nil && resumeAttempt {
		if stream != nil {
			_ = stream.Close()
		}
		slog.Warn("cursor_agent_resume_failed_rebuilding_history", "account_id", account.ID, "model", upstreamModel, "error", err.Error())
		conversationID = uuid.NewString()
		runOptions.ConversationID = conversationID
		runOptions.ConversationState = nil
		runOptions.Resume = false
		activeAccount, resp, stream, err = s.openCursorAgentStream(ctx, account, dialogue, runOptions, nil)
	}
	if err != nil {
		slog.Warn("cursor_agent_open_stream_failed", "account_id", account.ID, "model", upstreamModel, "error", err.Error())
		return nil, mapCursorError(err)
	}
	if resp == nil || resp.ProtoMajor != 2 {
		if stream != nil {
			_ = stream.Close()
		}
		proto := "unknown"
		if resp != nil {
			proto = resp.Proto
		}
		return nil, mapCursorError(cursorpkg.HTTPError(http.StatusBadGateway, "Agent run request", "Cursor Agent RPC requires HTTP/2; negotiated "+proto))
	}
	defer func() { _ = stream.Close() }()

	responseID := cursorResponseID(protocol)
	writer := newCursorIDEStreamWriter(c, protocol, responseID, requestModel)
	collected := cursorCollected{FinishReason: "stop"}
	var firstTokenMs *int
	committed := false
	finished := false

	for !finished {
		event, nextErr := nextCursorIDEEvent(stream, durationSeconds(s.cursorConfig().IDEStreamIdleTimeoutSeconds, 60))
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			if committed {
				_ = writer.WriteError(nextErr.Error())
			}
			slog.Warn("cursor_ide_stream_read_failed", "account_id", account.ID, "model", upstreamModel, "committed", committed, "error", nextErr.Error())
			return nil, mapCursorError(nextErr)
		}
		switch event.Type {
		case cursorpkg.IDEEventText:
			if event.Text == "" {
				continue
			}
			markCursorFirstToken(&firstTokenMs, start)
			collected.Text += event.Text
			collected.CleanText += event.Text
			if envelope.Stream {
				if err := writer.WriteText(event.Text); err != nil {
					return nil, err
				}
				committed = true
			}
		case cursorpkg.IDEEventThinking:
			if event.Thinking == "" {
				continue
			}
			markCursorFirstToken(&firstTokenMs, start)
			collected.Reasoning += event.Thinking
			if envelope.Stream {
				if err := writer.WriteThinking(event.Thinking); err != nil {
					return nil, err
				}
				committed = true
			}
		case cursorpkg.IDEEventToolCall:
			if event.ToolCall == nil {
				continue
			}
			action := *event.ToolCall
			if strings.TrimSpace(action.ID) == "" {
				action.ID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
			}
			markCursorFirstToken(&firstTokenMs, start)
			collected.Actions = append(collected.Actions, action)
			collected.FinishReason = "tool_calls"
			if envelope.Stream {
				if err := writer.WriteToolCall(action); err != nil {
					return nil, err
				}
				committed = true
			}
		case cursorpkg.IDEEventUsage:
			if event.Usage != nil {
				collected.Usage = *event.Usage
			}
		case cursorpkg.IDEEventFinish:
			if event.FinishReason != "" {
				collected.FinishReason = event.FinishReason
			}
			finished = true
		case cursorpkg.IDEEventError:
			message := "Cursor IDE chat stream failed"
			if event.Error != nil {
				message = strings.TrimSpace(event.Error.Message)
				if event.Error.Code != "" {
					message = event.Error.Code + ": " + message
				}
			}
			if committed {
				_ = writer.WriteError(message)
			}
			slog.Warn("cursor_ide_stream_event_failed", "account_id", account.ID, "model", upstreamModel, "committed", committed, "error", message)
			return nil, mapCursorError(cursorpkg.HTTPError(http.StatusBadGateway, "IDE chat stream", message))
		}
	}

	if err := validateCursorToolResult(dialogue, collected.Actions); err != nil {
		if committed {
			_ = writer.WriteError(err.Error())
		}
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
	}
	if collected.Usage.InputTokens <= 0 {
		collected.Usage.InputTokens = estimatedInput
	}
	if collected.Usage.OutputTokens <= 0 {
		collected.Usage.OutputTokens = cursorpkg.EstimateTokens(collected.CleanText) + cursorpkg.EstimateTokens(collected.Reasoning) + estimateCursorActionTokens(collected.Actions)
	}
	collected.Usage.TotalTokens = collected.Usage.InputTokens + collected.Usage.OutputTokens + collected.Usage.CacheWriteTokens + collected.Usage.CacheReadTokens

	storedDialogue := &cursorpkg.Dialogue{System: dialogue.System, Tools: dialogue.Tools, ToolChoice: dialogue.ToolChoice, Messages: append([]cursorpkg.DialogueMessage(nil), dialogue.Messages...)}
	storedDialogue.Messages = append(storedDialogue.Messages, cursorpkg.DialogueMessage{Role: "assistant", Text: collected.CleanText, ToolCalls: collected.Actions})
	pending := stream.PendingMCP()
	if protocol == cursorpkg.ProtocolResponses && (envelope.Store == nil || *envelope.Store) {
		if saveErr := s.saveCursorAgentResponse(ctx, c, responseID, storedDialogue, conversationID, stream.Checkpoint(), stream.Blobs(), pending); saveErr != nil {
			if committed {
				_ = writer.WriteError("failed to store Cursor response continuation")
			}
			return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte("failed to store Cursor response continuation: " + saveErr.Error())}
		}
	} else if pending != nil && strings.TrimSpace(pending.Action.ID) != "" {
		if saveErr := s.saveCursorAgentResponse(ctx, c, "tool:"+pending.Action.ID, storedDialogue, conversationID, stream.Checkpoint(), stream.Blobs(), pending); saveErr != nil {
			slog.Warn("cursor_agent_tool_continuation_store_failed", "account_id", account.ID, "error", saveErr.Error())
		}
	}
	if envelope.Stream {
		if err := writer.Finish(collected); err != nil {
			return nil, err
		}
	} else {
		writeCursorJSON(c, protocol, responseID, requestModel, envelope.PreviousResponseID, collected)
	}

	_ = activeAccount
	result := &ForwardResult{
		RequestID: responseID,
		Usage: ClaudeUsage{
			InputTokens: collected.Usage.InputTokens, OutputTokens: collected.Usage.OutputTokens,
			CacheCreationInputTokens: collected.Usage.CacheWriteTokens, CacheReadInputTokens: collected.Usage.CacheReadTokens,
		},
		Model: requestModel, UpstreamModel: differentOrEmpty(requestModel, upstreamModel), Stream: envelope.Stream,
		Duration: time.Since(start), FirstTokenMs: firstTokenMs,
	}
	if selection.Effort != "" {
		result.ReasoningEffort = &selection.Effort
	} else if variantPreference.Effort != "" {
		result.ReasoningEffort = &variantPreference.Effort
	}
	return result, nil
}

func (s *CursorGatewayService) openCursorAgentStream(ctx context.Context, account *Account, dialogue *cursorpkg.Dialogue, options cursorpkg.AgentRunOptions, blobs map[string][]byte) (*Account, *http.Response, *cursorAgentEventAdapter, error) {
	activeAccount := account
	if s.dashboardAuth != nil && activeAccount != nil && activeAccount.ID > 0 {
		refreshed, _, err := s.dashboardAuth.RefreshIfNeeded(ctx, activeAccount)
		if err != nil {
			return activeAccount, nil, nil, err
		}
		if refreshed != nil {
			activeAccount = refreshed
		}
	}
	client, credential, err := s.newCursorAgentClient(ctx, activeAccount)
	if err != nil {
		return activeAccount, nil, nil, err
	}
	resp, agentStream, err := client.Run(ctx, credential, dialogue, options)
	if err == nil || !cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) || s.dashboardAuth == nil || activeAccount.ID <= 0 {
		if agentStream == nil {
			if err == nil {
				err = errors.New("Cursor Agent stream is unavailable")
			}
			return activeAccount, resp, nil, err
		}
		return activeAccount, resp, newCursorAgentEventAdapter(agentStream, blobs), err
	}
	refreshed, refreshErr := s.dashboardAuth.forceRefresh(ctx, activeAccount)
	if refreshErr != nil {
		return activeAccount, resp, nil, refreshErr
	}
	client, credential, err = s.newCursorAgentClient(ctx, refreshed)
	if err != nil {
		return refreshed, nil, nil, err
	}
	resp, agentStream, err = client.Run(ctx, credential, dialogue, options)
	if agentStream == nil {
		if err == nil {
			err = errors.New("Cursor Agent stream is unavailable")
		}
		return refreshed, resp, nil, err
	}
	return refreshed, resp, newCursorAgentEventAdapter(agentStream, blobs), err
}

func (s *CursorGatewayService) openCursorIDEStream(ctx context.Context, account *Account, dialogue *cursorpkg.Dialogue, options cursorpkg.IDEChatOptions) (*Account, *http.Response, *cursorpkg.IDEEventStream, error) {
	activeAccount := account
	if s.dashboardAuth != nil && activeAccount != nil && activeAccount.ID > 0 {
		refreshed, _, err := s.dashboardAuth.RefreshIfNeeded(ctx, activeAccount)
		if err != nil {
			return activeAccount, nil, nil, err
		}
		if refreshed != nil {
			activeAccount = refreshed
		}
	}
	client, credential, err := s.newCursorIDEClient(ctx, activeAccount)
	if err != nil {
		return activeAccount, nil, nil, err
	}
	resp, stream, err := client.StreamUnifiedChatWithTools(ctx, credential, dialogue, options)
	if err == nil || !cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) || s.dashboardAuth == nil || activeAccount.ID <= 0 {
		return activeAccount, resp, stream, err
	}
	refreshed, refreshErr := s.dashboardAuth.forceRefresh(ctx, activeAccount)
	if refreshErr != nil {
		return activeAccount, resp, stream, refreshErr
	}
	client, credential, err = s.newCursorIDEClient(ctx, refreshed)
	if err != nil {
		return refreshed, nil, nil, err
	}
	resp, stream, err = client.StreamUnifiedChatWithTools(ctx, credential, dialogue, options)
	return refreshed, resp, stream, err
}

func (s *CursorGatewayService) newCursorAgentClient(ctx context.Context, account *Account) (*cursorpkg.AgentClient, cursorpkg.IDECredential, error) {
	if account == nil {
		return nil, cursorpkg.IDECredential{}, errors.New("Cursor account is required")
	}
	accessToken := strings.TrimSpace(account.GetCredential("dashboard_access_token"))
	if accessToken == "" {
		return nil, cursorpkg.IDECredential{}, cursorpkg.HTTPError(http.StatusUnauthorized, "create Agent client", "Cursor Dashboard access token is missing")
	}
	cfg := s.cursorConfig()
	baseURL, err := cursorChatEndpoint(cfg.ChatBaseURL)
	if err != nil {
		return nil, cursorpkg.IDECredential{}, err
	}
	clientVersion := cursorAccountSetting(account, "cursor_client_version")
	if clientVersion == "" {
		clientVersion = cfg.ClientVersion
	}
	client, err := cursorpkg.NewAgentClient(s.newCursorIDEHTTPClient(ctx, account), cursorpkg.AgentClientConfig{IDEClientConfig: cursorpkg.IDEClientConfig{
		BaseURL: baseURL, ClientVersion: clientVersion,
		ClientOS: runtime.GOOS, ClientArch: cursorIDEClientArch(runtime.GOARCH),
		ClientOSVersion: cursorAccountSetting(account, "cursor_client_os_version"),
		ConfigVersion:   cursorAccountSetting(account, "cursor_config_version"),
		Timezone:        time.Now().Location().String(), GhostMode: cfg.GhostMode,
		NewOnboardingCompleted: cfg.NewOnboardingCompleted,
		MaxFrameSize:           cfg.MaxFrameBytes, MaxBufferedBytes: cfg.MaxBufferedBytes, MaxErrorBody: 8 << 10,
	}})
	if err != nil {
		return nil, cursorpkg.IDECredential{}, err
	}
	return client, cursorpkg.IDECredential{AccessToken: accessToken, MachineID: cursorAccountSetting(account, "cursor_machine_id")}, nil
}

func (s *CursorGatewayService) newCursorIDEClient(ctx context.Context, account *Account) (*cursorpkg.IDEClient, cursorpkg.IDECredential, error) {
	if account == nil {
		return nil, cursorpkg.IDECredential{}, errors.New("Cursor account is required")
	}
	accessToken := strings.TrimSpace(account.GetCredential("dashboard_access_token"))
	if accessToken == "" {
		return nil, cursorpkg.IDECredential{}, cursorpkg.HTTPError(http.StatusUnauthorized, "create IDE client", "Cursor Dashboard access token is missing")
	}
	cfg := s.cursorConfig()
	baseURL, err := cursorChatEndpoint(cfg.ChatBaseURL)
	if err != nil {
		return nil, cursorpkg.IDECredential{}, err
	}
	clientVersion := cursorAccountSetting(account, "cursor_client_version")
	if clientVersion == "" {
		clientVersion = cfg.ClientVersion
	}
	client, err := cursorpkg.NewIDEClient(s.newCursorIDEHTTPClient(ctx, account), cursorpkg.IDEClientConfig{
		BaseURL: baseURL, ClientVersion: clientVersion,
		ClientOS: runtime.GOOS, ClientArch: cursorIDEClientArch(runtime.GOARCH),
		ClientOSVersion: cursorAccountSetting(account, "cursor_client_os_version"),
		ConfigVersion:   cursorAccountSetting(account, "cursor_config_version"),
		Timezone:        time.Now().Location().String(), GhostMode: cfg.GhostMode,
		NewOnboardingCompleted: cfg.NewOnboardingCompleted,
		MaxFrameSize:           cfg.MaxFrameBytes, MaxBufferedBytes: cfg.MaxBufferedBytes, MaxErrorBody: 8 << 10,
	})
	if err != nil {
		return nil, cursorpkg.IDECredential{}, err
	}
	return client, cursorpkg.IDECredential{AccessToken: accessToken, MachineID: cursorAccountSetting(account, "cursor_machine_id")}, nil
}

func cursorIDEClientArch(goArch string) string {
	switch strings.ToLower(strings.TrimSpace(goArch)) {
	case "amd64":
		return "x64"
	default:
		return strings.TrimSpace(goArch)
	}
}

func prepareCursorAgentMode(dialogue *cursorpkg.Dialogue) cursorpkg.AgentMode {
	if dialogue == nil {
		return cursorpkg.AgentModeAsk
	}
	mode := strings.ToLower(strings.TrimSpace(dialogue.ToolChoice.Mode))
	if mode == "none" {
		dialogue.Tools = nil
		return cursorpkg.AgentModeAsk
	}
	if len(dialogue.Tools) == 0 {
		return cursorpkg.AgentModeAsk
	}
	dialogue.System = appendCursorSystemConstraint(dialogue.System, "Use only the supplied MCP tools. Do not invoke local shell, filesystem, editor, or workspace tools.")
	switch mode {
	case "any", "required":
		dialogue.System = appendCursorSystemConstraint(dialogue.System, "You must call at least one of the supplied tools before completing this turn.")
	case "tool", "function":
		if dialogue.ToolChoice.Name != "" {
			name := dialogue.ToolChoice.Name
			filtered := make([]cursorpkg.ToolDefinition, 0, 1)
			for _, tool := range dialogue.Tools {
				if tool.Name == name {
					filtered = append(filtered, tool)
				}
			}
			if len(filtered) > 0 {
				dialogue.Tools = filtered
			}
			dialogue.System = appendCursorSystemConstraint(dialogue.System, fmt.Sprintf("For this turn, call the supplied tool %q.", name))
		}
	}
	return cursorpkg.AgentModeAgent
}

func appendCursorSystemConstraint(system, constraint string) string {
	if strings.TrimSpace(system) == "" {
		return constraint
	}
	return strings.TrimSpace(system) + "\n\n" + constraint
}

func estimateCursorDialogueTokens(dialogue *cursorpkg.Dialogue) int {
	if dialogue == nil {
		return 0
	}
	total := cursorpkg.EstimateTokens(dialogue.System)
	for _, message := range dialogue.Messages {
		total += 4 + cursorpkg.EstimateTokens(message.Text)
		for _, action := range message.ToolCalls {
			encoded, _ := json.Marshal(action.Arguments)
			total += cursorpkg.EstimateTokens(action.Name) + cursorpkg.EstimateTokens(string(encoded))
		}
	}
	for _, tool := range dialogue.Tools {
		total += cursorpkg.EstimateTokens(tool.Name) + cursorpkg.EstimateTokens(tool.Description) + cursorpkg.EstimateTokens(string(tool.InputSchema))
	}
	return total
}

func trimCursorDialogue(dialogue *cursorpkg.Dialogue, maxMessages, maxTokens int) {
	if dialogue == nil {
		return
	}
	if maxMessages > 0 && len(dialogue.Messages) > maxMessages {
		dialogue.Messages = append([]cursorpkg.DialogueMessage(nil), dialogue.Messages[len(dialogue.Messages)-maxMessages:]...)
	}
	if maxTokens <= 0 {
		return
	}
	for len(dialogue.Messages) > 1 && estimateCursorDialogueTokens(dialogue) > maxTokens {
		dialogue.Messages = dialogue.Messages[1:]
	}
}

type cursorIDENextResult struct {
	event cursorpkg.IDEEvent
	err   error
}

func nextCursorIDEEvent(stream cursorIDEEventSource, idle time.Duration) (cursorpkg.IDEEvent, error) {
	if stream == nil {
		return cursorpkg.IDEEvent{}, errors.New("Cursor IDE event stream is unavailable")
	}
	if idle <= 0 {
		return stream.Next()
	}
	result := make(chan cursorIDENextResult, 1)
	go func() {
		event, err := stream.Next()
		result <- cursorIDENextResult{event: event, err: err}
	}()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	select {
	case next := <-result:
		return next.event, next.err
	case <-timer.C:
		_ = stream.Close()
		return cursorpkg.IDEEvent{}, cursorpkg.HTTPError(http.StatusGatewayTimeout, "IDE chat stream", fmt.Sprintf("stream idle timeout after %s", idle))
	}
}

func markCursorFirstToken(target **int, start time.Time) {
	if *target != nil {
		return
	}
	milliseconds := int(time.Since(start).Milliseconds())
	*target = &milliseconds
}
