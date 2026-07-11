package cursor

import (
	"encoding/json"
	"net/http"
	"time"
)

const (
	DefaultIDEBaseURL = "https://api2.cursor.sh"
	IDEChatPath       = "/aiserver.v1.ChatService/StreamUnifiedChatWithTools"
	IDEModelsPath     = "/aiserver.v1.AiService/AvailableModels"
)

type IDEMode string

const (
	IDEModeAsk   IDEMode = "ask"
	IDEModeAgent IDEMode = "agent"
)

type IDECredential struct {
	AccessToken string
	MachineID   string
}

type IDEClientConfig struct {
	BaseURL                string
	ClientVersion          string
	ClientOS               string
	ClientArch             string
	ClientOSVersion        string
	ConfigVersion          string
	Timezone               string
	GhostMode              bool
	NewOnboardingCompleted bool
	Now                    func() time.Time
	UUID                   func() string
	MaxFrameSize           int
	MaxBufferedBytes       int
	MaxErrorBody           int
}

type IDEChatOptions struct {
	Model          string
	ConversationID string
	Mode           IDEMode
	Thinking       bool
	Compress       bool
}

type IDEEventType string

const (
	IDEEventText     IDEEventType = "text"
	IDEEventThinking IDEEventType = "thinking"
	IDEEventToolCall IDEEventType = "tool_call"
	IDEEventUsage    IDEEventType = "usage"
	IDEEventFinish   IDEEventType = "finish"
	IDEEventError    IDEEventType = "error"
)

type IDEStreamError struct {
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
}

type IDEEvent struct {
	Type         IDEEventType
	Text         string
	Thinking     string
	ToolCall     *Action
	Usage        *Usage
	FinishReason string
	Error        *IDEStreamError

	toolCallRawArgs string
	toolCallLast    bool
	toolCallHasLast bool
}

type ideToolAccumulator struct {
	id      string
	name    string
	rawArgs string
}

type IDEEventStream struct {
	response     *http.Response
	decoder      *ConnectDecoder
	pending      []IDEEvent
	toolCalls    map[string]*ideToolAccumulator
	toolBytes    int
	finishReason string
	sawEndStream bool
	finished     bool
	maxToolBytes int
	maxToolCalls int
}
