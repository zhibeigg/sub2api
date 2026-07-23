package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// EndpointProtocol identifies a public ingress protocol independently from the
// account provider used to execute the request.
type EndpointProtocol string

const (
	EndpointProtocolAnthropicMessages     EndpointProtocol = "anthropic_messages"
	EndpointProtocolOpenAIChatCompletions EndpointProtocol = "openai_chat_completions"
	EndpointProtocolOpenAIResponses       EndpointProtocol = "openai_responses"
	EndpointProtocolGeminiGenerateContent EndpointProtocol = "gemini_generate_content"
	EndpointProtocolOpenAIEmbeddings      EndpointProtocol = "openai_embeddings"
	EndpointProtocolOpenAIAlphaSearch     EndpointProtocol = "openai_alpha_search"
	EndpointProtocolOpenAIImages          EndpointProtocol = "openai_images"
	EndpointProtocolOpenAIVideos          EndpointProtocol = "openai_videos"
)

var endpointProtocolOrder = []EndpointProtocol{
	EndpointProtocolAnthropicMessages,
	EndpointProtocolOpenAIChatCompletions,
	EndpointProtocolOpenAIResponses,
	EndpointProtocolGeminiGenerateContent,
	EndpointProtocolOpenAIEmbeddings,
	EndpointProtocolOpenAIAlphaSearch,
	EndpointProtocolOpenAIImages,
	EndpointProtocolOpenAIVideos,
}

var endpointProtocolSet = func() map[EndpointProtocol]struct{} {
	out := make(map[EndpointProtocol]struct{}, len(endpointProtocolOrder))
	for _, protocol := range endpointProtocolOrder {
		out[protocol] = struct{}{}
	}
	return out
}()

func endpointProtocolList(protocols ...EndpointProtocol) []EndpointProtocol {
	return append([]EndpointProtocol(nil), protocols...)
}

// AllEndpointProtocols returns the canonical stable protocol order.
func AllEndpointProtocols() []EndpointProtocol {
	return append([]EndpointProtocol(nil), endpointProtocolOrder...)
}

func NormalizeEndpointProtocol(protocol EndpointProtocol) EndpointProtocol {
	return EndpointProtocol(strings.ToLower(strings.TrimSpace(string(protocol))))
}

func IsValidEndpointProtocol(protocol EndpointProtocol) bool {
	_, ok := endpointProtocolSet[NormalizeEndpointProtocol(protocol)]
	return ok
}

// NormalizeEndpointProtocols trims, lowercases, validates, deduplicates and
// returns protocols in the canonical stable order. An empty set is invalid.
func NormalizeEndpointProtocols(protocols []string) ([]string, error) {
	return normalizeEndpointProtocols(protocols, false)
}

// NormalizeEndpointProtocolsAllowEmpty is the explicit persistence/helper
// variant for optional fields where an empty set has a caller-defined meaning.
func NormalizeEndpointProtocolsAllowEmpty(protocols []string) ([]string, error) {
	return normalizeEndpointProtocols(protocols, true)
}

func normalizeEndpointProtocols(protocols []string, allowEmpty bool) ([]string, error) {
	seen := make(map[EndpointProtocol]struct{}, len(protocols))
	for _, raw := range protocols {
		protocol := NormalizeEndpointProtocol(EndpointProtocol(raw))
		if !IsValidEndpointProtocol(protocol) {
			return nil, fmt.Errorf("unsupported endpoint protocol: %s", strings.TrimSpace(raw))
		}
		seen[protocol] = struct{}{}
	}
	if len(seen) == 0 {
		if allowEmpty {
			return []string{}, nil
		}
		return nil, fmt.Errorf("at least one endpoint protocol is required")
	}
	out := make([]string, 0, len(seen))
	for _, protocol := range endpointProtocolOrder {
		if _, ok := seen[protocol]; ok {
			out = append(out, string(protocol))
		}
	}
	return out, nil
}

func protocolStrings(protocols []EndpointProtocol) []string {
	out := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		out = append(out, string(protocol))
	}
	return out
}

func containsEndpointProtocol(protocols []string, protocol EndpointProtocol) bool {
	protocol = NormalizeEndpointProtocol(protocol)
	for _, candidate := range protocols {
		if NormalizeEndpointProtocol(EndpointProtocol(candidate)) == protocol {
			return true
		}
	}
	return false
}

