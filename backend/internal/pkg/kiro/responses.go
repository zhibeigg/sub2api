package kiro

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultResponsesModel is used when a Responses request omits the model.
const DefaultResponsesModel = "claude-sonnet-4.5"

// -----------------------------------------------------------------------------
// Types (ported from Kiro-Go proxy/responses_types.go)
// -----------------------------------------------------------------------------

// ResponsesRequest is an OpenAI Responses API request.
type ResponsesRequest struct {
	Model              string            `json:"model"`
	Input              json.RawMessage   `json:"input"`
	Instructions       string            `json:"instructions,omitempty"`
	Stream             bool              `json:"stream,omitempty"`
	Tools              []OpenAITool      `json:"tools,omitempty"`
	ToolChoice         json.RawMessage   `json:"tool_choice,omitempty"`
	PreviousResponseID string            `json:"previous_response_id,omitempty"`
	Store              *bool             `json:"store,omitempty"`
	Temperature        *float64          `json:"temperature,omitempty"`
	MaxOutputTokens    *int              `json:"max_output_tokens,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// ResponsesObject is an OpenAI Responses API response object.
type ResponsesObject struct {
	ID                 string               `json:"id"`
	Object             string               `json:"object"`
	CreatedAt          int64                `json:"created_at"`
	Status             string               `json:"status"`
	Model              string               `json:"model"`
	Output             []ResponseOutputItem `json:"output"`
	Usage              ResponsesUsage       `json:"usage"`
	PreviousResponseID string               `json:"previous_response_id,omitempty"`
	Metadata           map[string]string    `json:"metadata,omitempty"`
	Error              *ResponsesError      `json:"error,omitempty"`
	Instructions       string               `json:"instructions,omitempty"`
	StoredInput        json.RawMessage      `json:"-"`
	StoredAt           int64                `json:"stored_at,omitempty"`
}

type ResponseOutputItem struct {
	ID        string                `json:"id"`
	Type      string                `json:"type"`
	Role      string                `json:"role,omitempty"`
	Status    string                `json:"status,omitempty"`
	Content   []ResponseContentPart `json:"content,omitempty"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments string                `json:"arguments,omitempty"`
}

type ResponseContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ResponsesError struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// -----------------------------------------------------------------------------
// Input parsing (ported from proxy/responses_input.go)
// -----------------------------------------------------------------------------

// ParseResponsesInput normalizes the polymorphic Responses `input` field into a
// sequence of OpenAI messages.
func ParseResponsesInput(raw json.RawMessage) ([]OpenAIMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("invalid input string: %w", err)
		}
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		return []OpenAIMessage{{Role: "user", Content: s}}, nil
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("invalid input array: %w", err)
		}
		return convertResponsesInputItems(items)
	case '{':
		return convertResponsesInputItems([]json.RawMessage{raw})
	default:
		return nil, fmt.Errorf("unsupported input shape")
	}
}

func convertResponsesInputItems(items []json.RawMessage) ([]OpenAIMessage, error) {
	messages := make([]OpenAIMessage, 0, len(items))
	var pendingUserParts []any

	flushPendingUser := func() {
		if len(pendingUserParts) == 0 {
			return
		}
		messages = append(messages, OpenAIMessage{Role: "user", Content: pendingUserParts})
		pendingUserParts = nil
	}

	for _, item := range items {
		var obj map[string]any
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		typ, _ := obj["type"].(string)
		role, _ := obj["role"].(string)

		switch {
		case typ == "message" || (typ == "" && role != ""):
			flushPendingUser()
			if msg := buildMessageFromInputItem(obj, role); msg != nil {
				messages = append(messages, *msg)
			}
		case typ == "function_call_output" || typ == "tool_result":
			flushPendingUser()
			callID, _ := obj["call_id"].(string)
			if callID == "" {
				callID, _ = obj["tool_call_id"].(string)
			}
			out := stringifyArbitrary(obj["output"])
			if out == "" {
				out = stringifyArbitrary(obj["content"])
			}
			messages = append(messages, OpenAIMessage{Role: "tool", Content: out, ToolCallID: callID})
		case typ == "function_call":
			flushPendingUser()
			tc := ToolCall{ID: stringField(obj, "call_id", "id"), Type: "function"}
			tc.Function.Name, _ = obj["name"].(string)
			tc.Function.Arguments = stringifyArbitrary(obj["arguments"])
			if n := len(messages); n > 0 &&
				messages[n-1].Role == "assistant" &&
				len(messages[n-1].ToolCalls) > 0 &&
				strings.TrimSpace(extractOpenAIMessageText(messages[n-1].Content)) == "" {
				messages[n-1].ToolCalls = append(messages[n-1].ToolCalls, tc)
			} else {
				messages = append(messages, OpenAIMessage{Role: "assistant", Content: "", ToolCalls: []ToolCall{tc}})
			}
		case typ == "input_text" || typ == "text":
			if text, _ := obj["text"].(string); text != "" {
				pendingUserParts = append(pendingUserParts, map[string]any{"type": "input_text", "text": text})
			}
		case typ == "input_image" || typ == "image" || typ == "image_url":
			pendingUserParts = append(pendingUserParts, obj)
		case typ == "output_text":
			flushPendingUser()
			if text, _ := obj["text"].(string); text != "" {
				messages = append(messages, OpenAIMessage{Role: "assistant", Content: text})
			}
		default:
			if role != "" {
				flushPendingUser()
				if msg := buildMessageFromInputItem(obj, role); msg != nil {
					messages = append(messages, *msg)
				}
			}
		}
	}
	flushPendingUser()
	return messages, nil
}

