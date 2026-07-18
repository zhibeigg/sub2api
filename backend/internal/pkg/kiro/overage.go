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
	"time"
)

// kiroQAPIBase is the AWS Q Developer endpoint that owns the user-level Overages
// switch. Distinct from kiroRestAPIBase (CodeWhisperer) used elsewhere.
const kiroQAPIBase = "https://q.us-east-1.amazonaws.com"

// OverageSnapshot captures the upstream Overages state for an account.
type OverageSnapshot struct {
	Status            string  `json:"status"`             // "ENABLED" | "DISABLED" | "UNKNOWN"
	Capability        string  `json:"capability"`         // "OVERAGE_CAPABLE" | ...
	SubscriptionTitle string  `json:"subscription_title"` // e.g. "KIRO PRO+"
	OverageCap        float64 `json:"overage_cap"`        // USD upper bound
	OverageRate       float64 `json:"overage_rate"`       // per-invocation USD
	CurrentOverages   float64 `json:"current_overages"`   // accumulated overage USD
	CheckedAt         int64   `json:"checked_at"`         // Unix seconds
}

// upstreamOverageResponse mirrors the parts of /getUsageLimits needed for the
// Overages switch.
type upstreamOverageResponse struct {
	OverageConfiguration *struct {
		OverageStatus string `json:"overageStatus"`
	} `json:"overageConfiguration"`
	SubscriptionInfo *struct {
		OverageCapability string `json:"overageCapability"`
		SubscriptionTitle string `json:"subscriptionTitle"`
	} `json:"subscriptionInfo"`
	UsageBreakdownList []struct {
		ResourceType    string  `json:"resourceType"`
		OverageCap      float64 `json:"overageCap"`
		OverageRate     float64 `json:"overageRate"`
		CurrentOverages float64 `json:"currentOverages"`
	} `json:"usageBreakdownList"`
}

// FetchOverageStatus calls AWS Q GET /getUsageLimits and extracts the Overages
// switch state plus subscription metadata.
func FetchOverageStatus(ctx context.Context, cred *Credential) (*OverageSnapshot, error) {
	if cred == nil {
		return nil, fmt.Errorf("credential is nil")
	}

	rawURL := regionalizeRESTURL(kiroQAPIBase+"/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true", cred)
	if profileArn := strings.TrimSpace(cred.ProfileArn); profileArn != "" {
		rawURL += "&profileArn=" + url.QueryEscape(profileArn)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	setKiroRuntimeHeaders(req, cred)

	resp, err := restClient(cred).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed upstreamOverageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode getUsageLimits: %w", err)
	}

	snap := &OverageSnapshot{
		Status:    "UNKNOWN",
		CheckedAt: time.Now().Unix(),
	}
	if parsed.OverageConfiguration != nil && parsed.OverageConfiguration.OverageStatus != "" {
		snap.Status = strings.ToUpper(parsed.OverageConfiguration.OverageStatus)
	}
	if parsed.SubscriptionInfo != nil {
		snap.Capability = parsed.SubscriptionInfo.OverageCapability
		snap.SubscriptionTitle = parsed.SubscriptionInfo.SubscriptionTitle
	}
	for _, bd := range parsed.UsageBreakdownList {
		if bd.OverageCap > 0 || bd.OverageRate > 0 || bd.CurrentOverages > 0 {
			snap.OverageCap = bd.OverageCap
			snap.OverageRate = bd.OverageRate
			snap.CurrentOverages = bd.CurrentOverages
			break
		}
	}
	return snap, nil
}

// SetOverageStatus flips the user-level Overages switch via POST
// /setUserPreference, then re-fetches the snapshot for write-through accuracy.
func SetOverageStatus(ctx context.Context, cred *Credential, enabled bool) (*OverageSnapshot, error) {
	if cred == nil {
		return nil, fmt.Errorf("credential is nil")
	}

	profileArn, err := ResolveProfileArn(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("resolve profileArn: %w", err)
	}

	status := "DISABLED"
	if enabled {
		status = "ENABLED"
	}
	payload := map[string]any{
		"overageConfiguration": map[string]string{
			"overageStatus": status,
		},
		"profileArn": profileArn,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, regionalizeRESTURL(kiroQAPIBase+"/setUserPreference", cred), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setKiroRuntimeHeaders(req, cred)
	req.Header.Set("Content-Type", "application/json")

	resp, err := restClient(cred).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("setUserPreference HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Best-effort re-read so cached fields stay accurate.
	snap, fetchErr := FetchOverageStatus(ctx, cred)
	if fetchErr != nil {
		return &OverageSnapshot{Status: status, CheckedAt: time.Now().Unix()}, nil
	}
	snap.Status = status // AWS sometimes lags; force the just-set value.
	return snap, nil
}
