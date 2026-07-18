package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/firefly"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AdobeMediaHandler owns the Adobe-only gateway surface. Runtime is injected through
// SetRuntime once the Firefly HTTP client adapter is available; catalog and cached task
// ownership remain available independently.
type AdobeMediaHandler struct {
	store              service.AdobeVideoTaskStore
	runtime            *service.AdobeVideoService
	gateway            *service.OpenAIGatewayService
	billing            *service.BillingCacheService
	apiKeys            *service.APIKeyService
	ops                *service.OpsService
	firefly            *service.AdobeFireflyAdapter
	contentModeration  *service.ContentModerationService
	concurrency        *ConcurrencyHelper
	maxAccountSwitches int
}

func NewAdobeMediaHandler(store service.AdobeVideoTaskStore, runtime *service.AdobeVideoService, gateway *service.OpenAIGatewayService, billing *service.BillingCacheService, apiKeys *service.APIKeyService, ops *service.OpsService, fireflyAdapter *service.AdobeFireflyAdapter, contentModeration *service.ContentModerationService, concurrencyService *service.ConcurrencyService, cfg *config.Config) *AdobeMediaHandler {
	var concurrency *ConcurrencyHelper
	if concurrencyService != nil {
		concurrency = NewConcurrencyHelper(concurrencyService, "", 0)
	}
	maxAccountSwitches := 3
	if cfg != nil && cfg.Gateway.MaxAccountSwitches > 0 {
		maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
	}
	return &AdobeMediaHandler{
		store: store, runtime: runtime, gateway: gateway, billing: billing, apiKeys: apiKeys,
		ops: ops, firefly: fireflyAdapter, contentModeration: contentModeration, concurrency: concurrency,
		maxAccountSwitches: maxAccountSwitches,
	}
}

func (h *AdobeMediaHandler) SetRuntime(runtime *service.AdobeVideoService) { h.runtime = runtime }

func (h *AdobeMediaHandler) checkContentModeration(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, model, prompt string) bool {
	if h == nil || h.contentModeration == nil {
		return true
	}
	body, err := json.Marshal(map[string]string{"model": strings.TrimSpace(model), "prompt": strings.TrimSpace(prompt)})
	if err != nil {
		writeAdobeGatewayError(c, http.StatusInternalServerError, "api_error", "Failed to prepare content moderation input")
		return false
	}
	decision := runContentModeration(c, nil, h.contentModeration, apiKey, subject, service.ContentModerationProtocolOpenAIImages, model, body)
	if decision != nil && decision.Blocked {
		writeAdobeGatewayError(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return false
	}
	return true
}

func (h *AdobeMediaHandler) acquireUserSlot(c *gin.Context, subject middleware2.AuthSubject) (func(), bool) {
	if h == nil || h.concurrency == nil {
		return nil, true
	}
	streamStarted := false
	release, err := h.concurrency.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		status, typ, message := concurrencyErrorResponse(err, "user")
		writeAdobeGatewayError(c, status, typ, message)
		return nil, false
	}
	return wrapReleaseOnDone(c.Request.Context(), release), true
}

func (h *AdobeMediaHandler) acquireAccountSlot(c *gin.Context, selection *service.AccountSelectionResult) (func(), bool) {
	if selection == nil || selection.Account == nil {
		markOpsRoutingCapacityLimited(c)
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "No Adobe account is available")
		return nil, false
	}
	if selection.Acquired {
		return wrapReleaseOnDone(c.Request.Context(), selection.ReleaseFunc), true
	}
	if selection.WaitPlan == nil || h == nil || h.concurrency == nil {
		markOpsRoutingCapacityLimited(c)
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "No Adobe account concurrency slot is available")
		return nil, false
	}

	ctx := c.Request.Context()
	account := selection.Account
	fastRelease, acquired, err := h.concurrency.TryAcquireAccountSlot(ctx, account.ID, selection.WaitPlan.MaxConcurrency)
	if err != nil {
		status, typ, message := concurrencyErrorResponse(err, "account")
		writeAdobeGatewayError(c, status, typ, message)
		return nil, false
	}
	if acquired {
		return wrapReleaseOnDone(ctx, fastRelease), true
	}

	canWait, waitErr := h.concurrency.IncrementAccountWaitCount(ctx, account.ID, selection.WaitPlan.MaxWaiting)
	if waitErr != nil {
		status, typ, message := concurrencyErrorResponse(waitErr, "account")
		writeAdobeGatewayError(c, status, typ, message)
		return nil, false
	}
	if !canWait {
		status, typ, message := concurrencyErrorResponse(&WaitQueueFullError{SlotType: "account"}, "account")
		writeAdobeGatewayError(c, status, typ, message)
		return nil, false
	}
	defer h.concurrency.DecrementAccountWaitCount(ctx, account.ID)

	streamStarted := false
	release, err := h.concurrency.AcquireAccountSlotWithWaitTimeout(c, account.ID, selection.WaitPlan.MaxConcurrency, selection.WaitPlan.Timeout, false, &streamStarted)
	if err != nil {
		status, typ, message := concurrencyErrorResponse(err, "account")
		writeAdobeGatewayError(c, status, typ, message)
		return nil, false
	}
	return wrapReleaseOnDone(ctx, release), true
}

