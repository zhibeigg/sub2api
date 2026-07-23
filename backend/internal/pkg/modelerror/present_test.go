package modelerror

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPresentSafeParams(t *testing.T) {
	ctx := WithLocale(context.Background(), LocaleEnglish, "en")
	presentation := Present(ctx, Descriptor{
		Code:   CodeModelUnsupported,
		Params: Params{Model: "gpt-test\nsecret"},
	})
	require.Contains(t, presentation.Message, `"gpt-test secret"`)
	require.True(t, strings.HasPrefix(presentation.Message, BrandPrefix))

	payload := Present(ctx, Descriptor{Code: CodePayloadTooLarge, Params: Params{LimitBytes: 32 * 1024 * 1024}})
	require.Contains(t, payload.Message, "32 MiB")
}

func TestUnknownCodeFallsBackSafely(t *testing.T) {
	presentation := Present(context.Background(), Descriptor{Code: Code("UNKNOWN")})
	require.Equal(t, Code("UNKNOWN"), presentation.Code)
	require.Contains(t, presentation.Message, "[PokeAPI]")
	require.Contains(t, presentation.Message, "request ID")
}
