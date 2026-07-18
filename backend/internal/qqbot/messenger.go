package qqbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	botGoAPIDomain        = "https://api.sgroup.qq.com"
	botGoSandboxAPIDomain = "https://sandbox.api.sgroup.qq.com"
	botGoTokenURL         = "https://bots.qq.com/app/getAppAccessToken"
)

type Messenger interface {
	Probe(ctx context.Context) (string, error)
	SendGroup(ctx context.Context, groupID, messageID, eventID, content string, sequence uint32) error
	SendC2C(ctx context.Context, userID, messageID, eventID, content string, sequence uint32) error
	SendChannel(ctx context.Context, channelID, messageID, eventID, content string, sequence uint32) error
}

type BotGoMessenger struct {
	appID       string
	secret      string
	baseURL     string
	tokenURL    string
	client      *http.Client
	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

type botGoMessage struct {
	Content string `json:"content,omitempty"`
	MsgType int    `json:"msg_type,omitempty"`
	MsgID   string `json:"msg_id,omitempty"`
	EventID string `json:"event_id,omitempty"`
	MsgSeq  uint32 `json:"msg_seq,omitempty"`
}

func NewBotGoMessenger(appID, secret string, sandbox bool, timeout time.Duration) (*BotGoMessenger, error) {
	appID = strings.TrimSpace(appID)
	secret = strings.TrimSpace(secret)
	if appID == "" || secret == "" {
		return nil, errors.New("bot credentials are required")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	baseURL := botGoAPIDomain
	if sandbox {
		baseURL = botGoSandboxAPIDomain
	}
	return &BotGoMessenger{appID: appID, secret: secret, baseURL: baseURL, tokenURL: botGoTokenURL, client: &http.Client{Timeout: timeout}}, nil
}

func (m *BotGoMessenger) Probe(ctx context.Context) (string, error) {
	if _, err := m.token(ctx); err != nil {
		return "", err
	}
	return m.appID, nil
}

func (m *BotGoMessenger) SendGroup(ctx context.Context, groupID, messageID, eventID, content string, sequence uint32) error {
	return m.send(ctx, "/v2/groups/"+url.PathEscape(groupID)+"/messages", messageID, eventID, content, sequence)
}
func (m *BotGoMessenger) SendC2C(ctx context.Context, userID, messageID, eventID, content string, sequence uint32) error {
	return m.send(ctx, "/v2/users/"+url.PathEscape(userID)+"/messages", messageID, eventID, content, sequence)
}
func (m *BotGoMessenger) SendChannel(ctx context.Context, channelID, messageID, eventID, content string, sequence uint32) error {
	return m.send(ctx, "/channels/"+url.PathEscape(channelID)+"/messages", messageID, eventID, content, sequence)
}
func (m *BotGoMessenger) send(ctx context.Context, endpoint, messageID, eventID, content string, sequence uint32) error {
	return m.do(ctx, http.MethodPost, endpoint, botGoMessage{Content: content, MsgType: 0, MsgID: messageID, EventID: eventID, MsgSeq: sequence}, nil)
}

func (m *BotGoMessenger) do(ctx context.Context, method, endpoint string, body any, result any) error {
	token, err := m.token(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		raw, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "QQBot "+token)
	req.Header.Set("X-Union-Appid", m.appID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("botgo api status %d", resp.StatusCode)
	}
	if result != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, result); err != nil {
			return err
		}
	}
	return nil
}

func (m *BotGoMessenger) token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accessToken != "" && time.Until(m.tokenExpiry) > 15*time.Second {
		return m.accessToken, nil
	}
	raw, err := json.Marshal(map[string]string{"appId": m.appID, "clientSecret": m.secret})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("botgo token status %d", resp.StatusCode)
	}
	var payload struct {
		Code        int             `json:"code"`
		Message     string          `json:"message"`
		AccessToken string          `json:"access_token"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if payload.Code != 0 || payload.AccessToken == "" {
		return "", fmt.Errorf("botgo token error %d", payload.Code)
	}
	expires := int64(7200)
	var asString string
	if json.Unmarshal(payload.ExpiresIn, &asString) == nil {
		_, _ = fmt.Sscan(asString, &expires)
	} else {
		_ = json.Unmarshal(payload.ExpiresIn, &expires)
	}
	m.accessToken = payload.AccessToken
	m.tokenExpiry = time.Now().Add(time.Duration(expires) * time.Second)
	return m.accessToken, nil
}
