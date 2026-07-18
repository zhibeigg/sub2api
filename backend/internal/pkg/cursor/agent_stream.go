package cursor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func (s *AgentStream) Response() *http.Response {
	if s == nil {
		return nil
	}
	return s.response
}

func (s *AgentStream) SendClientMessage(payload []byte) error {
	if s == nil {
		return errors.New("agent stream is nil")
	}
	message := append([]byte(nil), payload...)
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.sendClosed {
		return io.ErrClosedPipe
	}
	select {
	case s.send <- message:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *AgentStream) SendResume(options AgentRunOptions) error {
	return s.SendClientMessage(encodeAgentResumeAction(options))
}

func (s *AgentStream) SendKVGetResult(id uint64, blobData []byte, metadata ...[]byte) error {
	return s.SendClientMessage(encodeAgentKVGetResult(id, blobData, firstAgentKVMetadata(metadata)))
}

func (s *AgentStream) SendKVSetResult(id uint64, metadata ...[]byte) error {
	return s.SendClientMessage(encodeAgentKVSetResult(id, firstAgentKVMetadata(metadata)))
}

func firstAgentKVMetadata(metadata [][]byte) []byte {
	if len(metadata) == 0 {
		return nil
	}
	return metadata[0]
}

func (s *AgentStream) SendMCPResult(id uint64, execID, text string, isError bool) error {
	return s.SendClientMessage(encodeAgentMCPResult(id, execID, text, isError))
}

func (s *AgentStream) SendShellResult(id uint64, execID string, action *Action, text string, isError, streamed bool) error {
	messages := encodeAgentShellResult(id, execID, action, text, isError, streamed)
	for _, message := range messages {
		if err := s.SendClientMessage(message); err != nil {
			return err
		}
	}
	return nil
}

func (s *AgentStream) SendRequestContextResult(id uint64, execID string, tools []ToolDefinition, provider string) error {
	message, err := encodeAgentRequestContextResult(id, execID, tools, provider)
	if err != nil {
		return err
	}
	return s.SendClientMessage(message)
}

func (s *AgentStream) Cancel() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (s *AgentStream) CloseSend() error {
	if s == nil {
		return nil
	}
	if s.closeSend == nil {
		return nil
	}
	return s.closeSend()
}

func (s *AgentStream) closeSendQueue() error {
	s.sendMu.Lock()
	if !s.sendClosed {
		s.sendClosed = true
		close(s.send)
	}
	s.sendMu.Unlock()
	if s.writerErr == nil {
		return nil
	}
	err, ok := <-s.writerErr
	if !ok {
		return nil
	}
	if errors.Is(err, s.ctx.Err()) {
		return nil
	}
	return err
}

func (s *AgentStream) Next() (AgentEvent, error) {
	if s == nil || s.response == nil || s.response.Body == nil {
		return AgentEvent{}, io.EOF
	}
	if len(s.pending) > 0 {
		event := s.pending[0]
		s.pending = s.pending[1:]
		if event.terminal || event.Type == AgentEventError {
			s.finished = true
		}
		return event, nil
	}
	if s.finished {
		return AgentEvent{}, io.EOF
	}
	buffer := make([]byte, 32<<10)
	for {
		n, err := s.response.Body.Read(buffer)
		if n > 0 {
			frames, decodeErr := s.decoder.Feed(buffer[:n])
			if decodeErr != nil {
				s.finished = true
				return AgentEvent{}, decodeErr
			}
			for _, frame := range frames {
				if frame.EndStream() {
					s.sawEndStream = true
				}
				s.pending = append(s.pending, s.parseFrame(frame)...)
			}
			if len(s.pending) > 0 {
				event := s.pending[0]
				s.pending = s.pending[1:]
				if event.Type == AgentEventFinish || event.Type == AgentEventError {
					s.finished = true
				}
				return event, nil
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.finished = true
				return AgentEvent{}, transportError("read Agent stream", err)
			}
			if finishErr := s.decoder.Finish(); finishErr != nil {
				s.finished = true
				return AgentEvent{}, finishErr
			}
			s.finished = true
			if !s.sawEndStream {
				return AgentEvent{}, protocolError("read Agent stream", errors.New("stream ended before Connect end-stream envelope"))
			}
			return AgentEvent{}, io.EOF
		}
	}
}

func (s *AgentStream) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.finished = true
		if s.cancel != nil {
			s.cancel()
		}
		s.sendMu.Lock()
		if !s.sendClosed {
			s.sendClosed = true
			close(s.send)
		}
		s.sendMu.Unlock()
		if s.response != nil && s.response.Body != nil {
			s.closeErr = s.response.Body.Close()
		}
	})
	return s.closeErr
}

