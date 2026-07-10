package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	httpClient *http.Client
	credential Credential
	config     ClientConfig
}

func NewClient(httpClient *http.Client, credential Credential, config ClientConfig) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("cursor: injected http client is required")
	}
	config = config.withDefaults()
	endpoint, err := url.Parse(config.BaseURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("cursor: invalid base URL %q", config.BaseURL)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("cursor: unsupported base URL scheme %q", endpoint.Scheme)
	}
	return &Client{httpClient: httpClient, credential: credential, config: config}, nil
}

func (c *Client) Config() ClientConfig {
	return c.config
}

func (c *Client) BuildPayload(dialogue *Dialogue, options BuildOptions) (*Request, error) {
	if options.Model == "" {
		options.Model = c.config.Model
	}
	return BuildPayload(dialogue, options)
}

func (c *Client) Stream(ctx context.Context, payload *Request, handler SSEHandler) error {
	if payload == nil {
		return badRequest("send request", fmt.Errorf("nil payload"))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return badRequest("encode request", err)
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return transportError("create request", err)
	}
	c.applyHeaders(req)

	idle := newIdleDeadline(c.config.StreamIdleTimeout, cancel)
	idle.arm()
	resp, err := c.httpClient.Do(req)
	idle.disarm()
	if err != nil {
		if ctx.Err() != nil {
			return &Error{Kind: ErrorTransport, Operation: "send request", Err: ctx.Err()}
		}
		return transportError("send request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, readErr := io.ReadAll(io.LimitReader(resp.Body, c.config.MaxErrorBody+1))
		if readErr != nil {
			return transportError("read error response", readErr)
		}
		truncated := int64(len(limited)) > c.config.MaxErrorBody
		if truncated {
			limited = limited[:c.config.MaxErrorBody]
		}
		message := strings.TrimSpace(string(limited))
		if truncated {
			message += "..."
		}
		return HTTPError(resp.StatusCode, "send request", message)
	}
	reader := &idleReader{reader: resp.Body, deadline: idle}
	if err := ParseSSE(requestCtx, reader, handler); err != nil {
		if ctx.Err() != nil {
			return &Error{Kind: ErrorTransport, Operation: "stream response", Err: ctx.Err()}
		}
		return err
	}
	return nil
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Referer", c.config.Referer)
	req.Header.Set("X-Path", "/api/chat")
	req.Header.Set("X-Method", http.MethodPost)
	if c.credential.Cookie != "" {
		req.Header.Set("Cookie", c.credential.Cookie)
	}
	if endpoint, err := url.Parse(c.config.BaseURL); err == nil {
		req.Header.Set("Origin", endpoint.Scheme+"://"+endpoint.Host)
	}
}

type idleDeadline struct {
	mu      sync.Mutex
	timeout time.Duration
	cancel  context.CancelFunc
	timer   *time.Timer
}

func newIdleDeadline(timeout time.Duration, cancel context.CancelFunc) *idleDeadline {
	return &idleDeadline{timeout: timeout, cancel: cancel}
}

func (d *idleDeadline) arm() {
	if d == nil || d.timeout <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.timeout, d.cancel)
}

func (d *idleDeadline) disarm() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

type idleReader struct {
	reader   io.Reader
	deadline *idleDeadline
}

func (r *idleReader) Read(p []byte) (int, error) {
	r.deadline.arm()
	n, err := r.reader.Read(p)
	r.deadline.disarm()
	return n, err
}