func (h *AdobeMediaHandler) Models(c *gin.Context) {
	allowed := map[string]struct{}{}
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	custom := apiKey != nil && apiKey.Group != nil && apiKey.Group.CustomModelsListEnabled()
	if custom {
		for _, model := range apiKey.Group.ModelsListConfig.Models {
			allowed[strings.TrimSpace(model)] = struct{}{}
		}
	}
	data := make([]gin.H, 0)
	for _, model := range firefly.PublicModels() {
		if custom {
			if _, ok := allowed[model.ID]; !ok {
				continue
			}
		}
		data = append(data, gin.H{"id": model.ID, "object": "model", "owned_by": "adobe", "type": model.Type, "display_name": model.DisplayName})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

func (h *AdobeMediaHandler) Images(c *gin.Context) {
	if h == nil || h.firefly == nil || h.gateway == nil || h.billing == nil {
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Adobe Firefly image runtime is not initialized")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.User == nil {
		writeAdobeGatewayError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		writeAdobeGatewayError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	req, err := parseAdobeImageRequest(c)
	if err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	resolved, err := firefly.ResolveImageModel(req.Model, req.Size, req.Quality)
	if err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	setOpsRequestContext(c, req.Model, false)
	setOpsEndpointContext(c, resolved.ModelID, int16(service.RequestTypeFromLegacy(false, false)))
	if !h.checkContentModeration(c, apiKey, subject, req.Model, req.Prompt) {
		return
	}
	userRelease, acquired := h.acquireUserSlot(c, subject)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billing.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.PlatformAdobe); err != nil {
		writeAdobeGatewayError(c, http.StatusPaymentRequired, "billing_error", err.Error())
		return
	}
	loadedReferences, err := loadAdobeReferences(c.Request.Context(), req.References)
	if err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastUpstreamErr error
	for {
		selection, _, selectErr := h.gateway.SelectAccountWithSchedulerForCapability(c.Request.Context(), apiKey.GroupID, "", "", req.Model, failedAccountIDs, service.OpenAIUpstreamTransportHTTPSSE, "", false, false, false, service.PlatformAdobe)
		if selectErr != nil || selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimitedIfNoAvailable(c, selectErr)
			if lastUpstreamErr != nil {
				writeAdobeProviderError(c, lastUpstreamErr)
			} else {
				writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "No Adobe account is available")
			}
			return
		}
		accountRelease, acquired := h.acquireAccountSlot(c, selection)
		if !acquired {
			return
		}
		releaseAccount := func() {
			if accountRelease != nil {
				accountRelease()
				accountRelease = nil
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, service.PlatformAdobe)
		upstreamModel := account.GetMappedModel(req.Model)
		if upstreamModel == "" {
			upstreamModel = req.Model
		}
		attemptParams := resolved
		if upstreamModel != req.Model {
			attemptParams, err = firefly.ResolveImageModel(upstreamModel, req.Size, req.Quality)
			if err != nil {
				releaseAccount()
				writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", "Mapped Adobe model is unsupported")
				return
			}
		}
		setOpsEndpointContext(c, upstreamModel, int16(service.RequestTypeFromLegacy(false, false)))
		tier := strings.ToUpper(attemptParams.Quality)
		snapshot, priceErr := h.gateway.ResolveAdobeMediaPricingSnapshot(c.Request.Context(), service.ResolveAdobeMediaPricingInput{APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription, BillingMode: service.BillingModeImage, Tier: tier, Quantity: 1, RequestedModel: req.Model, ChannelModel: req.Model, UpstreamModel: upstreamModel})
		if priceErr != nil {
			releaseAccount()
			writeAdobeGatewayError(c, http.StatusBadRequest, "configuration_error", "Adobe image price is not configured for the resolved tier")
			return
		}
		assetIDs := make([]string, 0, len(loadedReferences))
		var attemptErr error
		for _, ref := range loadedReferences {
			assetID, uploadErr := h.firefly.UploadAsset(c.Request.Context(), account, ref.Name, ref.ContentType, ref.Data)
			if uploadErr != nil {
				attemptErr = uploadErr
				break
			}
			assetIDs = append(assetIDs, assetID)
		}
		var result *firefly.ImageResult
		if attemptErr == nil {
			result, attemptErr = h.firefly.GenerateImage(c.Request.Context(), account, attemptParams, req.Prompt, assetIDs)
		}
		if attemptErr != nil {
			releaseAccount()
			h.gateway.ReportOpenAIAccountScheduleResult(account.ID, upstreamModel, false, nil)
			h.gateway.HandleAdobeAccountFailure(c.Request.Context(), account.ID, attemptErr)
			lastUpstreamErr = attemptErr
			if c.Request.Context().Err() == nil && shouldFailoverAdobeError(attemptErr) && switchCount < h.maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				switchCount++
				h.gateway.RecordOpenAIAccountSwitch()
				continue
			}
			writeAdobeProviderError(c, attemptErr)
			return
		}
		if result == nil || strings.TrimSpace(result.TaskID) == "" || strings.TrimSpace(result.URL) == "" {
			releaseAccount()
			writeAdobeGatewayError(c, http.StatusBadGateway, "upstream_error", "Adobe image generation completed without a usable output")
			return
		}
		item := gin.H{}
		if req.ResponseFormat == "b64_json" {
			png, downloadErr := service.DownloadAdobePNG(c.Request.Context(), result.URL)
			if downloadErr != nil {
				releaseAccount()
				writeAdobeGatewayError(c, http.StatusBadGateway, "upstream_error", "Generated image could not be converted to b64_json")
				return
			}
			item["b64_json"] = base64.StdEncoding.EncodeToString(png)
		} else {
			item["url"] = result.URL
		}
		_, err = h.gateway.RecordMediaUsageFromSnapshot(c.Request.Context(), &service.RecordMediaUsageFromSnapshotInput{Snapshot: *snapshot, RequestID: result.TaskID, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription, InboundEndpoint: GetInboundEndpoint(c), UpstreamEndpoint: EndpointAdobeImageSubmit, UserAgent: c.GetHeader("User-Agent"), IPAddress: c.ClientIP(), APIKeyService: h.apiKeys})
		releaseAccount()
		if err != nil {
			if errors.Is(err, service.ErrAdobeMediaInsufficientFunds) {
				writeAdobeGatewayError(c, http.StatusPaymentRequired, "billing_error", "Insufficient balance or quota")
				return
			}
			writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Image usage settlement failed")
			return
		}
		h.gateway.ReportOpenAIAccountScheduleResult(account.ID, upstreamModel, true, nil)
		c.JSON(http.StatusOK, gin.H{"created": time.Now().Unix(), "data": []gin.H{item}})
		return
	}
}

