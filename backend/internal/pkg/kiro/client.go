package kiro

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

	"github.com/google/uuid"
)

// kiroEndpoint describes an upstream endpoint variant (auto-fallback on quota).
type kiroEndpoint struct {
	URL       string
	Origin    string
	AmzTarget string
	Name      string
}

var kiroEndpoints = []kiroEndpoint{
	{
		URL:       "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:    "AI_EDITOR",
		AmzTarget: "",
		Name:      "Kiro IDE",
	},
	{
		URL:       "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:    "AI_EDITOR",
		AmzTarget: "AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
		Name:      "CodeWhisperer",
	},
	{
		URL:       "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:    "AI_EDITOR",
		AmzTarget: "AmazonQDeveloperStreamingService.SendMessage",
		Name:      "AmazonQ",
	},
}

// httpClientCache caches http.Client instances keyed by "proxyURL|timeout".
var httpClientCache sync.Map

// GetHTTPClientForProxy returns a (cached) http.Client for the given proxy URL
// and timeout. An empty proxyURL uses the environment proxy.
func GetHTTPClientForProxy(proxyURL string, timeout time.Duration) *http.Client {
	key := fmt.Sprintf("%s|%s", proxyURL, timeout)
	if cached, ok := httpClientCache.Load(key); ok {
		if client, typeOK := cached.(*http.Client); typeOK {
			return client
		}
		httpClientCache.Delete(key)
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: buildTransport(proxyURL),
	}
	httpClientCache.Store(key, client)
	return client
}

func buildTransport(proxyURL string) *http.Transport {
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
			t.ForceAttemptHTTP2 = false // proxied connections cannot negotiate HTTP/2
		}
	} else {
		t.Proxy = http.ProxyFromEnvironment
	}
	return t
}

// StreamCallback receives incremental upstream events.
type StreamCallback struct {
	OnText         func(text string, isThinking bool)
	OnToolUse      func(toolUse KiroToolUse)
	OnComplete     func(inputTokens, outputTokens int)
	OnError        func(err error)
	OnCredits      func(credits float64)
	OnContextUsage func(percentage float64)
}

// APIError carries the upstream HTTP status so callers can map it to sub2api
// account/error handling (401/403/402/429).
type APIError struct {
	StatusCode int
	Endpoint   string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kiro: HTTP %d from %s: %s", e.StatusCode, e.Endpoint, e.Body)
}

// CallKiroAPI calls the Kiro streaming API, trying each endpoint with automatic
// fallback. It requires payload.ProfileArn (or cred.ProfileArn) to be set.
func CallKiroAPI(ctx context.Context, cred *Credential, payload *KiroPayload, callback *StreamCallback) error {
	if payload == nil {
		return fmt.Errorf("kiro: nil payload")
	}
	// Prefer credential's profileArn when the payload lacks one.
	if strings.TrimSpace(payload.ProfileArn) == "" && cred != nil {
		payload.ProfileArn = strings.TrimSpace(cred.ProfileArn)
	}

	// Wrap OnToolUse to restore original tool names for the client.
	if callback != nil && callback.OnToolUse != nil && len(payload.ToolNameMap) > 0 {
		originalOnToolUse := callback.OnToolUse
		nameMap := payload.ToolNameMap
		wrapped := *callback
		wrapped.OnToolUse = func(tu KiroToolUse) {
			if original, ok := nameMap[tu.Name]; ok {
				tu.Name = original
			}
			originalOnToolUse(tu)
		}
		callback = &wrapped
	}

	client := GetHTTPClientForProxy(proxyOf(cred), 5*time.Minute)

	var lastErr error
	for _, ep := range kiroEndpoints {
		payload.ConversationState.CurrentMessage.UserInputMessage.Origin = ep.Origin
		epURL := regionalizeURLForProfile(ep.URL, cred, payload.ProfileArn)

		reqBody, err := json.Marshal(payload)
		if err != nil {
			lastErr = fmt.Errorf("kiro: marshal request payload: %w", err)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, epURL, bytes.NewReader(reqBody))
		if err != nil {
			lastErr = err
			continue
		}

		host := ""
		if parsedURL, parseErr := url.Parse(epURL); parseErr == nil {
			host = parsedURL.Host
		}
		headerValues := buildStreamingHeaderValues(cred, host)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "*/*")
		if ep.AmzTarget != "" {
			req.Header.Set("X-Amz-Target", ep.AmzTarget)
		}
		applyKiroBaseHeaders(req, cred, headerValues)
		req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
		req.Header.Set("x-amzn-codewhisperer-optout", "true")
		req.Header.Set("Amz-Sdk-Request", "attempt=1; max=3")
		req.Header.Set("Amz-Sdk-Invocation-Id", uuid.New().String())

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 {
			body := "quota exhausted"
			if closeErr := resp.Body.Close(); closeErr != nil {
				body = fmt.Sprintf("%s; close response body: %v", body, closeErr)
			}
			lastErr = &APIError{StatusCode: 429, Endpoint: ep.Name, Body: body}
			continue
		}

		if resp.StatusCode != 200 {
			errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
			body := string(errBody)
			if readErr != nil {
				body = fmt.Sprintf("%s; read response body: %v", body, readErr)
			}
			if closeErr := resp.Body.Close(); closeErr != nil {
				body = fmt.Sprintf("%s; close response body: %v", body, closeErr)
			}
			apiErr := &APIError{StatusCode: resp.StatusCode, Endpoint: ep.Name, Body: body}
			lastErr = apiErr
			// Auth and payment errors are not retried across endpoints.
			if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 402 {
				return apiErr
			}
			continue
		}

		parseErr := parseEventStream(resp.Body, callback)
		closeErr := resp.Body.Close()
		if parseErr != nil {
			return parseErr
		}
		if closeErr != nil {
			return fmt.Errorf("kiro: close streaming response body: %w", closeErr)
		}
		return nil
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("kiro: all endpoints failed")
}

func proxyOf(cred *Credential) string {
	if cred != nil {
		return cred.ProxyURL
	}
	return ""
}

// regionalizeURLForProfile rewrites the host to the profile's data-plane region.
// Prefer profileArn because the OIDC region can differ from the profile region;
// fall back to the credential region when the ARN has not been resolved yet.
func regionalizeURLForProfile(rawURL string, cred *Credential, profileArn string) string {
	region := kiroRegionForProfile(cred, profileArn)
	if region == "us-east-1" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// CodeWhisperer host only exists in us-east-1; regional traffic uses q.{region}.
	if strings.HasPrefix(u.Host, "q.") || strings.HasPrefix(u.Host, "codewhisperer.") {
		u.Host = "q." + region + ".amazonaws.com"
	}
	return u.String()
}

func regionFromProfileArn(profileArn string) string {
	parts := strings.Split(profileArn, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}