func (s *AgentStream) parseFrame(frame ConnectFrame) []AgentEvent {
	if frame.EndStream() {
		streamErr, err := parseConnectEndStream(frame.Payload)
		if err != nil {
			return []AgentEvent{agentProtocolEvent(err)}
		}
		if streamErr != nil {
			return []AgentEvent{{Type: AgentEventError, Error: streamErr}}
		}
		return []AgentEvent{{Type: AgentEventFinish, FinishReason: "stop", terminal: true}}
	}
	return s.parseServerMessage(frame.Payload)
}

func (s *AgentStream) parseServerMessage(payload []byte) []AgentEvent {
	var events []AgentEvent
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return append(events, agentProtocolEvent(protowire.ParseError(n)))
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return append(events, agentProtocolEvent(protowire.ParseError(size)))
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			return append(events, agentProtocolEvent(protowire.ParseError(size)))
		}
		payload = payload[size:]
		switch number {
		case 1:
			events = append(events, s.parseInteractionUpdate(value)...)
		case 2:
			events = append(events, parseAgentExecServerMessage(value)...)
		case 3:
			events = append(events, AgentEvent{Type: AgentEventCheckpoint, Checkpoint: decodeAgentConversationState(value), CheckpointRaw: append([]byte(nil), value...)})
		case 4:
			if event := parseAgentKVServerMessage(value); event != nil {
				events = append(events, *event)
			}
		}
	}
	return events
}

func (s *AgentStream) parseInteractionUpdate(payload []byte) []AgentEvent {
	var events []AgentEvent
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return append(events, agentProtocolEvent(protowire.ParseError(n)))
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return append(events, agentProtocolEvent(protowire.ParseError(size)))
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			return append(events, agentProtocolEvent(protowire.ParseError(size)))
		}
		payload = payload[size:]
		switch number {
		case 1:
			if text := firstProtoString(value, 1); text != "" {
				events = append(events, AgentEvent{Type: AgentEventText, Text: text})
			}
		case 2:
			events = append(events, s.parseToolUpdate(AgentEventToolStarted, value)...)
		case 3:
			events = append(events, s.parseToolUpdate(AgentEventToolCompleted, value)...)
		case 4:
			if text := firstProtoString(value, 1); text != "" {
				events = append(events, AgentEvent{Type: AgentEventThinking, Thinking: text})
			}
		case 5:
			if len(events) == 0 {
				events = append(events, AgentEvent{Type: AgentEventThinking})
			}
		case 7:
			events = append(events, s.parseToolUpdate(AgentEventToolPartial, value)...)
		case 13:
			events = append(events, AgentEvent{Type: AgentEventHeartbeat})
		case 14:
			usage := parseAgentTurnEndedUsage(value)
			if usage != nil {
				events = append(events, AgentEvent{Type: AgentEventUsage, Usage: usage})
			}
			events = append(events,
				AgentEvent{Type: AgentEventTurnEnded, Usage: usage, FinishReason: "stop"},
				AgentEvent{Type: AgentEventFinish, Usage: usage, FinishReason: "stop"},
			)
		case 16:
			events = append(events, AgentEvent{Type: AgentEventStepStarted, StepID: firstProtoVarint(value, 1)})
		case 17:
			events = append(events, AgentEvent{Type: AgentEventStepCompleted, StepID: firstProtoVarint(value, 1), StepDuration: durationMilliseconds(firstProtoVarint(value, 2))})
		}
	}
	return events
}

