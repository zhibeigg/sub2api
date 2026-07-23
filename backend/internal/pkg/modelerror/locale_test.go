package modelerror

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestResolveAcceptLanguage(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		fallback string
		want     Locale
	}{
		{name: "zh cn", header: "zh-CN", fallback: "en", want: LocaleChinese},
		{name: "zh hans", header: "zh-Hans", fallback: "en", want: LocaleChinese},
		{name: "english", header: "en-US", fallback: "zh", want: LocaleEnglish},
		{name: "quality", header: "en;q=0.4, zh-CN;q=0.9", fallback: "en", want: LocaleChinese},
		{name: "zero quality ignored", header: "zh;q=0, en;q=0.8", fallback: "zh", want: LocaleEnglish},
		{name: "unsupported", header: "fr-FR, de;q=0.8", fallback: "zh", want: LocaleChinese},
		{name: "invalid", header: "not a language;;;", fallback: "en", want: LocaleEnglish},
		{name: "empty", header: "", fallback: "zh", want: LocaleChinese},
		{name: "invalid fallback", header: "", fallback: "fr", want: LocaleEnglish},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, ResolveAcceptLanguage(test.header, test.fallback))
		})
	}
}

func TestCatalogsHaveSameKeys(t *testing.T) {
	require.Equal(t, len(catalogEnglish), len(catalogChinese))
	for code := range catalogEnglish {
		require.NotEmpty(t, catalogChinese[code], code)
	}
	for code := range catalogChinese {
		require.NotEmpty(t, catalogEnglish[code], code)
	}
}

func TestPresentBrandsAndLocalizes(t *testing.T) {
	ctx := WithLocale(context.Background(), LocaleChinese, "en")
	presentation := Present(ctx, Descriptor{Code: CodeContextTooLarge})
	require.Equal(t, LocaleChinese, presentation.Locale)
	require.Equal(t, CodeContextTooLarge, presentation.Code)
	require.Contains(t, presentation.Message, "[PokeAPI]")
	require.Contains(t, presentation.Message, "上下文")

	custom := Present(ctx, Descriptor{Code: CodeInvalidRequest, CustomMessage: "[PokeAPI] 自定义提示"})
	require.Equal(t, "[PokeAPI] 自定义提示", custom.Message)
}

func TestApplyErrorHeaders(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-123")
	presentation := Presentation{Code: CodeInvalidRequest, Locale: LocaleChinese, Message: "[PokeAPI] x"}
	header := make(http.Header)
	header.Add("Vary", "Origin")
	ApplyErrorHeaders(ctx, header, presentation)

	require.Equal(t, "POKE_INVALID_REQUEST", header.Get(HeaderErrorCode))
	require.Equal(t, "req-123", header.Get(HeaderRequestID))
	require.Equal(t, "zh-CN", header.Get("Content-Language"))
	require.Equal(t, []string{"Origin", "Accept-Language"}, header.Values("Vary"))
}
