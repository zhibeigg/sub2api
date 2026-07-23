package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeEndpointProtocols(t *testing.T) {
	t.Parallel()

	got, err := NormalizeEndpointProtocols([]string{
		" OPENAI_IMAGES ",
		"openai_responses",
		"ANTHROPIC_MESSAGES",
		"openai_images",
		"openai_chat_completions",
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		string(EndpointProtocolAnthropicMessages),
		string(EndpointProtocolOpenAIChatCompletions),
		string(EndpointProtocolOpenAIResponses),
		string(EndpointProtocolOpenAIImages),
	}, got)

	_, err = NormalizeEndpointProtocols(nil)
	require.Error(t, err)
	require.Equal(t, []string{}, mustNormalizeEndpointProtocolsAllowEmpty(t, nil))

	_, err = NormalizeEndpointProtocols([]string{"openai_chat_completions", "future_protocol"})
	require.Error(t, err)
}

func mustNormalizeEndpointProtocolsAllowEmpty(t *testing.T, protocols []string) []string {
	t.Helper()
	normalized, err := NormalizeEndpointProtocolsAllowEmpty(protocols)
	require.NoError(t, err)
	return normalized
}

func TestLegacyEndpointProtocolsMatchesMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		group *Group
		want  []string
	}{
		{
			name:  "anthropic exposes all legacy text ingress",
			group: &Group{Platform: PlatformAnthropic},
			want:  protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses),
		},
		{
			name:  "openai text without messages dispatch",
			group: &Group{Platform: PlatformOpenAI},
			want: protocolNames(
				EndpointProtocolOpenAIChatCompletions,
				EndpointProtocolOpenAIResponses,
				EndpointProtocolOpenAIEmbeddings,
				EndpointProtocolOpenAIAlphaSearch,
			),
		},
		{
			name: "openai messages and media from batch flag",
			group: &Group{
				Platform:                  PlatformOpenAI,
				AllowMessagesDispatch:     true,
				AllowBatchImageGeneration: true,
			},
			want: protocolNames(
				EndpointProtocolAnthropicMessages,
				EndpointProtocolOpenAIChatCompletions,
				EndpointProtocolOpenAIResponses,
				EndpointProtocolOpenAIEmbeddings,
				EndpointProtocolOpenAIAlphaSearch,
				EndpointProtocolOpenAIImages,
				EndpointProtocolOpenAIVideos,
			),
		},
		{
			name:  "gemini legacy text adapters",
			group: &Group{Platform: PlatformGemini},
			want: protocolNames(
				EndpointProtocolAnthropicMessages,
				EndpointProtocolOpenAIChatCompletions,
				EndpointProtocolOpenAIResponses,
				EndpointProtocolGeminiGenerateContent,
			),
		},
		{
			name:  "antigravity image enabled by batch flag",
			group: &Group{Platform: PlatformAntigravity, AllowBatchImageGeneration: true},
			want: protocolNames(
				EndpointProtocolAnthropicMessages,
				EndpointProtocolOpenAIChatCompletions,
				EndpointProtocolOpenAIResponses,
				EndpointProtocolGeminiGenerateContent,
				EndpointProtocolOpenAIImages,
			),
		},
		{
			name:  "grok media reuses image flag",
			group: &Group{Platform: PlatformGrok, AllowImageGeneration: true},
			want: protocolNames(
				EndpointProtocolAnthropicMessages,
				EndpointProtocolOpenAIChatCompletions,
				EndpointProtocolOpenAIResponses,
				EndpointProtocolOpenAIImages,
				EndpointProtocolOpenAIVideos,
			),
		},
		{
			name:  "adobe media is always public",
			group: &Group{Platform: PlatformAdobe},
			want:  protocolNames(EndpointProtocolOpenAIImages, EndpointProtocolOpenAIVideos),
		},
		{
			name:  "cursor bridge",
			group: &Group{Platform: PlatformCursor},
			want:  protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses),
		},
		{
			name:  "opencode bridge",
			group: &Group{Platform: PlatformOpenCode},
			want:  protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses),
		},
		{
			name:  "kiro bridge",
			group: &Group{Platform: PlatformKiro},
			want:  protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, LegacyEndpointProtocols(tt.group))
		})
	}
}

