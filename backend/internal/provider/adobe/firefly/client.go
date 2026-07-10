package firefly

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var imageSubmitURL = "https://firefly-3p.ff.adobe.io/v2/3p-images/generate-async"
var videoSubmitURL = "https://firefly-3p.ff.adobe.io/v2/3p-videos/generate-async"
var uploadURL = "https://firefly-3p.ff.adobe.io/v2/storage/image"

type ClientConfig struct {
	ProxyURL                     string
	RequestTimeout, PollInterval time.Duration
	MaxPollAttempts              int
}
type Client struct {
	cfg       ClientConfig
	transport transportDoer
}
type SubmitResult struct {
	TaskID    string `json:"task_id"`
	StatusURL string `json:"status_url"`
}
type PollResult struct {
	TaskID       string        `json:"task_id"`
	Status       string        `json:"status"`
	OutputURL    string        `json:"output_url"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	ErrorCode    string        `json:"error_code"`
	ErrorMessage string        `json:"error_message"`
	RetryAfter   time.Duration `json:"retry_after"`
}
type ImageResult struct {
	TaskID string `json:"task_id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 60 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 3 * time.Second
	}
	if cfg.MaxPollAttempts <= 0 {
		cfg.MaxPollAttempts = 200
	}
	return &Client{cfg: cfg, transport: newTransport(cfg.ProxyURL, cfg.RequestTimeout)}
}

func (c *Client) UploadAsset(ctx context.Context, accessToken, assetName, contentType string, data []byte) (string, error) {
	if strings.TrimSpace(accessToken) == "" || len(data) == 0 {
		return "", fmt.Errorf("invalid upload input")
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(ct); err == nil {
		ct = parsed
	}
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		return "", fmt.Errorf("unsupported asset content type")
	}
	h := browserHeaders(accessToken)
	h["content-type"] = ct
	h["accept"] = "application/json"
	if assetName != "" {
		h["x-file-name"] = sanitizeAssetName(assetName)
	}
	resp, err := c.transport.do(ctx, http.MethodPost, uploadURL, h, data)
	if err != nil {
		return "", temporaryTransportError(err)
	}
	if resp.status != 200 && resp.status != 201 {
		return "", classifyError(resp.status, headerMap(resp.headers), resp.body)
	}
	var v struct {
		ID      string `json:"id"`
		ImageID string `json:"imageId"`
		Images  []struct {
			ID string `json:"id"`
		} `json:"images"`
	}
	if json.Unmarshal(resp.body, &v) != nil {
		return "", fmt.Errorf("invalid upload response")
	}
	if len(v.Images) > 0 && v.Images[0].ID != "" {
		return v.Images[0].ID, nil
	}
	if v.ID != "" {
		return v.ID, nil
	}
	if v.ImageID != "" {
		return v.ImageID, nil
	}
	return "", fmt.Errorf("upload response has no asset id")
}
func sanitizeAssetName(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}

func (c *Client) SubmitImage(ctx context.Context, token string, p *ResolvedParams, prompt string, refs []string) (*SubmitResult, error) {
	if p == nil || p.Type != ModelTypeImage {
		return nil, fmt.Errorf("invalid image parameters")
	}
	candidates := buildImagePayloadCandidates(p, prompt, refs)
	var last error
	for i, payload := range candidates {
		r, err := c.submit(ctx, imageSubmitURL, token, payload, prompt)
		if err == nil {
			return r, nil
		}
		last = err
		if i == len(candidates)-1 || !shouldTryNextCandidate(err) {
			break
		}
	}
	return nil, last
}
func (c *Client) SubmitVideo(ctx context.Context, token string, p *ResolvedParams, prompt string, refs []string) (*SubmitResult, error) {
	if p == nil || p.Type != ModelTypeVideo {
		return nil, fmt.Errorf("invalid video parameters")
	}
	payload, err := buildVideoPayload(p, prompt, refs)
	if err != nil {
		return nil, err
	}
	return c.submit(ctx, videoSubmitURL, token, payload, prompt)
}
func (c *Client) submit(ctx context.Context, endpoint, token string, payload Payload, prompt string) (*SubmitResult, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("access token is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	h := browserHeaders(token)
	if uid := extractUserIDFromJWT(token); uid != "" {
		h["x-nonce"] = computeNonce(uid, prompt)
	}
	resp, err := c.transport.do(ctx, http.MethodPost, endpoint, h, body)
	if err != nil {
		return nil, temporaryTransportError(err)
	}
	if resp.status != 200 && resp.status != 202 {
		return nil, classifyError(resp.status, headerMap(resp.headers), resp.body)
	}
	statusURL := resp.headers.Get("x-override-status-link")
	var v map[string]any
	_ = json.Unmarshal(resp.body, &v)
	if statusURL == "" {
		statusURL = findStatusURL(v)
	}
	if err := ValidateStatusURL(statusURL); err != nil {
		return nil, err
	}
	taskID := stringValue(v, "jobId", "taskId", "id")
	if taskID == "" {
		taskID = taskIDFromURL(statusURL)
	}
	return &SubmitResult{TaskID: taskID, StatusURL: statusURL}, nil
}

func (c *Client) Poll(ctx context.Context, token, statusURL string) (*PollResult, error) {
	if err := ValidateStatusURL(statusURL); err != nil {
		return nil, err
	}
	h := browserHeaders(token)
	h["cache-control"] = "no-cache"
	h["pragma"] = "no-cache"
	resp, err := c.transport.do(ctx, http.MethodGet, statusURL, h, nil)
	if err != nil {
		return nil, temporaryTransportError(err)
	}
	if resp.status != 200 {
		return nil, classifyError(resp.status, headerMap(resp.headers), resp.body)
	}
	result, _, err := parsePoll(resp)
	return result, err
}
func (c *Client) GenerateImage(ctx context.Context, token string, p *ResolvedParams, prompt string, refs []string) (*ImageResult, error) {
	submit, err := c.SubmitImage(ctx, token, p, prompt, refs)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < c.cfg.MaxPollAttempts; attempt++ {
		poll, err := c.Poll(ctx, token, submit.StatusURL)
		if err != nil {
			return nil, err
		}
		if poll.OutputURL != "" && (poll.Status == "COMPLETED" || poll.Status == "SUCCEEDED" || poll.Status == "") {
			return &ImageResult{TaskID: firstNonEmpty(poll.TaskID, submit.TaskID), URL: poll.OutputURL, Width: firstPositive(poll.Width, p.Width), Height: firstPositive(poll.Height, p.Height)}, nil
		}
		delay := poll.RetryAfter
		if delay <= 0 {
			delay = c.cfg.PollInterval
		}
		if !wait(ctx, delay) {
			return nil, ctx.Err()
		}
	}
	return nil, &ProviderError{Kind: ErrorTemporary, Code: "poll_timeout", Message: "poll attempts exhausted", Retryable: true}
}

func ValidateStatusURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("invalid Adobe status URL")
	}
	if !strings.EqualFold(u.Hostname(), "firefly-3p.ff.adobe.io") || (u.Port() != "" && u.Port() != "443") {
		return fmt.Errorf("untrusted Adobe status URL host")
	}
	decodedPath, err := url.PathUnescape(u.EscapedPath())
	if err != nil || !isAllowedStatusPath(decodedPath) {
		return fmt.Errorf("untrusted Adobe status URL path")
	}
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("untrusted Adobe status URL path")
		}
	}
	return nil
}

