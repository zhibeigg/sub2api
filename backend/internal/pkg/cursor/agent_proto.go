package cursor

import (
	"crypto/sha256"
	"encoding/base64"
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
		var actionImages []InlineImage
		if actionIndex > 0 {
			switch dialogue.Messages[actionIndex-1].Role {
			case "user":
				actionIndex--
				actionText = dialogue.Messages[actionIndex].Text
				actionImages = dialogue.Messages[actionIndex].Images
			case "tool":
				// A rebuilt tool-result turn still needs a non-empty user action after
				// the assistant/tool history, otherwise Agent treats it as an empty prompt.
				actionText = "Continue the conversation using the latest tool result."
			}
		}
		userMessage := appendString(nil, 1, actionText)
		userMessage = appendString(userMessage, 2, uuidFn())
		selectedContext, err := encodeAgentSelectedContext(actionImages)
		if err != nil {
			return nil, badRequest("encode Agent user images", err)
		}
		if len(selectedContext) > 0 {
			userMessage = appendBytes(userMessage, 3, selectedContext)
		}
		userMessage = appendVarint(userMessage, 4, uint64(mode))
		userAction := appendBytes(nil, 1, userMessage)
		userAction = appendBytes(userAction, 2, requestContext)
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
	if options.ClientSupportsImages || dialogueHasInlineImages(dialogue) {
		request = appendVarint(request, 19, 1)
	}
	if options.ClientSupportsSend {
		request = appendVarint(request, 23, 1)
	}
	return appendBytes(nil, 1, request), nil // AgentClientMessage.run_request
}

func encodeAgentSelectedContext(images []InlineImage) ([]byte, error) {
	var selectedContext []byte
	for _, image := range images {
		mediaType, err := normalizeImageMIMEType(image.MIMEType)
		if err != nil {
			return nil, err
		}
		if len(image.Data) == 0 {
			return nil, errors.New("image data is empty")
		}
		selectedImage := appendString(nil, 7, mediaType)
		selectedImage = appendBytes(selectedImage, 8, image.Data)
		selectedContext = appendBytes(selectedContext, 1, selectedImage)
	}
	return selectedContext, nil
}

func dialogueHasInlineImages(dialogue *Dialogue) bool {
	if dialogue == nil {
		return false
	}
	for _, message := range dialogue.Messages {
		if len(message.Images) > 0 {
			return true
		}
	}
	return false
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
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "sub2api"
	}
	var wrapper []byte
	for _, tool := range tools {
		toolName := strings.TrimSpace(tool.Name)
		if toolName == "" {
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
		// McpToolDefinition.name is Cursor's internal unique name. Cursor itself
		// composes it from server_identifier + "-" + tool_name; reusing the raw
		// client name collides with native tools such as Glob, Grep, Read, and Write.
		// Keep field 5 as the original client-visible name so returned MCP calls can
		// still be decoded without an alias table.
		// McpToolDefinition.input_schema is google.protobuf.Value, not Struct.
		// JSON objects therefore need the Value.struct_value wrapper (field 5).
		schemaValue := appendBytes(nil, 5, structValue)
		encoded := appendString(nil, 1, provider+"-"+toolName)
		encoded = appendString(encoded, 4, provider)
		encoded = appendString(encoded, 5, toolName)
		encoded = appendString(encoded, 2, tool.Description)
		encoded = appendBytes(encoded, 3, schemaValue)
		wrapper = appendBytes(wrapper, 1, encoded)
	}
	return wrapper, nil
}

// PrepareAgentConversationState converts inline compatibility history into the
// blob-backed ConversationStateStructure expected by Cursor Agent. State fields
// contain SHA-256 blob IDs; the corresponding payloads are served later through
// the Agent KV duplex channel.
func PrepareAgentConversationState(dialogue *Dialogue, state *AgentConversationState, blobs map[string][]byte, messageID func() string) (*AgentConversationState, map[string][]byte, error) {
	if dialogue == nil {
		return nil, blobs, badRequest("prepare Agent conversation state", errors.New("dialogue is required"))
	}
	prepared := cloneAgentConversationState(state)
	if prepared == nil {
		prepared = &AgentConversationState{}
	}
	if blobs == nil {
		blobs = make(map[string][]byte)
	}
	if messageID == nil {
		sequence := 0
		messageID = func() string {
			sequence++
			return fmt.Sprintf("history-message-%d", sequence)
		}
	}

	history := agentConversationHistoryPrefix(dialogue.Messages)
	if len(prepared.RootPromptMessagesJSON) == 0 {
		rootPromptIDs, err := encodeAgentRootPromptMessages(dialogue.System, history, blobs)
		if err != nil {
			return nil, blobs, badRequest("encode Agent root prompt messages", err)
		}
		prepared.RootPromptMessagesJSON = rootPromptIDs
	}

	if len(prepared.Turns) == 0 {
		turns, err := encodeAgentConversationTurns(history, blobs, messageID)
		if err != nil {
			return nil, blobs, badRequest("encode Agent conversation turns", err)
		}
		prepared.Turns = turns
	}
	return prepared, blobs, nil
}

