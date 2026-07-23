package qqbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
)

const (
	DefaultOneBotActionTimeout   = 10 * time.Second
	DefaultOneBotPingInterval    = 30 * time.Second
	DefaultOneBotPingTimeout     = 10 * time.Second
	DefaultOneBotMaxPending      = 256
	DefaultOneBotMaxMessageBytes = int64(1 << 20)
	DefaultOneBotEventBuffer     = 256
)

var (
	ErrOneBotDisconnected = errors.New("onebot reverse websocket is disconnected")
	ErrOneBotPendingLimit = errors.New("onebot pending action limit reached")
	ErrOneBotHubClosed    = errors.New("onebot hub is closed")
)

type OneBotEventHandler func(context.Context, InboundEvent) error

type OneBotHubOptions struct {
	SelfID          string
	AccessToken     string
	ActionTimeout   time.Duration
	MaxPending      int
	MaxMessageBytes int64
	EventBuffer     int
	EventHandler    OneBotEventHandler
}

type OneBotHubStatus struct {
	Connected         bool      `json:"connected"`
	ConnectionID      uint64    `json:"connection_id,omitempty"`
	SelfIDFingerprint string    `json:"self_id_fingerprint"`
	PendingActions    int       `json:"pending_actions"`
	ConnectedAt       time.Time `json:"connected_at,omitempty"`
	LastEventAt       time.Time `json:"last_event_at,omitempty"`
	LastActionAt      time.Time `json:"last_action_at,omitempty"`
	LastDisconnectAt  time.Time `json:"last_disconnect_at,omitempty"`
	LastErrorCode     string    `json:"last_error_code,omitempty"`
}

type OneBotActionError struct {
	Action  string
	Status  string
	RetCode int64
}

func (e *OneBotActionError) Error() string {
	if e == nil {
		return "onebot action failed"
	}
	return fmt.Sprintf("onebot action %s failed with retcode %d", e.Action, e.RetCode)
}

func (e *OneBotActionError) Definitive() bool {
	return e != nil
}

type oneBotHubSession struct {
	id      uint64
	conn    *coderws.Conn
	writeMu sync.Mutex
}

type oneBotPendingAction struct {
	sessionID uint64
	action    string
	result    chan oneBotActionResult
}

type oneBotActionResult struct {
	response oneBotActionResponse
	err      error
}

type oneBotActionRequest struct {
	Action string `json:"action"`
	Params any    `json:"params"`
	Echo   string `json:"echo"`
}

