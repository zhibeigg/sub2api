package service

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	opencodepkg "github.com/Wei-Shaw/sub2api/internal/pkg/opencode"
	"github.com/gin-gonic/gin"
)

func (s *OpenCodeGatewayService) forwardStream(c *gin.Context, body io.Reader, meta opencodepkg.RequestMeta, result *ForwardResult, start time.Time) error {
	transformer := opencodepkg.NewStreamTransformer(meta)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	wroteAny := false
	wroteSemantic := false

	writeFrames := func(frames []opencodepkg.StreamFrame) error {
		for _, frame := range frames {
			if frame.Event != "" {
				if _, err := fmt.Fprintf(c.Writer, "event: %s\n", frame.Event); err != nil {
					result.ClientDisconnect = true
					return err
				}
			}
			for _, line := range bytes.Split(frame.Data, []byte("\n")) {
				if _, err := fmt.Fprintf(c.Writer, "data: %s\n", line); err != nil {
					result.ClientDisconnect = true
					return err
				}
			}
			if _, err := io.WriteString(c.Writer, "\n"); err != nil {
				result.ClientDisconnect = true
				return err
			}
			wroteAny = true
			if frame.Semantic && !wroteSemantic {
				wroteSemantic = true
				elapsed := int(time.Since(start).Milliseconds())
				result.FirstTokenMs = &elapsed
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		return nil
	}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var event string
	var dataLines []string
	flushEvent := func() error {
		if len(dataLines) == 0 {
			event = ""
			return nil
		}
		frames, err := transformer.Push(event, []byte(strings.Join(dataLines, "\n")))
		event = ""
		dataLines = dataLines[:0]
		if err != nil {
			return err
		}
		return writeFrames(frames)
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flushEvent(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := flushEvent(); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		failure := opencodeNetworkFailure(err)
		failure.SafeToFailoverAfterWrite = wroteAny && !wroteSemantic
		return failure
	}
	frames, err := transformer.Finalize()
	if err != nil {
		return opencodeNetworkFailure(err)
	}
	if err := writeFrames(frames); err != nil {
		return err
	}
	result.Usage = openCodeClaudeUsage(transformer.Usage())
	if transformer.RequestID() != "" {
		result.RequestID = transformer.RequestID()
	}
	return nil
}

func opencodeHTTPFailure(response *http.Response) *UpstreamFailoverError {
	if response == nil {
		return opencodeNetworkFailure(errors.New("empty upstream response"))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, opencodeErrorBodyLimit))
	if err != nil {
		body = []byte(err.Error())
	}
	headers := response.Header.Clone()
	failure := &UpstreamFailoverError{
		StatusCode: response.StatusCode, ResponseBody: body, ResponseHeaders: headers,
		ClientStatusCode: response.StatusCode,
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		failure.Stage = GatewayFailureStageAccountAuth
		failure.Scope = GatewayFailureScopeAccount
		failure.Reason = GatewayFailureReason("opencode_credentials_rejected")
		failure.NextAccountAction = NextAccountRetry
	case response.StatusCode == http.StatusTooManyRequests:
		failure.Scope = GatewayFailureScopeAccount
		failure.Reason = GatewayFailureReason("opencode_rate_limited")
		failure.NextAccountAction = NextAccountRetry
	case response.StatusCode >= http.StatusInternalServerError:
		failure.Scope = GatewayFailureScopeProvider
		failure.Reason = GatewayFailureReason("opencode_upstream_unavailable")
		failure.NextAccountAction = NextAccountRetry
	case response.StatusCode >= http.StatusBadRequest:
		failure.Scope = GatewayFailureScopeRequest
		failure.Reason = GatewayFailureReason("opencode_request_rejected")
		failure.NextAccountAction = NextAccountStop
	default:
		failure.Scope = GatewayFailureScopeProvider
		failure.NextAccountAction = NextAccountRetry
	}
	return failure
}

func opencodeNetworkFailure(err error) *UpstreamFailoverError {
	message := "OpenCode upstream network error"
	if err != nil {
		message = err.Error()
	}
	return &UpstreamFailoverError{
		StatusCode: http.StatusBadGateway, ResponseBody: []byte(message),
		Scope: GatewayFailureScopeAccount, Reason: GatewayFailureReason("opencode_network_error"),
		NextAccountAction: NextAccountRetry, ClientStatusCode: http.StatusBadGateway,
	}
}

func opencodeRequestError(status int, body []byte) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode: status, ResponseBody: append([]byte(nil), body...),
		Scope: GatewayFailureScopeRequest, Reason: GatewayFailureReason("opencode_invalid_request"),
		NextAccountAction: NextAccountStop, ClientStatusCode: status,
	}
}

func opencodeAccountFailure(status int, body []byte, headers http.Header) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode: status, ResponseBody: append([]byte(nil), body...), ResponseHeaders: headers,
		Stage: GatewayFailureStageAccountAuth, Scope: GatewayFailureScopeAccount,
		Reason:            GatewayFailureReason("opencode_account_configuration"),
		NextAccountAction: NextAccountRetry, ClientStatusCode: status,
	}
}