type adobeImageReference struct {
	URL, Name, ContentType string
	Data                   []byte
}

type loadedAdobeReference struct {
	Name        string
	ContentType string
	Data        []byte
}

func loadAdobeReferences(ctx context.Context, refs []adobeImageReference) ([]loadedAdobeReference, error) {
	loaded := make([]loadedAdobeReference, 0, len(refs))
	for _, ref := range refs {
		data, contentType, name, err := ref.load(ctx)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, loadedAdobeReference{Name: name, ContentType: contentType, Data: data})
	}
	return loaded, nil
}

func shouldFailoverAdobeError(err error) bool {
	return err != nil && (firefly.IsAuthError(err) || firefly.IsRetryableError(err))
}

func (r adobeImageReference) load(ctx context.Context) ([]byte, string, string, error) {
	if strings.TrimSpace(r.URL) != "" {
		body, ct, err := service.DownloadAdobeReferenceImage(ctx, r.URL)
		return body, ct, "reference", err
	}
	if len(r.Data) == 0 {
		return nil, "", "", fmt.Errorf("empty reference image")
	}
	if len(r.Data) > 20<<20 {
		return nil, "", "", fmt.Errorf("reference image exceeds size limit")
	}
	detected := http.DetectContentType(r.Data)
	ct := strings.ToLower(strings.TrimSpace(r.ContentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	if ct == "" {
		ct = detected
	}
	if ct != "image/png" && ct != "image/jpeg" && ct != "image/webp" {
		return nil, "", "", fmt.Errorf("unsupported reference image MIME type")
	}
	if detected != ct {
		return nil, "", "", fmt.Errorf("reference image MIME mismatch")
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = "reference"
	}
	return r.Data, ct, name, nil
}

type adobeImageRequest struct {
	Model, Prompt, Size, Quality, ResponseFormat string
	References                                   []adobeImageReference
}

func parseAdobeImageRequest(c *gin.Context) (*adobeImageRequest, error) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseAdobeImageMultipart(c)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON request")
	}
	for _, key := range []string{"stream", "mask", "background", "style", "moderation"} {
		if _, exists := raw[key]; exists {
			return nil, fmt.Errorf("%s is not supported for Adobe images", key)
		}
	}
	var req adobeImageRequest
	decodeString := func(key string) string {
		var value string
		_ = json.Unmarshal(raw[key], &value)
		return strings.TrimSpace(value)
	}
	req.Model, req.Prompt, req.Size, req.Quality = decodeString("model"), decodeString("prompt"), decodeString("size"), decodeString("quality")
	if req.Model == "" {
		req.Model = "nano-banana-pro"
	}
	var n int
	if value, ok := raw["n"]; ok {
		if json.Unmarshal(value, &n) != nil || n != 1 {
			return nil, fmt.Errorf("n must be 1")
		}
	}
	outputFormat := strings.ToLower(decodeString("output_format"))
	if outputFormat != "" && outputFormat != "png" {
		return nil, fmt.Errorf("only PNG output is supported")
	}
	req.ResponseFormat = strings.ToLower(decodeString("response_format"))
	if req.ResponseFormat == "" {
		req.ResponseFormat = "url"
	}
	if req.ResponseFormat != "url" && req.ResponseFormat != "b64_json" {
		return nil, fmt.Errorf("response_format must be url or b64_json")
	}
	for _, key := range []string{"image", "images"} {
		if value, ok := raw[key]; ok {
			refs, err := parseAdobeJSONReferences(value)
			if err != nil {
				return nil, err
			}
			req.References = append(req.References, refs...)
		}
	}
	if len(req.References) > 3 {
		return nil, fmt.Errorf("too many reference images")
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	return &req, nil
}

func parseAdobeJSONReferences(raw json.RawMessage) ([]adobeImageReference, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("invalid image reference")
	}
	refs := []adobeImageReference{}
	var appendValue func(any) error
	appendValue = func(v any) error {
		switch x := v.(type) {
		case string:
			refs = append(refs, adobeImageReference{URL: x})
		case []any:
			for _, item := range x {
				if err := appendValue(item); err != nil {
					return err
				}
			}
		case map[string]any:
			if u, _ := x["image_url"].(string); u != "" {
				refs = append(refs, adobeImageReference{URL: u})
				return nil
			}
			if imageURL, ok := x["image_url"].(map[string]any); ok {
				if u, _ := imageURL["url"].(string); u != "" {
					refs = append(refs, adobeImageReference{URL: u})
					return nil
				}
			}
			if u, _ := x["url"].(string); u != "" {
				refs = append(refs, adobeImageReference{URL: u})
				return nil
			}
			return fmt.Errorf("image reference URL is required")
		default:
			return fmt.Errorf("invalid image reference")
		}
		return nil
	}
	return refs, appendValue(value)
}

