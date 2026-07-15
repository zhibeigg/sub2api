package cursor

import (
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
