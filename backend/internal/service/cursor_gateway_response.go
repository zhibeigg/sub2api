package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
)

func writeCursorJSON(c *gin.Context, protocol cursorpkg.Protocol, responseID, model, previousResponseID string, result cursorCollected) {
	switch protocol {
	case cursorpkg.ProtocolOpenAIChat:
		message := gin.H{"role": "assistant", "content": nullableCursorText(result.CleanText)}
		if result.Reasoning != "" {
			message["reasoning_content"] = result.Reasoning
		}
		if len(result.Actions) > 0 {
			message["tool_calls"] = cursorOpenAIToolCalls(result.Actions)
		}
		c.JSON(http.StatusOK, gin.H{
			"id": responseID, "object": "chat.completion", "created": time.Now().Unix(), "model": model,
			"choices": []gin.H{{"index": 0, "message": message, "finish_reason": cursorOpenAIFinishReason(result)}},
			"usage":   cursorOpenAIUsage(result.Usage),
		})
	case cursorpkg.ProtocolResponses:
		output := make([]gin.H, 0, 2+len(result.Actions))
		if result.Reasoning != "" {
			output = append(output, gin.H{"id": "rs_" + responseID, "type": "reasoning", "summary": []gin.H{{"type": "summary_text", "text": result.Reasoning}}})
		}
		if result.CleanText != "" {
			output = append(output, gin.H{"id": "msg_" + responseID, "type": "message", "role": "assistant", "status": "completed", "content": []gin.H{{"type": "output_text", "text": result.CleanText, "annotations": []any{}}}})
		}
		for _, action := range result.Actions {
			arguments, _ := json.Marshal(action.Arguments)
			output = append(output, gin.H{"id": "fc_" + action.ID, "type": "function_call", "status": "completed", "call_id": action.ID, "name": action.Name, "arguments": string(arguments)})
		}
		c.JSON(http.StatusOK, gin.H{
			"id": responseID, "object": "response", "created_at": time.Now().Unix(), "status": "completed", "model": model,
			"output": output, "previous_response_id": emptyToNil(previousResponseID), "usage": cursorResponsesUsage(result.Usage), "error": nil,
		})
	default:
		content := make([]gin.H, 0, 2+len(result.Actions))
		if result.Reasoning != "" {
			content = append(content, gin.H{"type": "thinking", "thinking": result.Reasoning, "signature": ""})
		}
		if result.CleanText != "" {
			content = append(content, gin.H{"type": "text", "text": result.CleanText})
		}
		for _, action := range result.Actions {
			content = append(content, gin.H{"type": "tool_use", "id": action.ID, "name": action.Name, "input": action.Arguments})
		}
		c.JSON(http.StatusOK, gin.H{
			"id": responseID, "type": "message", "role": "assistant", "model": model, "content": content,
			"stop_reason": cursorAnthropicStopReason(result), "stop_sequence": nil,
			"usage": cursorAnthropicUsage(result.Usage, true),
		})
	}
}

