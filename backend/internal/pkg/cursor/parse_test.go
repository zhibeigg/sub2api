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
