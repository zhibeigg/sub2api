package cursor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDashboardClientFetchUsageContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != dashboardUsagePath {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dashboard-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Connect-Protocol-Version"); got != "1" {
			t.Fatalf("Connect-Protocol-Version = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{}" {
			t.Fatalf("body = %q", string(body))
		}
		_, _ = w.Write([]byte(`{"enabled":true,"billingCycleStart":1000,"billingCycleEnd":2000,"planUsage":{"limit":2000,"totalSpend":20,"remaining":1980,"totalPercentUsed":1,"autoPercentUsed":0,"apiPercentUsed":1}}`))
	}))
	defer server.Close()

	client, err := NewDashboardClient(server.Client(), "dashboard-token", DashboardClientConfig{BaseURL: server.URL, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.PlanUsage == nil || usage.PlanUsage.TotalPercentUsed == nil || *usage.PlanUsage.TotalPercentUsed != 1 {
		t.Fatalf("usage = %#v", usage)
	}
	if usage.PlanUsage.APIPercentUsed == nil || *usage.PlanUsage.APIPercentUsed != 1 {
		t.Fatalf("api usage = %#v", usage.PlanUsage)
	}
}

func TestDashboardClientRefreshAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != dashboardRefreshPath {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["grant_type"] != "refresh_token" || body["client_id"] != dashboardOAuthClientID || body["refresh_token"] != "refresh-token" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer server.Close()

	client, err := NewDashboardClient(server.Client(), "old-access", DashboardClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RefreshAccessToken(context.Background(), "refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" || result.ExpiresIn != 3600 {
		t.Fatalf("refresh = %#v", result)
	}
}

func TestDashboardClientRejectsMissingRefreshAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"refresh_token":"new-refresh"}`))
	}))
	defer server.Close()
	client, err := NewDashboardClient(server.Client(), "old-access", DashboardClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RefreshAccessToken(context.Background(), "refresh-token"); err == nil {
		t.Fatal("expected protocol error")
	}
}
