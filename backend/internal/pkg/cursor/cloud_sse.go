package cursor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type CloudSSEHandler func(CloudSSEEvent) error

// ParseCloudSSE parses Cursor Cloud Agents named SSE events while preserving
// opaque event IDs for reconnect diagnostics.
func ParseCloudSSE(ctx context.Context, reader io.Reader, handler CloudSSEHandler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil {
		return protocolError("parse cloud SSE", fmt.Errorf("nil reader"))
	}

	buffered := bufio.NewReader(reader)
	var eventID, eventName string
	dataLines := make([]string, 0, 2)
	dispatch := func() error {
		if eventName == "" && len(dataLines) == 0 {
			return nil
		}
		event := CloudSSEEvent{
			ID:    eventID,
			Event: eventName,
			Data:  []byte(strings.Join(dataLines, "\n")),
		}
		eventID, eventName = "", ""
		dataLines = dataLines[:0]
		if event.Event == "" {
			return protocolError("parse cloud SSE", fmt.Errorf("event name is required"))
		}
		if handler != nil {
			return handler(event)
		}
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return transportError("parse cloud SSE", err)
		}
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if line == "" {
				if dispatchErr := dispatch(); dispatchErr != nil {
					return dispatchErr
				}
			} else if !strings.HasPrefix(line, ":") {
				field, value, found := strings.Cut(line, ":")
				if !found {
					field, value = line, ""
				}
				value = strings.TrimPrefix(value, " ")
				switch field {
				case "id":
					eventID = value
				case "event":
					eventName = value
				case "data":
					dataLines = append(dataLines, value)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return dispatch()
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return transportError("parse cloud SSE", ctxErr)
			}
			return transportError("read cloud SSE", err)
		}
	}
}
