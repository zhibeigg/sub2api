package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
)

type cursorIDEStreamWriter struct {
	c          *gin.Context
	protocol   cursorpkg.Protocol
	responseID string
	model      string
	createdAt  int64
	sequence   int
	started    bool

	anthropicIndex int
	anthropicOpen  bool
	anthropicKind  string

	responsesOutputIndex int
	responsesTextOpen    bool
	responsesTextItemID  string
	responsesTextIndex   int
	responsesReasonOpen  bool
	responsesReasonID    string
	responsesReasonIndex int

	openAIToolIndex int
}

func newCursorIDEStreamWriter(c *gin.Context, protocol cursorpkg.Protocol, responseID, model string) *cursorIDEStreamWriter {
	return &cursorIDEStreamWriter{c: c, protocol: protocol, responseID: responseID, model: model, createdAt: time.Now().Unix()}
}

func (w *cursorIDEStreamWriter) Start(usage cursorpkg.Usage) error {
	if w.started {
		return nil
	}
	w.c.Header("Content-Type", "text/event-stream")
	w.c.Header("Cache-Control", "no-cache")
	w.c.Header("Connection", "keep-alive")
	w.c.Header("X-Accel-Buffering", "no")
	w.c.Status(http.StatusOK)
	w.started = true

	switch w.protocol {
	case cursorpkg.ProtocolOpenAIChat:
		return w.write("", w.openAIChunk(gin.H{"role": "assistant", "content": ""}, nil, nil))
	case cursorpkg.ProtocolResponses:
		return w.writeResponses("response.created", gin.H{
			"type":     "response.created",
			"response": gin.H{"id": w.responseID, "object": "response", "created_at": w.createdAt, "status": "in_progress", "model": w.model, "output": []any{}},
		})
	default:
		return w.write("message_start", gin.H{
			"type": "message_start",
			"message": gin.H{
				"id": w.responseID, "type": "message", "role": "assistant", "model": w.model,
				"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
				"usage": cursorAnthropicUsage(usage, false),
			},
		})
	}
}

func (w *cursorIDEStreamWriter) WriteThinking(delta string) error {
	if delta == "" {
		return nil
	}
	if err := w.Start(cursorpkg.Usage{}); err != nil {
		return err
	}
	switch w.protocol {
	case cursorpkg.ProtocolOpenAIChat:
		return w.write("", w.openAIChunk(gin.H{"reasoning_content": delta}, nil, nil))
	case cursorpkg.ProtocolResponses:
		if err := w.ensureResponsesReasoning(); err != nil {
			return err
		}
		return w.writeResponses("response.reasoning_summary_text.delta", gin.H{
			"type": "response.reasoning_summary_text.delta", "item_id": w.responsesReasonID,
			"output_index": w.responsesReasonIndex, "summary_index": 0, "delta": delta,
		})
	default:
		if err := w.ensureAnthropicBlock("thinking"); err != nil {
			return err
		}
		return w.write("content_block_delta", gin.H{
			"type": "content_block_delta", "index": w.anthropicIndex,
			"delta": gin.H{"type": "thinking_delta", "thinking": delta},
		})
	}
}

func (w *cursorIDEStreamWriter) WriteText(delta string) error {
	if delta == "" {
		return nil
	}
	if err := w.Start(cursorpkg.Usage{}); err != nil {
		return err
	}
	switch w.protocol {
	case cursorpkg.ProtocolOpenAIChat:
		return w.write("", w.openAIChunk(gin.H{"content": delta}, nil, nil))
	case cursorpkg.ProtocolResponses:
		if err := w.ensureResponsesText(); err != nil {
			return err
		}
		return w.writeResponses("response.output_text.delta", gin.H{
			"type": "response.output_text.delta", "item_id": w.responsesTextItemID,
			"output_index": w.responsesTextIndex, "content_index": 0, "delta": delta,
		})
	default:
		if err := w.ensureAnthropicBlock("text"); err != nil {
			return err
		}
		return w.write("content_block_delta", gin.H{
			"type": "content_block_delta", "index": w.anthropicIndex,
			"delta": gin.H{"type": "text_delta", "text": delta},
		})
	}
}

