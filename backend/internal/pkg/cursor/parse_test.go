package cursor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAnthropicIgnoresPriorThinkingBlocks(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral"}}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"first"}]},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"private reasoning","signature":"signature"},
				{"type":"redacted_thinking","data":"redacted"},
				{"type":"text","text":"visible answer"}
			]},
			{"role":"user","content":[{"type":"text","text":"follow up"}]}
		]
	}`)

	dialogue, err := ParseAnthropic(body)
	require.NoError(t, err)
	require.Equal(t, "system prompt", dialogue.System)
	require.Equal(t, []DialogueMessage{
		{Role: "user", Text: "first"},
		{Role: "assistant", Text: "visible answer"},
		{Role: "user", Text: "follow up"},
	}, dialogue.Messages)
}

func TestParseAnthropicBase64Images(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
			]},
			{"role":"assistant","content":"received"},
			{"role":"user","content":[
				{"type":"text","text":"compare this"},
				{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"/9j/AA=="}}
			]}
		]
	}`)

	dialogue, err := ParseAnthropic(body)
	require.NoError(t, err)
	require.Equal(t, []DialogueMessage{
		{Role: "user", Images: []InlineImage{{MIMEType: "image/png", Data: []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'}}}},
		{Role: "assistant", Text: "received"},
		{Role: "user", Text: "compare this", Images: []InlineImage{{MIMEType: "image/jpeg", Data: []byte{'\xff', '\xd8', '\xff', '\x00'}}}},
	}, dialogue.Messages)
}

func TestParseAnthropicRejectsUnsupportedImageStructuresWithoutLeakingData(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		secret string
	}{
		{
			name:   "remote URL",
			body:   `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://secret.example/image.png"}}]}]}`,
			secret: "secret.example",
		},
		{
			name:   "invalid base64",
			body:   `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"not-base64-SECRET"}}]}]}`,
			secret: "not-base64-SECRET",
		},
		{
			name:   "non image MIME",
			body:   `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"text/plain","data":"iVBORw0KGgpJTUFHRS1TRUNSRVQ="}}]}]}`,
			secret: "iVBORw0KGgpJTUFHRS1TRUNSRVQ=",
		},
		{
			name:   "SVG MIME",
			body:   `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/svg+xml","data":"iVBORw0KGgo="}}]}]}`,
			secret: "iVBORw0KGgo=",
		},
		{
			name:   "MIME mismatch",
			body:   `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"/9j/AA=="}}]}]}`,
			secret: "/9j/AA==",
		},
		{
			name:   "assistant image",
			body:   `{"messages":[{"role":"assistant","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgpJTUFHRS1TRUNSRVQ="}}]}]}`,
			secret: "iVBORw0KGgpJTUFHRS1TRUNSRVQ=",
		},
		{
			name:   "system image",
			body:   `{"messages":[{"role":"system","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgpJTUFHRS1TRUNSRVQ="}}]}]}`,
			secret: "iVBORw0KGgpJTUFHRS1TRUNSRVQ=",
		},
		{
			name:   "tool result image",
			body:   `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgpJTUFHRS1TRUNSRVQ="}}]}]}]}`,
			secret: "iVBORw0KGgpJTUFHRS1TRUNSRVQ=",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseAnthropic([]byte(test.body))
			require.Error(t, err)
			require.True(t, IsKind(err, ErrorBadRequest))
			require.NotContains(t, err.Error(), test.secret)
		})
	}
}

func TestInlineImageBudgetLimits(t *testing.T) {
	countBudget := &inlineImageBudget{}
	for range maxInlineImageCount {
		require.NoError(t, countBudget.add(InlineImage{Data: []byte{1}}))
	}
	require.ErrorContains(t, countBudget.add(InlineImage{Data: []byte{1}}), "inline image count exceeds")

	totalBudget := &inlineImageBudget{}
	require.NoError(t, totalBudget.add(InlineImage{Data: make([]byte, 4<<20)}))
	require.ErrorContains(t, totalBudget.add(InlineImage{Data: make([]byte, 3<<20)}), "inline images exceed")
}

func TestParseAnthropicRejectsOversizedImageBeforeDecode(t *testing.T) {
	source := &struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	}{
		Type:      "base64",
		MediaType: "image/png",
		Data:      strings.Repeat("A", maxInlineImageBase64Length+1),
	}

	_, err := parseAnthropicInlineImage(source)
	require.ErrorContains(t, err, "inline image exceeds")
}

func TestParseAnthropicMergesSystemMessagesIntoTopLevelPrompt(t *testing.T) {
	body := []byte(`{
		"system":"top-level system",
		"messages":[
			{"role":"user","content":"question"},
			{"role":"system","content":"runtime system"},
			{"role":"developer","content":[{"type":"text","text":"developer instructions"}]}
		]
	}`)

	dialogue, err := ParseAnthropic(body)
	require.NoError(t, err)
	require.Equal(t, "top-level system\n\nruntime system\n\ndeveloper instructions", dialogue.System)
	require.Equal(t, []DialogueMessage{{Role: "user", Text: "question"}}, dialogue.Messages)
}

func TestParseAnthropicIgnoresServerToolsAndTheirHistoryBlocks(t *testing.T) {
	body := []byte(`{
		"tools":[
			{"type":"web_search_20250305","name":"web_search","max_uses":5},
			{"name":"local_tool","description":"client tool","input_schema":{"type":"object","properties":{"value":{"type":"string"}}}}
		],
		"messages":[
			{"role":"user","content":"find current information"},
			{"role":"assistant","content":[
				{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"current information"}},
				{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","title":"Example","url":"https://example.com","encrypted_content":"encrypted"}]},
				{"type":"text","text":"visible search summary"}
			]},
			{"role":"user","content":"continue"}
		]
	}`)

	dialogue, err := ParseAnthropic(body)
	require.NoError(t, err)
	require.Equal(t, []ToolDefinition{{
		Name:        "local_tool",
		Description: "client tool",
		InputSchema: []byte(`{"type":"object","properties":{"value":{"type":"string"}}}`),
	}}, dialogue.Tools)
	require.Equal(t, []DialogueMessage{
		{Role: "user", Text: "find current information"},
		{Role: "assistant", Text: "visible search summary"},
		{Role: "user", Text: "continue"},
	}, dialogue.Messages)
}

func TestNormalizeToolsKeepsRejectingUnknownClientToolTypes(t *testing.T) {
	_, err := normalizeTools([]rawTool{{Type: "computer_20250124"}})
	require.ErrorContains(t, err, `unsupported tool type "computer_20250124"`)
}
