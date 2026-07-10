package firefly

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type scriptedTransport struct {
	calls      int
	reqHeaders []map[string]string
}

func (s *scriptedTransport) do(_ context.Context, _ string, _ string, h map[string]string, _ []byte) (*transportResponse, error) {
	s.calls++
	s.reqHeaders = append(s.reqHeaders, h)
	if s.calls == 1 {
		return &transportResponse{status: 422, headers: http.Header{}, body: []byte(`{"message":"Unsupported field(s)"}`)}, nil
	}
	return &transportResponse{status: 202, headers: http.Header{"X-Override-Status-Link": []string{"https://firefly-3p.ff.adobe.io/jobs/task-1"}}, body: []byte(`{"jobId":"task-1"}`)}, nil
}
func tokenFor(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "x." + base64.RawURLEncoding.EncodeToString(b) + ".y"
}
func TestCandidateFallbackNonceAndHeaderOrder(t *testing.T) {
	p, _ := ResolveImageModel("nano-banana-pro", "16:9", "2k")
	tr := &scriptedTransport{}
	c := NewClient(ClientConfig{})
	c.transport = tr
	res, err := c.SubmitImage(context.Background(), tokenFor(map[string]any{"user_id": "user@AdobeID"}), p, "hello", []string{"ref"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TaskID != "task-1" || tr.calls != 2 {
		t.Fatalf("result=%+v calls=%d", res, tr.calls)
	}
	if len(tr.reqHeaders[0]["x-nonce"]) != 64 {
		t.Fatal("missing nonce")
	}
	order := orderedHeaderKeys(tr.reqHeaders[0])
	if order[0] != "authorization" || order[len(order)-1] != "x-arp-session-id" {
		t.Fatalf("order=%v", order)
	}
}
func TestErrorClassification(t *testing.T) {
	err := classifyError(401, map[string]string{}, []byte(`{"error":"invalid_token"}`))
	if !IsAuthError(err) || StatusCode(err) != 401 {
		t.Fatal(err)
	}
	err = classifyError(503, map[string]string{}, nil)
	if !IsRetryableError(err) {
		t.Fatal("503 should retry")
	}
	err = classifyError(429, map[string]string{"retry-after": "7"}, nil)
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.RetryAfter.Seconds() != 7 {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "token") {
		t.Fatal("error leaked token material")
	}
}

type onePollTransport struct{ calls int }

func (s *onePollTransport) do(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (*transportResponse, error) {
	s.calls++
	return &transportResponse{status: 200, headers: http.Header{"Retry-After": []string{"2"}}, body: []byte(`{"status":"IN_PROGRESS","taskId":"p1"}`)}, nil
}

func TestPollPerformsSingleRequest(t *testing.T) {
	tr := &onePollTransport{}
	c := NewClient(ClientConfig{})
	c.transport = tr
	got, err := c.Poll(context.Background(), "token", "https://firefly-3p.ff.adobe.io/jobs/p1")
	if err != nil || got.Status != "IN_PROGRESS" || got.RetryAfter.Seconds() != 2 || tr.calls != 1 {
		t.Fatalf("poll=%+v err=%v calls=%d", got, err, tr.calls)
	}
}

type completedWithoutOutputTransport struct{}

func (completedWithoutOutputTransport) do(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (*transportResponse, error) {
	return &transportResponse{status: 200, headers: http.Header{}, body: []byte(`{"status":"COMPLETED","taskId":"p2"}`)}, nil
}

func TestPollDoesNotExposeCompletedWithoutOutput(t *testing.T) {
	c := NewClient(ClientConfig{})
	c.transport = completedWithoutOutputTransport{}
	got, err := c.Poll(context.Background(), "token", "https://firefly-3p.ff.adobe.io/jobs/p2")
	if err != nil || got.Status != "IN_PROGRESS" || got.OutputURL != "" {
		t.Fatalf("poll=%+v err=%v", got, err)
	}
}

func TestValidateStatusURL(t *testing.T) {
	for _, raw := range []string{
		"https://firefly-3p.ff.adobe.io/jobs/1",
		"https://firefly-3p.ff.adobe.io/v2/status/1",
		"https://firefly-3p.ff.adobe.io/v2/3p-videos/generate-async/1",
	} {
		if ValidateStatusURL(raw) != nil {
			t.Fatalf("valid rejected: %s", raw)
		}
	}
	for _, raw := range []string{
		"https://evil.example/jobs/1",
		"https://evil.ff.adobe.io/jobs/1",
		"https://firefly-3p.ff.adobe.io/profile",
		"https://firefly-3p.ff.adobe.io/v2/storage/image",
		"https://firefly-3p.ff.adobe.io:444/jobs/1",
	} {
		if ValidateStatusURL(raw) == nil {
			t.Fatalf("untrusted status URL accepted: %s", raw)
		}
	}
}
