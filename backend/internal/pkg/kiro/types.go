// Package kiro ports the core logic of Quorinex/Kiro-Go into sub2api: it turns
// an AWS Kiro / CodeWhisperer account into a Claude-compatible upstream.
//
// The package is self-contained (no dependency on sub2api's service layer) so
// it can be unit-tested in isolation. Callers pass a lightweight Credential and
// a parsed Anthropic/OpenAI request; the package handles request translation,
// the AWS binary event-stream upstream call, and response translation.
package kiro

import "encoding/json"

// Credential is the minimal set of Kiro account fields needed to refresh tokens
// and call the upstream. It mirrors the credentials JSON exported by Kiro-Go.
type Credential struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	AuthMethod   string `json:"authMethod,omitempty"` // "idc" (AWS Builder ID / IAM Identity Center) or "social"
	Provider     string `json:"provider,omitempty"`
	Region       string `json:"region,omitempty"`
	ProfileArn   string `json:"profileArn,omitempty"`
	MachineID    string `json:"machineId,omitempty"`
	Email        string `json:"email,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"` // Unix seconds
	ProxyURL     string `json:"proxyURL,omitempty"`
}

// ==================== Kiro upstream request structs ====================

// KiroPayload is the top-level request body sent to the Kiro API.
type KiroPayload struct {
	ConversationState struct {
		AgentContinuationId string `json:"agentContinuationId,omitempty"`
		AgentTaskType       string `json:"agentTaskType,omitempty"`
		ChatTriggerType     string `json:"chatTriggerType"`
		ConversationID      string `json:"conversationId"`
		CurrentMessage      struct {
			UserInputMessage KiroUserInputMessage `json:"userInputMessage"`
		} `json:"currentMessage"`
		History []KiroHistoryMessage `json:"history,omitempty"`
	} `json:"conversationState"`
	ProfileArn      string           `json:"profileArn,omitempty"`
	InferenceConfig *InferenceConfig `json:"inferenceConfig,omitempty"`

	// ToolNameMap maps sanitized tool names (sent to Kiro) back to the original
	// names supplied by the client. Not serialized to the Kiro request body.
	ToolNameMap map[string]string `json:"-"`
}

type KiroUserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId,omitempty"`
	Origin                  string                   `json:"origin"`
	Images                  []KiroImage              `json:"images,omitempty"`
	UserInputMessageContext *UserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

type UserInputMessageContext struct {
	Tools       []KiroToolWrapper `json:"tools,omitempty"`
	ToolResults []KiroToolResult  `json:"toolResults,omitempty"`
}

type KiroToolWrapper struct {
	ToolSpecification struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		InputSchema InputSchema `json:"inputSchema"`
	} `json:"toolSpecification"`
}

type InputSchema struct {
	JSON any `json:"json"`
}

type KiroToolResult struct {
	ToolUseID string              `json:"toolUseId"`
	Content   []KiroResultContent `json:"content"`
	Status    string              `json:"status"`
}

type KiroResultContent struct {
	Text string `json:"text"`
}

type KiroImage struct {
	Format string `json:"format"`
	Source struct {
		Bytes string `json:"bytes"`
	} `json:"source"`
}

type KiroHistoryMessage struct {
	UserInputMessage         *KiroUserInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *KiroAssistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

type KiroAssistantResponseMessage struct {
	Content  string        `json:"content"`
	ToolUses []KiroToolUse `json:"toolUses,omitempty"`
}

type KiroToolUse struct {
	ToolUseID string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

type InferenceConfig struct {
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
}

// ==================== Anthropic (Claude) API types ====================

type ClaudeRequest struct {
	Model       string                `json:"model"`
	Messages    []ClaudeMessage       `json:"messages"`
	MaxTokens   int                   `json:"max_tokens"`
	Temperature float64               `json:"temperature,omitempty"`
	TopP        float64               `json:"top_p,omitempty"`
	Stream      bool                  `json:"stream,omitempty"`
	System      any                   `json:"system,omitempty"` // string or []SystemBlock
	Thinking    *ClaudeThinkingConfig `json:"thinking,omitempty"`
	Tools       []ClaudeTool          `json:"tools,omitempty"`
	ToolChoice  any                   `json:"tool_choice,omitempty"`
}

type ClaudeThinkingConfig struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"`
}

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentBlock
}

type ClaudeContentBlock struct {
	Type      string       `json:"type"`
	Text      string       `json:"text,omitempty"`
	Thinking  string       `json:"thinking,omitempty"`
	Signature string       `json:"signature,omitempty"`
	ID        string       `json:"id,omitempty"`
	Name      string       `json:"name,omitempty"`
	Input     any          `json:"input,omitempty"`
	ToolUseID string       `json:"tool_use_id,omitempty"`
	Content   any          `json:"content,omitempty"` // for tool_result
	Source    *ImageSource `json:"source,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ClaudeTool struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	InputSchema  any    `json:"input_schema"`
	CacheControl any    `json:"cache_control,omitempty"`
}

type ClaudeResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Content      []ClaudeContentBlock `json:"content"`
	Model        string               `json:"model"`
	StopReason   string               `json:"stop_reason"`
	StopSequence *string              `json:"stop_sequence"`
	Usage        ClaudeUsage          `json:"usage"`
}

type ClaudeCacheCreationUsage struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens,omitempty"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens,omitempty"`
}

type ClaudeUsage struct {
	InputTokens              int                       `json:"input_tokens"`
	OutputTokens             int                       `json:"output_tokens"`
	CacheCreationInputTokens int                       `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int                       `json:"cache_read_input_tokens,omitempty"`
	CacheCreation            *ClaudeCacheCreationUsage `json:"cache_creation,omitempty"`
}

// ==================== OpenAI API types ====================

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
}

type OpenAIMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type OpenAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

// UnmarshalJSON accepts both the Chat Completions tool shape (nested under
// "function") and the Responses API shape (name/description/parameters at top
// level), so Responses-style tools don't parse with an empty name (which Kiro
// rejects with HTTP 400).
func (t *OpenAITool) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type        string `json:"type"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
		Function    *struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  any    `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.Type = raw.Type
	if raw.Function != nil {
		t.Function.Name = raw.Function.Name
		t.Function.Description = raw.Function.Description
		t.Function.Parameters = raw.Function.Parameters
	}
	if t.Function.Name == "" {
		t.Function.Name = raw.Name
	}
	if t.Function.Description == "" {
		t.Function.Description = raw.Description
	}
	if t.Function.Parameters == nil {
		t.Function.Parameters = raw.Parameters
	}
	return nil
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
