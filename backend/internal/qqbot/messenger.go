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
	botGoMediaCacheTTL    = 5 * time.Minute
	botGoMediaCacheMax    = 4096
)

type botGoMediaStore interface {
	GetMediaFileInfo(ctx context.Context, key string) (string, bool, error)
	SetMediaFileInfo(ctx context.Context, key, fileInfo string) error
}

type Messenger interface {
	Probe(ctx context.Context) (string, error)
	SendGroup(ctx context.Context, groupID, messageID, eventID, content string, sequence uint32) error
	SendC2C(ctx context.Context, userID, messageID, eventID, content string, sequence uint32) error
	SendProactiveC2C(ctx context.Context, userID, content string) error
	SendChannel(ctx context.Context, channelID, messageID, eventID, content string, sequence uint32) error
	SendGroupImage(ctx context.Context, groupID, messageID, eventID, imageURL string, sequence uint32) error
	SendC2CImage(ctx context.Context, userID, messageID, eventID, imageURL string, sequence uint32) error
	SendChannelImage(ctx context.Context, channelID, messageID, eventID, imageURL string, sequence uint32) error
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

	mediaMu    sync.Mutex
	mediaCache map[string]cachedBotGoMedia
	mediaStore botGoMediaStore
}

type botGoMessage struct {
	Content string      `json:"content,omitempty"`
	MsgType int         `json:"msg_type,omitempty"`
	MsgID   string      `json:"msg_id,omitempty"`
	EventID string      `json:"event_id,omitempty"`
	MsgSeq  uint32      `json:"msg_seq,omitempty"`
	Media   *botGoMedia `json:"media,omitempty"`
	Image   string      `json:"image,omitempty"`
}

type botGoMedia struct {
	FileInfo string `json:"file_info"`
}

type botGoMediaUploadRequest struct {
	FileType   int    `json:"file_type"`
	URL        string `json:"url"`
	SrvSendMsg bool   `json:"srv_send_msg"`
}

type botGoMediaUploadResponse struct {
	FileInfo string `json:"file_info"`
}

type cachedBotGoMedia struct {
	FileInfo string
	Expires  time.Time
}

type BotGoAPIError struct {
	StatusCode int
	Code       int
	Message    string
}

func (e *BotGoAPIError) Error() string {
	if e == nil {
		return "botgo api error"
	}
	if e.Code != 0 {
		return fmt.Sprintf("botgo api status %d code %d", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("botgo api status %d", e.StatusCode)
}

func (e *BotGoAPIError) Definitive() bool {
	if e == nil || e.StatusCode < http.StatusBadRequest || e.StatusCode >= http.StatusInternalServerError {
		return false
	}
	switch e.StatusCode {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	default:
		return true
	}
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
	return &BotGoMessenger{appID: appID, secret: secret, baseURL: baseURL, tokenURL: botGoTokenURL, client: &http.Client{Timeout: timeout}, mediaCache: make(map[string]cachedBotGoMedia)}, nil
}

func (m *BotGoMessenger) setMediaStore(store botGoMediaStore) {
	if m != nil {
		m.mediaStore = store
	}
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
func (m *BotGoMessenger) SendProactiveC2C(ctx context.Context, userID, content string) error {
	return m.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(userID)+"/messages", botGoMessage{Content: content, MsgType: 0}, nil)
}
func (m *BotGoMessenger) SendChannel(ctx context.Context, channelID, messageID, eventID, content string, sequence uint32) error {
	return m.send(ctx, "/channels/"+url.PathEscape(channelID)+"/messages", messageID, eventID, content, sequence)
}

func (m *BotGoMessenger) SendGroupImage(ctx context.Context, groupID, messageID, eventID, imageURL string, sequence uint32) error {
	return m.sendUploadedImage(ctx, "/v2/groups/"+url.PathEscape(groupID), messageID, eventID, imageURL, sequence)
}

func (m *BotGoMessenger) SendC2CImage(ctx context.Context, userID, messageID, eventID, imageURL string, sequence uint32) error {
	return m.sendUploadedImage(ctx, "/v2/users/"+url.PathEscape(userID), messageID, eventID, imageURL, sequence)
}

func (m *BotGoMessenger) SendChannelImage(ctx context.Context, channelID, messageID, eventID, imageURL string, sequence uint32) error {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return errors.New("botgo image URL is required")
	}
	return m.do(ctx, http.MethodPost, "/channels/"+url.PathEscape(channelID)+"/messages", botGoMessage{Content: " ", MsgID: messageID, EventID: eventID, MsgSeq: sequence, Image: imageURL}, nil)
}

func (m *BotGoMessenger) sendUploadedImage(ctx context.Context, targetEndpoint, messageID, eventID, imageURL string, sequence uint32) error {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return errors.New("botgo image URL is required")
	}
	cacheKey := strings.Join([]string{m.appID, targetEndpoint, messageID, eventID}, "\x00")
	fileInfo, found, err := m.cachedFileInfo(ctx, cacheKey)
	if err != nil {
		return fmt.Errorf("load botgo image upload state: %w", err)
	}
	if !found {
		var upload botGoMediaUploadResponse
		if err := m.do(ctx, http.MethodPost, targetEndpoint+"/files", botGoMediaUploadRequest{FileType: 1, URL: imageURL, SrvSendMsg: false}, &upload); err != nil {
			return fmt.Errorf("upload botgo image: %w", err)
		}
		fileInfo = strings.TrimSpace(upload.FileInfo)
		if fileInfo == "" {
			return errors.New("botgo image upload returned empty file_info")
		}
		if err := m.cacheFileInfo(ctx, cacheKey, fileInfo); err != nil {
			return fmt.Errorf("persist botgo image upload state: %w", err)
		}
	}
	message := botGoMessage{Content: " ", MsgType: 7, MsgID: messageID, EventID: eventID, MsgSeq: sequence, Media: &botGoMedia{FileInfo: fileInfo}}
	return m.do(ctx, http.MethodPost, targetEndpoint+"/messages", message, nil)
}

