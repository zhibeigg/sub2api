package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const adobeMediaMaxImageBytes int64 = 20 << 20

func DownloadAdobeReferenceImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	validated, err := urlvalidator.ValidateHTTPSURL(rawURL, urlvalidator.ValidationOptions{AllowPrivate: false})
	if err != nil {
		return nil, "", err
	}
	u, _ := url.Parse(validated)
	if err := urlvalidator.ValidateResolvedIP(u.Hostname()); err != nil {
		return nil, "", err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			host = address
		}
		if err := urlvalidator.ValidateResolvedIP(host); err != nil {
			return nil, err
		}
		return dialer.DialContext(dialCtx, network, address)
	}}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		_, err := urlvalidator.ValidateHTTPSURL(req.URL.String(), urlvalidator.ValidationOptions{AllowPrivate: false})
		if err != nil {
			return err
		}
		return urlvalidator.ValidateResolvedIP(req.URL.Hostname())
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validated, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("reference image download failed: status %d", resp.StatusCode)
	}
	if resp.ContentLength > adobeMediaMaxImageBytes {
		return nil, "", fmt.Errorf("reference image exceeds size limit")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, adobeMediaMaxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > adobeMediaMaxImageBytes {
		return nil, "", fmt.Errorf("reference image exceeds size limit")
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	detected := http.DetectContentType(body)
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detected
	}
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return nil, "", fmt.Errorf("unsupported reference image MIME type")
	}
	if detected != contentType {
		return nil, "", fmt.Errorf("reference image MIME mismatch")
	}
	return body, contentType, nil
}

func DownloadAdobePNG(ctx context.Context, rawURL string) ([]byte, error) {
	body, contentType, err := DownloadAdobeReferenceImage(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if contentType != "image/png" {
		return nil, fmt.Errorf("adobe output is not PNG")
	}
	return body, nil
}
