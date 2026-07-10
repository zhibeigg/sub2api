package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationErrorCode(t *testing.T) {
	require.Equal(t, "content_policy_violation", contentModerationErrorCode(nil))
	require.Equal(t, "content_policy_violation", contentModerationErrorCode(&service.ContentModerationDecision{}))
	require.Equal(t, service.ContentModerationErrorCodeCyberAbuse, contentModerationErrorCode(&service.ContentModerationDecision{
		ErrorCode: service.ContentModerationErrorCodeCyberAbuse,
	}))
}