func (m *BotGoMessenger) cachedFileInfo(ctx context.Context, key string) (string, bool, error) {
	if m == nil || strings.TrimSpace(key) == "" {
		return "", false, nil
	}
	cacheKey := Fingerprint(key)
	if fileInfo, found := m.cachedFileInfoMemory(cacheKey); found {
		return fileInfo, true, nil
	}
	if m.mediaStore == nil {
		return "", false, nil
	}
	fileInfo, found, err := m.mediaStore.GetMediaFileInfo(ctx, key)
	if err != nil || !found {
		return "", false, err
	}
	m.cacheFileInfoMemory(cacheKey, fileInfo)
	return fileInfo, true, nil
}

func (m *BotGoMessenger) cacheFileInfo(ctx context.Context, key, fileInfo string) error {
	if m == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(fileInfo) == "" {
		return errors.New("botgo media cache input is invalid")
	}
	if m.mediaStore != nil {
		if err := m.mediaStore.SetMediaFileInfo(ctx, key, fileInfo); err != nil {
			return err
		}
	}
	m.cacheFileInfoMemory(Fingerprint(key), fileInfo)
	return nil
}

func (m *BotGoMessenger) cachedFileInfoMemory(key string) (string, bool) {
	now := time.Now()
	m.mediaMu.Lock()
	defer m.mediaMu.Unlock()
	for cacheKey, cached := range m.mediaCache {
		if !cached.Expires.After(now) {
			delete(m.mediaCache, cacheKey)
		}
	}
	cached, ok := m.mediaCache[key]
	if !ok || !cached.Expires.After(now) || strings.TrimSpace(cached.FileInfo) == "" {
		return "", false
	}
	return cached.FileInfo, true
}

func (m *BotGoMessenger) cacheFileInfoMemory(key, fileInfo string) {
	now := time.Now()
	m.mediaMu.Lock()
	defer m.mediaMu.Unlock()
	if m.mediaCache == nil {
		m.mediaCache = make(map[string]cachedBotGoMedia)
	}
	for cacheKey, cached := range m.mediaCache {
		if !cached.Expires.After(now) {
			delete(m.mediaCache, cacheKey)
		}
	}
	if len(m.mediaCache) >= botGoMediaCacheMax {
		for cacheKey := range m.mediaCache {
			delete(m.mediaCache, cacheKey)
			break
		}
	}
	m.mediaCache[key] = cachedBotGoMedia{FileInfo: strings.TrimSpace(fileInfo), Expires: now.Add(botGoMediaCacheTTL)}
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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var payload struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(responseBody, &payload)
		return &BotGoAPIError{StatusCode: resp.StatusCode, Code: payload.Code, Message: payload.Message}
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