type oneBotActionResponse struct {
	Status  string          `json:"status"`
	RetCode int64           `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Echo    json.RawMessage `json:"echo"`
}

type oneBotEnvelope struct {
	Echo json.RawMessage `json:"echo"`
}

type OneBotHub struct {
	selfID          string
	accessToken     string
	actionTimeout   time.Duration
	maxPending      int
	maxMessageBytes int64
	eventHandler    OneBotEventHandler
	events          chan InboundEvent
	rootCtx         context.Context
	cancel          context.CancelFunc

	mu               sync.Mutex
	accepting        bool
	closed           bool
	nextConnectionID uint64
	session          *oneBotHubSession
	pending          map[string]*oneBotPendingAction
	connectedAt      time.Time
	lastEventAt      time.Time
	lastActionAt     time.Time
	lastDisconnectAt time.Time
	lastErrorCode    string

	activeReaders atomic.Int64
	activeEvents  atomic.Int64
}

func NewOneBotHub(options OneBotHubOptions) (*OneBotHub, error) {
	options.SelfID = strings.TrimSpace(options.SelfID)
	options.AccessToken = strings.TrimSpace(options.AccessToken)
	if !validOneBotID(options.SelfID) {
		return nil, errors.New("onebot self ID is invalid")
	}
	if len([]byte(strings.TrimSpace(options.AccessToken))) < 32 {
		return nil, errors.New("onebot access token must contain at least 32 bytes")
	}
	if options.ActionTimeout <= 0 {
		options.ActionTimeout = DefaultOneBotActionTimeout
	}
	if options.MaxPending <= 0 {
		options.MaxPending = DefaultOneBotMaxPending
	}
	if options.MaxMessageBytes <= 0 {
		options.MaxMessageBytes = DefaultOneBotMaxMessageBytes
	}
	if options.EventBuffer <= 0 {
		options.EventBuffer = DefaultOneBotEventBuffer
	}
	rootCtx, cancel := context.WithCancel(context.Background())
	hub := &OneBotHub{
		selfID:          options.SelfID,
		accessToken:     options.AccessToken,
		actionTimeout:   options.ActionTimeout,
		maxPending:      options.MaxPending,
		maxMessageBytes: options.MaxMessageBytes,
		eventHandler:    options.EventHandler,
		rootCtx:         rootCtx,
		cancel:          cancel,
		pending:         make(map[string]*oneBotPendingAction),
		accepting:       true,
	}
	if options.EventHandler != nil {
		hub.events = make(chan InboundEvent, options.EventBuffer)
		go hub.eventLoop()
	}
	return hub, nil
}

// ValidateOneBotBearerToken compares the configured token without leaking its
// length through the secret comparison operation.
func ValidateOneBotBearerToken(authorization, expectedToken string) bool {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	providedDigest := sha256.Sum256([]byte(parts[1]))
	expectedDigest := sha256.Sum256([]byte(expectedToken))
	return subtle.ConstantTimeCompare(providedDigest[:], expectedDigest[:]) == 1
}

// ServeHTTP accepts a OneBot v11 reverse WebSocket connection. It is deliberately
// independent of Gin routing so callers can mount it on any HTTP stack.
func (h *OneBotHub) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if h == nil {
		http.Error(writer, "onebot hub unavailable", http.StatusServiceUnavailable)
		return
	}
	h.mu.Lock()
	accepting := h.accepting && !h.closed
	h.mu.Unlock()
	if !accepting {
		http.Error(writer, "onebot hub unavailable", http.StatusServiceUnavailable)
		return
	}
	if h.accessToken != "" && !ValidateOneBotBearerToken(request.Header.Get("Authorization"), h.accessToken) {
		writer.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	if strings.TrimSpace(request.Header.Get("X-Self-ID")) != h.selfID {
		http.Error(writer, "invalid self ID", http.StatusForbidden)
		return
	}

	conn, err := coderws.Accept(writer, request, &coderws.AcceptOptions{CompressionMode: coderws.CompressionDisabled})
	if err != nil {
		return
	}
	conn.SetReadLimit(h.maxMessageBytes)
	session, replaced, replacedPending, attachErr := h.attach(conn)
	if attachErr != nil {
		_ = conn.CloseNow()
		return
	}
	if replaced != nil {
		_ = replaced.conn.CloseNow()
		notifyOneBotPending(replacedPending, ErrOneBotDisconnected)
	}
	defer func() { _ = conn.CloseNow() }()
	h.activeReaders.Add(1)
	defer h.activeReaders.Add(-1)
	go h.pingLoop(session)
	h.readLoop(request.Context(), session)
}

func (h *OneBotHub) Call(ctx context.Context, action string, params any, result any) error {
	if h == nil {
		return ErrOneBotDisconnected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return errors.New("onebot action is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, h.actionTimeout)
	defer cancel()
	if params == nil {
		params = struct{}{}
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrOneBotHubClosed
	}
	session := h.session
	if session == nil {
		h.mu.Unlock()
		return ErrOneBotDisconnected
	}
	if len(h.pending) >= h.maxPending {
		h.mu.Unlock()
		return ErrOneBotPendingLimit
	}
	echo, err := newOneBotEcho()
	if err != nil {
		h.mu.Unlock()
		return err
	}
	pending := &oneBotPendingAction{sessionID: session.id, action: action, result: make(chan oneBotActionResult, 1)}
	h.pending[echo] = pending
	h.mu.Unlock()

	request := oneBotActionRequest{Action: action, Params: params, Echo: echo}
	raw, err := json.Marshal(request)
	if err != nil {
		h.removePending(echo, pending)
		return err
	}
	session.writeMu.Lock()
	writeErr := session.conn.Write(callCtx, coderws.MessageText, raw)
	session.writeMu.Unlock()
	if writeErr != nil {
		h.removePending(echo, pending)
		failed := h.disconnect(session, "write_failed")
		_ = session.conn.CloseNow()
		notifyOneBotPending(failed, ErrOneBotDisconnected)
		return ErrOneBotDisconnected
	}
	h.mu.Lock()
	if h.session == session {
		h.lastActionAt = time.Now().UTC()
	}
	h.mu.Unlock()

	select {
	case outcome := <-pending.result:
		if outcome.err != nil {
			return outcome.err
		}
		response := outcome.response
		if !strings.EqualFold(strings.TrimSpace(response.Status), "ok") || response.RetCode != 0 {
			return &OneBotActionError{Action: action, Status: response.Status, RetCode: response.RetCode}
		}
		if result != nil && len(bytes.TrimSpace(response.Data)) > 0 && !bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) {
			if err := json.Unmarshal(response.Data, result); err != nil {
				return fmt.Errorf("decode onebot action result: %w", err)
			}
		}
		return nil
	case <-callCtx.Done():
		h.removePending(echo, pending)
		return callCtx.Err()
	}
}

func newOneBotEcho() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate onebot action echo: %w", err)
	}
	return "onebot-" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (h *OneBotHub) Snapshot() OneBotHubStatus {
	if h == nil {
		return OneBotHubStatus{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	status := OneBotHubStatus{
		SelfIDFingerprint: Fingerprint(h.selfID),
		PendingActions:    len(h.pending),
		ConnectedAt:       h.connectedAt,
		LastEventAt:       h.lastEventAt,
		LastActionAt:      h.lastActionAt,
		LastDisconnectAt:  h.lastDisconnectAt,
		LastErrorCode:     h.lastErrorCode,
	}
	if h.session != nil {
		status.Connected = true
		status.ConnectionID = h.session.id
	}
	return status
}

func (h *OneBotHub) StopAccepting() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed || !h.accepting {
		h.mu.Unlock()
		return
	}
	h.accepting = false
	session := h.session
	h.session = nil
	pending := h.takePendingLocked(0)
	h.lastDisconnectAt = time.Now().UTC()
	h.lastErrorCode = "hub_retiring"
	h.mu.Unlock()
	if session != nil {
		_ = session.conn.CloseNow()
	}
	notifyOneBotPending(pending, ErrOneBotDisconnected)
}

func (h *OneBotHub) EventsDrained() bool {
	if h == nil {
		return true
	}
	return h.activeReaders.Load() == 0 && h.activeEvents.Load() == 0 && len(h.events) == 0
}

func (h *OneBotHub) Close() error {
	if h == nil {
		return nil
	}
	h.StopAccepting()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	pending := h.takePendingLocked(0)
	h.lastDisconnectAt = time.Now().UTC()
	h.lastErrorCode = "hub_closed"
	h.mu.Unlock()
	h.cancel()
	notifyOneBotPending(pending, ErrOneBotHubClosed)
	return nil
}

func (h *OneBotHub) attach(conn *coderws.Conn) (*oneBotHubSession, *oneBotHubSession, []*oneBotPendingAction, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || !h.accepting {
		return nil, nil, nil, ErrOneBotHubClosed
	}
	h.nextConnectionID++
	session := &oneBotHubSession{id: h.nextConnectionID, conn: conn}
	replaced := h.session
	var replacedPending []*oneBotPendingAction
	if replaced != nil {
		replacedPending = h.takePendingLocked(replaced.id)
		h.lastDisconnectAt = time.Now().UTC()
	}
	h.session = session
	h.connectedAt = time.Now().UTC()
	h.lastErrorCode = ""
	return session, replaced, replacedPending, nil
}

func (h *OneBotHub) pingLoop(session *oneBotHubSession) {
	ticker := time.NewTicker(DefaultOneBotPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.rootCtx.Done():
			return
		case <-ticker.C:
			h.mu.Lock()
			active := !h.closed && h.session == session
			h.mu.Unlock()
			if !active {
				return
			}
			pingCtx, cancel := context.WithTimeout(h.rootCtx, DefaultOneBotPingTimeout)
			err := session.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				pending := h.disconnect(session, "ping_failed")
				_ = session.conn.CloseNow()
				notifyOneBotPending(pending, ErrOneBotDisconnected)
				return
			}
		}
	}
}

func (h *OneBotHub) readLoop(ctx context.Context, session *oneBotHubSession) {
	for {
		_, raw, err := session.conn.Read(ctx)
		if err != nil {
			pending := h.disconnect(session, "read_failed")
			notifyOneBotPending(pending, ErrOneBotDisconnected)
			return
		}
		if !h.activeSession(session) {
			return
		}
		if h.deliverAction(session, raw) {
			continue
		}
		event, accepted, adaptErr := AdaptOneBotEvent(raw, h.selfID)
		if adaptErr != nil {
			h.setErrorCode("invalid_event")
			continue
		}
		if !accepted {
			continue
		}
		h.mu.Lock()
		if h.session != session {
			h.mu.Unlock()
			return
		}
		h.lastEventAt = time.Now().UTC()
		h.mu.Unlock()
		if h.events != nil {
			select {
			case h.events <- event:
			case <-h.rootCtx.Done():
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

func (h *OneBotHub) activeSession(session *oneBotHubSession) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.closed && h.session == session
}

func (h *OneBotHub) deliverAction(session *oneBotHubSession, raw []byte) bool {
	var envelope oneBotEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false
	}
	echo, ok := oneBotEchoString(envelope.Echo)
	if !ok {
		return false
	}
	var response oneBotActionResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		h.setErrorCode("invalid_action_response")
		return true
	}
	h.mu.Lock()
	if h.session != session {
		h.mu.Unlock()
		return true
	}
	pending := h.pending[echo]
	if pending != nil && pending.sessionID == session.id {
		delete(h.pending, echo)
	} else {
		pending = nil
	}
	h.mu.Unlock()
	if pending != nil {
		pending.result <- oneBotActionResult{response: response}
	}
	return true
}

func oneBotEchoString(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil && strings.TrimSpace(value) != "" {
		return value, true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var number json.Number
	if err := decoder.Decode(&number); err == nil {
		return number.String(), true
	}
	return "", false
}

func (h *OneBotHub) disconnect(session *oneBotHubSession, errorCode string) []*oneBotPendingAction {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session != session {
		return nil
	}
	h.session = nil
	h.lastDisconnectAt = time.Now().UTC()
	h.lastErrorCode = errorCode
	return h.takePendingLocked(session.id)
}

func (h *OneBotHub) takePendingLocked(sessionID uint64) []*oneBotPendingAction {
	result := make([]*oneBotPendingAction, 0, len(h.pending))
	for echo, pending := range h.pending {
		if sessionID != 0 && pending.sessionID != sessionID {
			continue
		}
		delete(h.pending, echo)
		result = append(result, pending)
	}
	return result
}

func (h *OneBotHub) removePending(echo string, expected *oneBotPendingAction) {
	h.mu.Lock()
	if h.pending[echo] == expected {
		delete(h.pending, echo)
	}
	h.mu.Unlock()
}

func (h *OneBotHub) setErrorCode(code string) {
	h.mu.Lock()
	h.lastErrorCode = code
	h.mu.Unlock()
}

func (h *OneBotHub) eventLoop() {
	for {
		select {
		case <-h.rootCtx.Done():
			return
		case event := <-h.events:
			h.activeEvents.Add(1)
			if err := h.eventHandler(h.rootCtx, event); err != nil {
				h.setErrorCode("event_handler_failed")
			}
			h.activeEvents.Add(-1)
		}
	}
}

func notifyOneBotPending(pending []*oneBotPendingAction, err error) {
	for _, item := range pending {
		item.result <- oneBotActionResult{err: err}
	}
}
