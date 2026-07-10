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

func TestClientHeadersAndStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("unexpected request target %s %s", r.Method, r.URL.Path)
		}
		expected := map[string]string{
			"Content-Type": "application/json",
			"Accept":       "text/event-stream",
			"User-Agent":   "test-agent",
			"Referer":      "https://example.test/docs",
			"X-Path":       "/api/chat",
			"X-Method":     "POST",
			"Cookie":       "session=secret",
		}
		for name, value := range expected {
			if got := r.Header.Get(name); got != value {
				t.Errorf("header %s = %q, want %q", name, got, value)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"text-delta\",\"delta\":\"ok\"}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), Credential{Cookie: "session=secret"}, ClientConfig{
		BaseURL: server.URL + "/api/chat", Referer: "https://example.test/docs", UserAgent: "test-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []SSEEvent
	err = client.Stream(context.Background(), &Request{Model: "m", ID: "id", Trigger: "submit-message"}, func(event SSEEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Delta != "ok" || events[1].Type != "finish" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestClientHTTPErrorClassificationAndTruncation(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(strings.Repeat("x", 50)))
			}))
			defer server.Close()
			client, err := NewClient(server.Client(), Credential{}, ClientConfig{BaseURL: server.URL, MaxErrorBody: 8})
			if err != nil {
				t.Fatal(err)
			}
			err = client.Stream(context.Background(), &Request{Model: "m", ID: "id", Trigger: "submit-message"}, nil)
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

func TestClientHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()
	defer close(release)
	client, err := NewClient(server.Client(), Credential{}, ClientConfig{BaseURL: server.URL, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.Stream(ctx, &Request{Model: "m", ID: "id", Trigger: "submit-message"}, nil)
	}()
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

func TestClientStreamIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(server.Client(), Credential{}, ClientConfig{
		BaseURL: server.URL, RequestTimeout: time.Second, StreamIdleTimeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = client.Stream(context.Background(), &Request{Model: "m", ID: "id", Trigger: "submit-message"}, nil)
	if err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("idle timeout did not abort promptly: %v", err)
	}
	var cursorErr *Error
	if !errors.As(err, &cursorErr) || cursorErr.Kind != ErrorTransport || !cursorErr.FailoverSafe {
		t.Fatalf("unexpected idle timeout error: %#v", err)
	}
}
