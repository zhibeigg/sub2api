package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

const testOneBotToken = "0123456789abcdef0123456789abcdef"

func newOneBotHubTestServer(t *testing.T, options OneBotHubOptions) (*OneBotHub, *httptest.Server, string) {
	t.Helper()
	if options.SelfID == "" {
		options.SelfID = "3944007489"
	}
	if options.AccessToken == "" {
		options.AccessToken = testOneBotToken
	}
	hub, err := NewOneBotHub(options)
	require.NoError(t, err)
	server := httptest.NewServer(hub)
	t.Cleanup(func() {
		server.Close()
		_ = hub.Close()
	})
	return hub, server, "ws" + strings.TrimPrefix(server.URL, "http")
}

func dialOneBot(t *testing.T, url, token, selfID string) *coderws.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Self-ID", selfID)
	conn, response, err := coderws.Dial(t.Context(), url, &coderws.DialOptions{HTTPHeader: header})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func TestOneBotHubRequiresBearerTokenAndExpectedSelfID(t *testing.T) {
	_, _, url := newOneBotHubTestServer(t, OneBotHubOptions{})

	_, response, err := coderws.Dial(t.Context(), url, &coderws.DialOptions{HTTPHeader: http.Header{"X-Self-ID": []string{"3944007489"}}})
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	response.Body.Close()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+testOneBotToken)
	header.Set("X-Self-ID", "111111111")
	_, response, err = coderws.Dial(t.Context(), url, &coderws.DialOptions{HTTPHeader: header})
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	response.Body.Close()

	conn := dialOneBot(t, url, testOneBotToken, "3944007489")
	require.NotNil(t, conn)
}

func TestOneBotHubCorrelatesActionEchoAndErrors(t *testing.T) {
	hub, _, url := newOneBotHubTestServer(t, OneBotHubOptions{ActionTimeout: time.Second})
	conn := dialOneBot(t, url, testOneBotToken, "3944007489")

	resultCh := make(chan error, 1)
	var result struct {
		UserID oneBotID `json:"user_id"`
	}
	go func() { resultCh <- hub.Call(t.Context(), "get_login_info", struct{}{}, &result) }()

	_, raw, err := conn.Read(t.Context())
	require.NoError(t, err)
	var request oneBotActionRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	require.Equal(t, "get_login_info", request.Action)
	require.NotEmpty(t, request.Echo)
	response := map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{"user_id": 3944007489}, "echo": request.Echo}
	responseRaw, _ := json.Marshal(response)
	require.NoError(t, conn.Write(t.Context(), coderws.MessageText, responseRaw))
	require.NoError(t, <-resultCh)
	require.Equal(t, "3944007489", result.UserID.String())

	go func() { resultCh <- hub.Call(t.Context(), "send_group_msg", map[string]any{"group_id": "1"}, nil) }()
	_, raw, err = conn.Read(t.Context())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &request))
	responseRaw, _ = json.Marshal(map[string]any{"status": "failed", "retcode": 1404, "data": nil, "echo": request.Echo})
	require.NoError(t, conn.Write(t.Context(), coderws.MessageText, responseRaw))
	var actionErr *OneBotActionError
	require.ErrorAs(t, <-resultCh, &actionErr)
	require.Equal(t, int64(1404), actionErr.RetCode)
}

func TestOneBotHubReplacesOldConnectionAndFailsItsPendingActions(t *testing.T) {
	hub, _, url := newOneBotHubTestServer(t, OneBotHubOptions{ActionTimeout: 5 * time.Second})
	first := dialOneBot(t, url, testOneBotToken, "3944007489")

	callResult := make(chan error, 1)
	go func() { callResult <- hub.Call(t.Context(), "get_login_info", nil, nil) }()
	_, _, err := first.Read(t.Context())
	require.NoError(t, err)

	second := dialOneBot(t, url, testOneBotToken, "3944007489")
	require.ErrorIs(t, <-callResult, ErrOneBotDisconnected)
	require.True(t, hub.Snapshot().Connected)

	readCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, err = first.Read(readCtx)
	require.Error(t, err)
	require.NotNil(t, second)
}

func TestOneBotHubLimitsPendingActionsAndDeliversEvents(t *testing.T) {
	events := make(chan InboundEvent, 1)
	hub, _, url := newOneBotHubTestServer(t, OneBotHubOptions{
		ActionTimeout: time.Second,
		MaxPending:    1,
		EventHandler: func(_ context.Context, event InboundEvent) error {
			events <- event
			return nil
		},
	})
	conn := dialOneBot(t, url, testOneBotToken, "3944007489")

	ctx, cancel := context.WithCancel(t.Context())
	callResult := make(chan error, 1)
	go func() { callResult <- hub.Call(ctx, "first", nil, nil) }()
	_, _, err := conn.Read(t.Context())
	require.NoError(t, err)
	require.ErrorIs(t, hub.Call(t.Context(), "second", nil, nil), ErrOneBotPendingLimit)
	cancel()
	require.True(t, errors.Is(<-callResult, context.Canceled))

	eventRaw := []byte(`{"time":1720000000,"self_id":3944007489,"post_type":"message","message_type":"group","message_id":101,"user_id":20001,"group_id":30001,"raw_message":"/help"}`)
	require.NoError(t, conn.Write(t.Context(), coderws.MessageText, eventRaw))
	select {
	case event := <-events:
		require.Equal(t, SceneGroup, event.Scene)
		require.Equal(t, "30001", event.SourceID)
	case <-time.After(time.Second):
		t.Fatal("onebot event was not delivered")
	}
}

func TestOneBotHubStopAcceptingDrainsBufferedEvents(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	processed := make(chan string, 2)
	hub, err := NewOneBotHub(OneBotHubOptions{
		SelfID:      "3944007489",
		AccessToken: testOneBotToken,
		EventBuffer: 2,
		EventHandler: func(_ context.Context, event InboundEvent) error {
			if event.EventID == "first" {
				close(started)
				<-release
			}
			processed <- event.EventID
			return nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = hub.Close() })

	hub.events <- InboundEvent{EventID: "first"}
	<-started
	hub.events <- InboundEvent{EventID: "second"}
	hub.StopAccepting()
	require.False(t, hub.EventsDrained())
	close(release)

	require.Eventually(t, hub.EventsDrained, time.Second, 10*time.Millisecond)
	seen := map[string]bool{<-processed: true, <-processed: true}
	require.True(t, seen["first"])
	require.True(t, seen["second"])
}

func TestTrustedOneBotPeerRejectsForwardedAndPublicRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/webhooks/qq/onebot", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	require.True(t, trustedOneBotPeer(request))
	request.Header.Set("X-Forwarded-For", "127.0.0.1")
	require.False(t, trustedOneBotPeer(request))
	request.Header.Del("X-Forwarded-For")
	request.RemoteAddr = "8.8.8.8:443"
	require.False(t, trustedOneBotPeer(request))
}
