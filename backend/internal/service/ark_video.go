package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Ark (Volcengine 火山方舟) Seedance video generation is an asynchronous task
// API, not the OpenAI-standard videos/generations shape:
//
//	Submit: POST {base}/contents/generations/tasks
//	        {"model":"...","content":[{"type":"text","text":"prompt --dur 5 --rs 720p --rt 16:9"}]}
//	        -> {"id":"cgt-..."}
//	Query:  GET  {base}/contents/generations/tasks/{id}
//	        -> {"status":"running|succeeded|failed","content":{"video_url":"..."},"usage":{"total_tokens":N}}
//
// This file adapts the gateway's unified /v1/videos/generations (submit) and
// /v1/videos/:request_id (query) to that task model, for openai-platform Ark
// accounts. Billing is token-based (usage.total_tokens on completion).

const arkVideoTasksPath = "/contents/generations/tasks"

// IsArkVideoModel reports whether a (mapped) model id is a火山 Seedance video model.
func IsArkVideoModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "seedance")
}

// arkVideoSubmitInfo is the parsed inbound generation request.
type arkVideoSubmitInfo struct {
	Model      string
	Prompt     string
	Seconds    int
	Resolution string // e.g. 480p / 720p / 1080p
	Ratio      string // e.g. 16:9 / 9:16 / 1:1 / adaptive
}

// parseArkVideoRequest parses the inbound JSON body. Accepts both the
// playground shape ({model,prompt,seconds,resolution,ratio}) and a raw
// {model,content:[...]} passthrough.
func parseArkVideoRequest(body []byte) arkVideoSubmitInfo {
	info := arkVideoSubmitInfo{}
	if !gjson.ValidBytes(body) {
		return info
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	if info.Prompt == "" {
		// Support OpenAI-style {input:"..."} as an alias.
		info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "input").String())
	}
	if v := gjson.GetBytes(body, "seconds"); v.Exists() && v.Type == gjson.Number {
		info.Seconds = int(v.Int())
	} else if v := gjson.GetBytes(body, "duration"); v.Exists() && v.Type == gjson.Number {
		info.Seconds = int(v.Int())
	}
	info.Resolution = strings.TrimSpace(gjson.GetBytes(body, "resolution").String())
	if info.Resolution == "" {
		info.Resolution = strings.TrimSpace(gjson.GetBytes(body, "size").String())
	}
	info.Ratio = strings.TrimSpace(gjson.GetBytes(body, "ratio").String())
	if info.Ratio == "" {
		info.Ratio = strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String())
	}
	return info
}

// buildArkVideoTaskBody constructs the Ark task-submit body. It appends
// Seedance CLI-style parameters (--dur/--rs/--rt) to the prompt text, which is
// how火山 accepts duration/resolution/ratio for text-to-video.
func buildArkVideoTaskBody(info arkVideoSubmitInfo, upstreamModel string) ([]byte, error) {
	text := info.Prompt
	if info.Seconds > 0 {
		text += fmt.Sprintf(" --dur %d", info.Seconds)
	}
	if r := normalizeArkResolution(info.Resolution); r != "" {
		text += " --rs " + r
	}
	if info.Ratio != "" && !strings.EqualFold(info.Ratio, "adaptive") && !strings.EqualFold(info.Ratio, "auto") {
		text += " --rt " + info.Ratio
	}
	payload := map[string]any{
		"model": upstreamModel,
		"content": []map[string]any{
			{"type": "text", "text": strings.TrimSpace(text)},
		},
	}
	return json.Marshal(payload)
}

func normalizeArkResolution(res string) string {
	res = strings.ToLower(strings.TrimSpace(res))
	switch res {
	case "480p", "720p", "1080p":
		return res
	default:
		return ""
	}
}

// ArkVideoSubmitResult is returned after submitting a generation task.
type ArkVideoSubmitResult struct {
	RequestID string
	Usage     OpenAIUsage
	Model     string
	Duration  time.Duration
	Raw       []byte
}

// ForwardArkVideoSubmit submits a Seedance text-to-video task to火山方舟 and
// returns the task id as request_id.
func (s *OpenAIGatewayService) ForwardArkVideoSubmit(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	start := time.Now()
	if account == nil {
		return nil, fmt.Errorf("ark account is required")
	}

	info := parseArkVideoRequest(body)
	if strings.TrimSpace(info.Model) == "" {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(`{"error":{"message":"model is required"}}`)}
	}
	if strings.TrimSpace(info.Prompt) == "" {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(`{"error":{"message":"prompt is required"}}`)}
	}

	upstreamModel := account.GetMappedModel(info.Model)
	upstreamBody, err := buildArkVideoTaskBody(info, upstreamModel)
	if err != nil {
		return nil, err
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	targetURL := arkVideoBaseURL(account.GetOpenAIBaseURL()) + arkVideoTasksPath

	respBody, status, respHeader, err := s.doArkVideoRequest(ctx, c, account, http.MethodPost, targetURL, token, upstreamBody)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, s.arkVideoUpstreamError(c, account, status, respBody, respHeader)
	}

	taskID := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if taskID == "" {
		taskID = strings.TrimSpace(gjson.GetBytes(respBody, "data.id").String())
	}
	if taskID == "" {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(`{"error":{"message":"ark video task id missing in response"}}`)}
	}

	// Return a normalized OpenAI-ish submit response to the client.
	out := map[string]any{
		"request_id": taskID,
		"id":         taskID,
		"status":     "pending",
		"model":      info.Model,
	}
	writeArkVideoJSON(c, http.StatusOK, out)

	return &OpenAIForwardResult{
		RequestID:     taskID,
		ResponseID:    taskID,
		Model:         info.Model,
		BillingModel:  info.Model,
		UpstreamModel: upstreamModel,
		Duration:      time.Since(start),
	}, nil
}

