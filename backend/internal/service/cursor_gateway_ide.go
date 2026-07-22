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
	"reflect"
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
		return nil, errors.New("cursor Agent model catalog loader is unavailable")
	}
	key := cursorIDEModelCatalogKey(account)
	value, err, _ := s.ideModelRefresh.Do(key, func() (any, error) {
		models, loadErr := loader(ctx)
		if loadErr != nil {
			return nil, loadErr
		}
		if len(models) == 0 {
			return nil, errors.New("cursor Agent returned no supported models")
		}
		s.storeIDEModelCatalog(account, models)
		return append([]cursorIDEModel(nil), models...), nil
	})
	if err != nil {
		return nil, err
	}
	models, ok := value.([]cursorIDEModel)
	if !ok || len(models) == 0 {
		return nil, errors.New("cursor Agent returned an invalid model catalog")
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
		return nil, errors.New("cursor IDE returned no supported models")
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
		return nil, errors.New("cursor IDE returned no logical models")
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
	if strings.EqualFold(requested, "grok-4.5") || strings.EqualFold(requested, "cursor-grok-4.5") {
		// The logical Cloud model name is not accepted by Agent Run. Keep the first
		// request off the model-discovery hot path while using Cursor's default Grok
		// execution variant until the prewarmed catalog becomes available.
		fallback.ServerName = "cursor-grok-4.5-high"
	}
	catalog, state := s.cachedIDEModelCatalogState(account)
	if state == cursorIDEModelCatalogMiss {
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
	if preference.Effort == "" && (strings.EqualFold(requested, "grok-4.5") || strings.EqualFold(requested, "cursor-grok-4.5")) {
		for _, item := range filtered {
			if strings.EqualFold(item.ServerName, "cursor-grok-4.5-high") {
				return cursorIDESelection(item), nil
			}
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
	stream               *cursorpkg.AgentStream
	checkpoint           *cursorpkg.AgentConversationState
	blobs                map[string][]byte
	tools                []cursorpkg.ToolDefinition
	pendingMCP           *cursorAgentPendingMCP
	lastTool             *cursorpkg.Action
	emitted              map[string]struct{}
	sawTool              bool
	pendingFinish        bool
	seededBlobCount      int
	kvGetCount           int
	kvMissCount          int
	kvMetadataCount      int
	blobExchangeReported bool
}

func newCursorAgentEventAdapter(stream *cursorpkg.AgentStream, blobs map[string][]byte, tools []cursorpkg.ToolDefinition) *cursorAgentEventAdapter {
	return &cursorAgentEventAdapter{
		stream: stream, blobs: cloneCursorAgentBlobs(blobs), tools: append([]cursorpkg.ToolDefinition(nil), tools...), emitted: make(map[string]struct{}),
		seededBlobCount: len(blobs),
	}
}

func (s *cursorAgentEventAdapter) Next() (cursorpkg.IDEEvent, error) {
	if s == nil || s.stream == nil {
		return cursorpkg.IDEEvent{}, io.EOF
	}
	if s.pendingFinish {
		s.pendingFinish = false
		return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventFinish, FinishReason: "tool_calls"}, nil
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
			action := s.resolveTool(event.Tool)
			if mapped, ok := s.mapTool(action); ok {
				s.lastTool = cloneCursorAgentAction(action)
				return mapped, nil
			}
			s.lastTool = nil
		case cursorpkg.AgentEventExecMCP:
			action := s.correlateExecTool(s.resolveTool(event.ExecMCP), event.ExecID, event.ExecRequestID)
			if action != nil {
				s.pendingMCP = &cursorAgentPendingMCP{
					Kind: cursorAgentExecMCP, RequestID: event.ExecRequestID, ExecID: event.ExecID, ExecField: event.ExecField, Action: *action,
					CursorAction: cloneCursorAgentAction(event.ExecMCP),
				}
			}
			if mapped, ok := s.mapTool(action); ok {
				s.pendingFinish = true
				return mapped, nil
			}
			if s.pendingMCP != nil {
				return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventFinish, FinishReason: "tool_calls"}, nil
			}
		case cursorpkg.AgentEventExecShell:
			action := s.correlateExecTool(s.resolveTool(event.ExecShell), event.ExecID, event.ExecRequestID)
			if action != nil {
				kind := cursorAgentExecShell
				if event.ExecField == 14 {
					kind = cursorAgentExecShellStream
				}
				s.pendingMCP = &cursorAgentPendingMCP{
					Kind: kind, RequestID: event.ExecRequestID, ExecID: event.ExecID, ExecField: event.ExecField, Action: *action,
					CursorAction: cloneCursorAgentAction(event.ExecShell),
				}
			}
			if mapped, ok := s.mapTool(action); ok {
				s.pendingFinish = true
				return mapped, nil
			}
			if s.pendingMCP != nil {
				return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventFinish, FinishReason: "tool_calls"}, nil
			}
		case cursorpkg.AgentEventExecRequestContext:
			if err := s.stream.SendRequestContextResult(event.ExecRequestID, event.ExecID, s.tools, "sub2api"); err != nil {
				return cursorpkg.IDEEvent{}, err
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
			if err := s.stream.SendKVSetResult(event.KV.ID, event.KV.Metadata); err != nil {
				return cursorpkg.IDEEvent{}, err
			}
		case cursorpkg.AgentEventKVGet:
			if event.KV == nil {
				continue
			}
			s.kvGetCount++
			if len(event.KV.Metadata) > 0 {
				s.kvMetadataCount++
			}
			key := base64.RawURLEncoding.EncodeToString(event.KV.BlobID)
			blobData, found := s.blobs[key]
			if !found {
				s.kvMissCount++
				slog.Warn("cursor_agent_blob_missing", "blob_id", key, "seeded_blob_count", s.seededBlobCount)
			}
			blob := append([]byte(nil), blobData...)
			if err := s.stream.SendKVGetResult(event.KV.ID, blob, event.KV.Metadata); err != nil {
				return cursorpkg.IDEEvent{}, err
			}
		case cursorpkg.AgentEventTurnEnded:
			s.reportBlobExchange()
			reason := strings.TrimSpace(event.FinishReason)
			if s.sawTool {
				reason = "tool_calls"
			} else if reason == "" {
				reason = "stop"
			}
			return cursorpkg.IDEEvent{Type: cursorpkg.IDEEventFinish, FinishReason: reason}, nil
		case cursorpkg.AgentEventFinish:
			s.reportBlobExchange()
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

func (s *cursorAgentEventAdapter) reportBlobExchange() {
	if s == nil || s.blobExchangeReported || (s.seededBlobCount <= 1 && s.kvGetCount == 0) {
		return
	}
	s.blobExchangeReported = true
	slog.Info("cursor_agent_blob_exchange",
		"seeded_blob_count", s.seededBlobCount,
		"get_count", s.kvGetCount,
		"miss_count", s.kvMissCount,
		"metadata_count", s.kvMetadataCount,
	)
}

func cloneCursorAgentAction(action *cursorpkg.Action) *cursorpkg.Action {
	if action == nil {
		return nil
	}
	cloned := *action
	cloned.Arguments = shallowCopyAnyMap(action.Arguments)
	return &cloned
}

func (s *cursorAgentEventAdapter) resolveTool(action *cursorpkg.Action) *cursorpkg.Action {
	if action == nil {
		return nil
	}
	resolved := *action
	resolved.Arguments = shallowCopyAnyMap(action.Arguments)
	if !strings.EqualFold(strings.TrimSpace(resolved.Name), "shell") {
		return &resolved
	}

	bestScore := 0
	var best *cursorpkg.ToolDefinition
	for index := range s.tools {
		tool := &s.tools[index]
		score := cursorAgentShellToolScore(*tool)
		if score > bestScore {
			bestScore = score
			best = tool
		}
	}
	if best == nil {
		return &resolved
	}
	resolved.Name = best.Name
	resolved.Arguments = cursorAgentShellToolArguments(resolved.Arguments, best.InputSchema)
	return &resolved
}

func (s *cursorAgentEventAdapter) correlateExecTool(action *cursorpkg.Action, execID string, requestID uint64) *cursorpkg.Action {
	if action == nil {
		action = cloneCursorAgentAction(s.lastTool)
	}
	if action == nil {
		return nil
	}
	if s.lastTool != nil && strings.EqualFold(strings.TrimSpace(action.Name), strings.TrimSpace(s.lastTool.Name)) {
		if strings.TrimSpace(action.ID) == "" {
			action.ID = strings.TrimSpace(s.lastTool.ID)
		}
		if len(action.Arguments) == 0 {
			action.Arguments = shallowCopyAnyMap(s.lastTool.Arguments)
		}
	}
	if strings.TrimSpace(action.ID) == "" {
		action.ID = strings.TrimSpace(execID)
	}
	if strings.TrimSpace(action.ID) == "" {
		action.ID = fmt.Sprintf("call_cursor_%d", requestID)
	}
	s.lastTool = nil
	return action
}

func cursorAgentShellToolScore(tool cursorpkg.ToolDefinition) int {
	name := strings.ToLower(strings.TrimSpace(tool.Name))
	base := name
	if parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '.' || r == '/' || r == ':' || r == '\\' || r == '-'
	}); len(parts) > 0 {
		base = parts[len(parts)-1]
	}
	score := 0
	switch base {
	case "bash", "shell":
		score = 120
	case "terminal", "run_terminal_cmd", "execute_command", "run_command", "command":
		score = 100
	default:
		if strings.Contains(base, "shell") || strings.Contains(base, "bash") || strings.Contains(base, "terminal") {
			score = 90
		}
	}
	if strings.Contains(strings.ToLower(tool.Description), "shell") || strings.Contains(strings.ToLower(tool.Description), "terminal") {
		score += 10
	}
	properties := cursorAgentToolSchemaProperties(tool.InputSchema)
	if cursorAgentSchemaProperty(properties, "command", "cmd", "script") != "" {
		score += 40
	}
	return score
}

func cursorAgentShellToolArguments(arguments map[string]any, schema json.RawMessage) map[string]any {
	properties := cursorAgentToolSchemaProperties(schema)
	if len(properties) == 0 {
		return shallowCopyAnyMap(arguments)
	}
	mapped := make(map[string]any)
	if command, ok := arguments["command"]; ok {
		key := cursorAgentSchemaProperty(properties, "command", "cmd", "script", "input")
		if key == "" {
			key = "command"
		}
		mapped[key] = command
	}
	if workingDirectory, ok := arguments["working_directory"]; ok {
		if key := cursorAgentSchemaProperty(properties, "working_directory", "workingDirectory", "cwd", "workdir", "work_dir"); key != "" {
			mapped[key] = workingDirectory
		}
	}
	if timeout, ok := arguments["timeout"]; ok {
		if key := cursorAgentSchemaProperty(properties, "timeout", "timeout_ms", "timeoutMs"); key != "" {
			mapped[key] = timeout
		}
	}
	if key := cursorAgentSchemaProperty(properties, "description"); key != "" {
		mapped[key] = "Run a Cursor Agent shell command"
	}
	return mapped
}

func cursorAgentToolSchemaProperties(schema json.RawMessage) map[string]json.RawMessage {
	if len(schema) == 0 {
		return nil
	}
	var decoded struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return nil
	}
	return decoded.Properties
}

