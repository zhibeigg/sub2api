package cursor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type SSEHandler func(SSEEvent) error

func ParseSSE(ctx context.Context, reader io.Reader, handler SSEHandler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil {
		return protocolError("parse SSE", fmt.Errorf("nil reader"))
	}
	buffered := bufio.NewReader(reader)
	dataLines := make([]string, 0, 2)
	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if strings.TrimSpace(data) == "" {
			return nil
		}
		if strings.TrimSpace(data) == "[DONE]" {
			if handler != nil {
				return handler(SSEEvent{Type: "finish", FinishReason: "stop"})
			}
			return nil
		}
		var event SSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return protocolError("decode SSE event", fmt.Errorf("invalid data %q: %w", truncateString(data, 256), err))
		}
		if event.Type == "" {
			return protocolError("decode SSE event", fmt.Errorf("event type is required"))
		}
		if usage := event.EventUsage(); usage != nil && usage.TotalTokens == 0 {
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		}
		if handler != nil {
			return handler(event)
		}
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return transportError("parse SSE", err)
		}
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			if line == "" {
				if dispatchErr := dispatch(); dispatchErr != nil {
					return dispatchErr
				}
			} else if !strings.HasPrefix(line, ":") {
				field, value, found := strings.Cut(line, ":")
				if !found {
					field, value = line, ""
				}
				if strings.HasPrefix(value, " ") {
					value = value[1:]
				}
				if field == "data" {
					dataLines = append(dataLines, value)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if dispatchErr := dispatch(); dispatchErr != nil {
					return dispatchErr
				}
				return nil
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return transportError("parse SSE", ctxErr)
			}
			return transportError("read SSE", err)
		}
	}
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