func parseAdobeImageMultipart(c *gin.Context) (*adobeImageRequest, error) {
	reader, err := c.Request.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("invalid multipart request")
	}
	req := &adobeImageRequest{ResponseFormat: "url"}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("invalid multipart request")
		}
		name := part.FormName()
		data, readErr := io.ReadAll(io.LimitReader(part, (20<<20)+1))
		_ = part.Close()
		if readErr != nil || len(data) > 20<<20 {
			return nil, fmt.Errorf("multipart field exceeds size limit")
		}
		if name == "mask" {
			return nil, fmt.Errorf("mask is not supported for Adobe images")
		}
		if part.FileName() != "" {
			if name != "image" && !strings.HasPrefix(name, "image[") {
				continue
			}
			req.References = append(req.References, adobeImageReference{Name: part.FileName(), ContentType: part.Header.Get("Content-Type"), Data: data})
			continue
		}
		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			req.Model = value
		case "prompt":
			req.Prompt = value
		case "size":
			req.Size = value
		case "quality":
			req.Quality = value
		case "response_format":
			req.ResponseFormat = strings.ToLower(value)
		case "output_format":
			if value != "" && !strings.EqualFold(value, "png") {
				return nil, fmt.Errorf("only PNG output is supported")
			}
		case "n":
			if value != "" && value != "1" {
				return nil, fmt.Errorf("n must be 1")
			}
		case "stream", "background", "style", "moderation":
			return nil, fmt.Errorf("%s is not supported for Adobe images", name)
		case "image_url":
			if value != "" {
				req.References = append(req.References, adobeImageReference{URL: value})
			}
		}
	}
	if req.Model == "" {
		req.Model = "nano-banana-pro"
	}
	if req.ResponseFormat != "url" && req.ResponseFormat != "b64_json" {
		return nil, fmt.Errorf("response_format must be url or b64_json")
	}
	if len(req.References) > 3 {
		return nil, fmt.Errorf("too many reference images")
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	return req, nil
}

