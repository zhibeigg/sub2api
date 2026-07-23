package modelerror

import (
	"context"
	"fmt"
	"strings"
)

const BrandPrefix = "[PokeAPI] "

func Present(ctx context.Context, descriptor Descriptor) Presentation {
	locale := LocaleFromContext(ctx)
	code := descriptor.Code
	if code == "" {
		code = CodeInternalError
	}

	message := strings.TrimSpace(descriptor.CustomMessage)
	if message != "" {
		message = SanitizeMessage(message)
	}
	if message == "" {
		message = catalogMessage(locale, code)
		message = formatParams(locale, code, message, descriptor.Params)
	}
	if strings.TrimSpace(message) == "" {
		code = CodeInternalError
		locale = LocaleEnglish
		message = catalogEnglish[CodeInternalError]
	}

	return Presentation{
		Code:    code,
		Locale:  locale,
		Message: BrandMessage(message),
	}
}

func catalogMessage(locale Locale, code Code) string {
	if locale == LocaleChinese {
		if message := strings.TrimSpace(catalogChinese[code]); message != "" {
			return message
		}
	}
	if message := strings.TrimSpace(catalogEnglish[code]); message != "" {
		return message
	}
	return catalogEnglish[CodeInternalError]
}

func formatParams(locale Locale, code Code, message string, params Params) string {
	model := safeModel(params.Model)
	switch code {
	case CodeModelUnsupported, CodeModelNotFound:
		if model == "" {
			return message
		}
		if locale == LocaleChinese {
			if code == CodeModelUnsupported {
				return fmt.Sprintf("当前分组不支持模型 %q，请检查模型名称或更换分组。", model)
			}
			return fmt.Sprintf("未找到模型 %q，或当前分组尚未配置该模型，请检查模型名称或分组配置。", model)
		}
		if code == CodeModelUnsupported {
			return fmt.Sprintf("Model %q is not supported by the current group. Check the model name or choose another group.", model)
		}
		return fmt.Sprintf("Model %q was not found or is not configured for the current group. Check the model name or group configuration.", model)
	case CodePayloadTooLarge:
		if params.LimitBytes <= 0 {
			return message
		}
		if locale == LocaleChinese {
			return fmt.Sprintf("请求内容超过 %s 的限制，请减少附件或请求正文后重试。", humanBytes(params.LimitBytes))
		}
		return fmt.Sprintf("The request exceeds the %s payload limit. Reduce attachments or request content and try again.", humanBytes(params.LimitBytes))
	case CodeConcurrencyLimit:
		scope := localizedScope(locale, params.Scope)
		if scope == "" {
			return message
		}
		if locale == LocaleChinese {
			return fmt.Sprintf("%s并发已达到上限，请等待正在执行的请求完成后重试。", scope)
		}
		return fmt.Sprintf("The %s concurrency limit has been reached. Wait for an active request to finish and retry.", scope)
	case CodeRateLimited, CodeUpstreamRateLimited:
		if params.RetryAfter <= 0 {
			return message
		}
		if locale == LocaleChinese {
			return fmt.Sprintf("请求受到限流，请等待约 %d 秒后重试。", params.RetryAfter)
		}
		return fmt.Sprintf("The request is rate-limited. Retry in about %d seconds.", params.RetryAfter)
	default:
		return message
	}
}

func BrandMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return BrandPrefix + catalogEnglish[CodeInternalError]
	}
	if strings.HasPrefix(message, BrandPrefix) || message == strings.TrimSpace(BrandPrefix) {
		return message
	}
	return BrandPrefix + message
}

func safeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	const maxModelRunes = 128
	runes := []rune(model)
	if len(runes) > maxModelRunes {
		runes = runes[:maxModelRunes]
	}
	for i, r := range runes {
		if r < 0x20 || r == 0x7f {
			runes[i] = ' '
		}
	}
	return strings.TrimSpace(string(runes))
}

func localizedScope(locale Locale, scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "user":
		if locale == LocaleChinese {
			return "用户级"
		}
		return "user"
	case "subscription":
		if locale == LocaleChinese {
			return "订阅级"
		}
		return "subscription"
	case "account":
		if locale == LocaleChinese {
			return "渠道账号级"
		}
		return "account"
	case "api_key", "apikey", "key":
		if locale == LocaleChinese {
			return "API Key 级"
		}
		return "API key"
	default:
		return ""
	}
}

func humanBytes(value int64) string {
	const (
		kiB = int64(1024)
		miB = 1024 * kiB
		giB = 1024 * miB
	)
	switch {
	case value >= giB && value%giB == 0:
		return fmt.Sprintf("%d GiB", value/giB)
	case value >= miB && value%miB == 0:
		return fmt.Sprintf("%d MiB", value/miB)
	case value >= kiB && value%kiB == 0:
		return fmt.Sprintf("%d KiB", value/kiB)
	default:
		return fmt.Sprintf("%d bytes", value)
	}
}