// LegacyEndpointProtocols mirrors the pre-registry group routing semantics.
// Media flags remain authoritative; OpenAI Messages remains opt-in.
func LegacyEndpointProtocols(group *Group) []string {
	if group == nil {
		return nil
	}
	protocols := make([]string, 0, 8)
	add := func(protocol EndpointProtocol) { protocols = append(protocols, string(protocol)) }

	switch NormalizePlatform(group.Platform) {
	case PlatformAnthropic:
		add(EndpointProtocolAnthropicMessages)
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
	case PlatformOpenAI:
		if group.AllowMessagesDispatch {
			add(EndpointProtocolAnthropicMessages)
		}
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
		add(EndpointProtocolOpenAIEmbeddings)
		add(EndpointProtocolOpenAIAlphaSearch)
		if group.AllowImageGeneration || group.AllowBatchImageGeneration {
			add(EndpointProtocolOpenAIImages)
			add(EndpointProtocolOpenAIVideos)
		}
	case PlatformGemini:
		add(EndpointProtocolAnthropicMessages)
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
		add(EndpointProtocolGeminiGenerateContent)
		if group.AllowImageGeneration || group.AllowBatchImageGeneration {
			add(EndpointProtocolOpenAIImages)
		}
	case PlatformAntigravity:
		add(EndpointProtocolAnthropicMessages)
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
		add(EndpointProtocolGeminiGenerateContent)
		if group.AllowImageGeneration || group.AllowBatchImageGeneration {
			add(EndpointProtocolOpenAIImages)
		}
	case PlatformGrok:
		add(EndpointProtocolAnthropicMessages)
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
		if group.AllowImageGeneration || group.AllowBatchImageGeneration {
			add(EndpointProtocolOpenAIImages)
			add(EndpointProtocolOpenAIVideos)
		}
	case PlatformAdobe:
		add(EndpointProtocolOpenAIImages)
		add(EndpointProtocolOpenAIVideos)
	case PlatformCursor, PlatformOpenCode, PlatformKiro:
		add(EndpointProtocolAnthropicMessages)
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
	}

	normalized, err := NormalizeEndpointProtocolsAllowEmpty(protocols)
	if err != nil {
		return nil
	}
	return normalized
}

// configuredGroupEndpointProtocols reads Group.EndpointProtocols without a
// compile-time dependency on the data-layer field being introduced in parallel.
func configuredGroupEndpointProtocols(group *Group) ([]string, bool) {
	if group == nil {
		return nil, false
	}
	value := reflect.ValueOf(group)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	field := value.FieldByName("EndpointProtocols")
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return nil, false
	}
	protocols := make([]string, 0, field.Len())
	for i := 0; i < field.Len(); i++ {
		item := field.Index(i)
		if item.Kind() != reflect.String {
			return nil, true
		}
		protocols = append(protocols, item.String())
	}
	return protocols, true
}

// GroupEndpointProtocols returns configured protocols when present, otherwise
// the legacy projection. Invalid configured values fail closed as an empty set.
func GroupEndpointProtocols(group *Group) []string {
	if group == nil {
		return protocolStrings(AllEndpointProtocols())
	}
	if configured, present := configuredGroupEndpointProtocols(group); present {
		if len(configured) == 0 {
			if group.Hydrated {
				return nil
			}
			return LegacyEndpointProtocols(group)
		}
		normalized, err := NormalizeEndpointProtocols(configured)
		if err != nil {
			return nil
		}
		return normalized
	}
	return LegacyEndpointProtocols(group)
}

func GroupAllowsEndpoint(group *Group, protocol EndpointProtocol) bool {
	protocol = NormalizeEndpointProtocol(protocol)
	return IsValidEndpointProtocol(protocol) && containsEndpointProtocol(GroupEndpointProtocols(group), protocol)
}

func platformSupportsEndpointProtocol(platform string, protocol EndpointProtocol) bool {
	capabilities, ok := GetPlatformCapabilities(platform)
	if !ok {
		return false
	}
	protocol = NormalizeEndpointProtocol(protocol)
	for _, supported := range capabilities.EndpointProtocols {
		if NormalizeEndpointProtocol(supported) == protocol {
			return true
		}
	}
	return false
}

