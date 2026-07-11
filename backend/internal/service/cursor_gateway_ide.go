package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *CursorGatewayService) FetchIDEModels(ctx context.Context, account *Account) ([]string, error) {
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
	client, credential, err := s.newCursorIDEClient(ctx, activeAccount)
	if err != nil {
		return nil, err
	}
	resp, err := client.AvailableModels(ctx, credential)
	if err != nil && cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) && s.dashboardAuth != nil && activeAccount.ID > 0 {
		activeAccount, err = s.dashboardAuth.forceRefresh(ctx, activeAccount)
		if err == nil {
			client, credential, err = s.newCursorIDEClient(ctx, activeAccount)
		}
		if err == nil {
			resp, err = client.AvailableModels(ctx, credential)
		}
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.ProtoMajor != 2 {
		return nil, cursorpkg.HTTPError(http.StatusBadGateway, "IDE models request", "Cursor IDE chat requires HTTP/2")
	}
	cfg := s.cursorConfig()
	bodyLimit := int64(cfg.MaxBufferedBytes) + 1
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, err
	}
	if len(body) > cfg.MaxBufferedBytes {
		return nil, cursorpkg.HTTPError(http.StatusBadGateway, "IDE models request", "model catalog response exceeds configured buffer limit")
	}
	parsedModels, err := cursorpkg.DecodeIDEAvailableModels(body, cfg.MaxFrameBytes, cfg.MaxBufferedBytes)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(parsedModels))
	models := make([]string, 0, len(parsedModels))
	for _, item := range parsedModels {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		models = append(models, name)
	}
	sort.Strings(models)
	if len(models) == 0 {
		return nil, cursorpkg.HTTPError(http.StatusBadGateway, "IDE models request", "Cursor returned no IDE models")
	}
	return models, nil
}

func (s *CursorGatewayService) probeCursorIDE(ctx context.Context, account *Account) (string, error) {
	models, err := s.FetchIDEModels(ctx, account)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Cursor IDE chat session verified (%d models)", len(models)), nil
}

func (s *CursorGatewayService) forwardIDE(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol cursorpkg.Protocol) (*ForwardResult, error) {
	start := time.Now()
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("cursor HTTP upstream is not configured")
	}
	if account == nil || !account.IsCursorAPIKey() {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte("a Cursor account is required")}
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
	trimCursorDialogue(dialogue, s.cursorConfig().MaxHistoryMessages, s.cursorConfig().MaxHistoryTokens)
	mode := prepareCursorIDEMode(dialogue)
	estimatedInput := estimateCursorDialogueTokens(dialogue)

	activeAccount, resp, stream, err := s.openCursorIDEStream(ctx, account, dialogue, cursorpkg.IDEChatOptions{
		Model: upstreamModel, ConversationID: uuid.NewString(), Mode: mode,
	})
	if err != nil {
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
		return nil, mapCursorError(cursorpkg.HTTPError(http.StatusBadGateway, "IDE chat request", "Cursor IDE chat requires HTTP/2; negotiated "+proto))
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

	if protocol == cursorpkg.ProtocolResponses && (envelope.Store == nil || *envelope.Store) {
		storedDialogue := &cursorpkg.Dialogue{System: dialogue.System, Tools: dialogue.Tools, ToolChoice: dialogue.ToolChoice, Messages: append([]cursorpkg.DialogueMessage(nil), dialogue.Messages...)}
		storedDialogue.Messages = append(storedDialogue.Messages, cursorpkg.DialogueMessage{Role: "assistant", Text: collected.CleanText, ToolCalls: collected.Actions})
		if saveErr := s.saveCursorResponse(ctx, c, responseID, storedDialogue); saveErr != nil {
			if committed {
				_ = writer.WriteError("failed to store Cursor response continuation")
			}
			return nil, &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte("failed to store Cursor response continuation: " + saveErr.Error())}
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
	return &ForwardResult{
		RequestID: responseID,
		Usage: ClaudeUsage{
			InputTokens: collected.Usage.InputTokens, OutputTokens: collected.Usage.OutputTokens,
			CacheCreationInputTokens: collected.Usage.CacheWriteTokens, CacheReadInputTokens: collected.Usage.CacheReadTokens,
		},
		Model: requestModel, UpstreamModel: differentOrEmpty(requestModel, upstreamModel), Stream: envelope.Stream,
		Duration: time.Since(start), FirstTokenMs: firstTokenMs,
	}, nil
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
		ClientOS: runtime.GOOS, ClientArch: runtime.GOARCH,
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

func prepareCursorIDEMode(dialogue *cursorpkg.Dialogue) cursorpkg.IDEMode {
	if dialogue == nil {
		return cursorpkg.IDEModeAsk
	}
	mode := strings.ToLower(strings.TrimSpace(dialogue.ToolChoice.Mode))
	if mode == "none" {
		dialogue.Tools = nil
		return cursorpkg.IDEModeAsk
	}
	if len(dialogue.Tools) == 0 {
		return cursorpkg.IDEModeAsk
	}
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
	return cursorpkg.IDEModeAgent
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

func nextCursorIDEEvent(stream *cursorpkg.IDEEventStream, idle time.Duration) (cursorpkg.IDEEvent, error) {
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
