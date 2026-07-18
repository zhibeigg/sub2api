package cursor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode"
)

func BuildPayload(dialogue *Dialogue, options BuildOptions) (*Request, error) {
	if _, err := validateDialogue(dialogue); err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, badRequest("build payload", fmt.Errorf("model is required"))
	}
	messages := make([]Message, 0, len(dialogue.Messages)+1)
	protected := 0
	var preamble []string
	if strings.TrimSpace(dialogue.System) != "" {
		preamble = append(preamble, "System instructions:\n"+strings.TrimSpace(dialogue.System))
	}
	toolText, err := ToolInstructions(dialogue.Tools, dialogue.ToolChoice)
	if err != nil {
		return nil, badRequest("build tool instructions", err)
	}
	if toolText != "" {
		preamble = append(preamble, toolText)
	}
	if len(preamble) > 0 {
		messages = append(messages, newMessage("user", strings.Join(preamble, "\n\n---\n\n"), 0))
		protected = 1
	}
	for i, msg := range dialogue.Messages {
		role := msg.Role
		text := msg.Text
		switch role {
		case "user", "assistant":
		case "tool":
			role = "user"
			label := "Tool output"
			if msg.IsError {
				label = "Tool error"
			}
			if msg.ToolCallID != "" {
				label += " for " + msg.ToolCallID
			}
			text = label + ":\n" + text
		default:
			return nil, badRequest("build payload", fmt.Errorf("unsupported dialogue role %q", msg.Role))
		}
		for _, call := range msg.ToolCalls {
			formatted, formatErr := FormatAction(call)
			if formatErr != nil {
				return nil, badRequest("build payload", formatErr)
			}
			if text != "" {
				text += "\n\n"
			}
			text += formatted
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		messages = append(messages, newMessage(role, text, i+protected+1))
	}
	messages = TrimHistory(messages, protected, options.MaxHistoryMessages, options.MaxHistoryTokens, options.HiddenOverheadTokens)
	if len(messages) == 0 || (len(messages) == protected && len(dialogue.Messages) > 0) {
		return nil, badRequest("build payload", fmt.Errorf("history limits removed all conversation messages"))
	}
	conversationID := options.ConversationID
	if conversationID == "" {
		conversationID = deriveConversationID(dialogue)
	}
	return &Request{Model: options.Model, ID: conversationID, Messages: messages, Trigger: "submit-message"}, nil
}

// RenderAgentPrompt converts the compatibility payload into a single Cloud
// Agent prompt. Cloud Agents is task-oriented rather than a raw chat API, so
// the wrapper explicitly asks for a conversational answer and forbids unrelated
// workspace changes.
func RenderAgentPrompt(request *Request) string {
	if request == nil {
		return ""
	}
	var builder strings.Builder
	_, _ = builder.WriteString("You are serving a chat-completion compatibility request through Cursor Cloud Agents. Answer the final user request directly. Do not create, edit, delete, commit, or push repository files unless the user explicitly asks for those actions in the transcript. Preserve requested tool-call JSON fences exactly.\n\nConversation transcript:\n")
	for _, message := range request.Messages {
		role := strings.ToUpper(strings.TrimSpace(message.Role))
		if role == "" {
			role = "USER"
		}
		_, _ = builder.WriteString("\n[")
		_, _ = builder.WriteString(role)
		_, _ = builder.WriteString("]\n")
		for _, part := range message.Parts {
			_, _ = builder.WriteString(part.Text)
		}
		_ = builder.WriteByte('\n')
	}
	_, _ = builder.WriteString("\n[ASSISTANT]\n")
	return builder.String()
}

func newMessage(role, text string, index int) Message {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", index, role, text)))
	return Message{Parts: []Part{{Type: "text", Text: text}}, ID: hex.EncodeToString(hash[:8]), Role: role}
}

func deriveConversationID(dialogue *Dialogue) string {
	var anchor string
	for _, msg := range dialogue.Messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Text) != "" {
			anchor = msg.Text
			break
		}
	}
	hash := sha256.Sum256([]byte(dialogue.System + "\x00" + anchor))
	return hex.EncodeToString(hash[:8])
}

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	var letters, digits, symbols, nonASCII int
	for _, r := range text {
		switch {
		case r > unicode.MaxASCII:
			nonASCII++
		case unicode.IsDigit(r):
			digits++
		case unicode.IsLetter(r) || unicode.IsSpace(r):
			letters++
		default:
			symbols++
		}
	}
	result := int(math.Ceil(float64(letters)/4.5 + float64(digits)/2 + float64(symbols)/1.5 + float64(nonASCII)/1.5))
	if result == 0 {
		return 1
	}
	return result
}

func EstimateMessageTokens(message Message) int {
	total := 4
	for _, part := range message.Parts {
		total += EstimateTokens(part.Text)
	}
	return total
}

func TrimHistory(messages []Message, protected, maxMessages, maxTokens, overhead int) []Message {
	if protected < 0 {
		protected = 0
	}
	if protected > len(messages) {
		protected = len(messages)
	}
	prefix := append([]Message(nil), messages[:protected]...)
	history := append([]Message(nil), messages[protected:]...)
	if maxMessages > 0 && len(history) > maxMessages {
		history = history[len(history)-maxMessages:]
	}
	if maxTokens > 0 {
		// maxTokens is the conversation-history budget. Protected preamble
		// messages contain fixed system instructions and tool schemas, which can
		// be larger than the configured history budget for agent clients such as
		// NarraFork. Charging that fixed overhead against history would evict all
		// prior turns even though the upstream request must include the preamble
		// regardless.
		budget := maxTokens - overhead
		if budget < 0 {
			budget = 0
		}
		used := 0
		keep := len(history)
		for i := len(history) - 1; i >= 0; i-- {
			cost := EstimateMessageTokens(history[i])
			if used+cost > budget && i < len(history)-1 {
				break
			}
			used += cost
			keep = i
		}
		history = history[keep:]
	}
	for len(history) > 0 && history[0].Role == "assistant" {
		history = history[1:]
	}
	return append(prefix, history...)
}
