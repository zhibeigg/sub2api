package cursor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAgentQueueSize         = 32
	defaultAgentHeartbeatInterval = 15 * time.Second
)

type AgentClient struct {
	httpClient *http.Client
	config     AgentClientConfig
	baseURL    string
}

func NewAgentClient(httpClient *http.Client, config AgentClientConfig) (*AgentClient, error) {
	if httpClient == nil {
		return nil, badRequest("create Agent client", errors.New("http client is required"))
	}
	applyIDEConfigDefaults(&config.IDEClientConfig)
	if config.QueueSize <= 0 {
		config.QueueSize = defaultAgentQueueSize
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultAgentHeartbeatInterval
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultIDEBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, badRequest("create Agent client", errors.New("invalid base URL"))
	}
	if config.MaxBufferedBytes < 5 || config.MaxFrameSize > config.MaxBufferedBytes-5 {
		return nil, badRequest("create Agent client", errors.New("max buffered bytes must accommodate one complete frame"))
	}
	return &AgentClient{httpClient: httpClient, config: config, baseURL: baseURL}, nil
}

func (c *AgentClient) Run(ctx context.Context, credential IDECredential, dialogue *Dialogue, options AgentRunOptions) (*http.Response, *AgentStream, error) {
	ctx, cancel := context.WithCancel(nonNilContext(ctx))
	if options.RequestContext.OSVersion == "" {
		options.RequestContext.OSVersion = c.config.ClientOSVersion
	}
	if options.RequestContext.TimeZone == "" {
		options.RequestContext.TimeZone = c.config.Timezone
	}
	payload, err := encodeAgentRunRequest(dialogue, options, c.config.UUID)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	headers, err := BuildIDEHeaders(credential, c.config.IDEClientConfig)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	reader, writer := io.Pipe()
	stream := &AgentStream{
		ctx: ctx, decoder: NewConnectDecoder(c.config.MaxFrameSize, c.config.MaxBufferedBytes),
		send: make(chan []byte, c.config.QueueSize), writerErr: make(chan error, 1), cancel: cancel,
		tools: make(map[string]*agentToolAccumulator), maxToolBytes: c.config.MaxBufferedBytes,
	}
	stream.closeSend = func() error { return stream.closeSendQueue() }
	go c.writeAgentRequest(ctx, writer, stream.send, stream.writerErr)
	if err := stream.SendClientMessage(payload); err != nil {
		_ = stream.Close()
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+AgentRunPath, reader)
	if err != nil {
		_ = stream.Close()
		return nil, nil, badRequest("create Agent run request", err)
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Accept", "application/connect+proto")
	req.Header.Set("Connect-Accept-Encoding", "gzip")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		_ = stream.Close()
		return nil, nil, transportError("Agent run request", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = stream.Close()
		return resp, nil, c.httpError("Agent run request", resp)
	}
	stream.response = resp
	return resp, stream, nil
}

func (c *AgentClient) writeAgentRequest(ctx context.Context, writer *io.PipeWriter, queue <-chan []byte, result chan<- error) {
	defer close(result)
	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()
	write := func(payload []byte) error {
		frame, err := EncodeConnectFrame(payload, false)
		if err != nil {
			return err
		}
		if len(frame)-5 > c.config.MaxFrameSize {
			return fmt.Errorf("agent frame size %d exceeds %d bytes", len(frame)-5, c.config.MaxFrameSize)
		}
		_, err = writer.Write(frame)
		return err
	}
	for {
		select {
		case <-ctx.Done():
			_ = writer.CloseWithError(ctx.Err())
			result <- ctx.Err()
			return
		case payload, ok := <-queue:
			if !ok {
				_, err := writer.Write(encodeAgentConnectEndStream())
				if err == nil {
					err = writer.Close()
				} else {
					_ = writer.CloseWithError(err)
				}
				result <- err
				return
			}
			if err := write(payload); err != nil {
				_ = writer.CloseWithError(err)
				result <- err
				return
			}
		case <-ticker.C:
			if err := write(encodeAgentClientHeartbeat()); err != nil {
				_ = writer.CloseWithError(err)
				result <- err
				return
			}
		}
	}
}

func (c *AgentClient) GetUsableModels(ctx context.Context, credential IDECredential, customModelIDs []string) ([]AgentModel, error) {
	var payload []byte
	for _, model := range customModelIDs {
		payload = appendString(payload, 1, model)
	}
	headers, err := BuildIDEHeaders(credential, c.config.IDEClientConfig)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(nonNilContext(ctx), http.MethodPost, c.baseURL+AgentGetUsableModelsPath, bytes.NewReader(payload))
	if err != nil {
		return nil, badRequest("create Agent models request", err)
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Accept", "application/proto")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, transportError("Agent models request", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, c.httpError("Agent models request", resp)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.config.MaxBufferedBytes)+1))
	if err != nil {
		return nil, transportError("read Agent models response", err)
	}
	if len(body) > c.config.MaxBufferedBytes {
		return nil, protocolError("decode Agent models response", errors.New("response exceeds configured buffer limit"))
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Encoding")), "gzip") || (len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b) {
		body, err = gunzipLimited(body, c.config.MaxBufferedBytes)
		if err != nil {
			return nil, protocolError("decompress Agent models response", err)
		}
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/connect+proto") {
		decoder := NewConnectDecoder(c.config.MaxFrameSize, c.config.MaxBufferedBytes)
		frames, decodeErr := decoder.Feed(body)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if decodeErr = decoder.Finish(); decodeErr != nil {
			return nil, decodeErr
		}
		body = nil
		for _, frame := range frames {
			if frame.EndStream() {
				streamErr, envelopeErr := parseConnectEndStream(frame.Payload)
				if envelopeErr != nil {
					return nil, protocolError("decode Agent models response", envelopeErr)
				}
				if streamErr != nil {
					return nil, protocolError("decode Agent models response", errors.New(streamErr.Message))
				}
				continue
			}
			body = append(body, frame.Payload...)
		}
	}
	models, err := decodeAgentModels(body)
	if err != nil {
		return nil, protocolError("decode Agent models response", err)
	}
	return models, nil
}

func (c *AgentClient) httpError(operation string, resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(c.config.MaxErrorBody)+1))
	truncated := len(body) > c.config.MaxErrorBody
	if truncated {
		body = body[:c.config.MaxErrorBody]
	}
	message := strings.TrimSpace(string(body))
	if truncated {
		message += "..."
	}
	return HTTPError(resp.StatusCode, operation, message)
}