func (w *cursorIDEStreamWriter) WriteToolCall(action cursorpkg.Action) error {
	if err := w.Start(cursorpkg.Usage{}); err != nil {
		return err
	}
	arguments, err := json.Marshal(action.Arguments)
	if err != nil {
		return err
	}
	switch w.protocol {
	case cursorpkg.ProtocolOpenAIChat:
		index := w.openAIToolIndex
		w.openAIToolIndex++
		delta := gin.H{"tool_calls": []gin.H{{
			"index": index, "id": action.ID, "type": "function",
			"function": gin.H{"name": action.Name, "arguments": string(arguments)},
		}}}
		return w.write("", w.openAIChunk(delta, nil, nil))
	case cursorpkg.ProtocolResponses:
		itemID := "fc_" + action.ID
		index := w.responsesOutputIndex
		w.responsesOutputIndex++
		pending := gin.H{"id": itemID, "type": "function_call", "status": "in_progress", "call_id": action.ID, "name": action.Name, "arguments": ""}
		if err := w.writeResponses("response.output_item.added", gin.H{"type": "response.output_item.added", "output_index": index, "item": pending}); err != nil {
			return err
		}
		if err := w.writeResponses("response.function_call_arguments.delta", gin.H{"type": "response.function_call_arguments.delta", "item_id": itemID, "output_index": index, "delta": string(arguments)}); err != nil {
			return err
		}
		if err := w.writeResponses("response.function_call_arguments.done", gin.H{"type": "response.function_call_arguments.done", "item_id": itemID, "output_index": index, "arguments": string(arguments)}); err != nil {
			return err
		}
		completed := gin.H{"id": itemID, "type": "function_call", "status": "completed", "call_id": action.ID, "name": action.Name, "arguments": string(arguments)}
		return w.writeResponses("response.output_item.done", gin.H{"type": "response.output_item.done", "output_index": index, "item": completed})
	default:
		if err := w.closeAnthropicBlock(); err != nil {
			return err
		}
		index := w.anthropicIndex
		if err := w.write("content_block_start", gin.H{
			"type": "content_block_start", "index": index,
			"content_block": gin.H{"type": "tool_use", "id": action.ID, "name": action.Name, "input": gin.H{}},
		}); err != nil {
			return err
		}
		if err := w.write("content_block_delta", gin.H{
			"type": "content_block_delta", "index": index,
			"delta": gin.H{"type": "input_json_delta", "partial_json": string(arguments)},
		}); err != nil {
			return err
		}
		if err := w.write("content_block_stop", gin.H{"type": "content_block_stop", "index": index}); err != nil {
			return err
		}
		w.anthropicIndex++
		return nil
	}
}

func (w *cursorIDEStreamWriter) Finish(result cursorCollected) error {
	if err := w.Start(result.Usage); err != nil {
		return err
	}
	switch w.protocol {
	case cursorpkg.ProtocolOpenAIChat:
		if err := w.write("", w.openAIChunk(gin.H{}, cursorOpenAIFinishReason(result), cursorOpenAIUsage(result.Usage))); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w.c.Writer, "data: [DONE]\n\n"); err != nil {
			return err
		}
		w.c.Writer.Flush()
		return nil
	case cursorpkg.ProtocolResponses:
		if w.responsesReasonOpen {
			if err := w.writeResponses("response.reasoning_summary_text.done", gin.H{
				"type": "response.reasoning_summary_text.done", "item_id": w.responsesReasonID,
				"output_index": w.responsesReasonIndex, "summary_index": 0, "text": result.Reasoning,
			}); err != nil {
				return err
			}
			item := gin.H{"id": w.responsesReasonID, "type": "reasoning", "status": "completed", "summary": []gin.H{{"type": "summary_text", "text": result.Reasoning}}}
			if err := w.writeResponses("response.output_item.done", gin.H{"type": "response.output_item.done", "output_index": w.responsesReasonIndex, "item": item}); err != nil {
				return err
			}
		}
		if w.responsesTextOpen {
			if err := w.writeResponses("response.output_text.done", gin.H{
				"type": "response.output_text.done", "item_id": w.responsesTextItemID,
				"output_index": w.responsesTextIndex, "content_index": 0, "text": result.CleanText,
			}); err != nil {
				return err
			}
			item := gin.H{"id": w.responsesTextItemID, "type": "message", "role": "assistant", "status": "completed", "content": []gin.H{{"type": "output_text", "text": result.CleanText, "annotations": []any{}}}}
			if err := w.writeResponses("response.output_item.done", gin.H{"type": "response.output_item.done", "output_index": w.responsesTextIndex, "item": item}); err != nil {
				return err
			}
		}
		return w.writeResponses("response.completed", gin.H{
			"type":     "response.completed",
			"response": cursorResponsesCompleted(w.responseID, w.model, result),
		})
	default:
		if err := w.closeAnthropicBlock(); err != nil {
			return err
		}
		if err := w.write("message_delta", gin.H{
			"type":  "message_delta",
			"delta": gin.H{"stop_reason": cursorAnthropicStopReason(result), "stop_sequence": nil},
			"usage": cursorAnthropicUsage(result.Usage, true),
		}); err != nil {
			return err
		}
		return w.write("message_stop", gin.H{"type": "message_stop"})
	}
}

