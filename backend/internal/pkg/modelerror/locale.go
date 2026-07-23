package modelerror

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"golang.org/x/text/language"
)

const maxAcceptLanguageBytes = 4096

// Locale is a supported model-error language.
type Locale string

const (
	LocaleEnglish Locale = "en"
	LocaleChinese Locale = "zh"
)

func NormalizeLocale(raw string) (Locale, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en", "en-us", "en-gb":
		return LocaleEnglish, true
	case "zh", "zh-cn", "zh-hans", "cn":
		return LocaleChinese, true
	default:
		return "", false
	}
}

func normalizeFallback(raw string) Locale {
	if locale, ok := NormalizeLocale(raw); ok {
		return locale
	}
	return LocaleEnglish
}

// ResolveAcceptLanguage resolves supported languages using HTTP q weights and
// falls back deterministically when the header is absent, invalid or unsupported.
func ResolveAcceptLanguage(raw, fallback string) Locale {
	fallbackLocale := normalizeFallback(fallback)
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxAcceptLanguageBytes {
		return fallbackLocale
	}

	tags, qualities, err := language.ParseAcceptLanguage(raw)
	if err != nil && len(tags) == 0 {
		return fallbackLocale
	}
	indices := make([]int, 0, len(tags))
	for i := range tags {
		if i < len(qualities) && qualities[i] <= 0 {
			continue
		}
		indices = append(indices, i)
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left, right := float32(1), float32(1)
		if indices[i] < len(qualities) {
			left = qualities[indices[i]]
		}
		if indices[j] < len(qualities) {
			right = qualities[indices[j]]
		}
		return left > right
	})

	for _, index := range indices {
		base, _ := tags[index].Base()
		switch base.String() {
		case "zh":
			return LocaleChinese
		case "en":
			return LocaleEnglish
		}
	}
	return fallbackLocale
}

func WithLocale(ctx context.Context, locale Locale, fallback string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if normalized, ok := NormalizeLocale(string(locale)); ok {
		locale = normalized
	} else {
		locale = normalizeFallback(fallback)
	}
	ctx = context.WithValue(ctx, ctxkey.ModelErrorLocale, string(locale))
	return context.WithValue(ctx, ctxkey.ModelErrorDefaultLocale, string(normalizeFallback(fallback)))
}

func LocaleFromContext(ctx context.Context) Locale {
	if ctx != nil {
		if value, ok := ctx.Value(ctxkey.ModelErrorLocale).(string); ok {
			if locale, valid := NormalizeLocale(value); valid {
				return locale
			}
		}
		if value, ok := ctx.Value(ctxkey.ModelErrorDefaultLocale).(string); ok {
			return normalizeFallback(value)
		}
	}
	return LocaleEnglish
}

func ContentLanguage(locale Locale) string {
	if locale == LocaleChinese {
		return "zh-CN"
	}
	return "en"
}