// SupportedEndpointProtocols returns account-specific ingress capabilities.
// OpenAI endpoint probes/configuration are reused instead of trusting the static
// platform declaration for Responses, Embeddings and Alpha Search.
func SupportedEndpointProtocols(account *Account) []string {
	if account == nil {
		return nil
	}
	protocols := make([]string, 0, 8)
	add := func(protocol EndpointProtocol) { protocols = append(protocols, string(protocol)) }

	switch NormalizePlatform(account.Platform) {
	case PlatformOpenAI:
		if account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions) {
			add(EndpointProtocolAnthropicMessages)
			add(EndpointProtocolOpenAIChatCompletions)
		}
		if account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses) {
			add(EndpointProtocolOpenAIResponses)
		}
		if account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings) {
			add(EndpointProtocolOpenAIEmbeddings)
		}
		if account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAlphaSearch) {
			add(EndpointProtocolOpenAIAlphaSearch)
		}
		if account.SupportsOpenAIImageCapability(OpenAIImagesCapabilityBasic) {
			add(EndpointProtocolOpenAIImages)
		}
		if account.Type == AccountTypeAPIKey {
			add(EndpointProtocolOpenAIVideos)
		}
	case PlatformGrok:
		add(EndpointProtocolAnthropicMessages)
		add(EndpointProtocolOpenAIChatCompletions)
		add(EndpointProtocolOpenAIResponses)
		if eligible, _ := account.GrokMediaGenerationEligibility(); eligible {
			add(EndpointProtocolOpenAIImages)
			add(EndpointProtocolOpenAIVideos)
		}
	default:
		capabilities, ok := GetPlatformCapabilities(account.Platform)
		if !ok {
			return nil
		}
		protocols = append(protocols, protocolStrings(capabilities.EndpointProtocols)...)
	}

	normalized, err := NormalizeEndpointProtocolsAllowEmpty(protocols)
	if err != nil {
		return nil
	}
	return normalized
}

var endpointProtocolCandidatePlatformOrder = []string{
	PlatformAnthropic,
	PlatformOpenAI,
	PlatformGemini,
	PlatformAntigravity,
	PlatformGrok,
	PlatformAdobe,
	PlatformCursor,
	PlatformOpenCode,
	PlatformKiro,
}

// CandidateAccountPlatforms returns provider platforms that have an adapter for
// the ingress protocol. A forced platform is an exact restriction, not a hint.
func CandidateAccountPlatforms(protocol EndpointProtocol, forcedPlatform ...string) []string {
	protocol = NormalizeEndpointProtocol(protocol)
	if !IsValidEndpointProtocol(protocol) {
		return nil
	}
	if len(forcedPlatform) > 0 && strings.TrimSpace(forcedPlatform[0]) != "" {
		platform := NormalizePlatform(forcedPlatform[0])
		if platformSupportsEndpointProtocol(platform, protocol) {
			return []string{platform}
		}
		return nil
	}
	out := make([]string, 0, len(endpointProtocolCandidatePlatformOrder))
	for _, platform := range endpointProtocolCandidatePlatformOrder {
		if platformSupportsEndpointProtocol(platform, protocol) {
			out = append(out, platform)
		}
	}
	return out
}

// RequestDescriptor contains provider-independent request requirements.
type RequestDescriptor struct {
	Protocol              EndpointProtocol
	Model                 string
	ForcedPlatform        string
	EndpointPath          string
	OpenAIImageCapability OpenAIImagesCapability
}

// AccountGroupCompatibilityOptions carries group/association policy without a
// direct dependency on account_group persistence fields being added in parallel.
type AccountGroupCompatibilityOptions struct {
	Context                      context.Context
	Group                        *Group
	GroupPlatform                string
	EndpointProtocols            []string
	ForcedPlatform               string
	AllowMixedScheduling         bool
	HasAccountGroupBinding       bool
	EndpointCompatibilityEnabled bool
	RequireOAuthOnly             bool
	RequirePrivacySet            bool
	SkipSchedulabilityChecks     bool
}

// AccountGroupCompatibilityOptionsFrom projects the association flag without
// coupling the compatibility predicate to repository or Ent types.
func AccountGroupCompatibilityOptionsFrom(accountGroup *AccountGroup) AccountGroupCompatibilityOptions {
	if accountGroup == nil {
		return AccountGroupCompatibilityOptions{}
	}
	return AccountGroupCompatibilityOptions{
		Group:                        accountGroup.Group,
		HasAccountGroupBinding:       true,
		EndpointCompatibilityEnabled: accountGroup.EndpointCompatibilityEnabled,
	}
}

