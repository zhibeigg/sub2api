package cursor

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func encodeAgentRunRequest(dialogue *Dialogue, options AgentRunOptions, uuidFn func() string) ([]byte, error) {
	if _, err := validateDialogue(dialogue); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(options.Model)
	if model == "" {
		return nil, badRequest("encode Agent run request", errors.New("model is required"))
	}
	mode := options.Mode
	if mode == AgentModeUnspecified {
		mode = AgentModeAgent
	}
	conversationID := strings.TrimSpace(options.ConversationID)
	if conversationID == "" {
		conversationID = uuidFn()
	}

	state := encodeAgentConversationState(options.ConversationState, mode)
	requestContext := encodeAgentRequestContext(options)
	var conversationAction []byte
	if options.Resume {
		resumeAction := appendBytes(nil, 2, requestContext)
		conversationAction = appendBytes(nil, 2, resumeAction)
	} else {
		actionIndex := len(dialogue.Messages)
		actionText := ""
		if actionIndex > 0 && dialogue.Messages[actionIndex-1].Role == "user" {
			actionIndex--
			actionText = dialogue.Messages[actionIndex].Text
		} else if actionIndex > 0 && dialogue.Messages[actionIndex-1].Role == "tool" {
			// A rebuilt tool-result turn still needs a non-empty user action after
			// the assistant/tool history, otherwise Agent treats it as an empty prompt.
			actionText = "Continue the conversation using the latest tool result."
		}
		userMessage := appendString(nil, 1, actionText)
		userMessage = appendString(userMessage, 2, uuidFn())
		userMessage = appendVarint(userMessage, 4, uint64(mode))
		history, err := encodeAgentConversationHistory(dialogue.Messages[:actionIndex])
		if err != nil {
			return nil, badRequest("encode Agent conversation history", err)
		}
		userAction := appendBytes(nil, 1, userMessage)
		userAction = appendBytes(userAction, 2, requestContext)
		if len(history) > 0 {
			userAction = appendBytes(userAction, 7, history)
		}
		conversationAction = appendBytes(nil, 1, userAction)
	}

	modelDetails := appendString(nil, 1, model)
	displayModel := strings.TrimSpace(options.DisplayModel)
	if displayModel == "" {
		displayModel = model
	}
	modelDetails = appendString(modelDetails, 3, displayModel)
	modelDetails = appendString(modelDetails, 4, displayModel)
	modelDetails = appendString(modelDetails, 5, displayModel)
	if options.MaxMode {
		modelDetails = appendVarint(modelDetails, 7, 1)
	}
	requestedModel := appendString(nil, 1, model)
	requestedModel = appendVarint(requestedModel, 2, boolVarint(options.MaxMode))
	requestedModel = appendVarint(requestedModel, 7, 1)

	mcpTools, err := encodeAgentMCPTools(dialogue.Tools, options.MCPProviderIdentifier)
	if err != nil {
		return nil, badRequest("encode Agent MCP tools", err)
	}
	request := appendBytes(nil, 1, state)
	request = appendBytes(request, 2, conversationAction)
	request = appendBytes(request, 3, modelDetails)
	request = appendBytes(request, 9, requestedModel)
	request = appendBytes(request, 4, mcpTools)
	request = appendString(request, 5, conversationID)
	if options.CustomSystemPrompt != "" {
		request = appendString(request, 8, options.CustomSystemPrompt)
	}
	if options.SuggestNextPrompt {
		request = appendVarint(request, 10, 1)
	}
	if options.ExcludeWorkspace {
		request = appendVarint(request, 12, 1)
	}
	if options.ConversationGroupID != "" {
		request = appendString(request, 16, options.ConversationGroupID)
	}
	if options.ClientSupportsImages {
		request = appendVarint(request, 19, 1)
	}
	if options.ClientSupportsSend {
		request = appendVarint(request, 23, 1)
	}
	return appendBytes(nil, 1, request), nil // AgentClientMessage.run_request
}

