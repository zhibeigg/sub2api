package cursor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateDashboardLoginPKCEAndBuildURL(t *testing.T) {
	flow, err := GenerateDashboardLoginPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if flow.UUID == "" || len(flow.Verifier) < 40 || len(flow.Challenge) < 40 {
		t.Fatalf("unexpected flow metadata")
	}
	client, err := NewDashboardAuthClient(http.DefaultClient, DashboardAuthClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.BuildLoginURL(flow, "login")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "cursor.com" || parsed.Path != dashboardLoginPath {
		t.Fatalf("unexpected URL %s", raw)
	}
	if parsed.Query().Get("uuid") != flow.UUID || parsed.Query().Get("challenge") != flow.Challenge || parsed.Query().Get("supportsSelectedTeamLogin") != "true" {
		t.Fatalf("unexpected query %s", parsed.RawQuery)
	}
}

func TestDashboardAuthPollPendingAndCompleted(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != dashboardPollPath || r.URL.Query().Get("uuid") != "uuid" || r.URL.Query().Get("verifier") != "verifier" {
			t.Fatalf("unexpected poll request %s", r.URL.String())
		}
		if calls == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"accessToken":"access","refreshToken":"refresh","authId":"auth","selectedTeamId":42}`))
	}))
	defer server.Close()
	client, err := NewDashboardAuthClient(server.Client(), DashboardAuthClientConfig{BaseURL: server.URL, WebsiteURL: "https://cursor.com"})
	if err != nil {
		t.Fatal(err)
	}
	flow := &DashboardLoginPKCE{UUID: "uuid", Verifier: "verifier", Challenge: "challenge"}
	pending, err := client.PollLogin(context.Background(), flow)
	if err != nil || !pending.Pending {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
	completed, err := client.PollLogin(context.Background(), flow)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Pending || completed.AccessToken != "access" || completed.RefreshToken != "refresh" || completed.SelectedTeamID == nil || *completed.SelectedTeamID != 42 {
		t.Fatalf("completed=%#v", completed)
	}
}

func TestDashboardRefreshFallsBackToAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"rotated"}`))
	}))
	defer server.Close()
	client, err := NewDashboardAuthClient(server.Client(), DashboardAuthClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RefreshAccessToken(context.Background(), "old")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "rotated" || result.RefreshToken != "rotated" {
		t.Fatalf("result=%#v", result)
	}
}

func TestDashboardRefreshShouldLogout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"shouldLogout":true}`))
	}))
	defer server.Close()
	client, err := NewDashboardAuthClient(server.Client(), DashboardAuthClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RefreshAccessToken(context.Background(), "old")
	if err != nil {
		t.Fatal(err)
	}
	if !result.ShouldLogout {
		t.Fatalf("result=%#v", result)
	}
}

func TestParseDashboardTokenMetadata(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	payload, _ := json.Marshal(map[string]any{"sub": "user", "iss": "issuer", "exp": exp})
	token := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
		base64.RawURLEncoding.EncodeToString(payload),
		"signature",
	}, ".")
	metadata, err := ParseDashboardTokenMetadata(token)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Subject != "user" || metadata.Issuer != "issuer" || metadata.ExpiresAt.Unix() != exp {
		t.Fatalf("metadata=%#v", metadata)
	}
}