func isAllowedStatusPath(path string) bool {
	for _, prefix := range []string{
		"/jobs/",
		"/v2/status/",
		"/v2/3p-images/generate-async/",
		"/v2/3p-videos/generate-async/",
	} {
		if strings.HasPrefix(path, prefix) && len(path) > len(prefix) {
			return true
		}
	}
	return false
}
func findStatusURL(v map[string]any) string {
	if s, _ := v["statusUrl"].(string); s != "" {
		return s
	}
	if links, ok := v["links"].(map[string]any); ok {
		if r, ok := links["result"].(map[string]any); ok {
			if s, _ := r["href"].(string); s != "" {
				return s
			}
		}
	}
	return ""
}
func parsePoll(resp *transportResponse) (*PollResult, bool, error) {
	var v map[string]any
	if json.Unmarshal(resp.body, &v) != nil {
		return nil, false, fmt.Errorf("invalid poll response")
	}
	status := strings.ToUpper(stringValue(v, "status", "taskStatus"))
	if status == "" {
		status = strings.ToUpper(resp.headers.Get("x-task-status"))
	}
	r := &PollResult{TaskID: stringValue(v, "jobId", "taskId", "id"), Status: status, RetryAfter: parseRetryAfter(resp.headers.Get("retry-after"))}
	r.OutputURL, r.Width, r.Height = extractOutput(v)
	if r.Width == 0 {
		r.Width = intValue(v, "width")
	}
	if r.Height == 0 {
		r.Height = intValue(v, "height")
	}
	if status == "FAILED" || status == "ERROR" || status == "CANCELLED" || status == "REJECTED" {
		r.ErrorCode = stringValue(v, "errorCode", "code")
		r.ErrorMessage = "generation failed"
		return r, true, &ProviderError{Kind: ErrorRequest, Code: firstNonEmpty(r.ErrorCode, "generation_failed"), Message: r.ErrorMessage}
	}
	if r.OutputURL != "" && (status == "COMPLETED" || status == "SUCCEEDED" || status == "SUCCESS" || status == "") {
		return r, true, nil
	}
	if status == "COMPLETED" || status == "SUCCEEDED" || status == "SUCCESS" {
		// Firefly may publish the terminal status before the presigned output URL is
		// visible. Keep the task retryable and never settle a result-less generation.
		r.Status = "IN_PROGRESS"
	}
	return r, false, nil
}
func extractOutput(v map[string]any) (string, int, int) {
	if outputs, ok := v["outputs"].([]any); ok && len(outputs) > 0 {
		if o, ok := outputs[0].(map[string]any); ok {
			for _, k := range []string{"image", "video"} {
				if m, ok := o[k].(map[string]any); ok {
					if s, _ := m["presignedUrl"].(string); s != "" {
						return s, intValue(m, "width"), intValue(m, "height")
					}
				}
			}
			for _, k := range []string{"presignedUrl", "url"} {
				if s, _ := o[k].(string); s != "" {
					return s, intValue(o, "width"), intValue(o, "height")
				}
			}
		}
	}
	if s, _ := v["presignedUrl"].(string); s != "" {
		return s, intValue(v, "width"), intValue(v, "height")
	}
	return "", 0, 0
}
func temporaryTransportError(error) error {
	return &ProviderError{Kind: ErrorTemporary, Code: "transport_error", Message: "upstream transport failed", Retryable: true}
}
func headerMap(h headerGetter) map[string]string {
	m := map[string]string{}
	for _, k := range []string{"x-access-error", "retry-after"} {
		m[k] = h.Get(k)
	}
	return m
}
func wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func stringValue(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
func intValue(v map[string]any, key string) int {
	if n, ok := v[key].(float64); ok {
		return int(n)
	}
	return 0
}
func taskIDFromURL(s string) string {
	u, _ := url.Parse(s)
	p := strings.Trim(u.Path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
func firstPositive(v ...int) int {
	for _, n := range v {
		if n > 0 {
			return n
		}
	}
	return 0
}