func cursorAgentSchemaProperty(properties map[string]json.RawMessage, candidates ...string) string {
	for _, candidate := range candidates {
		for property := range properties {
			if strings.EqualFold(property, candidate) {
				return property
			}
		}
	}
	return ""
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

func (s *cursorAgentEventAdapter) Cancel() {
	if s != nil && s.stream != nil {
		s.stream.Cancel()
	}
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

func (s *cursorAgentEventAdapter) SendPendingResult(pending *cursorAgentPendingMCP, result cursorpkg.DialogueMessage) error {
	if s == nil || s.stream == nil || pending == nil {
		return errors.New("cursor Agent tool resume state is unavailable")
	}
	kind := strings.TrimSpace(pending.Kind)
	if kind == "" {
		kind = cursorAgentExecMCP
	}
	var err error
	switch kind {
	case cursorAgentExecMCP:
		err = s.stream.SendMCPResult(pending.RequestID, pending.ExecID, result.Text, result.IsError)
	case cursorAgentExecShell, cursorAgentExecShellStream:
		action := pending.CursorAction
		if action == nil {
			action = &pending.Action
		}
		err = s.stream.SendShellResult(pending.RequestID, pending.ExecID, action, result.Text, result.IsError, kind == cursorAgentExecShellStream)
	default:
		err = fmt.Errorf("unsupported Cursor Agent pending tool kind %q", kind)
	}
	if err != nil {
		return err
	}
	s.pendingMCP = nil
	s.pendingFinish = false
	s.sawTool = false
	s.lastTool = nil
	return nil
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

func mergeCursorAgentDialogueMessages(previous, current []cursorpkg.DialogueMessage) []cursorpkg.DialogueMessage {
	if len(previous) == 0 {
		return append([]cursorpkg.DialogueMessage(nil), current...)
	}
	maxOverlap := len(previous)
	if len(current) < maxOverlap {
		maxOverlap = len(current)
	}
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if reflect.DeepEqual(previous[len(previous)-overlap:], current[:overlap]) {
			merged := append([]cursorpkg.DialogueMessage(nil), previous...)
			return append(merged, current[overlap:]...)
		}
	}
	merged := append([]cursorpkg.DialogueMessage(nil), previous...)
	return append(merged, current...)
}

func cursorAgentToolResultIDs(dialogue *cursorpkg.Dialogue) []string {
	if dialogue == nil {
		return nil
	}
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range dialogue.Messages {
		if message.Role != "tool" {
			continue
		}
		id := strings.TrimSpace(message.ToolCallID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
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

	dialogue, err := cursorpkg.ParseRequest(protocol, body)
	if err != nil {
		return nil, mapCursorError(err)
	}
	imageCount, imageBytes := cursorDialogueInlineImageStats(dialogue)
	if imageCount > 0 {
		if err := validateCursorInlineImageFrameBudget(imageBytes, s.cursorConfig().MaxFrameBytes); err != nil {
			return nil, err
		}
		slog.Info("cursor_agent_inline_images_prepared",
			"account_id", account.ID,
			"protocol", string(protocol),
			"model", requestModel,
			"image_count", imageCount,
			"image_bytes", imageBytes,
			"body_bytes", len(body),
		)
	}
	incomingToolResultIDs := cursorAgentToolResultIDs(dialogue)
	owner, ownerErr := cursorResponseOwner(c)
	var activeSession *cursorAgentActiveSession
	activeSessionExists := false
	if ownerErr == nil {
		activeSession, activeSessionExists = s.takeCursorAgentSession(owner, envelope.PreviousResponseID, incomingToolResultIDs)
	}
	if activeSession == nil && activeSessionExists {
		return nil, cursorRequestFailoverError("Cursor Agent is waiting for the matching tool result")
	}

	conversationID := uuid.NewString()
	var agentState *cursorpkg.AgentConversationState
	var agentBlobs map[string][]byte
	var pendingMCP *cursorAgentPendingMCP
	var previous *cursorStoredResponse
	if activeSession != nil {
		previous = activeSession.Stored
		pendingMCP = cloneCursorAgentPendingMCP(activeSession.Pending)
	} else if protocol == cursorpkg.ProtocolResponses && strings.TrimSpace(envelope.PreviousResponseID) != "" {
		loaded, loadErr := s.loadCursorStoredResponse(ctx, c, envelope.PreviousResponseID)
		if loadErr != nil {
			return nil, cursorRequestFailoverError(loadErr.Error())
		}
		previous = loaded
	} else {
		for index := len(incomingToolResultIDs) - 1; index >= 0; index-- {
			loaded, loadErr := s.loadCursorStoredResponse(ctx, c, "tool:"+incomingToolResultIDs[index])
			if loadErr == nil {
				previous = loaded
				break
			}
		}
	}
	if previous != nil {
		if previous.Dialogue == nil {
			if activeSession != nil {
				activeSession.Close()
			}
			return nil, cursorRequestFailoverError("previous_response_id has no stored dialogue")
		}
		if strings.TrimSpace(dialogue.System) == "" {
			dialogue.System = previous.Dialogue.System
		}
		if len(dialogue.Tools) == 0 && len(previous.Dialogue.Tools) > 0 {
			dialogue.Tools = append([]cursorpkg.ToolDefinition(nil), previous.Dialogue.Tools...)
		}
		dialogue.Messages = mergeCursorAgentDialogueMessages(previous.Dialogue.Messages, dialogue.Messages)
		if strings.TrimSpace(previous.AgentConversationID) != "" {
			conversationID = strings.TrimSpace(previous.AgentConversationID)
		}
		agentState = previous.AgentState
		agentBlobs = cloneCursorAgentBlobs(previous.AgentBlobs)
		if pendingMCP == nil {
			pendingMCP = cloneCursorAgentPendingMCP(previous.AgentPendingMCP)
		}
	}
	historyMessagesBefore := len(dialogue.Messages)
	historyTokensBefore := estimateCursorDialogueHistoryTokens(dialogue)
	fixedTokens := estimateCursorDialogueFixedTokens(dialogue)
	trimCursorDialogue(dialogue, s.cursorConfig().MaxHistoryMessages, s.cursorConfig().MaxHistoryTokens)
	if len(dialogue.Messages) < historyMessagesBefore {
		slog.Info("cursor_agent_history_trimmed",
			"before_messages", historyMessagesBefore,
			"after_messages", len(dialogue.Messages),
			"history_tokens_before", historyTokensBefore,
			"history_tokens_after", estimateCursorDialogueHistoryTokens(dialogue),
			"fixed_tokens", fixedTokens,
			"max_history_messages", s.cursorConfig().MaxHistoryMessages,
			"max_history_tokens", s.cursorConfig().MaxHistoryTokens)
	}
	mode := prepareCursorAgentMode(dialogue)
	estimatedInput := estimateCursorDialogueTokens(dialogue)
	variantPreference := envelope.variantPreference()
	if activeSession != nil && strings.TrimSpace(activeSession.UpstreamModel) != "" {
		upstreamModel = strings.TrimSpace(activeSession.UpstreamModel)
	}
	selection := cursorIDEModelSelection{ServerName: upstreamModel}
	if variantPreference.Thinking != nil {
		selection.Thinking = *variantPreference.Thinking
	}
	if activeSession == nil {
		resolvedSelection, resolveErr := s.resolveCursorIDEModel(ctx, account, upstreamModel, variantPreference)
		if resolveErr != nil {
			slog.Warn("cursor_ide_model_resolution_failed", "account_id", account.ID, "model", upstreamModel, "error", resolveErr.Error())
		} else if resolvedSelection.ServerName != "" {
			selection = resolvedSelection
			upstreamModel = resolvedSelection.ServerName
		}
	}

	toolResult, hasToolResult := cursorAgentToolResult(dialogue, pendingMCP)
	if activeSession != nil && (!hasToolResult || pendingMCP == nil) {
		activeSession.Close()
		return nil, cursorRequestFailoverError("Cursor Agent is waiting for the matching tool result")
	}
	resumeStoredMCP := false
	if activeSession == nil && pendingMCP != nil {
		if !hasToolResult {
			return nil, cursorRequestFailoverError("Cursor Agent is waiting for the matching tool result")
		}
		kind := strings.TrimSpace(pendingMCP.Kind)
		if kind == "" {
			kind = cursorAgentExecMCP
		}
		if kind != cursorAgentExecMCP {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusConflict, ResponseBody: []byte("cursor_agent_tool_session_expired: active Cursor Agent tool session is unavailable; retry the original request")}
		}
		resumeStoredMCP = true
	}
	if activeSession == nil {
		preparedState, preparedBlobs, prepareErr := cursorpkg.PrepareAgentConversationState(dialogue, agentState, agentBlobs, uuid.NewString)
		if prepareErr != nil {
			return nil, cursorRequestFailoverError(prepareErr.Error())
		}
		agentState = preparedState
		agentBlobs = preparedBlobs
	}

	runOptions := cursorpkg.AgentRunOptions{
		Model: upstreamModel, DisplayModel: requestModel, ConversationID: conversationID, Mode: mode,
		ConversationState: agentState, Resume: resumeStoredMCP, MCPProviderIdentifier: "sub2api",
		RequestContext: cursorpkg.AgentRequestContext{
			OSVersion: runtime.GOOS, TimeZone: time.Now().Location().String(), MCPInfoComplete: true, EnvInfoComplete: true,
		},
	}
	activeAccount := account
	var resp *http.Response
	var stream *cursorAgentEventAdapter
	var runCancel context.CancelFunc
	var stopRequestCancel func() bool
	reusedActiveSession := activeSession != nil
	if reusedActiveSession {
		stream = activeSession.Stream
		stopRequestCancel = context.AfterFunc(ctx, stream.Cancel)
		if activeSession.AccountID > 0 && activeSession.AccountID != account.ID {
			slog.Info("cursor_agent_active_session_account_reused", "selected_account_id", account.ID, "stream_account_id", activeSession.AccountID, "model", upstreamModel)
		}
		err = stream.SendPendingResult(pendingMCP, toolResult)
	} else {
		var runCtx context.Context
		runCtx, runCancel = context.WithCancel(context.WithoutCancel(ctx))
		stopRequestCancel = context.AfterFunc(ctx, runCancel)
		activeAccount, resp, stream, err = s.openCursorAgentStream(ctx, runCtx, account, dialogue, runOptions, agentBlobs)
		if err == nil && resumeStoredMCP {
			err = stream.SendPendingResult(pendingMCP, toolResult)
		}
		if err != nil && resumeStoredMCP {
			if stream != nil {
				_ = stream.Close()
			}
			slog.Warn("cursor_agent_resume_failed_rebuilding_history",
				"account_id", account.ID,
				"model", upstreamModel,
				"error_type", fmt.Sprintf("%T", err),
				"error_bytes", len(err.Error()),
			)
			conversationID = uuid.NewString()
			runOptions.ConversationID = conversationID
			runOptions.Resume = false
			rebuiltState, rebuiltBlobs, rebuildErr := cursorpkg.PrepareAgentConversationState(dialogue, nil, nil, uuid.NewString)
			if rebuildErr != nil {
				err = rebuildErr
			} else {
				runOptions.ConversationState = rebuiltState
				activeAccount, resp, stream, err = s.openCursorAgentStream(ctx, runCtx, account, dialogue, runOptions, rebuiltBlobs)
			}
		}
	}
	if err != nil {
		if stopRequestCancel != nil {
			stopRequestCancel()
		}
		if runCancel != nil {
			runCancel()
		}
		if activeSession != nil {
			activeSession.Close()
		}
		slog.Warn("cursor_agent_open_stream_failed",
			"account_id", account.ID,
			"model", upstreamModel,
			"reused_active_session", reusedActiveSession,
			"error_type", fmt.Sprintf("%T", err),
			"error_bytes", len(err.Error()),
		)
		return nil, mapCursorError(err)
	}
	if !reusedActiveSession && (resp == nil || resp.ProtoMajor != 2) {
		if stopRequestCancel != nil {
			stopRequestCancel()
		}
		if runCancel != nil {
			runCancel()
		}
		if stream != nil {
			_ = stream.Close()
		}
		proto := "unknown"
		if resp != nil {
			proto = resp.Proto
		}
		return nil, mapCursorError(cursorpkg.HTTPError(http.StatusBadGateway, "Agent run request", "Cursor Agent RPC requires HTTP/2; negotiated "+proto))
	}
	closeStream := true
	defer func() {
		if stopRequestCancel != nil {
			stopRequestCancel()
		}
		if closeStream && stream != nil {
			_ = stream.Close()
		}
		if closeStream && runCancel != nil {
			runCancel()
		}
	}()

	responseID := cursorResponseID(protocol)
	writer := newCursorIDEStreamWriter(c, protocol, responseID, requestModel)
	collected := cursorCollected{FinishReason: "stop"}
	var firstTokenMs *int
	committed := false
	finished := false
	usageReported := false

	for !finished {
		event, nextErr := nextCursorIDEEvent(stream, durationSeconds(s.cursorConfig().IDEStreamIdleTimeoutSeconds, 60))
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			slog.Warn("cursor_ide_stream_read_failed",
				"account_id", account.ID,
				"model", upstreamModel,
				"committed", committed,
				"error_type", fmt.Sprintf("%T", nextErr),
				"error_bytes", len(nextErr.Error()),
			)
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
				usageReported = true
			}
		case cursorpkg.IDEEventFinish:
			if event.FinishReason != "" {
				collected.FinishReason = event.FinishReason
			}
			finished = true
		case cursorpkg.IDEEventError:
			statusCode, code, message, details := cursorAgentStreamFailure(event.Error)
			slog.Warn("cursor_ide_stream_event_failed",
				"account_id", account.ID,
				"model", upstreamModel,
				"committed", committed,
				"status_code", statusCode,
				"code", code,
				"message_bytes", len(message),
				"details_bytes", len(details),
			)
			return nil, cursorAgentStreamFailoverError(statusCode, code, message)
		}
	}

	if err := validateCursorToolResult(dialogue, collected.Actions); err != nil {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(err.Error())}
	}
	pending := stream.PendingMCP()
	// Tool-result boundaries are not terminal turns; the final TurnEnded usage covers the whole Agent run.
	if !usageReported && pending == nil {
		collected.Usage.InputTokens = estimatedInput
		collected.Usage.OutputTokens = cursorpkg.EstimateTokens(collected.CleanText) + cursorpkg.EstimateTokens(collected.Reasoning) + estimateCursorActionTokens(collected.Actions)
	}
	collected.Usage.TotalTokens = collected.Usage.InputTokens + collected.Usage.OutputTokens + collected.Usage.CacheWriteTokens + collected.Usage.CacheReadTokens

	storedDialogue := &cursorpkg.Dialogue{System: dialogue.System, Tools: dialogue.Tools, ToolChoice: dialogue.ToolChoice, Messages: append([]cursorpkg.DialogueMessage(nil), dialogue.Messages...)}
	storedDialogue.Messages = append(storedDialogue.Messages, cursorpkg.DialogueMessage{Role: "assistant", Text: collected.CleanText, ToolCalls: collected.Actions})
	if pending != nil && stopRequestCancel != nil {
		stopRequestCancel()
		stopRequestCancel = nil
	}
	storedResponse := &cursorStoredResponse{
		Dialogue: storedDialogue, AgentConversationID: strings.TrimSpace(conversationID), AgentState: stream.Checkpoint(),
		AgentBlobs: cloneCursorAgentBlobs(stream.Blobs()), AgentPendingMCP: cloneCursorAgentPendingMCP(pending),
	}
	if protocol == cursorpkg.ProtocolResponses && (envelope.Store == nil || *envelope.Store) {
		if saveErr := s.saveCursorStoredResponse(ctx, c, responseID, storedResponse); saveErr != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte("failed to store Cursor response continuation: " + saveErr.Error())}
		}
	} else if pending != nil && strings.TrimSpace(pending.Action.ID) != "" {
		if saveErr := s.saveCursorStoredResponse(ctx, c, "tool:"+pending.Action.ID, storedResponse); saveErr != nil {
			slog.Warn("cursor_agent_tool_continuation_store_failed", "account_id", account.ID, "error", saveErr.Error())
		}
	}
	var registeredSession *cursorAgentActiveSession
	if pending != nil && strings.TrimSpace(pending.Action.ID) != "" {
		if ownerErr != nil {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusInternalServerError, ResponseBody: []byte(ownerErr.Error())}
		}
		accountID := account.ID
		if activeAccount != nil && activeAccount.ID > 0 {
			accountID = activeAccount.ID
		}
		registeredSession = &cursorAgentActiveSession{
			Stream: stream, Stored: storedResponse, Pending: cloneCursorAgentPendingMCP(pending), AccountID: accountID, UpstreamModel: upstreamModel,
		}
		s.storeCursorAgentSession(owner, responseID, pending.Action.ID, registeredSession)
		closeStream = false
	}
	if envelope.Stream {
		if err := writer.Finish(collected); err != nil {
			if registeredSession != nil && s.removeCursorAgentSession(registeredSession) {
				registeredSession.Close()
			}
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

func cursorAgentStreamFailure(streamErr *cursorpkg.IDEStreamError) (statusCode int, code, message, details string) {
	statusCode = http.StatusBadGateway
	message = "Cursor Agent stream failed"
	if streamErr == nil {
		return statusCode, "", message, ""
	}
	code = strings.ToLower(strings.TrimSpace(streamErr.Code))
	if text := strings.TrimSpace(streamErr.Message); text != "" {
		message = text
	}
	if len(streamErr.Details) > 0 {
		details = truncateString(strings.TrimSpace(string(streamErr.Details)), 2048)
	}
	switch code {
	case "invalid_argument", "failed_precondition", "out_of_range":
		statusCode = http.StatusBadRequest
	case "unauthenticated":
		statusCode = http.StatusUnauthorized
	case "permission_denied":
		statusCode = http.StatusForbidden
	case "not_found":
		statusCode = http.StatusNotFound
	case "already_exists", "aborted":
		statusCode = http.StatusConflict
	case "resource_exhausted":
		statusCode = http.StatusTooManyRequests
	case "unimplemented":
		statusCode = http.StatusNotImplemented
	case "unavailable":
		statusCode = http.StatusServiceUnavailable
	case "deadline_exceeded":
		statusCode = http.StatusGatewayTimeout
	}
	return statusCode, code, message, details
}

func cursorAgentStreamFailoverError(statusCode int, code, message string) error {
	if statusCode == 0 {
		statusCode = http.StatusBadGateway
	}
	if statusCode == http.StatusBadRequest {
		message = "Cursor rejected the request payload"
	} else if strings.TrimSpace(message) == "" {
		message = "Cursor Agent stream failed"
	}
	body, err := json.Marshal(map[string]any{
		"error": map[string]string{"type": code, "message": message},
	})
	if err != nil {
		body = []byte(message)
	}
	failure := &UpstreamFailoverError{StatusCode: statusCode, ResponseBody: body}
	if statusCode == http.StatusBadRequest {
		failure.Stage = GatewayFailureStageInference
		failure.Scope = GatewayFailureScopeRequest
		failure.Reason = GatewayFailureReason("cursor_invalid_request")
		failure.NextAccountAction = NextAccountStop
		failure.ClientStatusCode = http.StatusBadRequest
		failure.ClientMessage = message
	}
	return failure
}

func (s *CursorGatewayService) runCursorAgentWithOpenRetry(ctx context.Context, account *Account, client *cursorpkg.AgentClient, credential cursorpkg.IDECredential, dialogue *cursorpkg.Dialogue, options cursorpkg.AgentRunOptions) (*http.Response, *cursorpkg.AgentStream, error) {
	resp, stream, err := client.Run(ctx, credential, dialogue, options)
	if err == nil || ctx.Err() != nil || !isRetryableCursorAgentOpenError(err) {
		return resp, stream, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	slog.Warn("cursor_agent_open_retry",
		"account_id", accountID,
		"model", options.Model,
		"reason", "retryable_http2_transport",
		"error", truncateString(err.Error(), 2048),
	)
	return client.Run(ctx, credential, dialogue, options)
}

func isRetryableCursorAgentOpenError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var cursorErr *cursorpkg.Error
	if !errors.As(err, &cursorErr) || cursorErr.Kind != cursorpkg.ErrorTransport {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "goaway") || strings.Contains(message, "refused_stream") || strings.Contains(message, "refused stream")
}

func (s *CursorGatewayService) openCursorAgentStream(ctx, runCtx context.Context, account *Account, dialogue *cursorpkg.Dialogue, options cursorpkg.AgentRunOptions, blobs map[string][]byte) (*Account, *http.Response, *cursorAgentEventAdapter, error) {
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
	resp, agentStream, err := s.runCursorAgentWithOpenRetry(runCtx, activeAccount, client, credential, dialogue, options)
	if err == nil || !cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) || s.dashboardAuth == nil || activeAccount.ID <= 0 {
		if agentStream == nil {
			if err == nil {
				err = errors.New("cursor Agent stream is unavailable")
			}
			return activeAccount, resp, nil, err
		}
		return activeAccount, resp, newCursorAgentEventAdapter(agentStream, blobs, dialogue.Tools), err
	}
	refreshed, refreshErr := s.dashboardAuth.forceRefresh(ctx, activeAccount)
	if refreshErr != nil {
		return activeAccount, resp, nil, refreshErr
	}
	client, credential, err = s.newCursorAgentClient(ctx, refreshed)
	if err != nil {
		return refreshed, nil, nil, err
	}
	resp, agentStream, err = s.runCursorAgentWithOpenRetry(runCtx, refreshed, client, credential, dialogue, options)
	if agentStream == nil {
		if err == nil {
			err = errors.New("cursor Agent stream is unavailable")
		}
		return refreshed, resp, nil, err
	}
	return refreshed, resp, newCursorAgentEventAdapter(agentStream, blobs, dialogue.Tools), err
}

func (s *CursorGatewayService) newCursorAgentClient(ctx context.Context, account *Account) (*cursorpkg.AgentClient, cursorpkg.IDECredential, error) {
	if account == nil {
		return nil, cursorpkg.IDECredential{}, errors.New("cursor account is required")
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

func estimateCursorDialogueFixedTokens(dialogue *cursorpkg.Dialogue) int {
	if dialogue == nil {
		return 0
	}
	total := cursorpkg.EstimateTokens(dialogue.System)
	for _, tool := range dialogue.Tools {
		total += cursorpkg.EstimateTokens(tool.Name) + cursorpkg.EstimateTokens(tool.Description) + cursorpkg.EstimateTokens(string(tool.InputSchema))
	}
	return total
}

const cursorInlineImageEstimatedTokens = 1024

func estimateCursorDialogueHistoryTokens(dialogue *cursorpkg.Dialogue) int {
	if dialogue == nil {
		return 0
	}
	total := 0
	for _, message := range dialogue.Messages {
		total += 4 + cursorpkg.EstimateTokens(message.Text)
		total += len(message.Images) * cursorInlineImageEstimatedTokens
		for _, action := range message.ToolCalls {
			encoded, _ := json.Marshal(action.Arguments)
			total += cursorpkg.EstimateTokens(action.Name) + cursorpkg.EstimateTokens(string(encoded))
		}
	}
	return total
}

func cursorDialogueInlineImageStats(dialogue *cursorpkg.Dialogue) (count, totalBytes int) {
	if dialogue == nil {
		return 0, 0
	}
	for _, message := range dialogue.Messages {
		count += len(message.Images)
		for _, image := range message.Images {
			totalBytes += len(image.Data)
		}
	}
	return count, totalBytes
}

func validateCursorInlineImageFrameBudget(imageBytes, maxFrameBytes int) error {
	if imageBytes <= 0 {
		return nil
	}
	if maxFrameBytes <= 0 {
		maxFrameBytes = 8 << 20
	}
	reserve := maxFrameBytes / 4
	if reserve < 256<<10 {
		reserve = 256 << 10
	}
	if reserve > 2<<20 {
		reserve = 2 << 20
	}
	budget := maxFrameBytes - reserve
	if budget < 0 || imageBytes > budget {
		return cursorRequestPayloadTooLargeError("inline images exceed the configured Cursor Agent frame budget")
	}
	return nil
}

func estimateCursorDialogueTokens(dialogue *cursorpkg.Dialogue) int {
	return estimateCursorDialogueFixedTokens(dialogue) + estimateCursorDialogueHistoryTokens(dialogue)
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
	// maxTokens limits replayed conversation history only. System instructions
	// and MCP tool schemas are fixed request overhead; counting them against the
	// history budget causes large agent clients to lose every prior turn.
	for len(dialogue.Messages) > 1 && estimateCursorDialogueHistoryTokens(dialogue) > maxTokens {
		dialogue.Messages = dialogue.Messages[1:]
	}
}

type cursorIDENextResult struct {
	event cursorpkg.IDEEvent
	err   error
}

func nextCursorIDEEvent(stream cursorIDEEventSource, idle time.Duration) (cursorpkg.IDEEvent, error) {
	if stream == nil {
		return cursorpkg.IDEEvent{}, errors.New("cursor IDE event stream is unavailable")
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