func encodeAgentConversationState(state *AgentConversationState, mode AgentMode) []byte {
	var out []byte
	if state == nil {
		return appendVarint(out, 10, uint64(mode))
	}
	for _, value := range state.RootPromptMessagesJSON {
		out = appendBytes(out, 1, value)
	}
	for _, value := range state.Turns {
		out = appendBytes(out, 8, value)
	}
	for _, value := range state.Todos {
		out = appendBytes(out, 3, value)
	}
	for _, value := range state.PendingToolCalls {
		out = appendString(out, 4, value)
	}
	if len(state.Summary) > 0 {
		out = appendBytes(out, 6, state.Summary)
	}
	if len(state.Plan) > 0 {
		out = appendBytes(out, 7, state.Plan)
	}
	for _, value := range state.PreviousWorkspaceURIs {
		out = appendString(out, 9, value)
	}
	stateMode := state.Mode
	if stateMode == AgentModeUnspecified {
		stateMode = mode
	}
	out = appendVarint(out, 10, uint64(stateMode))
	for _, value := range state.ReadPaths {
		out = appendString(out, 18, value)
	}
	if state.ActiveBranchName != "" {
		out = appendString(out, 19, state.ActiveBranchName)
	}
	if state.AgentType != "" {
		out = appendString(out, 22, state.AgentType)
	}
	if !state.StartedAt.IsZero() {
		out = appendVarint(out, 26, uint64(state.StartedAt.UnixMilli()))
	}
	if state.StartedTimeZone != "" {
		out = appendString(out, 27, state.StartedTimeZone)
	}
	return out
}

func encodeAgentRequestContext(options AgentRunOptions) []byte {
	ctx := options.RequestContext
	if ctx.OSVersion == "" {
		ctx.OSVersion = "unknown"
	}
	if len(ctx.WorkspacePaths) == 0 {
		ctx.WorkspacePaths = options.WorkspacePaths
	}
	if ctx.Shell == "" {
		ctx.Shell = options.Shell
	}
	if ctx.ProjectFolder == "" {
		ctx.ProjectFolder = options.ProjectFolder
	}
	var env []byte
	env = appendString(env, 1, ctx.OSVersion)
	for _, path := range ctx.WorkspacePaths {
		env = appendString(env, 2, path)
	}
	env = appendString(env, 3, ctx.Shell)
	env = appendString(env, 10, ctx.TimeZone)
	env = appendString(env, 11, ctx.ProjectFolder)
	var out []byte
	out = appendBytes(out, 4, env)
	out = appendVarint(out, 17, boolVarint(ctx.WebSearchEnabled))
	out = appendVarint(out, 24, boolVarint(ctx.WebFetchEnabled))
	out = appendVarint(out, 36, boolVarint(ctx.MCPInfoComplete))
	out = appendVarint(out, 40, boolVarint(ctx.EnvInfoComplete))
	fileNames := make([]string, 0, len(ctx.FileContents))
	for key := range ctx.FileContents {
		fileNames = append(fileNames, key)
	}
	sort.Strings(fileNames)
	for _, key := range fileNames {
		entry := appendString(nil, 1, key)
		entry = appendString(entry, 2, ctx.FileContents[key])
		out = appendBytes(out, 20, entry)
	}
	return out
}

func encodeAgentMCPTools(tools []ToolDefinition, provider string) ([]byte, error) {
	if provider == "" {
		provider = "sub2api"
	}
	var wrapper []byte
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, errors.New("tool name is required")
		}
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		structValue, err := encodeProtoStructJSON(schema)
		if err != nil {
			return nil, fmt.Errorf("tool %q schema: %w", tool.Name, err)
		}
		// McpToolDefinition.input_schema is google.protobuf.Value, not Struct.
		// JSON objects therefore need the Value.struct_value wrapper (field 5).
		schemaValue := appendBytes(nil, 5, structValue)
		encoded := appendString(nil, 1, tool.Name)
		encoded = appendString(encoded, 4, provider)
		encoded = appendString(encoded, 5, tool.Name)
		encoded = appendString(encoded, 2, tool.Description)
		encoded = appendBytes(encoded, 3, schemaValue)
		wrapper = appendBytes(wrapper, 1, encoded)
	}
	return wrapper, nil
}