func agentConversationHistoryPrefix(messages []DialogueMessage) []DialogueMessage {
	end := len(messages)
	if end > 0 && messages[end-1].Role == "user" {
		end--
	}
	return messages[:end]
}

type agentRootPromptTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func encodeAgentRootPromptMessages(system string, history []DialogueMessage, blobs map[string][]byte) ([][]byte, error) {
	system = strings.TrimSpace(system)
	if system == "" {
		system = "You are a helpful assistant."
	}
	rootPrompt, err := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "system", Content: system})
	if err != nil {
		return nil, err
	}
	rootPromptIDs := [][]byte{storeAgentBlob(blobs, rootPrompt)}
	for _, message := range history {
		role := message.Role
		text := message.Text
		switch message.Role {
		case "user":
			text = textWithInlineImagePlaceholders(text, message.Images)
		case "assistant":
			text, err = agentHistoryStepText(message)
		case "tool":
			role = "user"
			text, err = agentHistoryStepText(message)
		default:
			err = fmt.Errorf("unsupported dialogue role %q", message.Role)
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		rootMessage, marshalErr := json.Marshal(struct {
			Role    string                    `json:"role"`
			Content []agentRootPromptTextPart `json:"content"`
		}{Role: role, Content: []agentRootPromptTextPart{{Type: "text", Text: text}}})
		if marshalErr != nil {
			return nil, marshalErr
		}
		rootPromptIDs = append(rootPromptIDs, storeAgentBlob(blobs, rootMessage))
	}
	return rootPromptIDs, nil
}

func encodeAgentConversationTurns(messages []DialogueMessage, blobs map[string][]byte, messageID func() string) ([][]byte, error) {
	turns := make([][]byte, 0)
	for index := 0; index < len(messages); {
		message := messages[index]
		if message.Role != "user" || !dialogueMessageHasContent(message) {
			index++
			continue
		}

		userMessage := appendString(nil, 1, message.Text)
		userMessage = appendString(userMessage, 2, messageID())
		selectedContext, err := encodeAgentSelectedContext(message.Images)
		if err != nil {
			return nil, err
		}
		if len(selectedContext) > 0 {
			userMessage = appendBytes(userMessage, 3, selectedContext)
		}
		userMessageBlobID := storeAgentBlob(blobs, userMessage)
		index++

		stepBlobIDs := make([][]byte, 0)
		for index < len(messages) && messages[index].Role != "user" {
			stepText, err := agentHistoryStepText(messages[index])
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(stepText) != "" {
				assistantMessage := appendString(nil, 1, stepText)
				conversationStep := appendBytes(nil, 1, assistantMessage)
				stepBlobIDs = append(stepBlobIDs, storeAgentBlob(blobs, conversationStep))
			}
			index++
		}

		agentTurn := appendBytes(nil, 1, userMessageBlobID)
		for _, stepBlobID := range stepBlobIDs {
			agentTurn = appendBytes(agentTurn, 2, stepBlobID)
		}
		agentTurn = appendString(agentTurn, 3, messageID())
		conversationTurn := appendBytes(nil, 1, agentTurn)
		turns = append(turns, storeAgentBlob(blobs, conversationTurn))
	}
	return turns, nil
}

func agentHistoryStepText(message DialogueMessage) (string, error) {
	parts := make([]string, 0, 1+len(message.ToolCalls))
	switch message.Role {
	case "assistant":
		if strings.TrimSpace(message.Text) != "" {
			parts = append(parts, message.Text)
		}
		for _, call := range message.ToolCalls {
			arguments, err := json.Marshal(call.Arguments)
			if err != nil {
				return "", fmt.Errorf("tool call %q arguments: %w", call.Name, err)
			}
			parts = append(parts, fmt.Sprintf("[Tool: %s]\nArguments: %s", call.Name, arguments))
		}
	case "tool":
		label := "Tool result"
		if message.IsError {
			label = "Tool error"
		}
		if strings.TrimSpace(message.ToolCallID) != "" {
			label += " for " + message.ToolCallID
		}
		parts = append(parts, fmt.Sprintf("[%s]\n%s", label, message.Text))
	default:
		return "", fmt.Errorf("unsupported dialogue role %q", message.Role)
	}
	return strings.Join(parts, "\n\n"), nil
}

func storeAgentBlob(blobs map[string][]byte, data []byte) []byte {
	hash := sha256.Sum256(data)
	blobID := append([]byte(nil), hash[:]...)
	key := base64.RawURLEncoding.EncodeToString(blobID)
	blobs[key] = append([]byte(nil), data...)
	return blobID
}