func (w *cursorIDEStreamWriter) WriteError(message string) error {
	if !w.started {
		return nil
	}
	switch w.protocol {
	case cursorpkg.ProtocolResponses:
		return w.writeResponses("response.failed", gin.H{"type": "response.failed", "response": gin.H{"id": w.responseID, "status": "failed", "error": gin.H{"type": "upstream_error", "message": message}}})
	default:
		return w.write("error", gin.H{"type": "error", "error": gin.H{"type": "upstream_error", "message": message}})
	}
}

func (w *cursorIDEStreamWriter) ensureAnthropicBlock(kind string) error {
	if w.anthropicOpen && w.anthropicKind == kind {
		return nil
	}
	if err := w.closeAnthropicBlock(); err != nil {
		return err
	}
	block := gin.H{"type": "text", "text": ""}
	if kind == "thinking" {
		block = gin.H{"type": "thinking", "thinking": ""}
	}
	if err := w.write("content_block_start", gin.H{"type": "content_block_start", "index": w.anthropicIndex, "content_block": block}); err != nil {
		return err
	}
	w.anthropicOpen = true
	w.anthropicKind = kind
	return nil
}

func (w *cursorIDEStreamWriter) closeAnthropicBlock() error {
	if !w.anthropicOpen {
		return nil
	}
	if err := w.write("content_block_stop", gin.H{"type": "content_block_stop", "index": w.anthropicIndex}); err != nil {
		return err
	}
	w.anthropicIndex++
	w.anthropicOpen = false
	w.anthropicKind = ""
	return nil
}

func (w *cursorIDEStreamWriter) ensureResponsesReasoning() error {
	if w.responsesReasonOpen {
		return nil
	}
	w.responsesReasonID = "rs_" + w.responseID
	index := w.responsesOutputIndex
	w.responsesOutputIndex++
	w.responsesReasonIndex = index
	w.responsesReasonOpen = true
	return w.writeResponses("response.output_item.added", gin.H{
		"type": "response.output_item.added", "output_index": index,
		"item": gin.H{"id": w.responsesReasonID, "type": "reasoning", "summary": []any{}},
	})
}

func (w *cursorIDEStreamWriter) ensureResponsesText() error {
	if w.responsesTextOpen {
		return nil
	}
	w.responsesTextItemID = "msg_" + w.responseID
	index := w.responsesOutputIndex
	w.responsesOutputIndex++
	w.responsesTextIndex = index
	w.responsesTextOpen = true
	return w.writeResponses("response.output_item.added", gin.H{
		"type": "response.output_item.added", "output_index": index,
		"item": gin.H{"id": w.responsesTextItemID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}},
	})
}

func (w *cursorIDEStreamWriter) openAIChunk(delta gin.H, finish any, usage any) gin.H {
	chunk := gin.H{
		"id": w.responseID, "object": "chat.completion.chunk", "created": w.createdAt, "model": w.model,
		"choices": []gin.H{{"index": 0, "delta": delta, "finish_reason": finish}},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
	return chunk
}

func (w *cursorIDEStreamWriter) writeResponses(event string, payload gin.H) error {
	payload["sequence_number"] = w.sequence
	w.sequence++
	return w.write(event, payload)
}

func (w *cursorIDEStreamWriter) write(event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if event != "" {
		if _, err = fmt.Fprintf(w.c.Writer, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintf(w.c.Writer, "data: %s\n\n", data); err != nil {
		return err
	}
	w.c.Writer.Flush()
	return nil
}

func cursorResponsesCompleted(responseID, model string, result cursorCollected) gin.H {
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
	return gin.H{
		"id": responseID, "object": "response", "created_at": time.Now().Unix(), "status": "completed", "model": model,
		"output": output, "usage": cursorResponsesUsage(result.Usage), "error": nil,
	}
}