type adobeVideoRequest struct {
	Model         string
	Prompt        string
	Input         string
	Resolution    string
	Size          string
	Duration      int
	Seconds       int
	AspectRatio   string
	Ratio         string
	GenerateAudio *bool
	ReferenceMode string
	N             int
	References    []adobeImageReference
}

func parseAdobeVideoRequest(c *gin.Context) (*adobeVideoRequest, error) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseAdobeVideoMultipart(c)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON request")
	}
	if _, exists := raw["stream"]; exists {
		return nil, fmt.Errorf("stream is not supported for Adobe videos")
	}
	decodeString := func(key string) string {
		var value string
		_ = json.Unmarshal(raw[key], &value)
		return strings.TrimSpace(value)
	}
	decodeInt := func(key string) (int, error) {
		value, ok := raw[key]
		if !ok {
			return 0, nil
		}
		var out int
		if err := json.Unmarshal(value, &out); err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return out, nil
	}
	req := &adobeVideoRequest{
		Model: decodeString("model"), Prompt: decodeString("prompt"), Input: decodeString("input"),
		Resolution: decodeString("resolution"), Size: decodeString("size"),
		AspectRatio: decodeString("aspect_ratio"), Ratio: decodeString("ratio"),
		ReferenceMode: decodeString("reference_mode"),
	}
	var err error
	if req.Duration, err = decodeInt("duration"); err != nil {
		return nil, err
	}
	if req.Seconds, err = decodeInt("seconds"); err != nil {
		return nil, err
	}
	if req.N, err = decodeInt("n"); err != nil {
		return nil, err
	}
	if value, ok := raw["generate_audio"]; ok {
		var enabled bool
		if err := json.Unmarshal(value, &enabled); err != nil {
			return nil, fmt.Errorf("generate_audio must be a boolean")
		}
		req.GenerateAudio = &enabled
	}
	for _, key := range []string{"image", "images"} {
		if value, ok := raw[key]; ok && string(value) != "null" {
			refs, err := parseAdobeJSONReferences(value)
			if err != nil {
				return nil, err
			}
			req.References = append(req.References, refs...)
		}
	}
	return validateAdobeVideoRequest(req)
}

func parseAdobeVideoMultipart(c *gin.Context) (*adobeVideoRequest, error) {
	reader, err := c.Request.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("invalid multipart request")
	}
	req := &adobeVideoRequest{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("invalid multipart request")
		}
		name := part.FormName()
		fileName := part.FileName()
		contentType := part.Header.Get("Content-Type")
		data, readErr := io.ReadAll(io.LimitReader(part, (20<<20)+1))
		_ = part.Close()
		if readErr != nil || len(data) > 20<<20 {
			return nil, fmt.Errorf("multipart field exceeds size limit")
		}
		if fileName != "" {
			if name == "image" || name == "images" || strings.HasPrefix(name, "image[") || strings.HasPrefix(name, "images[") {
				req.References = append(req.References, adobeImageReference{Name: fileName, ContentType: contentType, Data: data})
			}
			continue
		}
		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			req.Model = value
		case "prompt":
			req.Prompt = value
		case "input":
			req.Input = value
		case "resolution":
			req.Resolution = value
		case "size":
			req.Size = value
		case "aspect_ratio":
			req.AspectRatio = value
		case "ratio":
			req.Ratio = value
		case "reference_mode":
			req.ReferenceMode = value
		case "duration":
			if value != "" {
				req.Duration, err = strconv.Atoi(value)
				if err != nil {
					return nil, fmt.Errorf("duration must be an integer")
				}
			}
		case "seconds":
			if value != "" {
				req.Seconds, err = strconv.Atoi(value)
				if err != nil {
					return nil, fmt.Errorf("seconds must be an integer")
				}
			}
		case "n":
			if value != "" {
				req.N, err = strconv.Atoi(value)
				if err != nil {
					return nil, fmt.Errorf("n must be an integer")
				}
			}
		case "generate_audio":
			if value != "" {
				enabled, parseErr := strconv.ParseBool(value)
				if parseErr != nil {
					return nil, fmt.Errorf("generate_audio must be a boolean")
				}
				req.GenerateAudio = &enabled
			}
		case "image", "image_url":
			if value != "" {
				req.References = append(req.References, adobeImageReference{URL: value})
			}
		case "images":
			if value != "" {
				refs, parseErr := parseAdobeJSONReferences(json.RawMessage(value))
				if parseErr != nil {
					return nil, parseErr
				}
				req.References = append(req.References, refs...)
			}
		case "stream":
			return nil, fmt.Errorf("stream is not supported for Adobe videos")
		}
	}
	return validateAdobeVideoRequest(req)
}

