package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type playgroundResolverStub struct {
	addresses map[string][]netip.Addr
}

func (s playgroundResolverStub) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	addresses, ok := s.addresses[strings.TrimSuffix(strings.ToLower(host), ".")]
	if !ok {
		return nil, fmt.Errorf("host not found: %s", host)
	}
	return append([]netip.Addr(nil), addresses...), nil
}

func newPlaygroundFetchTestService(t *testing.T, server *httptest.Server, addresses map[string][]netip.Addr) *PlaygroundService {
	t.Helper()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	fetcher := &PlaygroundURLFetcher{
		resolver: playgroundResolverStub{addresses: addresses},
		dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, serverURL.Host)
		},
	}
	return &PlaygroundService{urlFetcher: fetcher}
}

func TestPlaygroundFetchURLsRejectsSSRFAddresses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	svc := newPlaygroundFetchTestService(t, server, map[string][]netip.Addr{
		"mixed.example": {netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.8")},
	})

	for _, rawURL := range []string{
		"http://localhost/",
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://user:pass@example.com/",
		"file:///etc/passwd",
		"http://mixed.example/",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := svc.FetchURLs(context.Background(), []string{rawURL})
			require.Error(t, err)
			require.Equal(t, "PLAYGROUND_FETCH_INVALID_URL", infraerrors.Reason(err))
		})
	}
}

func TestPlaygroundFetchURLsRevalidatesRedirectTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://private.example/secret", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("must not be reached"))
	}))
	defer server.Close()

	svc := newPlaygroundFetchTestService(t, server, map[string][]netip.Addr{
		"public.example":  {netip.MustParseAddr("8.8.8.8")},
		"private.example": {netip.MustParseAddr("192.168.1.10")},
	})

	_, err := svc.FetchURLs(context.Background(), []string{"http://public.example/redirect"})
	require.Error(t, err)
	require.Equal(t, "PLAYGROUND_FETCH_INVALID_URL", infraerrors.Reason(err))
}

func TestPlaygroundFetchURLsRejectsUnsupportedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not really a png"))
	}))
	defer server.Close()
	svc := newPlaygroundFetchTestService(t, server, map[string][]netip.Addr{"public.example": {netip.MustParseAddr("8.8.8.8")}})

	_, err := svc.FetchURLs(context.Background(), []string{"http://public.example/image"})
	require.Error(t, err)
	require.Equal(t, "PLAYGROUND_FETCH_UNSUPPORTED_CONTENT_TYPE", infraerrors.Reason(err))
}

func TestPlaygroundFetchURLRequestSupportsSingleAndBatchContracts(t *testing.T) {
	req := PlaygroundFetchURLRequest{URL: " http://one.example ", URLs: []string{"http://two.example"}, MaxBytes: 4096}
	require.Equal(t, []string{" http://one.example ", "http://two.example"}, req.RequestedURLs())
	require.Equal(t, int64(4096), req.ResponseLimit())

	req = PlaygroundFetchURLRequest{MaxBytesSnake: playgroundFetchMaxBodyBytes + 1}
	require.Equal(t, int64(playgroundFetchMaxBodyBytes), req.ResponseLimit())
}

func TestPlaygroundFetchURLsRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("x", playgroundFetchMaxBodyBytes+1)))
	}))
	defer server.Close()
	svc := newPlaygroundFetchTestService(t, server, map[string][]netip.Addr{"public.example": {netip.MustParseAddr("8.8.8.8")}})

	_, err := svc.FetchURLs(context.Background(), []string{"http://public.example/large"})
	require.Error(t, err)
	require.Equal(t, "PLAYGROUND_FETCH_RESPONSE_TOO_LARGE", infraerrors.Reason(err))
}

func TestPlaygroundFetchURLsReturnsSanitizedReadableHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Example</title><style>.hidden{}</style></head><body><h1>Hello &amp; welcome</h1><script>alert("secret")</script><p>Readable text.</p></body></html>`))
	}))
	defer server.Close()
	svc := newPlaygroundFetchTestService(t, server, map[string][]netip.Addr{"public.example": {netip.MustParseAddr("8.8.8.8")}})

	results, err := svc.FetchURLs(context.Background(), []string{"http://public.example/page"})
	require.NoError(t, err)
	require.Equal(t, []PlaygroundFetchedURL{{
		URL:         "http://public.example/page",
		StatusCode:  http.StatusOK,
		ContentType: "text/html",
		Content:     "Example Hello & welcome Readable text.",
	}}, results)
}

func TestPlaygroundFetchURLsLimitsBatchSize(t *testing.T) {
	svc := &PlaygroundService{urlFetcher: newPlaygroundURLFetcher()}
	_, err := svc.FetchURLs(context.Background(), []string{"http://example.com", "http://example.com", "http://example.com", "http://example.com"})
	require.ErrorIs(t, err, ErrPlaygroundFetchTooManyURLs)
}
