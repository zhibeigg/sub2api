package cursor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type Action struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"tool"`
	Arguments map[string]any `json:"parameters"`
}

func FormatAction(action Action) (string, error) {
	if strings.TrimSpace(action.Name) == "" {
		return "", fmt.Errorf("action name is required")
	}
	if action.Arguments == nil {
		return "", fmt.Errorf("action %q parameters are required", action.Name)
	}
	body, err := json.MarshalIndent(struct {
		Tool       string         `json:"tool"`
		Parameters map[string]any `json:"parameters"`
	}{Tool: action.Name, Parameters: action.Arguments}, "", "  ")
	if err != nil {
		return "", err
	}
	return "```json action\n" + string(body) + "\n```", nil
}

func ToolInstructions(tools []ToolDefinition, choice ToolChoice) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	type toolWire struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	wire := make([]toolWire, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return "", fmt.Errorf("tool name is required")
		}
		wire = append(wire, toolWire(tool))
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	constraint := ""
	switch choice.Mode {
	case "any", "required":
		constraint = " At least one action is required."
	case "tool", "function":
		if choice.Name != "" {
			constraint = fmt.Sprintf(" You must call only the requested tool %q for the required action.", choice.Name)
		}
	}
	return "Available tools are defined by this JSON: " + string(encoded) + "\n" +
		"When a tool is needed, emit exactly one or more fenced blocks of the form:\n" +
		"```json action\n{\"tool\":\"TOOL_NAME\",\"parameters\":{...}}\n```\n" +
		"Use only supplied argument values; never invent fallback parameters." + constraint, nil
}

func ParseActions(text string) (actions []Action, cleanText string, err error) {
	var removals [][2]int
	searchFrom := 0
	for searchFrom < len(text) {
		rel := strings.Index(text[searchFrom:], "```json")
		if rel < 0 {
			break
		}
		start := searchFrom + rel
		contentStart := start + len("```json")
		for contentStart < len(text) && (text[contentStart] == ' ' || text[contentStart] == '\t') {
			contentStart++
		}
		if strings.HasPrefix(text[contentStart:], "action") {
			contentStart += len("action")
		}
		for contentStart < len(text) && (text[contentStart] == '\r' || text[contentStart] == '\n' || text[contentStart] == ' ' || text[contentStart] == '\t') {
			contentStart++
		}
		closeAt := findFenceOutsideJSONString(text, contentStart)
		end := len(text)
		if closeAt >= 0 {
			end = closeAt + 3
		}
		candidateEnd := end
		if closeAt >= 0 {
			candidateEnd = closeAt
		}
		candidate := strings.TrimSpace(text[contentStart:candidateEnd])
		if looksLikeAction(candidate) {
			action, parseErr := parseActionJSON(candidate)
			if parseErr != nil {
				return nil, text, fmt.Errorf("parse action block: %w", parseErr)
			}
			actions = append(actions, action)
			removals = append(removals, [2]int{start, end})
		}
		if closeAt < 0 {
			break
		}
		searchFrom = end
	}
	cleanText = text
	for i := len(removals) - 1; i >= 0; i-- {
		cleanText = cleanText[:removals[i][0]] + cleanText[removals[i][1]:]
	}
	return actions, strings.TrimSpace(cleanText), nil
}

func parseActionJSON(input string) (Action, error) {
	fixed := repairJSON(input)
	var raw struct {
		Tool       string          `json:"tool"`
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
		Arguments  json.RawMessage `json:"arguments"`
		Input      json.RawMessage `json:"input"`
	}
	decoder := json.NewDecoder(strings.NewReader(fixed))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return Action{}, err
	}
	name := firstNonEmpty(raw.Tool, raw.Name)
	if strings.TrimSpace(name) == "" {
		return Action{}, fmt.Errorf("tool name is required")
	}
	argsRaw := raw.Parameters
	if len(argsRaw) == 0 {
		argsRaw = raw.Arguments
	}
	if len(argsRaw) == 0 {
		argsRaw = raw.Input
	}
	if len(argsRaw) == 0 || string(argsRaw) == "null" {
		return Action{}, fmt.Errorf("tool %q parameters are required", name)
	}
	var args map[string]any
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return Action{}, fmt.Errorf("tool %q parameters: %w", name, err)
	}
	return Action{Name: name, Arguments: args}, nil
}

func repairJSON(input string) string {
	var out strings.Builder
	out.Grow(len(input) + 8)
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inString {
			if escaped {
				_ = out.WriteByte(ch)
				escaped = false
				continue
			}
			if ch == '\\' {
				_ = out.WriteByte(ch)
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
				_ = out.WriteByte(ch)
				continue
			}
			switch ch {
			case '\n':
				_, _ = out.WriteString(`\n`)
			case '\r':
				_, _ = out.WriteString(`\r`)
			case '\t':
				_, _ = out.WriteString(`\t`)
			default:
				_ = out.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
			_ = out.WriteByte(ch)
		case '{':
			stack = append(stack, '}')
			_ = out.WriteByte(ch)
		case '[':
			stack = append(stack, ']')
			_ = out.WriteByte(ch)
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == ch {
				stack = stack[:len(stack)-1]
			}
			_ = out.WriteByte(ch)
		default:
			_ = out.WriteByte(ch)
		}
	}
	if inString {
		_ = out.WriteByte('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		_ = out.WriteByte(stack[i])
	}
	fixed := bytes.TrimSpace([]byte(out.String()))
	fixed = bytes.ReplaceAll(fixed, []byte(",}"), []byte("}"))
	fixed = bytes.ReplaceAll(fixed, []byte(",]"), []byte("]"))
	return string(fixed)
}

func findFenceOutsideJSONString(text string, start int) int {
	inString := false
	escaped := false
	for i := start; i+2 < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if text[i:i+3] == "```" {
			return i
		}
	}
	return -1
}

func looksLikeAction(candidate string) bool {
	return strings.Contains(candidate, `"tool"`) || strings.Contains(candidate, `"name"`)
}
