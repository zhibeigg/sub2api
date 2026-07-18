package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/net/html"
)

const (
	playgroundFetchTimeout      = 10 * time.Second
	playgroundFetchMaxURLs      = 3
	playgroundFetchMaxBodyBytes = 1 << 20
	playgroundFetchMaxTextBytes = 100 << 10
	playgroundFetchMaxRedirects = 5
)

var (
	ErrPlaygroundFetchURLsRequired = infraerrors.BadRequest("PLAYGROUND_FETCH_URLS_REQUIRED", "at least one URL is required")
	ErrPlaygroundFetchTooManyURLs  = infraerrors.BadRequest("PLAYGROUND_FETCH_TOO_MANY_URLS", "at most 3 URLs are allowed")
	ErrPlaygroundFetchInvalidURL   = infraerrors.BadRequest("PLAYGROUND_FETCH_INVALID_URL", "URL must be a public http or https URL")
	ErrPlaygroundFetchUnsupported  = infraerrors.New(http.StatusUnsupportedMediaType, "PLAYGROUND_FETCH_UNSUPPORTED_CONTENT_TYPE", "URL response must use a textual content type")
	ErrPlaygroundFetchTooLarge     = infraerrors.New(http.StatusRequestEntityTooLarge, "PLAYGROUND_FETCH_RESPONSE_TOO_LARGE", "URL response exceeds the allowed size limit")
	ErrPlaygroundFetchTimeout      = infraerrors.New(http.StatusGatewayTimeout, "PLAYGROUND_FETCH_TIMEOUT", "URL fetch timed out")
	ErrPlaygroundFetchFailed       = infraerrors.New(http.StatusBadGateway, "PLAYGROUND_FETCH_FAILED", "failed to fetch URL")
)

type PlaygroundFetchURLRequest struct {
	URL           string   `json:"url"`
	URLs          []string `json:"urls"`
	MaxBytes      int64    `json:"maxBytes"`
	MaxBytesSnake int64    `json:"max_bytes"`
}

func (r PlaygroundFetchURLRequest) RequestedURLs() []string {
	urls := make([]string, 0, len(r.URLs)+1)
	if strings.TrimSpace(r.URL) != "" {
		urls = append(urls, r.URL)
	}
	urls = append(urls, r.URLs...)
	return urls
}

func (r PlaygroundFetchURLRequest) ResponseLimit() int64 {
	limit := r.MaxBytes
	if limit <= 0 {
		limit = r.MaxBytesSnake
	}
	if limit <= 0 || limit > playgroundFetchMaxBodyBytes {
		return playgroundFetchMaxBodyBytes
	}
	return limit
}