func (s *AgentStream) parseToolUpdate(eventType AgentEventType, payload []byte) []AgentEvent {
	callID := firstProtoString(payload, 1)
	toolPayload := firstProtoBytes(payload, 2)
	delta := ""
	if eventType == AgentEventToolPartial {
		delta = firstProtoString(payload, 3)
	}
	tool, supported, toolField := decodeAgentToolCall(toolPayload)
	if tool != nil && tool.ID == "" {
		tool.ID = callID
	}
	if callID == "" && tool != nil {
		callID = tool.ID
	}
	if !supported {
		return []AgentEvent{{Type: AgentEventUnsupportedExec, CallID: callID, Unsupported: &AgentUnsupportedExec{Field: toolField, Payload: append([]byte(nil), toolPayload...)}}}
	}
	if tool == nil {
		tool = &Action{ID: callID, Arguments: map[string]any{}}
	}
	acc := s.tools[callID]
	if acc == nil {
		acc = &agentToolAccumulator{id: callID, name: tool.Name}
		s.tools[callID] = acc
	}
	if tool.Name != "" {
		acc.name = tool.Name
	}
	if eventType == AgentEventToolStarted {
		acc.started = true
	}
	if delta != "" {
		if len(delta) > s.maxToolBytes-s.toolBytes {
			return []AgentEvent{{Type: AgentEventError, Error: &IDEStreamError{Code: "tool_arguments_too_large", Message: "tool arguments exceed configured stream buffer limit"}}}
		}
		acc.rawArgs += delta
		s.toolBytes += len(delta)
	}
	if eventType == AgentEventToolCompleted {
		delete(s.tools, callID)
		s.toolBytes -= len(acc.rawArgs)
		if len(tool.Arguments) == 0 && acc.rawArgs != "" {
			arguments, err := decodeJSONObject(acc.rawArgs)
			if err != nil {
				return []AgentEvent{{Type: AgentEventError, Error: &IDEStreamError{Code: "invalid_tool_arguments", Message: err.Error()}}}
			}
			tool.Arguments = arguments
		}
		tool.ID = acc.id
		tool.Name = acc.name
	}
	return []AgentEvent{{Type: eventType, Tool: tool, CallID: callID, ArgumentsDelta: delta}}
}

func decodeAgentToolCall(payload []byte) (*Action, bool, int) {
	if len(payload) == 0 {
		return nil, true, 0
	}
	id := firstProtoString(payload, 57)
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
		switch number {
		case 1:
			action := decodeAgentShellArgs(firstProtoBytes(value, 1))
			if action != nil && action.ID == "" {
				action.ID = id
			}
			return action, true, 1
		case 15:
			args := firstProtoBytes(value, 1)
			action := decodeAgentMCPArgs(args)
			if action != nil && action.ID == "" {
				action.ID = id
			}
			return action, true, 15
		case 54, 57:
			continue
		default:
			return nil, false, int(number)
		}
	}
	return nil, true, 0
}

func decodeAgentShellArgs(payload []byte) *Action {
	if len(payload) == 0 {
		return nil
	}
	arguments := map[string]any{"command": firstProtoString(payload, 1)}
	if workingDirectory := firstProtoString(payload, 2); workingDirectory != "" {
		arguments["working_directory"] = workingDirectory
	}
	if timeout := firstProtoVarint(payload, 3); timeout > 0 {
		arguments["timeout"] = int(timeout)
	}
	return &Action{ID: firstProtoString(payload, 4), Name: "shell", Arguments: arguments}
}

func decodeAgentMCPArgs(payload []byte) *Action {
	if len(payload) == 0 {
		return nil
	}
	name := firstProtoString(payload, 5)
	if name == "" {
		name = firstProtoString(payload, 1)
	}
	id := firstProtoString(payload, 3)
	arguments := make(map[string]any)
	for _, entry := range allProtoBytes(payload, 2) {
		key, value := decodeProtoStructEntry(entry)
		if key != "" {
			arguments[key] = value
		}
	}
	return &Action{ID: id, Name: name, Arguments: arguments}
}

func parseAgentExecServerMessage(payload []byte) []AgentEvent {
	id := firstProtoVarint(payload, 1)
	execID := firstProtoString(payload, 15)
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return []AgentEvent{agentProtocolEvent(protowire.ParseError(n))}
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
		switch number {
		case 2, 14:
			action := decodeAgentShellArgs(value)
			return []AgentEvent{{Type: AgentEventExecShell, ExecShell: action, Tool: action, ExecRequestID: id, ExecID: execID, ExecField: int(number)}}
		case 10:
			return []AgentEvent{{Type: AgentEventExecRequestContext, ExecRequestID: id, ExecID: execID, ExecField: int(number)}}
		case 11:
			action := decodeAgentMCPArgs(value)
			return []AgentEvent{{Type: AgentEventExecMCP, ExecMCP: action, Tool: action, ExecRequestID: id, ExecID: execID, ExecField: int(number)}}
		case 15, 19:
			continue
		default:
			return []AgentEvent{{Type: AgentEventUnsupportedExec, Unsupported: &AgentUnsupportedExec{ID: id, ExecID: execID, Field: int(number), Payload: append([]byte(nil), value...)}}}
		}
	}
	return nil
}