// ForwardArkVideoStatus queries a Seedance task and maps it to a normalized
// status payload. On completion it fills Usage (token-based) for billing.
func (s *OpenAIGatewayService) ForwardArkVideoStatus(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	requestID string,
) (*OpenAIForwardResult, error) {
	start := time.Now()
	if account == nil {
		return nil, fmt.Errorf("ark account is required")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(`{"error":{"message":"request_id is required"}}`)}
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	targetURL := arkVideoBaseURL(account.GetOpenAIBaseURL()) + arkVideoTasksPath + "/" + requestID

	respBody, status, respHeader, err := s.doArkVideoRequest(ctx, c, account, http.MethodGet, targetURL, token, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, s.arkVideoUpstreamError(c, account, status, respBody, respHeader)
	}

	upstreamStatus := strings.ToLower(strings.TrimSpace(gjson.GetBytes(respBody, "status").String()))
	videoURL := strings.TrimSpace(gjson.GetBytes(respBody, "content.video_url").String())
	model := strings.TrimSpace(gjson.GetBytes(respBody, "model").String())
	totalTokens := int(gjson.GetBytes(respBody, "usage.total_tokens").Int())

	normalized := map[string]any{
		"request_id": requestID,
		"id":         requestID,
	}
	if model != "" {
		normalized["model"] = model
	}
	switch upstreamStatus {
	case "succeeded":
		normalized["status"] = "completed"
		if videoURL != "" {
			normalized["url"] = videoURL
			normalized["video_url"] = videoURL
		}
	case "failed", "canceled", "cancelled":
		normalized["status"] = "failed"
		if em := strings.TrimSpace(gjson.GetBytes(respBody, "error.message").String()); em != "" {
			normalized["error"] = em
		}
	default:
		normalized["status"] = "processing"
	}
	if totalTokens > 0 {
		normalized["usage"] = map[string]any{"total_tokens": totalTokens}
	}
	writeArkVideoJSON(c, http.StatusOK, normalized)

	result := &OpenAIForwardResult{
		RequestID:     requestID,
		ResponseID:    requestID,
		Model:         model,
		BillingModel:  model,
		UpstreamModel: model,
		Duration:      time.Since(start),
	}
	// Bill by token usage only when the task has completed successfully.
	if upstreamStatus == "succeeded" && totalTokens > 0 {
		result.Usage = OpenAIUsage{OutputTokens: totalTokens}
	}
	return result, nil
}

func (s *OpenAIGatewayService) doArkVideoRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method, url, token string,
	body []byte,
) ([]byte, int, http.Header, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(upstreamCtx, method, url, reader)
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sub2api-ark/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, 0, nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, 0, nil, err
	}
	return respBody, resp.StatusCode, resp.Header.Clone(), nil
}

func (s *OpenAIGatewayService) arkVideoUpstreamError(
	c *gin.Context,
	account *Account,
	status int,
	body []byte,
	header http.Header,
) error {
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("ark upstream returned status %d", status)
	}
	setOpsUpstreamError(c, status, upstreamMsg, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: status,
		Kind:               "http_error",
		Message:            upstreamMsg,
	})
	if s.shouldFailoverUpstreamError(status) {
		return &UpstreamFailoverError{StatusCode: status, ResponseBody: body, ResponseHeaders: header}
	}
	MarkResponseCommitted(c)
	writeArkVideoJSON(c, status, map[string]any{
		"error": map[string]any{
			"type":    grokMediaErrorType(status),
			"message": upstreamMsg,
		},
	})
	return fmt.Errorf("ark video upstream error: %d %s", status, upstreamMsg)
}

// arkVideoBaseURL trims a trailing slash from the account base URL.
func arkVideoBaseURL(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "https://ark.cn-beijing.volces.com/api/v3"
	}
	return strings.TrimRight(base, "/")
}

func writeArkVideoJSON(c *gin.Context, status int, payload map[string]any) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(status, payload)
}

// ArkVideoRequestSessionHash derives the sticky-session hash binding a video
// task id to the account that created it, so status polls hit the same account.
func ArkVideoRequestSessionHash(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	return "ark-video:" + DeriveSessionHashFromSeed(requestID)
}

// BindArkVideoRequestAccount records the task id -> account binding.
func (s *OpenAIGatewayService) BindArkVideoRequestAccount(ctx context.Context, groupID *int64, requestID string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, ArkVideoRequestSessionHash(requestID), accountID)
}
