package cursor

import "encoding/json"

const DefaultCloudBaseURL = "https://api.cursor.com"

type APIKeyInfo struct {
	APIKeyName    string `json:"apiKeyName"`
	CreatedAt     string `json:"createdAt"`
	UserID        *int64 `json:"userId,omitempty"`
	UserEmail     string `json:"userEmail,omitempty"`
	UserFirstName string `json:"userFirstName,omitempty"`
	UserLastName  string `json:"userLastName,omitempty"`
}

type CloudModelParameterValue struct {
	Value       string `json:"value"`
	DisplayName string `json:"displayName,omitempty"`
}

type CloudModelParameter struct {
	ID          string                     `json:"id"`
	DisplayName string                     `json:"displayName,omitempty"`
	Values      []CloudModelParameterValue `json:"values,omitempty"`
}

type CloudModelVariant struct {
	Params      []ModelParam `json:"params,omitempty"`
	DisplayName string       `json:"displayName"`
	Description string       `json:"description,omitempty"`
	IsDefault   bool         `json:"isDefault,omitempty"`
}

type CloudModel struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"displayName"`
	Description string                `json:"description,omitempty"`
	Aliases     []string              `json:"aliases,omitempty"`
	Parameters  []CloudModelParameter `json:"parameters,omitempty"`
	Variants    []CloudModelVariant   `json:"variants,omitempty"`
}

type ModelParam struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type ModelRef struct {
	ID     string       `json:"id"`
	Params []ModelParam `json:"params,omitempty"`
}

type CloudPrompt struct {
	Text   string       `json:"text"`
	Images []CloudImage `json:"images,omitempty"`
}

type CloudImage struct {
	Data     string `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

type CreateAgentRequest struct {
	Prompt CloudPrompt `json:"prompt"`
	Model  *ModelRef   `json:"model,omitempty"`
	Name   string      `json:"name,omitempty"`
	Mode   string      `json:"mode,omitempty"`
}

type CloudAgent struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Status      string `json:"status"`
	URL         string `json:"url,omitempty"`
	LatestRunID string `json:"latestRunId,omitempty"`
}

type CloudRun struct {
	ID         string          `json:"id"`
	AgentID    string          `json:"agentId"`
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result,omitempty"`
	DurationMs int             `json:"durationMs,omitempty"`
	Usage      *CloudUsage     `json:"usage,omitempty"`
	Git        map[string]any  `json:"git,omitempty"`
}

type CreateAgentResponse struct {
	Agent CloudAgent `json:"agent"`
	Run   CloudRun   `json:"run"`
}

type CloudUsage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CacheWriteTokens int `json:"cacheWriteTokens"`
	CacheReadTokens  int `json:"cacheReadTokens"`
	TotalTokens      int `json:"totalTokens"`
	ReasoningTokens  int `json:"reasoningTokens,omitempty"`
}

type CloudAPIError struct {
	Error struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		HelpURL  string `json:"helpUrl,omitempty"`
		Provider string `json:"provider,omitempty"`
	} `json:"error"`
}

type CloudSSEEvent struct {
	ID    string
	Event string
	Data  json.RawMessage
}

type CloudRunResultEvent struct {
	RunID      string         `json:"runId"`
	Status     string         `json:"status"`
	Text       string         `json:"text,omitempty"`
	DurationMs int            `json:"durationMs,omitempty"`
	Git        map[string]any `json:"git,omitempty"`
}
