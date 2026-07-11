package cursor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudClientLifecycleAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer cursor-key" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/me":
			_, _ = w.Write([]byte(`{"apiKeyName":"sub2api","userEmail":"user@example.com"}`))
		case "GET /v1/models":
			_, _ = w.Write([]byte(`{"items":[{"id":"auto","displayName":"Auto"}]}`))
		case "POST /v1/agents":
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}
			_, _ = w.Write([]byte(`{"agent":{"id":"agent-1","status":"RUNNING"},"run":{"id":"run-1","agentId":"agent-1","status":"RUNNING"}}`))
		case "GET /v1/agents/agent-1/runs/run-1/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: interaction_update\ndata: {\"type\":\"text_delta\",\"text\":\"ok\"}\n\nevent: result\ndata: {\"status\":\"FINISHED\",\"text\":\"ok\"}\n\n"))
		case "POST /v1/agents/agent-1/runs/run-1/cancel", "DELETE /v1/agents/agent-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewCloudClient(server.Client(), "cursor-key", CloudClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := client.Me(context.Background())
	if err != nil || identity.UserEmail != "user@example.com" {
		t.Fatalf("unexpected identity: %#v %v", identity, err)
	}
	models, err := client.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "auto" {
		t.Fatalf("unexpected models: %#v %v", models, err)
	}
	created, err := client.CreateAgent(context.Background(), CreateAgentRequest{Prompt: CloudPrompt{Text: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := client.StreamRun(context.Background(), created.Agent.ID, created.Run.ID, func(event CloudSSEEvent) error {
		names = append(names, event.Event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "interaction_update" || names[1] != "result" {
		t.Fatalf("unexpected events: %#v", names)
	}
	if err := client.CancelRun(context.Background(), created.Agent.ID, created.Run.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteAgent(context.Background(), created.Agent.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCloudClientHTTPErrorClassificationAndTruncation(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(strings.Repeat("x", 50)))
			}))
			defer server.Close()
			client, err := NewCloudClient(server.Client(), "key", CloudClientConfig{BaseURL: server.URL, MaxErrorBody: 8})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Me(context.Background())
			var cursorErr *Error
			if !errors.As(err, &cursorErr) || cursorErr.StatusCode != status {
				t.Fatalf("unexpected error: %v", err)
			}
			if cursorErr.Body != "xxxxxxxx..." {
				t.Fatalf("error body was not truncated: %q", cursorErr.Body)
			}
			wantSafe := status == 429 || status >= 500
			if cursorErr.FailoverSafe != wantSafe {
				t.Fatalf("FailoverSafe=%v, want %v", cursorErr.FailoverSafe, wantSafe)
			}
		})
	}
}

func TestCloudClientHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewCloudClient(server.Client(), "key", CloudClientConfig{BaseURL: server.URL, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.StreamRun(ctx, "agent", "run", nil) }()
	<-started
	cancel()
	select {
	case err := <-done:
		var cursorErr *Error
		if !errors.As(err, &cursorErr) || !errors.Is(err, context.Canceled) || cursorErr.FailoverSafe {
			t.Fatalf("unexpected cancellation error: %#v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after context cancellation")
	}
}

func TestCloudClientStreamIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewCloudClient(server.Client(), "key", CloudClientConfig{
		BaseURL: server.URL, RequestTimeout: time.Second, StreamIdleTimeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = client.StreamRun(context.Background(), "agent", "run", nil)
	if err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("idle timeout did not abort promptly: %v", err)
	}
	var cursorErr *Error
	if !errors.As(err, &cursorErr) || cursorErr.Kind != ErrorTransport || !cursorErr.FailoverSafe {
		t.Fatalf("unexpected idle timeout error: %#v", err)
	}
}
