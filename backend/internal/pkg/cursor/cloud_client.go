package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultCloudMaxErrorBody = 8 << 10

type CloudClientConfig struct {
	BaseURL           string
	RequestTimeout    time.Duration
	StreamIdleTimeout time.Duration
	MaxErrorBody      int
}

type CloudClient struct {
	httpClient        *http.Client
	apiKey            string
	baseURL           string
	requestTimeout    time.Duration
	streamIdleTimeout time.Duration
	maxErrorBody      int
}

func NewCloudClient(httpClient *http.Client, apiKey string, config CloudClientConfig) (*CloudClient, error) {
	if httpClient == nil {
		return nil, badRequest("create cloud client", fmt.Errorf("http client is required"))
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, badRequest("create cloud client", fmt.Errorf("API key is required"))
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultCloudBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, badRequest("create cloud client", fmt.Errorf("invalid base URL"))
	}
	maxErrorBody := config.MaxErrorBody
	if maxErrorBody <= 0 {
		maxErrorBody = defaultCloudMaxErrorBody
	}
	return &CloudClient{
		httpClient:        httpClient,
		apiKey:            apiKey,
		baseURL:           baseURL,
		requestTimeout:    config.RequestTimeout,
		streamIdleTimeout: config.StreamIdleTimeout,
		maxErrorBody:      maxErrorBody,
	}, nil
}

func (c *CloudClient) Me(ctx context.Context) (*APIKeyInfo, error) {
	var result APIKeyInfo
	if err := c.doJSON(ctx, http.MethodGet, "/v1/me", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *CloudClient) ListModels(ctx context.Context) ([]CloudModel, error) {
	var result struct {
		Items  []CloudModel `json:"items"`
		Models []CloudModel `json:"models,omitempty"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/models", nil, &result); err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		return result.Items, nil
	}
	return result.Models, nil
}

func (c *CloudClient) CreateAgent(ctx context.Context, request CreateAgentRequest) (*CreateAgentResponse, error) {
	if strings.TrimSpace(request.Prompt.Text) == "" && len(request.Prompt.Images) == 0 {
		return nil, badRequest("create agent", fmt.Errorf("prompt is required"))
	}
	var result CreateAgentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/agents", request, &result); err != nil {
		return nil, err
	}
	if result.Agent.ID == "" || result.Run.ID == "" {
		return nil, protocolError("create agent", fmt.Errorf("response did not include agent and run IDs"))
	}
	return &result, nil
}

func (c *CloudClient) GetRun(ctx context.Context, agentID, runID string) (*CloudRun, error) {
	var result CloudRun
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs/" + url.PathEscape(runID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *CloudClient) StreamRun(ctx context.Context, agentID, runID string, handler CloudSSEHandler) error {
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs/" + url.PathEscape(runID) + "/stream"
	req, cancel, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer cancel()
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.wrapRequestError("stream run", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.httpError("stream run", resp)
	}
	err = c.parseCloudStream(req.Context(), resp.Body, handler)
	if errors.Is(err, context.Canceled) {
		return &Error{Kind: ErrorTransport, Operation: "stream run", Err: context.Canceled}
	}
	return err
}

func (c *CloudClient) CancelRun(ctx context.Context, agentID, runID string) error {
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs/" + url.PathEscape(runID) + "/cancel"
	return c.doJSON(ctx, http.MethodPost, path, nil, nil)
}

func (c *CloudClient) DeleteAgent(ctx context.Context, agentID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/agents/"+url.PathEscape(agentID), nil, nil)
}

func (c *CloudClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return badRequest(strings.ToLower(method)+" "+path, err)
		}
	}
	req, cancel, err := c.newRequest(ctx, method, path, bodyBytes)
	if err != nil {
		return err
	}
	defer cancel()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.wrapRequestError(strings.ToLower(method)+" "+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.httpError(strings.ToLower(method)+" "+path, resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return protocolError(strings.ToLower(method)+" "+path, err)
	}
	return nil
}

func (c *CloudClient) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, context.CancelFunc, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cancel := func() {}
	if c.requestTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.requestTimeout)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		cancel()
		return nil, func() {}, badRequest("create request", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, cancel, nil
}

func (c *CloudClient) parseCloudStream(ctx context.Context, body io.Reader, handler CloudSSEHandler) error {
	if c.streamIdleTimeout <= 0 {
		return ParseCloudSSE(ctx, body, handler)
	}
	events := make(chan CloudSSEEvent)
	errorsCh := make(chan error, 1)
	go func() {
		errorsCh <- ParseCloudSSE(ctx, body, func(event CloudSSEEvent) error {
			select {
			case events <- event:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	timer := time.NewTimer(c.streamIdleTimeout)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(c.streamIdleTimeout)
			if handler != nil {
				if err := handler(event); err != nil {
					return err
				}
			}
		case err := <-errorsCh:
			return err
		case <-timer.C:
			return transportError("stream run", fmt.Errorf("stream idle timeout after %s", c.streamIdleTimeout))
		case <-ctx.Done():
			return transportError("stream run", ctx.Err())
		}
	}
}

func (c *CloudClient) httpError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxErrorBody)+1))
	message := strings.TrimSpace(string(body))
	if len(body) > c.maxErrorBody {
		message = strings.TrimSpace(string(body[:c.maxErrorBody])) + "..."
	}
	var apiErr CloudAPIError
	if json.Unmarshal(body, &apiErr) == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
		message = apiErr.Error.Message
		if apiErr.Error.Code != "" {
			message = apiErr.Error.Code + ": " + message
		}
	}
	return HTTPError(resp.StatusCode, operation, message)
}

func (c *CloudClient) wrapRequestError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: ErrorTransport, Operation: operation, Err: err}
	}
	return transportError(operation, err)
}