func cloneAgentConversationState(state *AgentConversationState) *AgentConversationState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.RootPromptMessagesJSON = cloneAgentByteSlices(state.RootPromptMessagesJSON)
	cloned.Turns = cloneAgentByteSlices(state.Turns)
	cloned.Todos = cloneAgentByteSlices(state.Todos)
	cloned.PendingToolCalls = append([]string(nil), state.PendingToolCalls...)
	cloned.Summary = append([]byte(nil), state.Summary...)
	cloned.Plan = append([]byte(nil), state.Plan...)
	cloned.PreviousWorkspaceURIs = append([]string(nil), state.PreviousWorkspaceURIs...)
	cloned.ReadPaths = append([]string(nil), state.ReadPaths...)
	return &cloned
}

func cloneAgentByteSlices(values [][]byte) [][]byte {
	if len(values) == 0 {
		return nil
	}
	cloned := make([][]byte, len(values))
	for index, value := range values {
		cloned[index] = append([]byte(nil), value...)
	}
	return cloned
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

func encodeAgentKVGetResult(id uint64, blobData, metadata []byte) []byte {
	result := appendBytes(nil, 1, blobData)
	message := appendVarint(nil, 1, id)
	message = appendBytes(message, 2, result)
	if len(metadata) > 0 {
		message = appendBytes(message, 4, metadata)
	}
	return appendBytes(nil, 3, message)
}

func encodeAgentKVSetResult(id uint64, metadata []byte) []byte {
	message := appendVarint(nil, 1, id)
	message = appendBytes(message, 3, nil)
	if len(metadata) > 0 {
		message = appendBytes(message, 4, metadata)
	}
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
	return encodeAgentExecClientMessage(id, execID, 11, result)
}

func encodeAgentShellResult(id uint64, execID string, action *Action, text string, isError, streamed bool) [][]byte {
	command, workingDirectory := "", ""
	if action != nil {
		for _, key := range []string{"command", "cmd", "script", "input"} {
			if value, ok := action.Arguments[key]; ok {
				command = fmt.Sprint(value)
				break
			}
		}
		for _, key := range []string{"working_directory", "workingDirectory", "cwd", "workdir", "work_dir"} {
			if value, ok := action.Arguments[key]; ok {
				workingDirectory = fmt.Sprint(value)
				break
			}
		}
	}

	details := appendString(nil, 1, command)
	details = appendString(details, 2, workingDirectory)
	if isError {
		details = appendVarint(details, 3, 1)
		details = appendString(details, 6, text)
	} else {
		details = appendVarint(details, 3, 0)
		details = appendString(details, 5, text)
	}
	details = appendVarint(details, 7, 0)
	var shellResult []byte
	if isError {
		shellResult = appendBytes(shellResult, 2, details)
	} else {
		shellResult = appendBytes(shellResult, 1, details)
	}

	messages := make([][]byte, 0, 5)
	if streamed {
		messages = append(messages, encodeAgentShellStream(id, execID, 4, nil))
		if text != "" {
			output := appendString(nil, 1, text)
			field := protowire.Number(1)
			if isError {
				field = 2
			}
			messages = append(messages, encodeAgentShellStream(id, execID, field, output))
		}
		exit := appendVarint(nil, 1, boolVarint(isError))
		exit = appendString(exit, 2, workingDirectory)
		messages = append(messages, encodeAgentShellStream(id, execID, 3, exit))
	}
	messages = append(messages, encodeAgentExecClientMessage(id, execID, 2, shellResult))
	if streamed {
		messages = append(messages, encodeAgentExecStreamClose(id))
	}
	return messages
}

func encodeAgentShellStream(id uint64, execID string, field protowire.Number, value []byte) []byte {
	stream := appendBytes(nil, field, value)
	return encodeAgentExecClientMessage(id, execID, 14, stream)
}

func encodeAgentExecStreamClose(id uint64) []byte {
	closeMessage := appendVarint(nil, 1, id)
	control := appendBytes(nil, 1, closeMessage)
	return appendBytes(nil, 5, control)
}

func encodeAgentRequestContextResult(id uint64, execID string, tools []ToolDefinition, provider string) ([]byte, error) {
	encodedTools, err := encodeAgentMCPTools(tools, provider)
	if err != nil {
		return nil, err
	}
	var requestContext []byte
	for _, tool := range allProtoBytes(encodedTools, 1) {
		requestContext = appendBytes(requestContext, 7, tool)
	}
	success := appendBytes(nil, 1, requestContext)
	result := appendBytes(nil, 1, success)
	return encodeAgentExecClientMessage(id, execID, 10, result), nil
}

func encodeAgentExecClientMessage(id uint64, execID string, field protowire.Number, result []byte) []byte {
	exec := appendVarint(nil, 1, id)
	if execID != "" {
		exec = appendString(exec, 15, execID)
	}
	exec = appendBytes(exec, field, result)
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