func TestGroupEndpointProtocolsConfiguredOverride(t *testing.T) {
	t.Parallel()

	group := &Group{
		Platform: PlatformOpenAI,
		EndpointProtocols: []string{
			" OPENAI_IMAGES ",
			"anthropic_messages",
			"openai_images",
		},
	}
	require.Equal(t, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIImages), GroupEndpointProtocols(group))
	require.True(t, GroupAllowsEndpoint(group, EndpointProtocolOpenAIImages))
	require.False(t, GroupAllowsEndpoint(group, EndpointProtocolOpenAIResponses))

	group.EndpointProtocols = []string{"future_protocol"}
	require.Empty(t, GroupEndpointProtocols(group), "invalid persisted capabilities must fail closed")
}

func TestPlatformEndpointProtocolRegistry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		platform string
		want     []string
	}{
		{PlatformAnthropic, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses)},
		{PlatformOpenAI, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses, EndpointProtocolOpenAIEmbeddings, EndpointProtocolOpenAIAlphaSearch, EndpointProtocolOpenAIImages, EndpointProtocolOpenAIVideos)},
		{PlatformGemini, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses, EndpointProtocolGeminiGenerateContent, EndpointProtocolOpenAIImages)},
		{PlatformAntigravity, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses, EndpointProtocolGeminiGenerateContent, EndpointProtocolOpenAIImages)},
		{PlatformGrok, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses, EndpointProtocolOpenAIImages, EndpointProtocolOpenAIVideos)},
		{PlatformAdobe, protocolNames(EndpointProtocolOpenAIImages, EndpointProtocolOpenAIVideos)},
		{PlatformCursor, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses)},
		{PlatformOpenCode, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses)},
		{PlatformKiro, protocolNames(EndpointProtocolAnthropicMessages, EndpointProtocolOpenAIChatCompletions, EndpointProtocolOpenAIResponses)},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			capabilities, ok := GetPlatformCapabilities(tt.platform)
			require.True(t, ok)
			require.Equal(t, tt.want, protocolStrings(capabilities.EndpointProtocols))
		})
	}
}

func TestCandidateAccountPlatforms(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		[]string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok, PlatformCursor, PlatformOpenCode, PlatformKiro},
		CandidateAccountPlatforms(EndpointProtocolOpenAIResponses),
	)
	require.Equal(t, []string{PlatformOpenCode}, CandidateAccountPlatforms(EndpointProtocolOpenAIResponses, " OPENCODE "))
	require.Empty(t, CandidateAccountPlatforms(EndpointProtocolOpenAIEmbeddings, PlatformAnthropic))
	require.Empty(t, CandidateAccountPlatforms(EndpointProtocol("future_protocol")))
}

func TestSupportedEndpointProtocolsUsesAccountCapabilities(t *testing.T) {
	t.Parallel()

	geek2APIStyle := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":             "redacted",
			"base_url":            "https://relay.example.invalid/v1",
			"openai_capabilities": []string{"chat_completions", "embeddings"},
		},
		Extra: map[string]any{"openai_responses_mode": "force_chat_completions"},
	}
	require.Equal(t, protocolNames(
		EndpointProtocolAnthropicMessages,
		EndpointProtocolOpenAIChatCompletions,
		EndpointProtocolOpenAIEmbeddings,
		EndpointProtocolOpenAIAlphaSearch,
		EndpointProtocolOpenAIImages,
		EndpointProtocolOpenAIVideos,
	), SupportedEndpointProtocols(geek2APIStyle))

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NotContains(t, SupportedEndpointProtocols(oauth), string(EndpointProtocolOpenAIEmbeddings))
	require.NotContains(t, SupportedEndpointProtocols(oauth), string(EndpointProtocolOpenAIVideos))
}

func TestIsAccountCompatibleForRequestTextAdapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account *Account
		request RequestDescriptor
		options AccountGroupCompatibilityOptions
		want    bool
	}{
		{
			name:    "anthropic native accepts OpenAI chat ingress",
			account: schedulableProtocolAccount(PlatformAnthropic, AccountTypeAPIKey),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIChatCompletions, Model: "claude-sonnet-4-6"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformAnthropic}},
			want:    true,
		},
		{
			name: "OpenCode known model through explicit association capability",
			account: protocolAccountWithExtra(PlatformOpenCode, AccountTypeAPIKey,
				map[string]any{"model_protocols": map[string]any{"custom-chat": "chat_completions"}}, nil),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIResponses, Model: "custom-chat"},
			options: AccountGroupCompatibilityOptions{
				Group:                        &Group{Platform: PlatformOpenAI},
				EndpointCompatibilityEnabled: true,
			},
			want: true,
		},
		{
			name: "OpenCode unknown model fails closed",
			account: protocolAccountWithExtra(PlatformOpenCode, AccountTypeAPIKey,
				map[string]any{"api_key": "redacted"}, map[string]any{"mixed_scheduling": true}),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIChatCompletions, Model: "unknown-opencode-model"},
			options: AccountGroupCompatibilityOptions{
				Group:                &Group{Platform: PlatformOpenAI},
				AllowMixedScheduling: true,
			},
			want: false,
		},
		{
			name: "Geek2API style OpenAI APIKey accepts chat",
			account: protocolAccountWithExtra(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
				"api_key":             "redacted",
				"base_url":            "https://relay.example.invalid/v1",
				"openai_capabilities": []string{"chat_completions"},
			}, map[string]any{"openai_responses_mode": "force_chat_completions"}),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIChatCompletions, Model: "custom-model"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformOpenAI}},
			want:    true,
		},
		{
			name:    "Cursor bridge",
			account: protocolAccountWithExtra(PlatformCursor, AccountTypeAPIKey, nil, nil),
			request: RequestDescriptor{Protocol: EndpointProtocolAnthropicMessages, Model: "claude-sonnet-4-6"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformAnthropic}, EndpointCompatibilityEnabled: true},
			want:    true,
		},
		{
			name:    "Kiro text compatibility remains permissive",
			account: schedulableProtocolAccount(PlatformKiro, AccountTypeOAuth),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIResponses, Model: "custom-text-alias"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformOpenAI}, EndpointCompatibilityEnabled: true},
			want:    true,
		},
		{
			name:    "Gemini native generate content",
			account: schedulableProtocolAccount(PlatformGemini, AccountTypeAPIKey),
			request: RequestDescriptor{Protocol: EndpointProtocolGeminiGenerateContent, Model: "gemini-2.5-pro"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformGemini}},
			want:    true,
		},
		{
			name:    "Antigravity responses adapter",
			account: schedulableProtocolAccount(PlatformAntigravity, AccountTypeOAuth),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIResponses, Model: "claude-sonnet-4-5"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformAntigravity}},
			want:    true,
		},
		{
			name:    "Grok responses adapter",
			account: schedulableProtocolAccount(PlatformGrok, AccountTypeOAuth),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIResponses, Model: "grok-4.5"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformGrok}},
			want:    true,
		},
		{
			name:    "cross platform requires association or legacy mixed switch",
			account: schedulableProtocolAccount(PlatformCursor, AccountTypeAPIKey),
			request: RequestDescriptor{Protocol: EndpointProtocolOpenAIResponses, Model: "claude-sonnet-4-6"},
			options: AccountGroupCompatibilityOptions{Group: &Group{Platform: PlatformOpenAI}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsAccountCompatibleForRequest(tt.account, tt.request, tt.options))
		})
	}
}

