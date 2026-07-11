package cursor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

type IDEClient struct {
	httpClient *http.Client
	config     IDEClientConfig
	baseURL    string
}

func NewIDEClient(httpClient *http.Client, config IDEClientConfig) (*IDEClient, error) {
	if httpClient == nil {
		return nil, badRequest("create IDE client", fmt.Errorf("http client is required"))
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultIDEBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, badRequest("create IDE client", fmt.Errorf("invalid base URL"))
	}
	applyIDEConfigDefaults(&config)
	if config.MaxBufferedBytes < 5 || config.MaxFrameSize > config.MaxBufferedBytes-5 {
		return nil, badRequest("create IDE client", fmt.Errorf("max buffered bytes must accommodate one complete frame"))
	}
	return &IDEClient{httpClient: httpClient, config: config, baseURL: baseURL}, nil
}

func (c *IDEClient) StreamUnifiedChatWithTools(ctx context.Context, credential IDECredential, dialogue *Dialogue, options IDEChatOptions) (*http.Response, *IDEEventStream, error) {
	headers, err := BuildIDEHeaders(credential, c.config)
	if err != nil {
		return nil, nil, err
	}
	metadata := encodeIDEMetadata(c.config)
	payload, err := encodeIDEChatRequest(dialogue, options, c.config.UUID, metadata)
	if err != nil {
		return nil, nil, err
	}
	frame, err := EncodeConnectFrame(payload, options.Compress)
	if err != nil {
		return nil, nil, badRequest("encode IDE chat frame", err)
	}
	if len(frame)-5 > c.config.MaxFrameSize {
		return nil, nil, badRequest("encode IDE chat frame", fmt.Errorf("frame size %d exceeds %d bytes", len(frame)-5, c.config.MaxFrameSize))
	}
	req, err := http.NewRequestWithContext(nonNilContext(ctx), http.MethodPost, c.baseURL+IDEChatPath, bytes.NewReader(frame))
	if err != nil {
		return nil, nil, badRequest("create IDE chat request", err)
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Accept", "application/connect+proto")
	req.Header.Set("Connect-Accept-Encoding", "gzip")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, transportError("IDE chat request", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp, nil, c.httpError("IDE chat request", resp)
	}
	stream := &IDEEventStream{
		response: resp, decoder: NewConnectDecoder(c.config.MaxFrameSize, c.config.MaxBufferedBytes),
		maxToolBytes: c.config.MaxBufferedBytes, maxToolCalls: 64,
	}
	return resp, stream, nil
}

func (c *IDEClient) AvailableModels(ctx context.Context, credential IDECredential) (*http.Response, error) {
	frame, err := EncodeConnectFrame(nil, false)
	if err != nil {
		return nil, badRequest("encode IDE models frame", err)
	}
	resp, err := c.availableModelsRequest(ctx, credential, "application/connect+proto", frame)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnsupportedMediaType {
		_ = resp.Body.Close()
		resp, err = c.availableModelsRequest(ctx, credential, "application/json", []byte("{}"))
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp, c.httpError("IDE models request", resp)
	}
	return resp, nil
}

func (c *IDEClient) availableModelsRequest(ctx context.Context, credential IDECredential, contentType string, body []byte) (*http.Response, error) {
	headers, err := BuildIDEHeaders(credential, c.config)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(nonNilContext(ctx), http.MethodPost, c.baseURL+IDEModelsPath, bytes.NewReader(body))
	if err != nil {
		return nil, badRequest("create IDE models request", err)
	}
	req.Header = headers
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", contentType)
	if contentType == "application/connect+proto" {
		req.Header.Set("Connect-Accept-Encoding", "gzip")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, transportError("IDE models request", err)
	}
	return resp, nil
}

func BuildIDEHeaders(credential IDECredential, config IDEClientConfig) (http.Header, error) {
	applyIDEConfigDefaults(&config)
	token := strings.TrimSpace(credential.AccessToken)
	if token == "" {
		return nil, badRequest("build IDE headers", fmt.Errorf("access token is required"))
	}
	machineID := strings.TrimSpace(credential.MachineID)
	if machineID == "" {
		hash := sha256.Sum256([]byte(token + "machineId"))
		machineID = hex.EncodeToString(hash[:])
	}
	clientKeyHash := sha256.Sum256([]byte(token))
	requestID := config.UUID()
	configVersion := config.ConfigVersion
	if strings.TrimSpace(configVersion) == "" {
		configVersion = config.UUID()
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Connect-Protocol-Version", "1")
	headers.Set("Accept-Encoding", "gzip")
	headers.Set("User-Agent", "connect-es/1.6.1")
	headers.Set("X-Amzn-Trace-Id", "Root="+requestID)
	headers.Set("X-Client-Key", hex.EncodeToString(clientKeyHash[:]))
	headers.Set("X-Cursor-Checksum", BuildJyhChecksum(config.Now(), machineID))
	headers.Set("X-Cursor-Client-Version", config.ClientVersion)
	headers.Set("X-Cursor-Client-Type", "ide")
	headers.Set("X-Cursor-Client-Os", config.ClientOS)
	headers.Set("X-Cursor-Client-Arch", config.ClientArch)
	headers.Set("X-Cursor-Client-Os-Version", config.ClientOSVersion)
	headers.Set("X-Cursor-Client-Device-Type", "desktop")
	headers.Set("X-Cursor-Config-Version", configVersion)
	headers.Set("X-Cursor-Timezone", config.Timezone)
	headers.Set("X-Ghost-Mode", fmt.Sprintf("%t", config.GhostMode))
	headers.Set("X-New-Onboarding-Completed", fmt.Sprintf("%t", config.NewOnboardingCompleted))
	headers.Set("X-Session-Id", uuid.NewSHA1(uuid.NameSpaceDNS, []byte(token)).String())
	headers.Set("X-Request-Id", requestID)
	return headers, nil
}

func BuildJyhChecksum(now time.Time, machineID string) string {
	timestamp := uint64(now.UnixMilli() / 1_000_000)
	encoded := []byte{
		byte(timestamp >> 40), byte(timestamp >> 32), byte(timestamp >> 24),
		byte(timestamp >> 16), byte(timestamp >> 8), byte(timestamp),
	}
	previous := byte(165)
	for index := range encoded {
		encoded[index] = (encoded[index] ^ previous) + byte(index)
		previous = encoded[index]
	}
	return base64.RawURLEncoding.EncodeToString(encoded) + machineID
}

func applyIDEConfigDefaults(config *IDEClientConfig) {
	if config.ClientVersion == "" {
		config.ClientVersion = "2.6.22"
	}
	if config.ClientOS == "" {
		config.ClientOS = normalizeIDEOS(runtime.GOOS)
	}
	if config.ClientArch == "" {
		config.ClientArch = normalizeIDEArch(runtime.GOARCH)
	}
	if config.ClientOSVersion == "" {
		config.ClientOSVersion = "unknown"
	}
	if config.Timezone == "" {
		config.Timezone = "UTC"
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.UUID == nil {
		config.UUID = func() string { return uuid.NewString() }
	}
	if config.MaxFrameSize <= 0 {
		config.MaxFrameSize = defaultIDEMaxFrameSize
	}
	if config.MaxBufferedBytes <= 0 {
		config.MaxBufferedBytes = defaultIDEMaxBufferedBytes
	}
	if config.MaxErrorBody <= 0 {
		config.MaxErrorBody = defaultIDEMaxErrorBody
	}
}

func normalizeIDEOS(value string) string {
	switch value {
	case "windows":
		return "win32"
	case "darwin":
		return "darwin"
	default:
		return value
	}
}

func normalizeIDEArch(value string) string {
	switch value {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return value
	}
}

func encodeIDEMetadata(config IDEClientConfig) []byte {
	var metadata []byte
	metadata = appendString(metadata, 1, config.ClientOS)
	metadata = appendString(metadata, 2, config.ClientArch)
	metadata = appendString(metadata, 3, config.ClientOSVersion)
	metadata = appendString(metadata, 4, "sub2api")
	metadata = appendString(metadata, 5, config.Now().Format(time.RFC3339Nano))
	return metadata
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (c *IDEClient) httpError(operation string, resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(c.config.MaxErrorBody)+1))
	truncated := len(body) > c.config.MaxErrorBody
	if truncated {
		body = body[:c.config.MaxErrorBody]
	}
	message := strings.TrimSpace(string(body))
	if truncated {
		message += "..."
	}
	return HTTPError(resp.StatusCode, operation, message)
}