func writeCursorStream(c *gin.Context, protocol cursorpkg.Protocol, responseID, model string, result cursorCollected) error {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	write := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if event != "" {
			if _, err = fmt.Fprintf(c.Writer, "event: %s\n", event); err != nil {
				return err
			}
		}
		if _, err = fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	switch protocol {
	case cursorpkg.ProtocolOpenAIChat:
		base := func(delta gin.H, finish any) gin.H {
			return gin.H{"id": responseID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model, "choices": []gin.H{{"index": 0, "delta": delta, "finish_reason": finish}}}
		}
		if err := write("", base(gin.H{"role": "assistant", "content": ""}, nil)); err != nil {
			return err
		}
		if result.CleanText != "" {
			if err := write("", base(gin.H{"content": result.CleanText}, nil)); err != nil {
				return err
			}
		}
		for index, action := range result.Actions {
			arguments, _ := json.Marshal(action.Arguments)
			delta := gin.H{"tool_calls": []gin.H{{"index": index, "id": action.ID, "type": "function", "function": gin.H{"name": action.Name, "arguments": string(arguments)}}}}
			if err := write("", base(delta, nil)); err != nil {
				return err
			}
		}
		if err := write("", base(gin.H{}, cursorOpenAIFinishReason(result))); err != nil {
			return err
		}
		if _, err := fmt.Fprint(c.Writer, "data: [DONE]\n\n"); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	case cursorpkg.ProtocolResponses:
		sequence := 0
		if err := write("response.created", gin.H{"type": "response.created", "sequence_number": sequence, "response": gin.H{"id": responseID, "object": "response", "created_at": time.Now().Unix(), "status": "in_progress", "model": model, "output": []any{}}}); err != nil {
			return err
		}
		sequence++
		outputIndex := 0
		if result.CleanText != "" {
			itemID := "msg_" + responseID
			if err := write("response.output_item.added", gin.H{"type": "response.output_item.added", "sequence_number": sequence, "output_index": outputIndex, "item": gin.H{"id": itemID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}}); err != nil {
				return err
			}
			sequence++
			if err := write("response.output_text.delta", gin.H{"type": "response.output_text.delta", "sequence_number": sequence, "item_id": itemID, "output_index": outputIndex, "content_index": 0, "delta": result.CleanText}); err != nil {
				return err
			}
			sequence++
			if err := write("response.output_text.done", gin.H{"type": "response.output_text.done", "sequence_number": sequence, "item_id": itemID, "output_index": outputIndex, "content_index": 0, "text": result.CleanText}); err != nil {
				return err
			}
			sequence++
			outputIndex++
		}
		for _, action := range result.Actions {
			arguments, _ := json.Marshal(action.Arguments)
			item := gin.H{"id": "fc_" + action.ID, "type": "function_call", "status": "completed", "call_id": action.ID, "name": action.Name, "arguments": string(arguments)}
			if err := write("response.output_item.added", gin.H{"type": "response.output_item.added", "sequence_number": sequence, "output_index": outputIndex, "item": item}); err != nil {
				return err
			}
			sequence++
			if err := write("response.output_item.done", gin.H{"type": "response.output_item.done", "sequence_number": sequence, "output_index": outputIndex, "item": item}); err != nil {
				return err
			}
			sequence++
			outputIndex++
		}
		return write("response.completed", gin.H{"type": "response.completed", "sequence_number": sequence, "response": gin.H{"id": responseID, "object": "response", "status": "completed", "model": model, "usage": cursorResponsesUsage(result.Usage)}})
	default:
		if err := write("message_start", gin.H{"type": "message_start", "message": gin.H{"id": responseID, "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": cursorAnthropicUsage(result.Usage, false)}}); err != nil {
			return err
		}
		index := 0
		if result.CleanText != "" {
			if err := write("content_block_start", gin.H{"type": "content_block_start", "index": index, "content_block": gin.H{"type": "text", "text": ""}}); err != nil {
				return err
			}
			if err := write("content_block_delta", gin.H{"type": "content_block_delta", "index": index, "delta": gin.H{"type": "text_delta", "text": result.CleanText}}); err != nil {
				return err
			}
			if err := write("content_block_stop", gin.H{"type": "content_block_stop", "index": index}); err != nil {
				return err
			}
			index++
		}
		for _, action := range result.Actions {
			if err := write("content_block_start", gin.H{"type": "content_block_start", "index": index, "content_block": gin.H{"type": "tool_use", "id": action.ID, "name": action.Name, "input": gin.H{}}}); err != nil {
				return err
			}
			arguments, _ := json.Marshal(action.Arguments)
			if err := write("content_block_delta", gin.H{"type": "content_block_delta", "index": index, "delta": gin.H{"type": "input_json_delta", "partial_json": string(arguments)}}); err != nil {
				return err
			}
			if err := write("content_block_stop", gin.H{"type": "content_block_stop", "index": index}); err != nil {
				return err
			}
			index++
		}
		if err := write("message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": cursorAnthropicStopReason(result), "stop_sequence": nil}, "usage": gin.H{"output_tokens": result.Usage.OutputTokens}}); err != nil {
			return err
		}
		return write("message_stop", gin.H{"type": "message_stop"})
	}
}

func cursorOpenAIToolCalls(actions []cursorpkg.Action) []gin.H {
	calls := make([]gin.H, 0, len(actions))
	for _, action := range actions {
		arguments, _ := json.Marshal(action.Arguments)
		calls = append(calls, gin.H{"id": action.ID, "type": "function", "function": gin.H{"name": action.Name, "arguments": string(arguments)}})
	}
	return calls
}

func cursorOpenAIFinishReason(result cursorCollected) string {
	if len(result.Actions) > 0 {
		return "tool_calls"
	}
	if result.FinishReason == "length" || result.FinishReason == "max_tokens" {
		return "length"
	}
	return "stop"
}

func cursorAnthropicStopReason(result cursorCollected) string {
	if len(result.Actions) > 0 {
		return "tool_use"
	}
	if result.FinishReason == "length" || result.FinishReason == "max_tokens" {
		return "max_tokens"
	}
	return "end_turn"
}

func cursorOpenAIUsage(usage cursorpkg.Usage) gin.H {
	inputTokens := cursorTotalInputTokens(usage)
	return gin.H{
		"prompt_tokens":     inputTokens,
		"completion_tokens": usage.OutputTokens,
		"total_tokens":      inputTokens + usage.OutputTokens,
		"prompt_tokens_details": gin.H{
			"cached_tokens":      usage.CacheReadTokens,
			"cache_write_tokens": usage.CacheWriteTokens,
		},
	}
}

func cursorResponsesUsage(usage cursorpkg.Usage) gin.H {
	inputTokens := cursorTotalInputTokens(usage)
	return gin.H{
		"input_tokens":  inputTokens,
		"output_tokens": usage.OutputTokens,
		"total_tokens":  inputTokens + usage.OutputTokens,
		"input_tokens_details": gin.H{
			"cached_tokens":      usage.CacheReadTokens,
			"cache_write_tokens": usage.CacheWriteTokens,
		},
		"output_tokens_details": gin.H{"reasoning_tokens": usage.ReasoningTokens},
	}
}

func cursorAnthropicUsage(usage cursorpkg.Usage, includeOutput bool) gin.H {
	outputTokens := 0
	if includeOutput {
		outputTokens = usage.OutputTokens
	}
	return gin.H{
		"input_tokens":                usage.InputTokens,
		"output_tokens":               outputTokens,
		"cache_creation_input_tokens": usage.CacheWriteTokens,
		"cache_read_input_tokens":     usage.CacheReadTokens,
	}
}

func cursorTotalInputTokens(usage cursorpkg.Usage) int {
	return usage.InputTokens + usage.CacheWriteTokens + usage.CacheReadTokens
}

func nullableCursorText(text string) any {
	if text == "" {
		return nil
	}
	return text
}
func emptyToNil(text string) any {
	if text == "" {
		return nil
	}
	return text
}
