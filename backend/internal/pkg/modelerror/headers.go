package modelerror

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	HeaderErrorCode = "X-PokeAPI-Error-Code"
	HeaderRequestID = "X-PokeAPI-Request-ID"
)

func ApplyRequestIDHeader(ctx context.Context, header http.Header) {
	if header == nil || strings.TrimSpace(header.Get(HeaderRequestID)) != "" {
		return
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		header.Set(HeaderRequestID, requestID)
	}
}

func ApplyErrorHeaders(ctx context.Context, header http.Header, presentation Presentation) {
	if header == nil {
		return
	}
	ApplyRequestIDHeader(ctx, header)
	header.Set(HeaderErrorCode, string(presentation.Code))
	header.Set("Content-Language", ContentLanguage(presentation.Locale))
	appendVary(header, "Accept-Language")
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(ctxkey.RequestID).(string)
	return strings.TrimSpace(requestID)
}

func appendVary(header http.Header, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, current := range header.Values("Vary") {
		for _, part := range strings.Split(current, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