func validateAdobeVideoRequest(req *adobeVideoRequest) (*adobeVideoRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("invalid video request")
	}
	if req.N != 0 && req.N != 1 {
		return nil, fmt.Errorf("n must be 1")
	}
	if len(req.References) > 3 {
		return nil, fmt.Errorf("too many reference images")
	}
	return req, nil
}

func (h *AdobeMediaHandler) VideoGeneration(c *gin.Context) {
	if h == nil || h.runtime == nil || h.gateway == nil || h.billing == nil {
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Adobe Firefly video runtime is not initialized")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.User == nil {
		writeAdobeGatewayError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		writeAdobeGatewayError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	req, err := parseAdobeVideoRequest(c)
	if err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(req.Input)
	}
	if prompt == "" {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", "prompt or input is required")
		return
	}
	resolution := strings.TrimSpace(req.Resolution)
	if resolution == "" {
		resolution = strings.TrimSpace(req.Size)
	}
	duration := req.Duration
	if duration <= 0 {
		duration = req.Seconds
	}
	aspect := strings.TrimSpace(req.AspectRatio)
	if aspect == "" {
		aspect = strings.TrimSpace(req.Ratio)
	}
	generateAudio := true
	if req.GenerateAudio != nil {
		generateAudio = *req.GenerateAudio
	}
	requestedModel := strings.TrimSpace(req.Model)
	resolved, err := firefly.ResolveVideoModel(requestedModel, resolution, duration, aspect, req.GenerateAudio, req.ReferenceMode)
	if err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if requestedModel == "" {
		requestedModel = resolved.ModelID
	}
	if err := firefly.ValidateVideoReferenceCount(resolved, len(req.References)); err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	setOpsRequestContext(c, requestedModel, false)
	setOpsEndpointContext(c, resolved.ModelID, int16(service.RequestTypeFromLegacy(false, false)))
	if !h.checkContentModeration(c, apiKey, subject, requestedModel, prompt) {
		return
	}
	userRelease, acquired := h.acquireUserSlot(c, subject)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billing.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.PlatformAdobe); err != nil {
		writeAdobeGatewayError(c, http.StatusPaymentRequired, "billing_error", err.Error())
		return
	}
	if err := h.runtime.Preflight(c.Request.Context()); err != nil {
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Adobe video task storage is unavailable")
		return
	}
	if len(req.References) > 0 && h.firefly == nil {
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Adobe Firefly asset runtime is not initialized")
		return
	}
	loadedReferences, err := loadAdobeReferences(c.Request.Context(), req.References)
	if err != nil {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastUpstreamErr error
	for {
		selection, _, selectErr := h.gateway.SelectAccountWithSchedulerForCapability(c.Request.Context(), apiKey.GroupID, "", "", requestedModel, failedAccountIDs, service.OpenAIUpstreamTransportHTTPSSE, "", false, false, false, service.PlatformAdobe)
		if selectErr != nil || selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimitedIfNoAvailable(c, selectErr)
			if lastUpstreamErr != nil {
				writeAdobeProviderError(c, lastUpstreamErr)
			} else {
				writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "No Adobe account is available")
			}
			return
		}
		accountRelease, acquired := h.acquireAccountSlot(c, selection)
		if !acquired {
			return
		}
		releaseAccount := func() {
			if accountRelease != nil {
				accountRelease()
				accountRelease = nil
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, service.PlatformAdobe)
		upstreamModel := account.GetMappedModel(requestedModel)
		if upstreamModel == "" {
			upstreamModel = requestedModel
		}
		attemptParams := resolved
		if upstreamModel != requestedModel {
			attemptParams, err = firefly.ResolveVideoModel(upstreamModel, resolution, duration, aspect, req.GenerateAudio, req.ReferenceMode)
			if err != nil {
				releaseAccount()
				writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", "Mapped Adobe model is unsupported")
				return
			}
		}
		setOpsEndpointContext(c, upstreamModel, int16(service.RequestTypeFromLegacy(false, false)))
		if err := firefly.ValidateVideoReferenceCount(attemptParams, len(loadedReferences)); err != nil {
			releaseAccount()
			writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		snapshot, priceErr := h.gateway.ResolveAdobeMediaPricingSnapshot(c.Request.Context(), service.ResolveAdobeMediaPricingInput{
			APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
			BillingMode: service.BillingModeVideo, Tier: attemptParams.VideoResolution, Quantity: attemptParams.Duration,
			RequestedModel: requestedModel, ChannelModel: requestedModel, UpstreamModel: upstreamModel,
		})
		if priceErr != nil {
			releaseAccount()
			writeAdobeGatewayError(c, http.StatusBadRequest, "configuration_error", "Adobe video price is not configured for the resolved tier")
			return
		}
		assetIDs := make([]string, 0, len(loadedReferences))
		var attemptErr error
		for _, ref := range loadedReferences {
			assetID, uploadErr := h.firefly.UploadAsset(c.Request.Context(), account, ref.Name, ref.ContentType, ref.Data)
			if uploadErr != nil {
				attemptErr = uploadErr
				break
			}
			assetIDs = append(assetIDs, assetID)
		}
		var task *service.AdobeVideoTask
		if attemptErr == nil {
			task, attemptErr = h.runtime.Submit(c.Request.Context(), &service.SubmitAdobeVideoInput{Account: account, APIKey: apiKey, User: apiKey.User, Subscription: subscription, Snapshot: *snapshot, Request: service.AdobeVideoSubmitRequest{Model: upstreamModel, Prompt: prompt, Resolution: attemptParams.VideoResolution, DurationSeconds: attemptParams.Duration, AspectRatio: attemptParams.AspectRatio, GenerateAudio: generateAudio, ReferenceMode: attemptParams.ReferenceMode, ReferenceAssets: assetIDs}})
		}
		if attemptErr != nil {
			releaseAccount()
			if task != nil && strings.TrimSpace(task.TaskID) != "" {
				h.recordCriticalTaskOps(c, apiKey, account, task.TaskID, requestedModel, "adobe_video_orphan", "Firefly accepted task but Redis persistence failed")
				writeAdobeProviderError(c, attemptErr)
				return
			}
			h.gateway.ReportOpenAIAccountScheduleResult(account.ID, upstreamModel, false, nil)
			h.gateway.HandleAdobeAccountFailure(c.Request.Context(), account.ID, attemptErr)
			lastUpstreamErr = attemptErr
			if c.Request.Context().Err() == nil && shouldFailoverAdobeError(attemptErr) && switchCount < h.maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				switchCount++
				h.gateway.RecordOpenAIAccountSwitch()
				continue
			}
			writeAdobeProviderError(c, attemptErr)
			return
		}
		releaseAccount()
		h.gateway.ReportOpenAIAccountScheduleResult(account.ID, upstreamModel, true, nil)
		c.JSON(http.StatusOK, gin.H{"request_id": task.TaskID, "id": task.TaskID, "status": "pending", "model": requestedModel})
		return
	}
}

