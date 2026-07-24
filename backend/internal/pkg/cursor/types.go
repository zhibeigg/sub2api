// Package cursor implements the Cursor Cloud Agents API adapter and protocol
// compatibility transformations used by the gateway.
package cursor

import "encoding/json"

type Part struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Message struct {
	Parts []Part `json:"parts"`
	ID    string `json:"id"`
	Role  string `json:"role"`
}

type Request struct {
	Context  []Context `json:"context,omitempty"`
	Model    string    `json:"model"`
	ID       string    `json:"id"`
	Messages []Message `json:"messages"`
	Trigger  string    `json:"trigger"`
}

type Context struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	FilePath string `json:"filePath"`
}

type Usage struct {
	InputTokens      int `json:"inputTokens,omitempty"`
	OutputTokens     int `json:"outputTokens,omitempty"`
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
	ReasoningTokens  int `json:"reasoningTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

// HasTokens reports whether Cursor supplied at least one usable usage counter.
// Empty TurnEnded payloads are not valid usage reports and must allow callers
// to apply their normal estimation fallback.
func (u Usage) HasTokens() bool {
	return u.InputTokens != 0 ||
		u.OutputTokens != 0 ||
		u.CacheWriteTokens != 0 ||
		u.CacheReadTokens != 0 ||
		u.ReasoningTokens != 0 ||
		u.TotalTokens != 0
}

type Protocol string

const (
	ProtocolAnthropic  Protocol = "anthropic"
	ProtocolOpenAIChat Protocol = "openai_chat"
	ProtocolResponses  Protocol = "responses"
)

type Dialogue struct {
	System     string
	Messages   []DialogueMessage
	Tools      []ToolDefinition
	ToolChoice ToolChoice
}

type InlineImage struct {
	MIMEType string
	Data     []byte
}

type DialogueMessage struct {
	Role       string
	Text       string
	Images     []InlineImage
	ToolCalls  []Action
	ToolCallID string
	IsError    bool
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type ToolChoice struct {
	Mode string
	Name string
}

type BuildOptions struct {
	Model                string
	ConversationID       string
	MaxHistoryMessages   int
	MaxHistoryTokens     int
	HiddenOverheadTokens int
}