func TestIsAccountCompatibleForRequestSpecificCapabilitiesFailClosed(t *testing.T) {
	t.Parallel()

	openAIAPIKey := protocolAccountWithExtra(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"openai_capabilities": []string{"chat_completions", "embeddings"},
	}, map[string]any{"openai_responses_mode": "force_chat_completions"})
	openAIOAuth := schedulableProtocolAccount(PlatformOpenAI, AccountTypeOAuth)
	anthropic := schedulableProtocolAccount(PlatformAnthropic, AccountTypeAPIKey)
	grokAPIKey := schedulableProtocolAccount(PlatformGrok, AccountTypeAPIKey)
	adobe := schedulableProtocolAccount(PlatformAdobe, AccountTypeOAuth)

	tests := []struct {
		name     string
		account  *Account
		protocol EndpointProtocol
		model    string
		want     bool
	}{
		{"OpenAI APIKey embeddings", openAIAPIKey, EndpointProtocolOpenAIEmbeddings, "text-embedding-3-small", true},
		{"OpenAI OAuth embeddings rejected", openAIOAuth, EndpointProtocolOpenAIEmbeddings, "text-embedding-3-small", false},
		{"Anthropic embeddings rejected", anthropic, EndpointProtocolOpenAIEmbeddings, "voyage-large", false},
		{"Anthropic image rejected", anthropic, EndpointProtocolOpenAIImages, "gpt-image-2", false},
		{"OpenAI image model", openAIAPIKey, EndpointProtocolOpenAIImages, "dall-e-3", true},
		{"OpenAI text model on image endpoint rejected", openAIAPIKey, EndpointProtocolOpenAIImages, "gpt-5", false},
		{"Grok APIKey video", grokAPIKey, EndpointProtocolOpenAIVideos, "grok-imagine-video", true},
		{"Adobe image", adobe, EndpointProtocolOpenAIImages, "nano-banana", true},
		{"Adobe video", adobe, EndpointProtocolOpenAIVideos, "veo3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{Platform: tt.account.Platform, EndpointProtocols: []string{string(tt.protocol)}}
			request := RequestDescriptor{Protocol: tt.protocol, Model: tt.model}
			require.Equal(t, tt.want, IsAccountCompatibleForRequest(tt.account, request, AccountGroupCompatibilityOptions{Group: group}))
		})
	}
}

func TestIsAccountCompatibleForRequestForcedProvider(t *testing.T) {
	t.Parallel()

	account := schedulableProtocolAccount(PlatformOpenAI, AccountTypeAPIKey)
	request := RequestDescriptor{
		Protocol:       EndpointProtocolOpenAIChatCompletions,
		Model:          "gpt-5",
		ForcedPlatform: PlatformAnthropic,
	}
	require.False(t, IsAccountCompatibleForRequest(account, request, AccountGroupCompatibilityOptions{}))

	request.ForcedPlatform = PlatformOpenAI
	require.True(t, IsAccountCompatibleForRequest(account, request, AccountGroupCompatibilityOptions{}))

	account.Status = StatusDisabled
	require.False(t, IsAccountCompatibleForRequest(account, request, AccountGroupCompatibilityOptions{}))
}

func TestEndpointProtocolContextAndPathMapping(t *testing.T) {
	t.Parallel()

	ctx := WithEndpointProtocol(context.Background(), EndpointProtocolOpenAIResponses)
	protocol, ok := EndpointProtocolFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, EndpointProtocolOpenAIResponses, protocol)

	tests := map[string]EndpointProtocol{
		"/v1/messages":                                EndpointProtocolAnthropicMessages,
		"/v1/chat/completions":                        EndpointProtocolOpenAIChatCompletions,
		"/backend-api/codex/responses/compact":        EndpointProtocolOpenAIResponses,
		"/v1beta/models/gemini:streamGenerateContent": EndpointProtocolGeminiGenerateContent,
		"/v1/embeddings":                              EndpointProtocolOpenAIEmbeddings,
		"/v1/alpha/search":                            EndpointProtocolOpenAIAlphaSearch,
		"/v1/images/batches":                          EndpointProtocolOpenAIImages,
		"/v1/videos/generations":                      EndpointProtocolOpenAIVideos,
	}
	for path, want := range tests {
		got, found := EndpointProtocolForRequestPath(path)
		require.True(t, found, path)
		require.Equal(t, want, got, path)
	}
}

func protocolNames(protocols ...EndpointProtocol) []string {
	return protocolStrings(protocols)
}

func schedulableProtocolAccount(platform, accountType string) *Account {
	return &Account{
		Platform:    platform,
		Type:        accountType,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func protocolAccountWithExtra(platform, accountType string, credentials, extra map[string]any) *Account {
	account := schedulableProtocolAccount(platform, accountType)
	account.Credentials = credentials
	account.Extra = extra
	return account
}