func parseAgentKVServerMessage(payload []byte) *AgentEvent {
	id := firstProtoVarint(payload, 1)
	metadata := append([]byte(nil), firstProtoBytes(payload, 4)...)
	if value := firstProtoBytes(payload, 2); value != nil {
		return &AgentEvent{Type: AgentEventKVGet, KV: &AgentKVRequest{
			ID: id, BlobID: append([]byte(nil), firstProtoBytes(value, 1)...), Metadata: metadata,
		}}
	}
	if value := firstProtoBytes(payload, 3); value != nil {
		return &AgentEvent{Type: AgentEventKVSet, KV: &AgentKVRequest{
			ID: id, BlobID: append([]byte(nil), firstProtoBytes(value, 1)...), BlobData: append([]byte(nil), firstProtoBytes(value, 2)...), Metadata: metadata,
		}}
	}
	return nil
}

func parseAgentTurnEndedUsage(payload []byte) *Usage {
	// Newer Agent models wrap usage in field 1, while Grok still returns the
	// usage counters directly on TurnEnded. Accept both layouts without treating
	// a wrapper that only contains the finish-reason field as zero usage.
	if usagePayload := firstProtoBytes(payload, 1); len(usagePayload) > 0 {
		usage := parseAgentTurnUsage(usagePayload)
		return &usage
	}
	if _, ok := firstProtoVarintPresent(payload, 1); !ok {
		return nil
	}
	usage := parseAgentTurnUsage(payload)
	return &usage
}

func parseAgentTurnUsage(payload []byte) Usage {
	// Cursor reports field 1 as total input, including cache reads and writes.
	rawInputTokens := int(firstProtoVarint(payload, 1))
	cacheReadTokens := int(firstProtoVarint(payload, 3))
	cacheWriteTokens := int(firstProtoVarint(payload, 4))
	inputTokens := rawInputTokens - cacheReadTokens - cacheWriteTokens
	if inputTokens < 0 {
		inputTokens = 0
	}
	usage := Usage{
		InputTokens: inputTokens, OutputTokens: int(firstProtoVarint(payload, 2)),
		CacheReadTokens: cacheReadTokens, CacheWriteTokens: cacheWriteTokens, ReasoningTokens: int(firstProtoVarint(payload, 5)),
	}
	usage.TotalTokens = usage.InputTokens + usage.CacheReadTokens + usage.CacheWriteTokens + usage.OutputTokens
	return usage
}

func firstProtoBytes(payload []byte, wanted protowire.Number) []byte {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil
		}
		payload = payload[n:]
		if wireType == protowire.BytesType {
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return nil
			}
			payload = payload[size:]
			if number == wanted {
				return value
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			return nil
		}
		payload = payload[size:]
	}
	return nil
}

func allProtoBytes(payload []byte, wanted protowire.Number) [][]byte {
	var values [][]byte
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		if wireType == protowire.BytesType {
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			if number == wanted {
				values = append(values, value)
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
	}
	return values
}

func firstProtoString(payload []byte, wanted protowire.Number) string {
	return string(firstProtoBytes(payload, wanted))
}

func firstProtoVarint(payload []byte, wanted protowire.Number) uint64 {
	value, _ := firstProtoVarintPresent(payload, wanted)
	return value
}

func firstProtoVarintPresent(payload []byte, wanted protowire.Number) (uint64, bool) {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return 0, false
		}
		payload = payload[n:]
		if wireType == protowire.VarintType {
			value, size := protowire.ConsumeVarint(payload)
			if size < 0 {
				return 0, false
			}
			payload = payload[size:]
			if number == wanted {
				return value, true
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			return 0, false
		}
		payload = payload[size:]
	}
	return 0, false
}

func decodeJSONObject(raw string) (map[string]any, error) {
	decoder := jsonDecoder(strings.NewReader(raw))
	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("multiple JSON values in tool arguments")
	}
	return arguments, nil
}

func jsonDecoder(reader io.Reader) *json.Decoder {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	return decoder
}

func durationMilliseconds(value uint64) time.Duration { return time.Duration(value) * time.Millisecond }

func agentProtocolEvent(err error) AgentEvent {
	return AgentEvent{Type: AgentEventError, Error: &IDEStreamError{Code: "protocol_error", Message: fmt.Sprint(err)}}
}
