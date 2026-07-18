package cursor

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const (
	AgentRunPath             = "/agent.v1.AgentService/Run"
	AgentGetUsableModelsPath = "/agent.v1.AgentService/GetUsableModels"
)

type AgentMode uint64

const (
	AgentModeUnspecified AgentMode = 0
	AgentModeAgent       AgentMode = 1
	AgentModeAsk         AgentMode = 2
	AgentModePlan        AgentMode = 3
	AgentModeDebug       AgentMode = 4
)

type AgentClientConfig struct {
	IDEClientConfig
	QueueSize         int
	HeartbeatInterval time.Duration
}

type AgentRunOptions struct {
	Model                 string
	DisplayModel          string
	ConversationID        string
	ConversationGroupID   string
	Mode                  AgentMode
	MaxMode               bool
	WorkspacePaths        []string
	ProjectFolder         string
	Shell                 string
	CustomSystemPrompt    string
	ExcludeWorkspace      bool
	SuggestNextPrompt     bool
	ClientSupportsImages  bool
	ClientSupportsSend    bool
	ConversationState     *AgentConversationState
	Resume                bool
	RequestContext        AgentRequestContext
	MCPProviderIdentifier string
}

type AgentConversationState struct {
	RootPromptMessagesJSON [][]byte
	Turns                  [][]byte
	Todos                  [][]byte
	PendingToolCalls       []string
	Summary                []byte
	Plan                   []byte
	PreviousWorkspaceURIs  []string
	Mode                   AgentMode
	ReadPaths              []string
	ActiveBranchName       string
	AgentType              string
	StartedAt              time.Time
	StartedTimeZone        string
}

type AgentRequestContext struct {
	OSVersion        string
	WorkspacePaths   []string
	Shell            string
	TimeZone         string
	ProjectFolder    string
	FileContents     map[string]string
	WebSearchEnabled bool
	WebFetchEnabled  bool
	MCPInfoComplete  bool
	EnvInfoComplete  bool
	Extra            map[string]string
}

type AgentModel struct {
	ID               string
	DisplayID        string
	DisplayName      string
	ShortDisplayName string
	Aliases          []string
	SupportsThinking bool
	SupportsMaxMode  bool
}

type AgentEventType string

const (
	AgentEventText               AgentEventType = "text"
	AgentEventThinking           AgentEventType = "thinking"
	AgentEventToolStarted        AgentEventType = "tool_started"
	AgentEventToolPartial        AgentEventType = "tool_partial"
	AgentEventToolCompleted      AgentEventType = "tool_completed"
	AgentEventTurnEnded          AgentEventType = "turn_ended"
	AgentEventUsage              AgentEventType = "usage"
	AgentEventCheckpoint         AgentEventType = "checkpoint"
	AgentEventKVGet              AgentEventType = "kv_get"
	AgentEventKVSet              AgentEventType = "kv_set"
	AgentEventExecMCP            AgentEventType = "exec_mcp"
	AgentEventExecShell          AgentEventType = "exec_shell"
	AgentEventExecRequestContext AgentEventType = "exec_request_context"
	AgentEventUnsupportedExec    AgentEventType = "unsupported_exec"
	AgentEventHeartbeat          AgentEventType = "heartbeat"
	AgentEventStepStarted        AgentEventType = "step_started"
	AgentEventStepCompleted      AgentEventType = "step_completed"
	AgentEventFinish             AgentEventType = "finish"
	AgentEventError              AgentEventType = "error"
)

type AgentKVRequest struct {
	ID       uint64
	BlobID   []byte
	BlobData []byte
	Metadata []byte
}

type AgentUnsupportedExec struct {
	ID      uint64
	ExecID  string
	Field   int
	Payload []byte
}

type AgentEvent struct {
	Type           AgentEventType
	Text           string
	Thinking       string
	Tool           *Action
	CallID         string
	ArgumentsDelta string
	Usage          *Usage
	Checkpoint     *AgentConversationState
	CheckpointRaw  []byte
	KV             *AgentKVRequest
	ExecMCP        *Action
	ExecShell      *Action
	ExecRequestID  uint64
	ExecID         string
	ExecField      int
	Unsupported    *AgentUnsupportedExec
	StepID         uint64
	StepDuration   time.Duration
	FinishReason   string
	Error          *IDEStreamError

	terminal bool
}

type agentToolAccumulator struct {
	id      string
	name    string
	rawArgs string
	started bool
}

type AgentStream struct {
	response *http.Response
	decoder  *ConnectDecoder
	ctx      context.Context
	pending  []AgentEvent
	tools    map[string]*agentToolAccumulator

	send       chan []byte
	writerErr  chan error
	cancel     func()
	closeSend  func() error
	sendMu     sync.RWMutex
	sendClosed bool
	closeOnce  sync.Once
	closeErr   error

	finished     bool
	sawEndStream bool
	maxToolBytes int
	toolBytes    int
}
