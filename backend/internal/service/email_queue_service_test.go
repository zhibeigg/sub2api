package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailQueueRecipientHashDoesNotExposeAddress(t *testing.T) {
	const recipient = "user@example.com"

	hash := emailQueueRecipientHash(recipient)

	require.Len(t, hash, 64)
	require.NotContains(t, hash, recipient)
	require.NotContains(t, hash, "example.com")
}
