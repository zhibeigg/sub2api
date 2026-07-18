package firefly

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const maxResponseBody = 1 << 20
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
const secCHUA = `"Google Chrome";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`

var preferredHeaderOrder = []string{"authorization", "x-api-key", "content-type", "accept", "origin", "referer", "accept-language", "cache-control", "pragma", "priority", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest", "user-agent", "x-nonce", "x-arp-session-id"}

type headerGetter interface{ Get(string) string }
type transportResponse struct {
	status  int
	headers headerGetter
	body    []byte
}
type transportDoer interface {
	do(context.Context, string, string, map[string]string, []byte) (*transportResponse, error)
}
type tlsTransport struct {
	client  tlsclient.HttpClient
	initErr error
}

func newTransport(proxyURL string, timeout time.Duration) *tlsTransport {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	opts := []tlsclient.HttpClientOption{tlsclient.WithTimeoutSeconds(int(timeout.Seconds())), tlsclient.WithClientProfile(profiles.Chrome_133), tlsclient.WithNotFollowRedirects(), tlsclient.WithRandomTLSExtensionOrder()}
	if strings.TrimSpace(proxyURL) != "" {
		opts = append(opts, tlsclient.WithProxyUrl(proxyURL))
	}
	c, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	return &tlsTransport{client: c, initErr: err}
}
func (t *tlsTransport) do(ctx context.Context, method, url string, headers map[string]string, body []byte) (*transportResponse, error) {
	if t.initErr != nil {
		return nil, t.initErr
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	r, err := fhttp.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	r = r.WithContext(ctx)
	r.Header = fhttp.Header{}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	r.Header[fhttp.HeaderOrderKey] = orderedHeaderKeys(headers)
	resp, err := t.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxResponseBody {
		return nil, fmt.Errorf("upstream response exceeds size limit")
	}
	return &transportResponse{status: resp.StatusCode, headers: resp.Header, body: b}, nil
}

func orderedHeaderKeys(headers map[string]string) []string {
	actual := map[string]string{}
	for k := range headers {
		actual[strings.ToLower(k)] = k
	}
	out := make([]string, 0, len(headers))
	used := map[string]bool{}
	for _, k := range preferredHeaderOrder {
		if a, ok := actual[k]; ok {
			out = append(out, a)
			used[k] = true
		}
	}
	rest := []string{}
	for lower, k := range actual {
		if !used[lower] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}
func browserHeaders(token string) map[string]string {
	return map[string]string{"authorization": "Bearer " + token, "x-api-key": "clio-playground-web", "content-type": "application/json", "accept": "*/*", "origin": "https://firefly.adobe.com", "referer": "https://firefly.adobe.com/", "accept-language": "en-GB,en-US;q=0.9,en;q=0.8", "sec-ch-ua": secCHUA, "sec-ch-ua-mobile": "?0", "sec-ch-ua-platform": `"Windows"`, "sec-fetch-site": "cross-site", "sec-fetch-mode": "cors", "sec-fetch-dest": "empty", "user-agent": userAgent, "x-arp-session-id": generateARPSessionID()}
}
func computeNonce(userID, prompt string) string {
	if userID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(userID + "-" + firstRunes(prompt, 256)))
	return hex.EncodeToString(sum[:])
}
func extractUserIDFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var c map[string]any
	if json.Unmarshal(raw, &c) != nil {
		return ""
	}
	for _, k := range []string{"user_id", "aa_id", "sub"} {
		if v, ok := c[k].(string); ok {
			return v
		}
	}
	return ""
}
func generateARPSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	payload := map[string]string{"sid": fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), "ark": "web|pk=BBCC314C-4937-4CCD-B0A3-FDF0F0F7603C|r=ap-southeast-1", "ftr": hex.EncodeToString(b) + fmt.Sprintf("_%d__UDF43-m4_31ck_v2_tt", time.Now().UnixMilli())}
	raw, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(raw)
}