func buildMessageFromInputItem(obj map[string]any, role string) *OpenAIMessage {
	if role == "" {
		role = "user"
	}
	if content, ok := obj["content"]; ok {
		switch v := content.(type) {
		case string:
			return &OpenAIMessage{Role: role, Content: v}
		case []any:
			parts := make([]any, 0, len(v))
			var textOnly strings.Builder
			anyNonText := false
			for _, p := range v {
				part, ok := p.(map[string]any)
				if !ok {
					continue
				}
				ptype, _ := part["type"].(string)
				switch ptype {
				case "input_text", "text", "output_text":
					if t, ok := part["text"].(string); ok {
						_, _ = textOnly.WriteString(t)
						parts = append(parts, map[string]any{"type": "input_text", "text": t})
					}
				case "input_image", "image", "image_url":
					anyNonText = true
					parts = append(parts, part)
				default:
					if t, ok := part["text"].(string); ok && t != "" {
						_, _ = textOnly.WriteString(t)
						parts = append(parts, map[string]any{"type": "input_text", "text": t})
					}
				}
			}
			if !anyNonText {
				return &OpenAIMessage{Role: role, Content: textOnly.String()}
			}
			return &OpenAIMessage{Role: role, Content: parts}
		case map[string]any:
			return buildMessageFromInputItem(v, role)
		}
	}
	if text, ok := obj["text"].(string); ok && text != "" {
		return &OpenAIMessage{Role: role, Content: text}
	}
	return nil
}