type PlaygroundFetchedURL struct {
	URL         string `json:"url"`
	FinalURL    string `json:"final_url,omitempty"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	StatusCode  int    `json:"status_code,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type playgroundURLResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type playgroundDialContext func(ctx context.Context, network, address string) (net.Conn, error)

type PlaygroundURLFetcher struct {
	resolver playgroundURLResolver
	dial     playgroundDialContext
}

func newPlaygroundURLFetcher() *PlaygroundURLFetcher {
	dialer := &net.Dialer{Timeout: playgroundFetchTimeout}
	return &PlaygroundURLFetcher{
		resolver: net.DefaultResolver,
		dial:     dialer.DialContext,
	}
}

func (s *PlaygroundService) FetchURLs(ctx context.Context, urls []string) ([]PlaygroundFetchedURL, error) {
	return s.FetchURLsWithLimit(ctx, urls, playgroundFetchMaxBodyBytes)
}

func (s *PlaygroundService) FetchURLsWithLimit(ctx context.Context, urls []string, maxBytes int64) ([]PlaygroundFetchedURL, error) {
	if len(urls) == 0 {
		return nil, ErrPlaygroundFetchURLsRequired
	}
	if len(urls) > playgroundFetchMaxURLs {
		return nil, ErrPlaygroundFetchTooManyURLs
	}

	ctx, cancel := context.WithTimeout(ctx, playgroundFetchTimeout)
	defer cancel()

	if maxBytes <= 0 || maxBytes > playgroundFetchMaxBodyBytes {
		maxBytes = playgroundFetchMaxBodyBytes
	}
	results := make([]PlaygroundFetchedURL, 0, len(urls))
	for _, rawURL := range urls {
		result, err := s.urlFetcher.fetch(ctx, rawURL, maxBytes)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (f *PlaygroundURLFetcher) fetch(ctx context.Context, rawURL string, maxBytes int64) (PlaygroundFetchedURL, error) {
	parsed, err := f.validateURL(ctx, rawURL)
	if err != nil {
		return PlaygroundFetchedURL{}, err
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: playgroundFetchTimeout,
		DialContext:           f.safeDialContext,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   playgroundFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= playgroundFetchMaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			_, err := f.validateURL(req.Context(), req.URL.String())
			return err
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return PlaygroundFetchedURL{}, ErrPlaygroundFetchInvalidURL.WithCause(err)
	}
	req.Header.Set("Accept", "text/html,application/json,application/xml,text/plain;q=0.9,text/*;q=0.8")
	req.Header.Set("User-Agent", "sub2api-playground-fetch/1")

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return PlaygroundFetchedURL{}, ErrPlaygroundFetchTimeout.WithCause(err)
		}
		var appErr *infraerrors.ApplicationError
		if errors.As(err, &appErr) {
			return PlaygroundFetchedURL{}, appErr
		}
		return PlaygroundFetchedURL{}, ErrPlaygroundFetchFailed.WithCause(err)
	}
	defer func() { _ = resp.Body.Close() }()

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !isPlaygroundTextMediaType(mediaType) {
		return PlaygroundFetchedURL{}, ErrPlaygroundFetchUnsupported
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return PlaygroundFetchedURL{}, ErrPlaygroundFetchFailed.WithCause(err)
	}
	if int64(len(body)) > maxBytes {
		return PlaygroundFetchedURL{}, ErrPlaygroundFetchTooLarge
	}

	text := string(body)
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		text, err = readableHTMLText(body)
		if err != nil {
			return PlaygroundFetchedURL{}, ErrPlaygroundFetchFailed.WithCause(err)
		}
	}
	truncated := len(text) > playgroundFetchMaxTextBytes
	text = truncatePlaygroundFetchText(text, playgroundFetchMaxTextBytes)
	originalURL := strings.TrimSpace(rawURL)
	finalURL := resp.Request.URL.String()
	if finalURL == originalURL {
		finalURL = ""
	}

	return PlaygroundFetchedURL{
		URL:         originalURL,
		FinalURL:    finalURL,
		Content:     text,
		StatusCode:  resp.StatusCode,
		ContentType: mediaType,
		Truncated:   truncated,
	}, nil
}

func (f *PlaygroundURLFetcher) validateURL(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, ErrPlaygroundFetchInvalidURL.WithCause(err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrPlaygroundFetchInvalidURL
	}
	if parsed.User != nil || parsed.Hostname() == "" {
		return nil, ErrPlaygroundFetchInvalidURL
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, ErrPlaygroundFetchInvalidURL
	}
	if err := f.validateHostAddresses(ctx, host); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (f *PlaygroundURLFetcher) validateHostAddresses(ctx context.Context, host string) error {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicPlaygroundAddress(addr) {
			return ErrPlaygroundFetchInvalidURL
		}
		return nil
	}

	addresses, err := f.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return ErrPlaygroundFetchFailed.WithCause(err)
	}
	for _, addr := range addresses {
		if !isPublicPlaygroundAddress(addr) {
			return ErrPlaygroundFetchInvalidURL
		}
	}
	return nil
}

func (f *PlaygroundURLFetcher) safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := f.resolver.LookupNetIP(ctx, "ip", strings.TrimSuffix(host, "."))
	if err != nil || len(addresses) == 0 {
		return nil, ErrPlaygroundFetchFailed.WithCause(err)
	}
	for _, addr := range addresses {
		if !isPublicPlaygroundAddress(addr) {
			return nil, ErrPlaygroundFetchInvalidURL
		}
	}
	return f.dial(ctx, network, net.JoinHostPort(addresses[0].String(), port))
}

func isPublicPlaygroundAddress(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range playgroundBlockedAddressPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

var playgroundBlockedAddressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func isPlaygroundTextMediaType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/ld+json", "application/xml", "application/xhtml+xml", "application/rss+xml", "application/atom+xml", "application/javascript", "application/x-javascript":
		return true
	default:
		return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
	}
}

func readableHTMLText(body []byte) (string, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, hidden bool) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "script", "style", "noscript", "svg", "canvas", "template":
				hidden = true
			}
		}
		if node.Type == html.TextNode && !hidden {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				if builder.Len() > 0 {
					_ = builder.WriteByte(' ')
				}
				_, _ = builder.WriteString(text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, hidden)
		}
	}
	walk(doc, false)
	return strings.Join(strings.FieldsFunc(builder.String(), unicode.IsSpace), " "), nil
}

func truncatePlaygroundFetchText(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	text = text[:maxBytes]
	for len(text) > 0 && !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return strings.TrimSpace(text)
}
