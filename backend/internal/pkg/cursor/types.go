// Package cursor implements the safe, transport-agnostic core of Cursor's
// documented /api/chat protocol.
package cursor

import (
	"encoding/json"
	"time"
)

const DefaultBaseURL = "https://cursor.com/api/chat"

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
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}

type MessageMetadata struct {
	Usage *Usage `json:"usage,omitempty"`
}

type SSEEvent struct {
	Type            string           `json:"type"`
	Delta           string           `json:"delta,omitempty"`
	FinishReason    string           `json:"finishReason,omitempty"`
	MessageMetadata *MessageMetadata `json:"messageMetadata,omitempty"`
	Usage           *Usage           `json:"usage,omitempty"`
}

func (e SSEEvent) EventUsage() *Usage {
	if e.MessageMetadata != nil && e.MessageMetadata.Usage != nil {
		return e.MessageMetadata.Usage
	}
	return e.Usage
}

type Credential struct {
	Cookie string `json:"cookie"`
}

type ClientConfig struct {
	BaseURL           string
	Model             string
	Referer           string
	UserAgent         string
	Proxy             string
	RequestTimeout    time.Duration
	StreamIdleTimeout time.Duration
	MaxErrorBody      int64
}

func (c ClientConfig) withDefaults() ClientConfig {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.Referer == "" {
		c.Referer = "https://cursor.com/docs"
	}
	if c.UserAgent == "" {
		c.UserAgent = "sub2api-cursor/1"
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 5 * time.Minute
	}
	if c.StreamIdleTimeout <= 0 {
		c.StreamIdleTimeout = 90 * time.Second
	}
	if c.MaxErrorBody <= 0 {
		c.MaxErrorBody = 8 << 10
	}
	return c
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

type DialogueMessage struct {
	Role       string
	Text       string
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