// IsAccountCompatibleForRequest is the common provider-independent eligibility
// predicate. Text adapters keep existing permissive mapping semantics; endpoint-
// specific capabilities and media model classification fail closed.
func IsAccountCompatibleForRequest(account *Account, request RequestDescriptor, options AccountGroupCompatibilityOptions) bool {
	if account == nil {
		return false
	}
	protocol := NormalizeEndpointProtocol(request.Protocol)
	if !IsValidEndpointProtocol(protocol) {
		return false
	}

	if options.Group != nil && !GroupAllowsEndpoint(options.Group, protocol) {
		return false
	}
	if len(options.EndpointProtocols) > 0 {
		normalized, err := NormalizeEndpointProtocols(options.EndpointProtocols)
		if err != nil || !containsEndpointProtocol(normalized, protocol) {
			return false
		}
	}
	if !containsEndpointProtocol(SupportedEndpointProtocols(account), protocol) {
		return false
	}

	forcedPlatform := NormalizePlatform(request.ForcedPlatform)
	if forcedPlatform == "" {
		forcedPlatform = NormalizePlatform(options.ForcedPlatform)
	}
	if forcedPlatform != "" && NormalizePlatform(account.Platform) != forcedPlatform {
		return false
	}

	groupPlatform := NormalizePlatform(options.GroupPlatform)
	if groupPlatform == "" && options.Group != nil {
		groupPlatform = NormalizePlatform(options.Group.Platform)
	}
	if forcedPlatform != "" {
		groupPlatform = forcedPlatform
	}
	if groupPlatform != "" && NormalizePlatform(account.Platform) != groupPlatform {
		if options.HasAccountGroupBinding || options.EndpointCompatibilityEnabled {
			// A concrete cross-provider relation is fail-closed unless that exact
			// relation was explicitly enabled. Account-level mixed_scheduling must
			// never bypass a disabled account_groups row.
			if !options.EndpointCompatibilityEnabled {
				return false
			}
		} else {
			// Preserve the legacy ungrouped/simple-mode mixed-scheduling behavior
			// only when there is no concrete relation whose policy could be bypassed.
			legacyMixedAllowed := options.AllowMixedScheduling && account.IsMixedSchedulingEnabled()
			if !legacyMixedAllowed {
				return false
			}
		}
	}

	if options.RequireOAuthOnly && account.Type == AccountTypeAPIKey {
		return false
	}
	if options.RequirePrivacySet && !account.IsPrivacySet() {
		return false
	}

	model := strings.TrimSpace(request.Model)
	if !options.SkipSchedulabilityChecks {
		ctx := options.Context
		if ctx == nil {
			ctx = context.Background()
		}
		if !account.IsSchedulableForModelWithContext(ctx, model) {
			return false
		}
	}
	if model != "" {
		if account.IsOpenCode() {
			if !account.IsOpenCodeModelSupported(model) {
				return false
			}
		} else if !account.IsModelSupported(model) {
			return false
		}
	}

	switch protocol {
	case EndpointProtocolAnthropicMessages,
		EndpointProtocolOpenAIChatCompletions,
		EndpointProtocolOpenAIResponses,
		EndpointProtocolGeminiGenerateContent:
		return true
	case EndpointProtocolOpenAIEmbeddings:
		return account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings)
	case EndpointProtocolOpenAIAlphaSearch:
		return account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAlphaSearch)
	case EndpointProtocolOpenAIImages:
		return accountCompatibleWithImageRequest(account, model, request)
	case EndpointProtocolOpenAIVideos:
		return accountCompatibleWithVideoRequest(account, model)
	default:
		return false
	}
}

func accountCompatibleWithImageRequest(account *Account, model string, request RequestDescriptor) bool {
	if model == "" {
		return false
	}
	if account.IsOpenAI() {
		capability := request.OpenAIImageCapability
		if capability == "" {
			capability = OpenAIImagesCapabilityNative
		}
		endpoint := strings.TrimSpace(request.EndpointPath)
		if endpoint == "" {
			endpoint = openAIImagesGenerationsEndpoint
		}
		return account.SupportsOpenAIImageRequest(model, endpoint, capability)
	}
	if account.IsGrok() {
		eligible, _ := account.GrokMediaGenerationEligibility()
		return eligible && isGrokImageGenerationModel(account.GetMappedModel(model))
	}
	if !PlatformSupportsImageGeneration(account.Platform) {
		return false
	}
	return ModelMediaType(account.GetMappedModel(model)) == PlaygroundCapabilityImage
}