func stringifyArbitrary(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func stringField(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := obj[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// -----------------------------------------------------------------------------
// History expansion + in-memory store (ported from responses_history/store.go)
// -----------------------------------------------------------------------------

const maxResponsesHistoryDepth = 64

// ResponsesStore persists Responses objects for previous_response_id chaining.
// This uses an in-memory TTL store (per-process) rather than the filesystem,
// keeping the gateway stateless-friendly; entries expire after the TTL.
type ResponsesStore struct {
	mu      sync.RWMutex
	entries map[string]*storedResponse
	ttl     time.Duration
}

type storedResponse struct {
	obj      *ResponsesObject
	storedAt time.Time
}

// NewResponsesStore creates a store with the given TTL (defaults to 24h).
func NewResponsesStore(ttl time.Duration) *ResponsesStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &ResponsesStore{entries: make(map[string]*storedResponse), ttl: ttl}
}

func (s *ResponsesStore) Save(obj *ResponsesObject) {
	if s == nil || obj == nil || obj.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[obj.ID] = &storedResponse{obj: obj, storedAt: time.Now()}
	s.reapLocked()
}

func (s *ResponsesStore) Load(id string) (*ResponsesObject, bool) {
	if s == nil || id == "" {
		return nil, false
	}
	s.mu.RLock()
	entry, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(entry.storedAt) > s.ttl {
		s.mu.Lock()
		delete(s.entries, id)
		s.mu.Unlock()
		return nil, false
	}
	return entry.obj, true
}

func (s *ResponsesStore) reapLocked() {
	now := time.Now()
	for id, e := range s.entries {
		if now.Sub(e.storedAt) > s.ttl {
			delete(s.entries, id)
		}
	}
}

// ExpandPreviousResponseHistory rebuilds prior conversation messages from the
// previous_response_id chain (oldest-first).
func (s *ResponsesStore) ExpandPreviousResponseHistory(prev *ResponsesObject) []OpenAIMessage {
	if prev == nil {
		return nil
	}
	chain := s.collectAncestorChain(prev)
	messages := make([]OpenAIMessage, 0)
	for _, node := range chain {
		if node.Instructions != "" {
			messages = append(messages, OpenAIMessage{Role: "system", Content: node.Instructions})
		}
		if prior, err := ParseResponsesInput(node.StoredInput); err == nil {
			messages = append(messages, prior...)
		}
		messages = append(messages, outputToMessages(node.Output)...)
	}
	return messages
}

func (s *ResponsesStore) collectAncestorChain(prev *ResponsesObject) []*ResponsesObject {
	stack := []*ResponsesObject{prev}
	visited := map[string]bool{prev.ID: true}
	cursor := prev
	for depth := 0; depth < maxResponsesHistoryDepth; depth++ {
		if cursor.PreviousResponseID == "" || visited[cursor.PreviousResponseID] {
			break
		}
		ancestor, ok := s.Load(cursor.PreviousResponseID)
		if !ok || ancestor == nil {
			break
		}
		visited[ancestor.ID] = true
		stack = append(stack, ancestor)
		cursor = ancestor
	}
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack
}

func outputToMessages(items []ResponseOutputItem) []OpenAIMessage {
	if len(items) == 0 {
		return nil
	}
	out := make([]OpenAIMessage, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case "message":
			text := joinTextParts(item.Content)
			role := item.Role
			if role == "" {
				role = "assistant"
			}
			if text == "" && role == "assistant" {
				continue
			}
			out = append(out, OpenAIMessage{Role: role, Content: text})
		case "function_call":
			tc := ToolCall{ID: item.CallID, Type: "function"}
			if tc.ID == "" {
				tc.ID = item.ID
			}
			tc.Function.Name = item.Name
			tc.Function.Arguments = item.Arguments
			out = append(out, OpenAIMessage{Role: "assistant", Content: "", ToolCalls: []ToolCall{tc}})
		}
	}
	return out
}

func joinTextParts(parts []ResponseContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "output_text" || p.Type == "text" || p.Type == "input_text" {
			_, _ = b.WriteString(p.Text)
		}
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// ID generation + response object builder
// -----------------------------------------------------------------------------

// GenerateResponseID returns a resp_ prefixed id.
func GenerateResponseID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	return "resp_" + hex.EncodeToString(buf) + fmt.Sprintf("%08x", time.Now().Unix()&0xffffffff)
}

func generateOutputItemID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// BuildResponsesObject assembles a completed Responses object from content and
// tool calls.
func BuildResponsesObject(id, model, content string, toolUses []KiroToolUse, inputTokens, outputTokens int, req *ResponsesRequest) *ResponsesObject {
	output := make([]ResponseOutputItem, 0, 1+len(toolUses))
	if strings.TrimSpace(content) != "" {
		output = append(output, ResponseOutputItem{
			ID:      generateOutputItemID("msg"),
			Type:    "message",
			Role:    "assistant",
			Status:  "completed",
			Content: []ResponseContentPart{{Type: "output_text", Text: content}},
		})
	}
	for _, tu := range toolUses {
		args, _ := json.Marshal(tu.Input)
		output = append(output, ResponseOutputItem{
			ID:        generateOutputItemID("fc"),
			Type:      "function_call",
			Status:    "completed",
			CallID:    tu.ToolUseID,
			Name:      tu.Name,
			Arguments: string(args),
		})
	}
	if len(output) == 0 {
		output = append(output, ResponseOutputItem{
			ID:      generateOutputItemID("msg"),
			Type:    "message",
			Role:    "assistant",
			Status:  "completed",
			Content: []ResponseContentPart{{Type: "output_text", Text: ""}},
		})
	}
	obj := &ResponsesObject{
		ID:        id,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     model,
		Output:    output,
		Usage:     ResponsesUsage{InputTokens: inputTokens, OutputTokens: outputTokens, TotalTokens: inputTokens + outputTokens},
	}
	if req != nil {
		obj.PreviousResponseID = req.PreviousResponseID
		obj.Metadata = req.Metadata
	}
	return obj
}

// BuildResponsesMessages assembles the final OpenAI message list for a Responses
// request: expanded history + optional instructions + current input.
func (s *ResponsesStore) BuildResponsesMessages(req *ResponsesRequest) ([]OpenAIMessage, error) {
	var history []OpenAIMessage
	if req.PreviousResponseID != "" {
		if prev, ok := s.Load(req.PreviousResponseID); ok {
			history = s.ExpandPreviousResponseHistory(prev)
		}
	}
	inputMessages, err := ParseResponsesInput(req.Input)
	if err != nil {
		return nil, err
	}
	final := make([]OpenAIMessage, 0, len(history)+len(inputMessages)+1)
	final = append(final, history...)
	if strings.TrimSpace(req.Instructions) != "" {
		final = append(final, OpenAIMessage{Role: "system", Content: req.Instructions})
	}
	final = append(final, inputMessages...)
	return final, nil
}
