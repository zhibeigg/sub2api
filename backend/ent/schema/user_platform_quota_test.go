package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserPlatformQuotaPlatformValidatorAllowsOpenCode(t *testing.T) {
	var validator func(string) error
	for _, entField := range (UserPlatformQuota{}).Fields() {
		descriptor := entField.Descriptor()
		if descriptor.Name != "platform" {
			continue
		}
		require.NotEmpty(t, descriptor.Validators)
		var ok bool
		validator, ok = descriptor.Validators[len(descriptor.Validators)-1].(func(string) error)
		require.True(t, ok)
		break
	}
	require.NotNil(t, validator)

	for _, platform := range []string{"anthropic", "openai", "gemini", "antigravity", "grok", "adobe", "cursor", "opencode"} {
		require.NoError(t, validator(platform), platform)
	}
	require.Error(t, validator("unknown"))
}