func (h *AdobeMediaHandler) VideoStatus(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		writeAdobeGatewayError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	taskID := strings.TrimSpace(c.Param("request_id"))
	if taskID == "" {
		writeAdobeGatewayError(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
	}
	if h == nil || h.runtime == nil {
		if h != nil && h.store != nil {
			task, err := h.store.Get(c.Request.Context(), taskID)
			if err == nil {
				if task.UserID != apiKey.UserID || task.GroupID != apiKey.Group.ID {
					writeAdobeGatewayError(c, http.StatusForbidden, "permission_error", "Video task does not belong to this user and group")
					return
				}
				if task.CanExposeResult() {
					c.JSON(http.StatusOK, gin.H{"request_id": task.TaskID, "id": task.TaskID, "status": task.Status, "model": task.RequestedModel, "urls": task.ResultURLs})
					return
				}
				if task.Status == service.AdobeVideoTaskFailed || task.Status == service.AdobeVideoTaskCanceled {
					c.JSON(http.StatusOK, gin.H{"request_id": task.TaskID, "id": task.TaskID, "status": task.Status, "model": task.RequestedModel, "error": task.LastError})
					return
				}
			}
		}
		writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Adobe Firefly video runtime is not initialized")
		return
	}
	result, err := h.runtime.GetStatus(c.Request.Context(), &service.GetAdobeVideoStatusInput{TaskID: taskID, CurrentUserID: apiKey.UserID, CurrentGroupID: apiKey.Group.ID, InboundEndpoint: GetInboundEndpoint(c), UpstreamEndpoint: EndpointAdobeVideoStatus, UserAgent: c.GetHeader("User-Agent"), IPAddress: c.ClientIP(), APIKeyService: h.apiKeys})
	if err != nil {
		if errors.Is(err, service.ErrAdobeVideoTaskImmutableConflict) || errors.Is(err, service.ErrAdobeMediaSnapshotInvalid) || errors.Is(err, service.ErrAdobeMediaSnapshotConflict) || errors.Is(err, service.ErrUsageBillingRequestConflict) {
			h.recordCriticalTaskOps(c, apiKey, nil, taskID, "", "adobe_video_integrity_conflict", "Adobe task identity or pricing snapshot conflict; settlement blocked")
		}
		switch err {
		case service.ErrAdobeVideoTaskNotFound:
			writeAdobeGatewayError(c, http.StatusNotFound, "not_found_error", "Video task not found")
		case service.ErrAdobeVideoTaskOwnerMismatch:
			writeAdobeGatewayError(c, http.StatusForbidden, "permission_error", "Video task does not belong to this user and group")
		case service.ErrAdobeMediaInsufficientFunds:
			writeAdobeGatewayError(c, http.StatusPaymentRequired, "billing_error", "Insufficient balance or quota to settle completed video")
		default:
			writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Video status or settlement is temporarily unavailable")
		}
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *AdobeMediaHandler) Unsupported(c *gin.Context) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	writeAdobeGatewayError(c, http.StatusNotFound, "not_found_error", "This API is not supported for Adobe groups")
}

func (h *AdobeMediaHandler) recordCriticalTaskOps(c *gin.Context, apiKey *service.APIKey, account *service.Account, taskID, model, errorType, message string) {
	if h == nil || h.ops == nil || apiKey == nil {
		return
	}
	userID, apiKeyID := apiKey.UserID, apiKey.ID
	var groupID *int64
	if apiKey.Group != nil {
		id := apiKey.Group.ID
		groupID = &id
	}
	var accountID *int64
	if account != nil {
		id := account.ID
		accountID = &id
	}
	_ = h.ops.RecordError(c.Request.Context(), &service.OpsInsertErrorLogInput{
		RequestID: taskID, UserID: &userID, APIKeyID: &apiKeyID, AccountID: accountID, GroupID: groupID,
		Platform: service.PlatformAdobe, Model: model, RequestPath: c.Request.URL.Path,
		InboundEndpoint: GetInboundEndpoint(c), UpstreamEndpoint: GetUpstreamEndpoint(c, service.PlatformAdobe),
		ErrorPhase: "upstream", ErrorType: errorType, Severity: "critical", StatusCode: http.StatusServiceUnavailable,
		ErrorMessage: message, ErrorSource: "adobe_gateway", ErrorOwner: "system",
	})
}

func writeAdobeProviderError(c *gin.Context, err error) {
	var providerErr *firefly.ProviderError
	if errors.As(err, &providerErr) {
		status := providerErr.HTTPStatus
		if status <= 0 {
			status = http.StatusBadGateway
		}
		typ := "upstream_error"
		if providerErr.Kind == firefly.ErrorRequest || providerErr.Kind == firefly.ErrorContentPolicy {
			typ = "invalid_request_error"
		}
		writeAdobeGatewayError(c, status, typ, providerErr.Message)
		return
	}
	writeAdobeGatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "Adobe task submission could not be persisted or completed")
}

func writeAdobeGatewayError(c *gin.Context, status int, typ, message string) {
	c.JSON(status, gin.H{"error": gin.H{"type": typ, "message": message}})
}