func encodeAgentConversationHistory(messages []DialogueMessage) ([]byte, error) {
	var history []byte
	toolNames := make(map[string]string)
	for _, message := range messages {
		var encoded []byte
		switch message.Role {
		case "user":
			content := appendBytes(nil, 1, appendString(nil, 1, message.Text))
			encoded = appendBytes(nil, 1, appendBytes(nil, 1, content))
		case "assistant":
			var assistant []byte
			if message.Text != "" {
				assistant = appendBytes(assistant, 1, appendBytes(nil, 1, appendString(nil, 1, message.Text)))
			}
			for _, call := range message.ToolCalls {
				args, err := json.Marshal(call.Arguments)
				if err != nil {
					return nil, fmt.Errorf("tool call %q arguments: %w", call.Name, err)
				}
				if call.ID != "" {
					toolNames[call.ID] = call.Name
				}
				toolCall := appendString(nil, 1, call.ID)
				toolCall = appendString(toolCall, 2, call.Name)
				toolCall = appendString(toolCall, 3, string(args))
				assistant = appendBytes(assistant, 1, appendBytes(nil, 4, toolCall))
			}
			encoded = appendBytes(nil, 2, assistant)
		case "tool":
			tool := appendString(nil, 1, message.ToolCallID)
			if name := toolNames[message.ToolCallID]; name != "" {
				tool = appendString(tool, 2, name)
			}
			tool = appendBytes(tool, 3, appendBytes(nil, 1, appendString(nil, 1, message.Text)))
			if message.IsError {
				tool = appendVarint(tool, 4, 1)
			}
			encoded = appendBytes(nil, 3, tool)
		default:
			return nil, fmt.Errorf("unsupported dialogue role %q", message.Role)
		}
		history = appendBytes(history, 1, encoded)
	}
	return history, nil
}

func encodeProtoStructJSON(raw json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("JSON schema must be an object")
	}
	return encodeProtoStruct(object), nil
}

func encodeProtoStruct(object map[string]any) []byte {
	var out []byte
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := appendString(nil, 1, key)
		entry = appendBytes(entry, 2, encodeProtoValue(object[key]))
		out = appendBytes(out, 1, entry)
	}
	return out
}

func encodeProtoValue(value any) []byte {
	switch typed := value.(type) {
	case nil:
		return appendVarint(nil, 1, 0)
	case bool:
		return appendVarint(nil, 4, boolVarint(typed))
	case string:
		return appendString(nil, 3, typed)
	case json.Number:
		number, _ := typed.Float64()
		out := protowire.AppendTag(nil, 2, protowire.Fixed64Type)
		return protowire.AppendFixed64(out, math.Float64bits(number))
	case float64:
		out := protowire.AppendTag(nil, 2, protowire.Fixed64Type)
		return protowire.AppendFixed64(out, math.Float64bits(typed))
	case map[string]any:
		return appendBytes(nil, 5, encodeProtoStruct(typed))
	case []any:
		var list []byte
		for _, item := range typed {
			list = appendBytes(list, 1, encodeProtoValue(item))
		}
		return appendBytes(nil, 6, list)
	default:
		return appendString(nil, 3, fmt.Sprint(typed))
	}
}

func encodeAgentClientHeartbeat() []byte {
	return appendBytes(nil, 7, nil)
}

func encodeAgentResumeAction(options AgentRunOptions) []byte {
	resume := appendBytes(nil, 2, encodeAgentRequestContext(options))
	conversationAction := appendBytes(nil, 2, resume)
	return appendBytes(nil, 4, conversationAction)
}

func encodeAgentKVGetResult(id uint64, blobData []byte) []byte {
	result := appendBytes(nil, 1, blobData)
	message := appendVarint(nil, 1, id)
	message = appendBytes(message, 2, result)
	return appendBytes(nil, 3, message)
}

func encodeAgentKVSetResult(id uint64) []byte {
	message := appendVarint(nil, 1, id)
	message = appendBytes(message, 3, nil)
	return appendBytes(nil, 3, message)
}

func encodeAgentMCPResult(id uint64, execID, text string, isError bool) []byte {
	textContent := appendString(nil, 1, text)
	contentItem := appendBytes(nil, 1, textContent)
	success := appendBytes(nil, 1, contentItem)
	if isError {
		success = appendVarint(success, 2, 1)
	}
	result := appendBytes(nil, 1, success)
	exec := appendVarint(nil, 1, id)
	if execID != "" {
		exec = appendString(exec, 15, execID)
	}
	exec = appendBytes(exec, 11, result)
	return appendBytes(nil, 2, exec)
}

func encodeAgentConnectEndStream() []byte {
	return []byte{2, 0, 0, 0, 2, '{', '}'}
}

// DecodeAgentUsableModels decodes an unframed GetUsableModelsResponse protobuf payload.
func DecodeAgentUsableModels(payload []byte) ([]AgentModel, error) {
	return decodeAgentModels(payload)
}

