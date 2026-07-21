package qqbot

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewConfigManager,
	NewReliableQueue,
	NewChannelCheckSigner,
	NewChannelStatusRenderer,
	NewChannelCheckService,
	NewRuntime,
)
