package service

import (
	"fmt"
	"strings"
)

// PlatformCapabilities centralizes platform validation and behavior switches.
type PlatformCapabilities struct {
	DisplayName          string
	AccountTypes         map[string]struct{}
	EndpointProtocols    []EndpointProtocol
	ImageGeneration      bool
	VideoGeneration      bool
	BatchImageGeneration bool
	MixedScheduling      bool
	UpstreamModelSync    bool
	DefaultConcurrency   int
}

var platformCapabilities = map[string]PlatformCapabilities{
	PlatformAnthropic: {
		AccountTypes: accountTypeSet(AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey, AccountTypeUpstream, AccountTypeBedrock, AccountTypeServiceAccount),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
		),
		UpstreamModelSync: true,
	},
	PlatformOpenAI: {
		AccountTypes: accountTypeSet(AccountTypeOAuth, AccountTypeAPIKey, AccountTypeUpstream),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
			EndpointProtocolOpenAIEmbeddings,
			EndpointProtocolOpenAIAlphaSearch,
			EndpointProtocolOpenAIImages,
			EndpointProtocolOpenAIVideos,
		),
		ImageGeneration:   true,
		VideoGeneration:   true,
		UpstreamModelSync: true,
	},
	PlatformGemini: {
		AccountTypes: accountTypeSet(AccountTypeOAuth, AccountTypeAPIKey, AccountTypeServiceAccount),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
			EndpointProtocolGeminiGenerateContent,
			EndpointProtocolOpenAIImages,
		),
		ImageGeneration:      true,
		BatchImageGeneration: true,
		UpstreamModelSync:    true,
	},
	PlatformAntigravity: {
		AccountTypes: accountTypeSet(AccountTypeOAuth),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
			EndpointProtocolGeminiGenerateContent,
			EndpointProtocolOpenAIImages,
		),
		ImageGeneration:   true,
		MixedScheduling:   true,
		UpstreamModelSync: true,
	},
	PlatformGrok: {
		AccountTypes: accountTypeSet(AccountTypeOAuth),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
			EndpointProtocolOpenAIImages,
			EndpointProtocolOpenAIVideos,
		),
		ImageGeneration:    true,
		VideoGeneration:    true,
		UpstreamModelSync:  true,
		DefaultConcurrency: 1,
	},
	PlatformAdobe: {
		AccountTypes: accountTypeSet(AccountTypeOAuth),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolOpenAIImages,
			EndpointProtocolOpenAIVideos,
		),
		ImageGeneration:    true,
		VideoGeneration:    true,
		UpstreamModelSync:  false,
		DefaultConcurrency: 1,
	},
	PlatformCursor: {
		AccountTypes: accountTypeSet(AccountTypeAPIKey),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
		),
		MixedScheduling:    true,
		UpstreamModelSync:  true,
		DefaultConcurrency: 1,
	},
	PlatformOpenCode: {
		DisplayName:  PlatformOpenCodeDisplayName,
		AccountTypes: accountTypeSet(AccountTypeAPIKey),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
		),
		MixedScheduling:    true,
		UpstreamModelSync:  true,
		DefaultConcurrency: 1,
	},
	PlatformKiro: {
		AccountTypes: accountTypeSet(AccountTypeOAuth),
		EndpointProtocols: endpointProtocolList(
			EndpointProtocolAnthropicMessages,
			EndpointProtocolOpenAIChatCompletions,
			EndpointProtocolOpenAIResponses,
		),
		MixedScheduling:    true,
		UpstreamModelSync:  false,
		DefaultConcurrency: 1,
	},
}

func accountTypeSet(types ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(types))
	for _, accountType := range types {
		out[accountType] = struct{}{}
	}
	return out
}

func NormalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func GetPlatformCapabilities(platform string) (PlatformCapabilities, bool) {
	capabilities, ok := platformCapabilities[NormalizePlatform(platform)]
	return capabilities, ok
}

func IsValidPlatform(platform string) bool {
	_, ok := GetPlatformCapabilities(platform)
	return ok
}

func ValidatePlatform(platform string) error {
	if !IsValidPlatform(platform) {
		return fmt.Errorf("unsupported platform: %s", strings.TrimSpace(platform))
	}
	return nil
}

func ValidatePlatformAccountType(platform, accountType string) error {
	capabilities, ok := GetPlatformCapabilities(platform)
	if !ok {
		return fmt.Errorf("unsupported platform: %s", strings.TrimSpace(platform))
	}
	accountType = strings.ToLower(strings.TrimSpace(accountType))
	if _, ok := capabilities.AccountTypes[accountType]; !ok {
		return fmt.Errorf("account type %q is not supported for platform %q", accountType, NormalizePlatform(platform))
	}
	return nil
}

func PlatformSupportsImageGeneration(platform string) bool {
	capabilities, ok := GetPlatformCapabilities(platform)
	return ok && capabilities.ImageGeneration
}

func PlatformSupportsVideoGeneration(platform string) bool {
	capabilities, ok := GetPlatformCapabilities(platform)
	return ok && capabilities.VideoGeneration
}

func PlatformSupportsBatchImageGeneration(platform string) bool {
	capabilities, ok := GetPlatformCapabilities(platform)
	return ok && capabilities.BatchImageGeneration
}

func PlatformSupportsUpstreamModelSync(platform string) bool {
	capabilities, ok := GetPlatformCapabilities(platform)
	return ok && capabilities.UpstreamModelSync
}

func PlatformSupportsMixedScheduling(platform string) bool {
	capabilities, ok := GetPlatformCapabilities(platform)
	return ok && capabilities.MixedScheduling
}

func DefaultAccountConcurrency(platform string) int {
	capabilities, ok := GetPlatformCapabilities(platform)
	if ok && capabilities.DefaultConcurrency > 0 {
		return capabilities.DefaultConcurrency
	}
	return 0
}