func accountCompatibleWithVideoRequest(account *Account, model string) bool {
	if model == "" || !PlatformSupportsVideoGeneration(account.Platform) {
		return false
	}
	if account.IsGrok() {
		eligible, _ := account.GrokMediaGenerationEligibility()
		if !eligible {
			return false
		}
	}
	return ModelMediaType(account.GetMappedModel(model)) == PlaygroundCapabilityVideo
}

// legacyMixedSchedulingCandidatePlatforms preserves the old query surface while
// centralizing platform normalization/validation in the registry.
func legacyMixedSchedulingCandidatePlatforms(groupPlatform string) []string {
	groupPlatform = NormalizePlatform(groupPlatform)
	platforms := []string{groupPlatform}
	switch groupPlatform {
	case PlatformAnthropic:
		platforms = append(platforms, PlatformAntigravity, PlatformKiro, PlatformCursor, PlatformOpenCode)
	case PlatformGemini:
		platforms = append(platforms, PlatformAntigravity, PlatformKiro, PlatformCursor)
	case PlatformOpenAI:
		platforms = append(platforms, PlatformAntigravity, PlatformKiro, PlatformCursor, PlatformOpenCode)
	case PlatformGrok:
		platforms = append(platforms, PlatformCursor)
	}
	out := make([]string, 0, len(platforms))
	seen := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platform = NormalizePlatform(platform)
		if !IsValidPlatform(platform) {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		out = append(out, platform)
	}
	return out
}

type endpointProtocolContextKey struct{}
type crossProviderCompatibilityContextKey struct{}

// WithCrossProviderCompatibilityEnabled stores the request-scoped rollout state
// used by all compatibility predicates. The zero/absent value is fail-closed.
func WithCrossProviderCompatibilityEnabled(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, crossProviderCompatibilityContextKey{}, enabled)
}

func CrossProviderCompatibilityEnabledFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(crossProviderCompatibilityContextKey{}).(bool)
	return enabled
}

// WithEndpointProtocol stores a validated ingress protocol in context.
func WithEndpointProtocol(ctx context.Context, protocol EndpointProtocol) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	protocol = NormalizeEndpointProtocol(protocol)
	if !IsValidEndpointProtocol(protocol) {
		return ctx
	}
	return context.WithValue(ctx, endpointProtocolContextKey{}, protocol)
}

func EndpointProtocolFromContext(ctx context.Context) (EndpointProtocol, bool) {
	if ctx == nil {
		return "", false
	}
	protocol, ok := ctx.Value(endpointProtocolContextKey{}).(EndpointProtocol)
	protocol = NormalizeEndpointProtocol(protocol)
	return protocol, ok && IsValidEndpointProtocol(protocol)
}

// EndpointProtocolForRequestPath maps public route families to registry values.
func EndpointProtocolForRequestPath(path string) (EndpointProtocol, bool) {
	path = strings.ToLower(strings.TrimSpace(path))
	if query := strings.IndexByte(path, '?'); query >= 0 {
		path = path[:query]
	}
	switch {
	case strings.Contains(path, "/images/batches"):
		return EndpointProtocolOpenAIImages, true
	case strings.Contains(path, "/alpha/search"):
		return EndpointProtocolOpenAIAlphaSearch, true
	case strings.Contains(path, "/chat/completions"):
		return EndpointProtocolOpenAIChatCompletions, true
	case strings.Contains(path, "/embeddings"):
		return EndpointProtocolOpenAIEmbeddings, true
	case strings.Contains(path, "/images/"):
		return EndpointProtocolOpenAIImages, true
	case strings.Contains(path, "/videos/"):
		return EndpointProtocolOpenAIVideos, true
	case strings.Contains(path, "/responses"):
		return EndpointProtocolOpenAIResponses, true
	case strings.Contains(path, "/messages/count_tokens"):
		return EndpointProtocolAnthropicMessages, true
	case strings.Contains(path, "/v1beta/") && (strings.Contains(path, ":generatecontent") || strings.Contains(path, ":streamgeneratecontent")):
		return EndpointProtocolGeminiGenerateContent, true
	case path == "/v1/messages" || path == "/messages" || strings.HasSuffix(path, "/v1/messages"):
		return EndpointProtocolAnthropicMessages, true
	default:
		return "", false
	}
}