func decodeAgentModels(payload []byte) ([]AgentModel, error) {
	var models []AgentModel
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		payload = payload[n:]
		if number == 1 && wireType == protowire.BytesType {
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return nil, protowire.ParseError(size)
			}
			payload = payload[size:]
			models = append(models, decodeAgentModel(value))
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			return nil, protowire.ParseError(size)
		}
		payload = payload[size:]
	}
	return models, nil
}

func decodeAgentModel(payload []byte) AgentModel {
	var model AgentModel
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		switch wireType {
		case protowire.BytesType:
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return model
			}
			payload = payload[size:]
			switch number {
			case 1:
				model.ID = string(value)
			case 2:
				model.SupportsThinking = true
			case 3:
				model.DisplayID = string(value)
			case 4:
				model.DisplayName = string(value)
			case 5:
				model.ShortDisplayName = string(value)
			case 6:
				model.Aliases = append(model.Aliases, string(value))
			}
		case protowire.VarintType:
			value, size := protowire.ConsumeVarint(payload)
			if size < 0 {
				return model
			}
			payload = payload[size:]
			if number == 7 {
				model.SupportsMaxMode = value != 0
			}
		default:
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return model
			}
			payload = payload[size:]
		}
	}
	return model
}

func decodeProtoStruct(payload []byte) map[string]any {
	result := make(map[string]any)
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		if number == 1 && wireType == protowire.BytesType {
			entry, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			key, value := decodeProtoStructEntry(entry)
			if key != "" {
				result[key] = value
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
	}
	return result
}

func decodeProtoStructEntry(payload []byte) (string, any) {
	var key string
	var value any
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
		field, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
		if number == 1 {
			key = string(field)
		}
		if number == 2 {
			value = decodeProtoValue(field)
		}
	}
	return key, value
}

func decodeProtoValue(payload []byte) any {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil
		}
		payload = payload[n:]
		switch {
		case number == 1 && wireType == protowire.VarintType:
			return nil
		case number == 2 && wireType == protowire.Fixed64Type:
			value, _ := protowire.ConsumeFixed64(payload)
			return math.Float64frombits(value)
		case (number == 3 || number == 5 || number == 6) && wireType == protowire.BytesType:
			value, _ := protowire.ConsumeBytes(payload)
			if number == 3 {
				return string(value)
			}
			if number == 5 {
				return decodeProtoStruct(value)
			}
			var list []any
			for len(value) > 0 {
				_, wt, tn := protowire.ConsumeTag(value)
				if tn < 0 {
					break
				}
				value = value[tn:]
				if wt != protowire.BytesType {
					break
				}
				item, sn := protowire.ConsumeBytes(value)
				if sn < 0 {
					break
				}
				value = value[sn:]
				list = append(list, decodeProtoValue(item))
			}
			return list
		case number == 4 && wireType == protowire.VarintType:
			value, _ := protowire.ConsumeVarint(payload)
			return value != 0
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			return nil
		}
		payload = payload[size:]
	}
	return nil
}

func appendFixed64(dst []byte, field protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, field, protowire.Fixed64Type)
	return binary.LittleEndian.AppendUint64(dst, value)
}

func decodeAgentConversationState(payload []byte) *AgentConversationState {
	state := &AgentConversationState{}
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
			switch number {
			case 1:
				state.RootPromptMessagesJSON = append(state.RootPromptMessagesJSON, append([]byte(nil), value...))
			case 3:
				state.Todos = append(state.Todos, append([]byte(nil), value...))
			case 4:
				state.PendingToolCalls = append(state.PendingToolCalls, string(value))
			case 6:
				state.Summary = append([]byte(nil), value...)
			case 7:
				state.Plan = append([]byte(nil), value...)
			case 8:
				state.Turns = append(state.Turns, append([]byte(nil), value...))
			case 9:
				state.PreviousWorkspaceURIs = append(state.PreviousWorkspaceURIs, string(value))
			case 18:
				state.ReadPaths = append(state.ReadPaths, string(value))
			case 19:
				state.ActiveBranchName = string(value)
			case 22:
				state.AgentType = string(value)
			case 27:
				state.StartedTimeZone = string(value)
			}
			continue
		}
		if wireType == protowire.VarintType {
			value, size := protowire.ConsumeVarint(payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			if number == 10 {
				state.Mode = AgentMode(value)
			}
			if number == 26 {
				state.StartedAt = time.UnixMilli(int64(value))
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
	}
	return state
}
